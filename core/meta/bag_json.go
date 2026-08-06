// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package meta

import (
	"encoding/json"
	"fmt"
	"slices"

	"go.thesmos.sh/eidos/core/position"
)

// bagJSON is the wire shape [Bag] marshals into and unmarshals out
// of. The shape is a sorted slice rather than a map so encoded output
// is byte-deterministic — important for cache key composition where
// a Bag's JSON form feeds into a hash.
type bagJSON struct {
	// Entries lists every recorded (name, authority) entry in
	// ascending order: name first, then authority within name.
	Entries []bagEntryJSON `json:"entries"`
}

// bagEntryJSON is the wire form of a single [entry] slot. Value is
// stored as raw JSON so the Bag can round-trip values of any type
// without knowing the Go-side static type at decode time — the
// typed [Key.Get] path lazily decodes RawMessage to T on read.
type bagEntryJSON struct {
	Name      string          `json:"name"`
	Authority Authority       `json:"authority"`
	Tombstone bool            `json:"tombstone,omitempty"`
	Value     json.RawMessage `json:"value,omitempty"`
	SetBy     string          `json:"set_by"`
	Pos       position.Pos    `json:"pos,omitzero"`
}

// MarshalJSON encodes the Bag's recorded entries in a stable order.
// Observers are deliberately omitted — they are callbacks attached
// at runtime and have no meaningful wire form. After UnmarshalJSON,
// observer wiring is re-attached by the caller (the store's
// indexCommon, for example).
func (b *Bag) MarshalJSON() ([]byte, error) {
	// The nil bag is the empty bag, and encoding a value must not
	// require it to have been written to first. `null` rather than
	// an empty entry list because that is what encoding/json
	// produces for a nil pointer anywhere else, and because an
	// owner holding a nil bag omits the field entirely — the two
	// spellings never appear in the same document.
	if b == nil {
		return []byte("null"), nil
	}
	b.mu.RLock()
	defer b.mu.RUnlock()

	names := make([]string, 0, len(b.entries))
	for name := range b.entries {
		names = append(names, name)
	}
	slices.Sort(names)

	out := bagJSON{Entries: make([]bagEntryJSON, 0, len(names)*numAuthorities)}
	for _, name := range names {
		slot := b.entries[name]
		for auth := range Authority(numAuthorities) {
			e := slot[auth]
			if e == nil {
				continue
			}
			payload, err := marshalEntryValue(e)
			if err != nil {
				return nil, fmt.Errorf("meta: marshal entry %q@%v: %w", name, auth, err)
			}
			out.Entries = append(out.Entries, bagEntryJSON{
				Name:      name,
				Authority: auth,
				Tombstone: e.tombstone,
				Value:     payload,
				SetBy:     e.setBy,
				Pos:       e.pos,
			})
		}
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("meta: marshal bag: %w", err)
	}
	return encoded, nil
}

// UnmarshalJSON populates the Bag from the wire form produced by
// [Bag.MarshalJSON]. Stored values arrive as [json.RawMessage] so
// the Bag does not need static type information for them; the
// typed [Key.Get] path resolves the message into T on first read.
//
// UnmarshalJSON replaces any previously-recorded entries in the
// receiver. Observers are not part of the wire form; callers that
// need observer wiring after unmarshal re-attach via the usual
// [Bag.AddObserver] path.
func (b *Bag) UnmarshalJSON(data []byte) error {
	var wire bagJSON
	if err := json.Unmarshal(data, &wire); err != nil {
		return fmt.Errorf("meta: unmarshal bag: %w", err)
	}
	entries := make(map[string]*[numAuthorities]*entry, len(wire.Entries))
	for _, e := range wire.Entries {
		if e.Authority < 0 || e.Authority >= numAuthorities {
			return fmt.Errorf("meta: unmarshal bag: authority %d out of range", e.Authority)
		}
		slot, ok := entries[e.Name]
		if !ok {
			slot = new([numAuthorities]*entry)
			entries[e.Name] = slot
		}
		var value any
		if !e.Tombstone && len(e.Value) > 0 {
			// Preserve the raw JSON; Key.Get decodes lazily.
			value = json.RawMessage(append([]byte(nil), e.Value...))
		}
		slot[e.Authority] = &entry{
			tombstone: e.Tombstone,
			value:     value,
			setBy:     e.SetBy,
			pos:       e.Pos,
		}
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.entries = entries
	b.observers = nil
	return nil
}

// marshalEntryValue produces the JSON wire form of an entry's value.
// Tombstones contribute no value bytes; the caller's outer envelope
// records the tombstone flag separately.
func marshalEntryValue(e *entry) (json.RawMessage, error) {
	if e.tombstone || e.value == nil {
		return nil, nil
	}
	// If the value is already RawMessage (round-tripped through
	// Unmarshal), emit it verbatim. This preserves byte-equality
	// across encode → decode → encode cycles.
	if rm, ok := e.value.(json.RawMessage); ok {
		return rm, nil
	}
	encoded, err := json.Marshal(e.value)
	if err != nil {
		return nil, fmt.Errorf("meta: marshal value: %w", err)
	}
	return encoded, nil
}
