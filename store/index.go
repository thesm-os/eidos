// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store

import (
	"fmt"
	"iter"
	"sync"
)

// Bucket is an insertion-ordered, qualified-name-keyed collection of
// T. Each per-kind store ([NodeView.Structs], [NodeView.Methods], …)
// uses a Bucket internally: a slice of items captures the insertion
// order for deterministic iteration, and a parallel map provides
// O(1) qualified-name lookup. Concurrent-safe via RWMutex.
//
// Bucket is exported so the [NodeView] / [EmitView] accessors can
// surface the same shape per kind without repeating the storage code
// in every per-kind file.
type Bucket[T any] struct {
	mu      sync.RWMutex
	items   []T
	byQName map[string]T
}

// NewBucket returns an empty Bucket ready for use.
func NewBucket[T any]() *Bucket[T] {
	return &Bucket[T]{byQName: map[string]T{}}
}

// Add records item under qname. Returns [ErrDuplicateQName] when
// qname is already present.
func (b *Bucket[T]) Add(qname string, item T) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, dup := b.byQName[qname]; dup {
		return fmt.Errorf("%w: %q", ErrDuplicateQName, qname)
	}
	b.items = append(b.items, item)
	b.byQName[qname] = item
	return nil
}

// ByQName returns the item recorded under qname, or the zero value
// and false when no item matches.
func (b *Bucket[T]) ByQName(qname string) (T, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	v, ok := b.byQName[qname]
	return v, ok
}

// Items returns a copy of the bucket's items in insertion order. The
// returned slice is safe for the caller to mutate and iterate while
// the bucket continues to accept writes.
func (b *Bucket[T]) Items() []T {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]T, len(b.items))
	copy(out, b.items)
	return out
}

// itemsRef returns the bucket's live backing slice without copying.
//
// The caller must not mutate the result and must not retain it past
// the point the bucket can accept another write, because an append
// may reallocate the array underneath it.
//
// That constraint is why this is unexported and why it is used from
// the node-side Reader accessors only. The pipeline freezes the
// NodeView at the end of the frontend phase, before any annotator or
// generator runs, so every reader that can observe a node bucket
// observes a bucket that will never be written to again. The EmitView
// is not frozen until the end of layout, and reference generators
// read emit buckets during the generator phase while later generators
// are still calling AddPackage — those accessors keep [Bucket.Items]
// and its copy. A new accessor added to store/reader.go has to pick
// the right one, and the reason is not visible from the call site.
func (b *Bucket[T]) itemsRef() []T {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.items
}

// All returns an iterator over the bucket's items in insertion order.
//
// The iterator takes a point-in-time snapshot of the slice header
// under the read lock and releases it before yielding anything, so a
// concurrent append is not observed mid-iteration and the yield
// function may safely call back into the bucket. That last part is
// the difference from [Bucket.Range], whose docblock forbids it:
// plugin.Walk yields to arbitrary plugin hooks, and holding a store
// lock across those is a deadlock waiting for a plugin author.
//
// Unlike [Bucket.Items] the result is not a private copy, so callers
// that need to mutate keep using Items.
func (b *Bucket[T]) All() iter.Seq[T] {
	b.mu.RLock()
	items := b.items
	b.mu.RUnlock()
	return func(yield func(T) bool) {
		for _, item := range items {
			if !yield(item) {
				return
			}
		}
	}
}

// Len returns the number of items in the bucket.
func (b *Bucket[T]) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return len(b.items)
}

// Range invokes fn for each item in insertion order. Returning false
// from fn stops iteration. The iteration holds the read lock for its
// duration; fn must not call back into write methods on the same
// bucket.
func (b *Bucket[T]) Range(fn func(T) bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for _, item := range b.items {
		if !fn(item) {
			return
		}
	}
}

// MultiIndex is a generic many-to-one keyed index: many keys, each
// mapping to an insertion-ordered list of values. Used for
// cross-cutting indices like "by directive presence" and "by
// declaring package" where one key has many entries. Concurrent-safe
// via RWMutex. Both the per-key value list and the set of distinct
// keys are tracked in first-insertion order, so iteration via
// [MultiIndex.Keys] is deterministic across runs.
type MultiIndex[K comparable, V any] struct {
	mu      sync.RWMutex
	entries map[K][]V
	order   []K
}

// NewMultiIndex returns an empty MultiIndex ready for use.
func NewMultiIndex[K comparable, V any]() *MultiIndex[K, V] {
	return &MultiIndex[K, V]{entries: map[K][]V{}}
}

// newMultiIndexSize returns a MultiIndex pre-sized for keys distinct
// keys, reserving both the map and the insertion-order slice.
//
// For the by-Target index the order slice holds [emit.Target] values
// at 64 bytes each, so growing it from nil is the larger half of the
// cost — a rebuild over a thousand files walks that ladder from zero
// every time.
//
// The caller supplies a key count, not an entity count. Sizing from
// the sum of the routable buckets would be wrong in the direction
// that looks right: the whole purpose of a by-Target index is many
// entities per Target.
func newMultiIndexSize[K comparable, V any](keys int) *MultiIndex[K, V] {
	return &MultiIndex[K, V]{
		entries: make(map[K][]V, keys),
		order:   make([]K, 0, keys),
	}
}

// Add appends value to the list under key, preserving insertion
// order. The first Add for a key also records the key in the
// index-wide first-insertion-order list surfaced by
// [MultiIndex.Keys].
// Two map operations, not three. The obvious spelling — a
// presence check, then m.entries[key] = append(m.entries[key], …) —
// hashes the key three times: once to probe, once to read the slice
// append extends, once to store it back. Reusing the value the probe
// already returned removes the middle one. For the by-Target
// instantiation the key is a struct of four strings, so every cycle
// saved is four string hashes saved.
func (m *MultiIndex[K, V]) Add(key K, value V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur, seen := m.entries[key]
	if !seen {
		m.order = append(m.order, key)
	}
	m.entries[key] = append(cur, value)
}

// Get returns a copy of the values recorded under key in insertion
// order. Returns nil when the key has no entries.
func (m *MultiIndex[K, V]) Get(key K) []V {
	m.mu.RLock()
	defer m.mu.RUnlock()
	src, ok := m.entries[key]
	if !ok {
		return nil
	}
	out := make([]V, len(src))
	copy(out, src)
	return out
}

// Has reports whether key has at least one recorded value.
func (m *MultiIndex[K, V]) Has(key K) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.entries[key]
	return ok
}

// Len returns the number of distinct keys in the index.
func (m *MultiIndex[K, V]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.entries)
}

// Keys returns a copy of the distinct keys in first-insertion order.
// The returned slice is safe for the caller to mutate; mutations
// have no effect on the index. Empty index returns a non-nil
// zero-length slice.
func (m *MultiIndex[K, V]) Keys() []K {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]K, len(m.order))
	copy(out, m.order)
	return out
}
