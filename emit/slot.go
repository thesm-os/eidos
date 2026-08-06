// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

import (
	"errors"
	"fmt"

	"go.thesmos.sh/eidos/core/kind"
)

// ErrSlotElementType is returned by [Slot.Append], [Slot.Prepend],
// and the insertion helpers when the supplied item does not match
// the slot's declared element kind.
var ErrSlotElementType = errors.New("emit: slot element kind mismatch")

// ErrProvenanceNotFound is returned by [Slot.InsertBefore] and
// [Slot.InsertAfter] when no item in the slot carries a matching
// [Provenance.ID]. Plugins that target other plugins' contributions
// must coordinate on the ID convention (typically via a shared
// const exported by the producing plugin).
var ErrProvenanceNotFound = errors.New("emit: provenance ID not found in slot")

// Slot is a named, kind-checked, provenance-tracked region attached
// to an emit [Node]. Generators append items with [Provenance]
// identifying the contributing plugin; cross-cutting generators
// inject contributions into other plugins' emit without owning the
// host node.
//
// Slot is the primary composition mechanism in emit: when a
// cross-cutter (validation, audit, debug) wants to add a statement
// to every Method's `prebody`, it calls method.Slot("prebody").Append(...)
// without needing to coordinate with the method's owning generator.
// Backends render slot contributions in append order alongside the
// host's typed content.
//
// Reserved slot names carry a fixed element kind, listed below. The
// kind is a property of the NAME: reaching a reserved slot through the
// host's typed accessor and through [Slot] by string yields one slot
// with one constraint, whichever a plugin calls first. Slot creation is
// lookup-or-create, so without that rule the surviving constraint would
// be decided by plugin registration order, and a contributor could land
// an unvalidated node purely by running early.
//
//   - [File]:        "imports" ([KindImport])
//   - [Struct]:      "fields" ([KindField]), "methods" ([KindMethod]),
//     "embeds" ([KindEmbed])
//   - [Interface]:   "methods" ([KindMethod]), "embeds" ([KindEmbed])
//   - [Alias]:       "methods" ([KindMethod])
//   - [Method]:      "prebody", "postbody" ([KindStmt]),
//     "params" ([KindParam]), "returns" ([KindReturn])
//   - [Function]:    "prebody", "postbody" ([KindStmt]),
//     "params" ([KindParam]), "returns" ([KindReturn])
//   - [Enum]:        "variants" ([KindEnumVariant])
//
// The host's typed field model remains the source of truth for direct
// content; these slots are for cross-cutting injection.
//
// [File]'s "top", "bottom" and "init", [Field]'s "tags", and every
// [Package] slot are deliberately unconstrained, as are all custom
// names — plugin-defined emit kinds declare their own conventions and
// append their own kinds.
//
// Slots are append-only by default. Insertion helpers
// ([Slot.Prepend], [Slot.InsertAt]) provide positioning when needed.
// Order matters for rendering: backends emit contributions in slot
// order, so plugin ordering (via the pipeline's capability topo)
// determines visible output order.
type Slot struct {
	BaseEmit

	// SlotName is the name this slot is registered under on its
	// owner.
	SlotName string `json:"slot_name"`

	// Owner is the host emit node that exposes this slot.
	//
	// Owner is excluded from JSON encoding to break the host →
	// slot cycle. Deserialized graphs re-wire Owner via
	// [RewireOwners].
	Owner Node `json:"-"`

	// ElemKind, when non-empty, constrains [Slot.Append] to accept
	// only items whose Kind() matches. An empty ElemKind accepts
	// any kind (use when the slot's content is intentionally
	// heterogeneous).
	ElemKind kind.Kind `json:"elem_kind,omitempty"`

	// Items holds the appended contributions in their final order.
	Items []Node `json:"-"`

	// Provenance is a parallel slice with Items: ProvenanceList[i]
	// records who appended Items[i] and where the contribution
	// originated.
	ProvenanceList []Provenance `json:"provenance,omitempty"`
}

// Kind returns [KindSlot].
func (*Slot) Kind() kind.Kind { return KindSlot }

// Len returns the number of items currently in the slot.
func (s *Slot) Len() int { return len(s.Items) }

// Append adds item to the end of the slot's Items, recording prov in
// the parallel ProvenanceList. Returns [ErrSlotElementType] wrapped
// with the offending kind when ElemKind is set and item.Kind() does
// not match.
func (s *Slot) Append(item Node, prov Provenance) error {
	if err := s.checkKind(item); err != nil {
		return err
	}
	s.Items = append(s.Items, item)
	s.ProvenanceList = append(s.ProvenanceList, prov)
	return nil
}

// Prepend inserts item at the beginning of the slot's Items.
// Subsequent items shift one position to the right.
func (s *Slot) Prepend(item Node, prov Provenance) error {
	return s.InsertAt(0, item, prov)
}

// InsertAt inserts item at the given index. Items at and after the
// index shift one position to the right. Out-of-range indexes return
// an error wrapping [ErrSlotElementType] (or a separate sentinel for
// the bound case below).
func (s *Slot) InsertAt(index int, item Node, prov Provenance) error {
	if index < 0 || index > len(s.Items) {
		return fmt.Errorf("emit: slot %q insert index %d out of range [0, %d]", s.SlotName, index, len(s.Items))
	}
	if err := s.checkKind(item); err != nil {
		return err
	}
	s.Items = append(s.Items[:index], append([]Node{item}, s.Items[index:]...)...)
	s.ProvenanceList = append(s.ProvenanceList[:index], append([]Provenance{prov}, s.ProvenanceList[index:]...)...)
	return nil
}

// InsertBefore inserts item immediately before the first existing
// item whose [Provenance.ID] equals id. Returns
// [ErrProvenanceNotFound] when no item carries that ID (including
// the empty-string case — an empty id never matches).
//
// Cross-cutting plugins use InsertBefore to position their
// contributions relative to a specific prior contribution rather
// than at a numeric index, which would shift as other plugins
// append.
func (s *Slot) InsertBefore(id string, item Node, prov Provenance) error {
	if id == "" {
		return fmt.Errorf("%w: empty id", ErrProvenanceNotFound)
	}
	for i, p := range s.ProvenanceList {
		if p.ID == id {
			return s.InsertAt(i, item, prov)
		}
	}
	return fmt.Errorf("%w: %q in slot %q", ErrProvenanceNotFound, id, s.SlotName)
}

// InsertAfter inserts item immediately after the first existing
// item whose [Provenance.ID] equals id. Returns
// [ErrProvenanceNotFound] when no item carries that ID.
func (s *Slot) InsertAfter(id string, item Node, prov Provenance) error {
	if id == "" {
		return fmt.Errorf("%w: empty id", ErrProvenanceNotFound)
	}
	for i, p := range s.ProvenanceList {
		if p.ID == id {
			return s.InsertAt(i+1, item, prov)
		}
	}
	return fmt.Errorf("%w: %q in slot %q", ErrProvenanceNotFound, id, s.SlotName)
}

// At returns the item at the given index, or nil when index is out
// of range.
func (s *Slot) At(i int) Node {
	if i < 0 || i >= len(s.Items) {
		return nil
	}
	return s.Items[i]
}

// ProvenanceAt returns the Provenance recorded for the item at the
// given index, or the zero Provenance when index is out of range.
func (s *Slot) ProvenanceAt(i int) Provenance {
	if i < 0 || i >= len(s.ProvenanceList) {
		return Provenance{}
	}
	return s.ProvenanceList[i]
}

// checkKind validates item against ElemKind. An empty ElemKind
// accepts any item; otherwise the item's Kind must match exactly.
func (s *Slot) checkKind(item Node) error {
	if s.ElemKind == "" {
		return nil
	}
	if item == nil {
		return fmt.Errorf("%w: slot %q: expected %s, got nil", ErrSlotElementType, s.SlotName, s.ElemKind)
	}
	if got := item.Kind(); got != s.ElemKind {
		return fmt.Errorf("%w: slot %q: expected %s, got %s", ErrSlotElementType, s.SlotName, s.ElemKind, got)
	}
	return nil
}

// SlotHost is the interface every emit value that owns slots
// satisfies — [Struct], [Interface], [Function], [Method], [Enum],
// [Alias], [File], and [Package] all expose `Slot(name)` for
// generic, name-based slot access. Template helpers and tooling
// that operate on "the slot named X on this host" without caring
// about the concrete host type accept SlotHost.
type SlotHost interface {
	Node

	// Slot returns the named slot on the host, creating it lazily
	// without an element-kind constraint.
	Slot(name string) *Slot
}

// Reserved slot names, spelled once.
//
// The name is the key the per-host kind tables and the typed accessors
// both look up, so a typo in either would silently mint a second,
// unconstrained slot under a near-miss name rather than fail — the
// same class of defect as the call-order dependence these tables
// exist to remove.
const (
	slotImports  = "imports"
	slotFields   = "fields"
	slotMethods  = "methods"
	slotEmbeds   = "embeds"
	slotVariants = "variants"
	slotPrebody  = "prebody"
	slotPostbody = "postbody"
	slotParams   = "params"
	slotReturns  = "returns"
)

// slotMap is a small reusable type carrying a lazily-allocated map
// of slots; embedded by every host kind that exposes slots. The
// map's key is the slot name.
type slotMap struct {
	slots map[string]*Slot
}

// NewSlot returns a slot named name that accepts items whose Kind
// matches elemKind. An empty elemKind accepts any kind.
//
// Built-in emit kinds get their slots from an embedded slot map and
// never call this. It exists for a plugin that declares its own emit
// kind and wants to expose a slot other plugins contribute into — the
// composition pattern where a plugin depends on a plugin rather than
// on a core kind. Without it the machinery is unreachable from
// outside this package: the lazy-creation helper is unexported, so an
// author's only option was a struct literal that silently permits a
// missing SlotName.
//
// Owner is deliberately not a parameter. A host assigns it when it
// hands the slot out, which is the only point at which the back
// pointer is knowable, and leaving it nil until then keeps a
// half-constructed slot from claiming an owner it does not have:
//
//	func (s *Stack) Chain() *Slot {
//		if s.chain == nil {
//			s.chain = NewSlot("chain", KindExpr)
//			s.chain.Owner = s
//		}
//		return s.chain
//	}
//
// The host must also satisfy [SlotHost] so the backend's `slot`
// template helper can reach the slot by name.
func NewSlot(name string, elemKind kind.Kind) *Slot {
	return &Slot{SlotName: name, ElemKind: elemKind}
}

// slot returns the named slot, creating it lazily with the supplied
// owner and element-kind constraint. Repeat calls return the same
// instance.
func (m *slotMap) slot(owner Node, name string, elemKind kind.Kind) *Slot {
	if m.slots == nil {
		m.slots = map[string]*Slot{}
	}
	if existing, ok := m.slots[name]; ok {
		return existing
	}
	sl := &Slot{SlotName: name, Owner: owner, ElemKind: elemKind}
	m.slots[name] = sl
	return sl
}

// SlotsByName returns every slot registered on this host, indexed
// by name. The returned map aliases the host's internal state;
// callers must not mutate it. Useful for tooling that walks a
// host's full slot surface (provenance reporters, drift checkers,
// per-target plugin-attribution collectors).
func (m *slotMap) SlotsByName() map[string]*Slot {
	if m.slots == nil {
		m.slots = map[string]*Slot{}
	}
	return m.slots
}
