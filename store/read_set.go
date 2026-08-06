// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sync"
)

// ReadSet is the per-plugin record of what a plugin observed during
// a single pipeline run. Each query terminal call ([NodeView.Structs],
// directive lookup, etc.) appends a deterministic key to the active
// plugin's ReadSet via the [Reader] context plumbing.
//
// Later milestones derive the cache key for a plugin's outputs from
// the [ReadSet.Hash] of its ReadSet: identical reads (same nodes,
// same metadata, same directives) imply identical inputs and so
// identical outputs may be reused.
//
// ReadSet is safe for concurrent recording: a plugin running in
// parallel goroutines can append concurrently and the final hash is
// deterministic.
type ReadSet struct {
	mu    sync.Mutex
	items map[string]struct{}
}

// NewReadSet returns an empty ReadSet ready to accept reads.
func NewReadSet() *ReadSet {
	return &ReadSet{items: map[string]struct{}{}}
}

// Record adds key to the read set. Recording the same key twice is
// idempotent — only one entry is kept.
func (r *ReadSet) Record(key string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.items[key] = struct{}{}
}

// Len returns the number of distinct keys recorded so far.
func (r *ReadSet) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.items)
}

// Has reports whether key has been recorded.
func (r *ReadSet) Has(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.items[key]
	return ok
}

// Keys returns every recorded key in sorted order. The order is
// lexicographic so the result is stable across runs that observe
// the same set of keys.
func (r *ReadSet) Keys() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, 0, len(r.items))
	for k := range r.items {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// Hash returns the SHA-256 hex digest of the recorded keys in sorted
// order. The hash is deterministic for the same set of recorded keys
// regardless of the order in which they were recorded; cache layers
// use it as a stable cache-key fragment.
func (r *ReadSet) Hash() string {
	keys := r.Keys()
	total := len(keys) // one separator byte per key
	for _, k := range keys {
		total += len(k)
	}
	// One buffer and a by-value digest, rather than streaming through
	// a hash.Hash interface. sha256.New allocates a 224-byte digest on
	// the heap and Sum(nil) allocates its 32-byte result; Sum256 keeps
	// both on the stack.
	//
	// The byte stream is identical — each key followed by a zero
	// separator, in the same order — so the digest is unchanged.
	buf := make([]byte, 0, total)
	for _, k := range keys {
		buf = append(buf, k...)
		buf = append(buf, 0) // unambiguous separator
	}
	sum := sha256.Sum256(buf)
	return hex.EncodeToString(sum[:])
}
