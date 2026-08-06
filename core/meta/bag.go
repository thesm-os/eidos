// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package meta

import (
	"slices"
	"strings"
	"sync"

	"go.thesmos.sh/eidos/core/position"
)

// numAuthorities is the count of declared Authority values. Storage
// uses a fixed-size array indexed by Authority for compact access.
const numAuthorities = 3

// entry records one operation against a single name at a single
// authority. A nil entry means no operation has been recorded yet at
// that (name, authority) slot.
type entry struct {
	tombstone bool
	value     any
	setBy     string
	pos       position.Pos
}

// Bag is the per-owner metadata store. It records [Provenance] entries
// keyed by string name and authority; lookups resolve to a single
// "winning" entry according to authority order and the prefix-
// tombstone rule.
//
// All methods are safe to call concurrently. The internal lock is a
// RWMutex so reads (the common case) are uncontended.
//
// # The nil Bag is the empty Bag
//
// Every read method tolerates a nil receiver and returns the answer
// an empty bag would give. That is what lets an owner hold a nil
// *Bag until something actually writes metadata, instead of
// allocating one the moment anybody asks a question — an allocation
// that is also a write, and therefore a race when two goroutines
// ask at once.
//
// Writes through a nil Bag panic, by design. The panic marks a
// caller that took a write path without asking its owner for a bag
// first (`EnsureMeta` on the emit and node accessors). On the
// plugin boundary it surfaces through diag.RecoverAs as an
// attributed diagnostic rather than a crash.
//
// The zero value is unusable; construct with [NewBag].
type Bag struct {
	mu        sync.RWMutex
	entries   map[string]*[numAuthorities]*entry
	observers []Observer
}

// NewBag returns an empty Bag ready for use.
func NewBag() *Bag {
	return &Bag{entries: map[string]*[numAuthorities]*entry{}}
}

// setEntry records value v at (name, auth), replacing any prior entry
// at that slot. Registered observers fire after the slot mutation
// while the write lock is still held.
func (b *Bag) setEntry(name string, v any, auth Authority, setBy string, pos position.Pos) {
	b.mu.Lock()
	defer b.mu.Unlock()
	slot := b.slotLocked(name)
	slot[auth] = &entry{value: v, setBy: setBy, pos: pos}
	b.fireObservers(name)
}

// setTombstone records a tombstone at (name, auth), replacing any
// prior entry at that slot.
func (b *Bag) setTombstone(name string, auth Authority, setBy string, pos position.Pos) {
	b.mu.Lock()
	defer b.mu.Unlock()
	slot := b.slotLocked(name)
	slot[auth] = &entry{tombstone: true, setBy: setBy, pos: pos}
}

// slotLocked returns the [numAuthorities]*entry slot for name,
// creating it if absent. The caller must already hold the write lock.
func (b *Bag) slotLocked(name string) *[numAuthorities]*entry {
	slot, ok := b.entries[name]
	if !ok {
		slot = new([numAuthorities]*entry)
		b.entries[name] = slot
	}
	return slot
}

// Has reports whether name resolves to a value (i.e. is set and not
// tombstoned).
func (b *Bag) Has(name string) bool {
	_, ok := b.RawValue(name)
	return ok
}

// Tombstoned reports whether the resolution for name is a tombstone
// (either direct or via a prefix ancestor).
func (b *Bag) Tombstoned(name string) bool {
	if b == nil {
		return false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	p, ok := b.winningLocked(name)
	return ok && p.Tombstone
}

// RawValue returns the resolved value for name as a typed-erased any.
// The second return is false when name resolves to "not set" — either
// because no entry exists or because the winning entry is a tombstone.
func (b *Bag) RawValue(name string) (any, bool) {
	if b == nil {
		return nil, false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	p, ok := b.winningLocked(name)
	if !ok || p.Tombstone {
		return nil, false
	}
	return p.Value, true
}

// Winning returns the Provenance entry the resolution algorithm picks
// for name. The second return is false when no entry covers the name.
func (b *Bag) Winning(name string) (Provenance, bool) {
	if b == nil {
		return Provenance{}, false
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.winningLocked(name)
}

// winningLocked computes the winning Provenance for name. The caller
// must already hold at least the read lock.
//
// Resolution: walk authorities from highest to lowest; at each
// authority, consider direct entries at name plus tombstones on any
// dotted ancestor. Within a single authority, a tombstone (direct or
// via prefix) beats a value. The first authority with any qualifying
// entry wins.
func (b *Bag) winningLocked(name string) (Provenance, bool) {
	for auth := Authority(numAuthorities - 1); auth >= 0; auth-- {
		if p, ok := b.entryAtAuthorityLocked(name, auth); ok {
			return p, true
		}
	}
	return Provenance{}, false
}

// entryAtAuthorityLocked returns the winning Provenance at a single
// authority for name, accounting for prefix tombstones on ancestors.
// Tombstones beat direct values when both exist at the same
// authority.
func (b *Bag) entryAtAuthorityLocked(name string, auth Authority) (Provenance, bool) {
	var direct *entry
	if slot, ok := b.entries[name]; ok {
		direct = slot[auth]
	}
	prefixTombstone, prefixName := b.prefixTombstoneLocked(name, auth)

	switch {
	case direct != nil && direct.tombstone:
		return entryToProv(direct, auth, ""), true
	case prefixTombstone != nil:
		return entryToProv(prefixTombstone, auth, prefixName), true
	case direct != nil:
		return entryToProv(direct, auth, ""), true
	default:
		return Provenance{}, false
	}
}

// prefixTombstoneLocked returns the tombstone entry on the closest
// dotted ancestor of name at the given authority, if any. The second
// return is the ancestor's name; the caller uses it to fill
// Provenance.CoveredBy.
func (b *Bag) prefixTombstoneLocked(name string, auth Authority) (*entry, string) {
	for ancestor := parentName(name); ancestor != ""; ancestor = parentName(ancestor) {
		slot, ok := b.entries[ancestor]
		if !ok {
			continue
		}
		if e := slot[auth]; e != nil && e.tombstone {
			return e, ancestor
		}
	}
	return nil, ""
}

// History returns every recorded operation for the exact name in
// ascending authority order. Prefix tombstones on ancestor names are
// not included; use [Bag.Winning] for the resolved entry that
// accounts for ancestors.
func (b *Bag) History(name string) []Provenance {
	if b == nil {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	slot, ok := b.entries[name]
	if !ok {
		return nil
	}
	var out []Provenance
	for auth := range Authority(numAuthorities) {
		if e := slot[auth]; e != nil {
			out = append(out, entryToProv(e, auth, ""))
		}
	}
	return out
}

// Names returns every name that has at least one recorded entry, in
// sorted order. Tombstones count: a name with only a tombstone entry
// still appears in the result.
func (b *Bag) Names() []string {
	if b == nil {
		return nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]string, 0, len(b.entries))
	for name := range b.entries {
		out = append(out, name)
	}
	slices.Sort(out)
	return out
}

// TombstonePrefix records a tombstone at the prefix name itself at
// the given authority. Subsequent reads against any descendant —
// prefix.x, prefix.y.z — honour the tombstone via the ancestor walk
// performed by Winning / RawValue / Has.
//
// The tombstone is recorded at the prefix node only (not eagerly
// expanded to every descendant), so descendants that are set later
// still see the tombstone via the prefix-walk resolution rule.
func (b *Bag) TombstonePrefix(prefix string, auth Authority, setBy string, pos position.Pos) {
	b.setTombstone(prefix, auth, setBy, pos)
}

// entryToProv converts the internal entry into a public Provenance.
// authority and coveredBy are supplied by the caller because entry
// itself does not record them (they are inferred from the slot the
// entry occupies and the resolution path).
func entryToProv(e *entry, auth Authority, coveredBy string) Provenance {
	return Provenance{
		Authority: auth,
		Tombstone: e.tombstone,
		Value:     e.value,
		SetBy:     e.setBy,
		Pos:       e.pos,
		CoveredBy: coveredBy,
	}
}

// parentName returns the dotted parent of name, or "" if name has no
// parent. "shape.writer.method" → "shape.writer"; "shape" → "".
func parentName(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i < 0 {
		return ""
	}
	return name[:i]
}
