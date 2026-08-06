// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit_test

import (
	"bytes"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/emit"
)

func TestSlot_Kind(t *testing.T) {
	t.Parallel()

	t.Run("reports KindSlot", func(t *testing.T) {
		t.Parallel()
		s := &emit.Slot{}
		if s.Kind() != emit.KindSlot {
			t.Fatalf("Kind = %s, want %s", s.Kind(), emit.KindSlot)
		}
	})
}

func TestSlot_Append(t *testing.T) {
	t.Parallel()

	t.Run("appends item with provenance and increments Len", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.FieldsSlot()
		err := slot.Append(&emit.Field{Name: "ID"}, emit.Provenance{SetBy: "id-gen"})
		assertNoError(t, err)
		if slot.Len() != 1 {
			t.Fatalf("Len = %d, want 1", slot.Len())
		}
	})

	t.Run("rejects an item whose Kind does not match ElemKind", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.FieldsSlot()
		err := slot.Append(&emit.Method{Name: "Save"}, emit.Provenance{})
		if !errors.Is(err, emit.ErrSlotElementType) {
			t.Fatalf("Append should return ErrSlotElementType; got %v", err)
		}
		if !strings.Contains(err.Error(), "fields") {
			t.Fatalf("error should mention slot name; got %q", err.Error())
		}
	})

	t.Run("rejects nil item when ElemKind is set", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.FieldsSlot()
		if err := slot.Append(nil, emit.Provenance{}); !errors.Is(err, emit.ErrSlotElementType) {
			t.Fatalf("nil item should be rejected when ElemKind is set; got %v", err)
		}
	})

	t.Run("accepts heterogeneous items when ElemKind is empty", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.Slot("custom")
		if err := slot.Append(&emit.Field{Name: "X"}, emit.Provenance{}); err != nil {
			t.Fatalf("Append should accept any item; got %v", err)
		}
		if err := slot.Append(&emit.Method{Name: "M"}, emit.Provenance{}); err != nil {
			t.Fatalf("Append should accept any item; got %v", err)
		}
	})
}

// TestSlotReservedName_KindIsIndependentOfCallOrder pins the element
// kind of a reserved slot to the slot's NAME.
//
// Slot creation is lookup-or-create, so before this was pinned the
// constraint belonged to whichever accessor touched the host first:
// Prebody() minted an emit.stmt-constrained slot, Slot("prebody")
// minted an unconstrained one, and the loser silently inherited the
// winner's rules. Two plugins contributing to one method therefore got
// different validation depending on registration order, and the
// unchecked path was reachable by accident rather than by choice.
func TestSlotReservedName_KindIsIndependentOfCallOrder(t *testing.T) {
	t.Parallel()

	for _, tc := range slotKindCases() {
		t.Run(tc.label(), func(t *testing.T) {
			t.Parallel()

			typedFirst := tc.newHost()
			viaTyped := tc.typed(typedFirst)
			thenGeneric := typedFirst.Slot(tc.slotName)

			genericFirst := tc.newHost()
			viaGeneric := genericFirst.Slot(tc.slotName)
			thenTyped := tc.typed(genericFirst)

			for label, got := range map[string]*emit.Slot{
				"typed accessor, called first":  viaTyped,
				"Slot(name), called second":     thenGeneric,
				"Slot(name), called first":      viaGeneric,
				"typed accessor, called second": thenTyped,
			} {
				if got.ElemKind != tc.want {
					t.Errorf("%s: ElemKind = %q, want %q", label, got.ElemKind, tc.want)
				}
			}
		})
	}
}

// TestSlotReservedName_BothAccessorsReturnOneSlot asserts the typed
// accessor and Slot(name) address the same slot rather than two that
// happen to share a name. Distinct slots would split a host's
// contributions in two, and only one of them would reach the renderer.
func TestSlotReservedName_BothAccessorsReturnOneSlot(t *testing.T) {
	t.Parallel()

	for _, tc := range slotKindCases() {
		t.Run(tc.label(), func(t *testing.T) {
			t.Parallel()

			host := tc.newHost()
			if typed, generic := tc.typed(host), host.Slot(tc.slotName); typed != generic {
				t.Fatalf("typed accessor and Slot(%q) returned different slots (%p vs %p)",
					tc.slotName, typed, generic)
			}
		})
	}
}

// TestSlotReservedName_RejectsForeignKindViaEitherAccessor drives the
// behaviour the kind constraint exists for, through the accessor that
// used to escape it. Reaching a reserved slot by string is the path a
// template-driven plugin takes, so an unconstrained slot there meant a
// foreign node reached the renderer and failed as malformed output
// instead of as a named plugin at append time.
func TestSlotReservedName_RejectsForeignKindViaEitherAccessor(t *testing.T) {
	t.Parallel()

	for _, tc := range slotKindCases() {
		t.Run(tc.label(), func(t *testing.T) {
			t.Parallel()

			generic := tc.newHost().Slot(tc.slotName)
			err := generic.Append(tc.foreign(), emit.Provenance{SetBy: "foreign-plugin"})
			if !errors.Is(err, emit.ErrSlotElementType) {
				t.Fatalf("Slot(%q).Append(foreign) = %v, want ErrSlotElementType",
					tc.slotName, err)
			}
			if !strings.Contains(err.Error(), tc.slotName) {
				t.Errorf("error should name the slot; got %q", err.Error())
			}
		})
	}
}

func TestSlot_Prepend(t *testing.T) {
	t.Parallel()

	t.Run("inserts item at the head, shifting existing items right", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.FieldsSlot()
		first := &emit.Field{Name: "First"}
		second := &emit.Field{Name: "Second"}
		assertNoError(t, slot.Append(first, emit.Provenance{SetBy: "a"}))
		assertNoError(t, slot.Prepend(second, emit.Provenance{SetBy: "b"}))
		if slot.At(0).(*emit.Field).Name != "Second" {
			t.Fatalf("Prepend should place item at index 0")
		}
	})

	t.Run("propagates kind-mismatch errors", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.FieldsSlot()
		err := slot.Prepend(&emit.Method{Name: "Save"}, emit.Provenance{})
		if !errors.Is(err, emit.ErrSlotElementType) {
			t.Fatalf("Prepend should propagate kind errors; got %v", err)
		}
	})
}

func TestSlot_InsertAt(t *testing.T) {
	t.Parallel()

	t.Run("inserts at the requested index", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.FieldsSlot()
		assertNoError(t, slot.Append(&emit.Field{Name: "A"}, emit.Provenance{}))
		assertNoError(t, slot.Append(&emit.Field{Name: "B"}, emit.Provenance{}))
		assertNoError(t, slot.InsertAt(1, &emit.Field{Name: "Mid"}, emit.Provenance{}))
		if slot.Len() != 3 || slot.At(1).(*emit.Field).Name != "Mid" {
			t.Fatalf("InsertAt order incorrect: %+v", slot.Items)
		}
	})

	t.Run("inserts at the boundary index equal to Len", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.FieldsSlot()
		assertNoError(t, slot.Append(&emit.Field{Name: "A"}, emit.Provenance{}))
		err := slot.InsertAt(1, &emit.Field{Name: "B"}, emit.Provenance{})
		assertNoError(t, err)
	})

	t.Run("rejects negative or out-of-range indexes", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.FieldsSlot()
		if err := slot.InsertAt(-1, &emit.Field{}, emit.Provenance{}); err == nil {
			t.Fatalf("expected error for negative index")
		}
		if err := slot.InsertAt(5, &emit.Field{}, emit.Provenance{}); err == nil {
			t.Fatalf("expected error for out-of-range index")
		}
	})

	t.Run("propagates kind-mismatch errors", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.FieldsSlot()
		err := slot.InsertAt(0, &emit.Method{Name: "Save"}, emit.Provenance{})
		if !errors.Is(err, emit.ErrSlotElementType) {
			t.Fatalf("InsertAt should propagate kind errors; got %v", err)
		}
	})
}

func TestSlot_At(t *testing.T) {
	t.Parallel()

	t.Run("returns the item at the supplied index", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.FieldsSlot()
		f := &emit.Field{Name: "X"}
		assertNoError(t, slot.Append(f, emit.Provenance{}))
		if slot.At(0) != f {
			t.Fatalf("At(0) should return the appended item")
		}
	})

	t.Run("returns nil for out-of-range indexes", func(t *testing.T) {
		t.Parallel()
		var s emit.Slot
		if s.At(0) != nil || s.At(-1) != nil {
			t.Fatalf("At should return nil for out-of-range indexes")
		}
	})
}

func TestSlot_ProvenanceAt(t *testing.T) {
	t.Parallel()

	t.Run("returns the recorded provenance", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.FieldsSlot()
		assertNoError(t, slot.Append(&emit.Field{Name: "X"}, emit.Provenance{SetBy: "tagger"}))
		if slot.ProvenanceAt(0).SetBy != "tagger" {
			t.Fatalf("ProvenanceAt should return the recorded value")
		}
	})

	t.Run("returns the zero provenance for out-of-range indexes", func(t *testing.T) {
		t.Parallel()
		var s emit.Slot
		got := s.ProvenanceAt(0)
		if got != (emit.Provenance{}) {
			t.Fatalf("ProvenanceAt out-of-range = %+v, want zero value", got)
		}
	})
}

func TestSlot_LazyHostSlots(t *testing.T) {
	t.Parallel()

	t.Run("repeat Slot lookups return the same instance", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		first := host.Slot("custom")
		second := host.Slot("custom")
		if first != second {
			t.Fatalf("Slot lookup should be idempotent")
		}
	})
}

func TestSlot_InsertBefore(t *testing.T) {
	t.Parallel()

	t.Run("inserts immediately before the item matching the supplied ID", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.FieldsSlot()
		assertNoError(t, slot.Append(&emit.Field{Name: "first"}, emit.Provenance{ID: "a"}))
		assertNoError(t, slot.Append(&emit.Field{Name: "last"}, emit.Provenance{ID: "c"}))
		assertNoError(t, slot.InsertBefore("c", &emit.Field{Name: "middle"}, emit.Provenance{ID: "b"}))
		names := []string{
			slot.At(0).(*emit.Field).Name,
			slot.At(1).(*emit.Field).Name,
			slot.At(2).(*emit.Field).Name,
		}
		if names[0] != "first" || names[1] != "middle" || names[2] != "last" {
			t.Fatalf("InsertBefore order mismatch: %v", names)
		}
	})

	t.Run("returns ErrProvenanceNotFound for unknown ID", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.FieldsSlot()
		assertNoError(t, slot.Append(&emit.Field{Name: "x"}, emit.Provenance{ID: "a"}))
		err := slot.InsertBefore("nope", &emit.Field{Name: "y"}, emit.Provenance{})
		if !errors.Is(err, emit.ErrProvenanceNotFound) {
			t.Fatalf("InsertBefore on unknown ID should return ErrProvenanceNotFound; got %v", err)
		}
	})

	t.Run("returns ErrProvenanceNotFound for empty ID", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.FieldsSlot()
		assertNoError(t, slot.Append(&emit.Field{Name: "x"}, emit.Provenance{}))
		err := slot.InsertBefore("", &emit.Field{Name: "y"}, emit.Provenance{})
		if !errors.Is(err, emit.ErrProvenanceNotFound) {
			t.Fatalf("InsertBefore(\"\") should return ErrProvenanceNotFound; got %v", err)
		}
	})
}

func TestSlot_InsertAfter(t *testing.T) {
	t.Parallel()

	t.Run("inserts immediately after the item matching the supplied ID", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.FieldsSlot()
		assertNoError(t, slot.Append(&emit.Field{Name: "first"}, emit.Provenance{ID: "a"}))
		assertNoError(t, slot.Append(&emit.Field{Name: "last"}, emit.Provenance{ID: "c"}))
		assertNoError(t, slot.InsertAfter("a", &emit.Field{Name: "middle"}, emit.Provenance{ID: "b"}))
		names := []string{
			slot.At(0).(*emit.Field).Name,
			slot.At(1).(*emit.Field).Name,
			slot.At(2).(*emit.Field).Name,
		}
		if names[0] != "first" || names[1] != "middle" || names[2] != "last" {
			t.Fatalf("InsertAfter order mismatch: %v", names)
		}
	})

	t.Run("returns ErrProvenanceNotFound for unknown ID", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.FieldsSlot()
		assertNoError(t, slot.Append(&emit.Field{Name: "x"}, emit.Provenance{ID: "a"}))
		err := slot.InsertAfter("nope", &emit.Field{Name: "y"}, emit.Provenance{})
		if !errors.Is(err, emit.ErrProvenanceNotFound) {
			t.Fatalf("InsertAfter on unknown ID should return ErrProvenanceNotFound; got %v", err)
		}
	})

	t.Run("returns ErrProvenanceNotFound for empty ID", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.FieldsSlot()
		err := slot.InsertAfter("", &emit.Field{Name: "y"}, emit.Provenance{})
		if !errors.Is(err, emit.ErrProvenanceNotFound) {
			t.Fatalf("InsertAfter(\"\") should return ErrProvenanceNotFound; got %v", err)
		}
	})

	t.Run("InsertAfter the last item appends to the end", func(t *testing.T) {
		t.Parallel()
		host := &emit.Struct{Name: "User"}
		slot := host.FieldsSlot()
		assertNoError(t, slot.Append(&emit.Field{Name: "only"}, emit.Provenance{ID: "a"}))
		assertNoError(t, slot.InsertAfter("a", &emit.Field{Name: "appended"}, emit.Provenance{}))
		if slot.Len() != 2 || slot.At(1).(*emit.Field).Name != "appended" {
			t.Fatalf("InsertAfter last should land at the end")
		}
	})
}

// naiveSlot is the reference implementation [FuzzSlot_Insert]
// differentials [emit.Slot] against. It models the same contract —
// two parallel slices, items and provenance, kept in lock-step — but
// splices with [slices.Insert] instead of the production
// `append(s[:i], append([]T{x}, s[i:]...)...)` surgery, and never
// shares a backing array between the two.
//
// The point of the duplication is that the naive version is obviously
// correct by inspection. A divergence between the two therefore
// indicts the slice arithmetic in [emit.Slot], not a disagreement
// about what insertion means.
type naiveSlot struct {
	texts []string
	ids   []string
}

// insertAt splices text and its provenance id in at index i. Every
// caller derives i from the model's own length or from indexOf, so
// the index is always in range.
func (n *naiveSlot) insertAt(i int, text, id string) {
	n.texts = slices.Insert(n.texts, i, text)
	n.ids = slices.Insert(n.ids, i, id)
}

// indexOf returns the position of the first entry stamped with id, or
// -1. First-match is the rule [emit.Slot.InsertBefore] documents for
// duplicate ids, which is why the fuzz target draws ids from an
// alphabet small enough that duplicates are common rather than rare.
func (n *naiveSlot) indexOf(id string) int { return slices.Index(n.ids, id) }

// FuzzSlot_Insert drives a random program of slot mutations against
// [emit.Slot] and an obviously-correct reference model, then compares
// the two orderings element by element.
//
// Slot is the composition mechanism of the whole emit graph: every
// cross-cutting plugin lands its contribution here, and rendering
// walks the result in order. Its dangerous failure is silent
// misordering or — worse — the item slice and the provenance slice
// drifting apart, which would attribute one plugin's code to another
// in `eidos explain` while still rendering valid-looking output.
// Neither failure crashes, so a target that only checked for panics
// would pass through both. The properties asserted are ordering
// (differential against the model), lock-step length (checked after
// every single step, not just at the end), and that a rejected
// operation leaves the slot untouched.
//
// Each program byte pair is one operation: the first selects among
// append / prepend / insert-at / out-of-range insert-at /
// insert-before / insert-after / kind-mismatched insert, the second
// supplies the index or anchor. Ids are drawn from {"", a, b, c} so
// duplicate anchors and the rejected empty anchor both occur; item
// texts are the step offset, so they stay unique and the ordering
// comparison is exact.
func FuzzSlot_Insert(f *testing.F) {
	for _, seed := range [][]byte{
		{},                             // no operations at all
		{0},                            // odd-length program: the trailing byte is ignored
		{0, 0},                         // a single append
		{1, 1},                         // prepend into an empty slot
		{0, 1, 1, 2},                   // prepend onto a non-empty slot, where head and tail differ
		{2, 0},                         // InsertAt(0) into an empty slot — the boundary index
		{3, 0},                         // out-of-range InsertAt against an empty slot
		{4, 1},                         // InsertBefore an anchor that does not exist
		{5, 1},                         // InsertAfter an anchor that does not exist
		{6, 1},                         // a kind-mismatched append
		{0, 0, 4, 0},                   // the empty anchor, which never matches
		{0, 1, 0, 1, 4, 1},             // duplicate ids, then insert before the first of them
		{0, 1, 0, 1, 5, 1},             // duplicate ids, then insert after the first of them
		{0, 1, 0, 2, 2, 1, 5, 2, 4, 1}, // mixed positional and anchored inserts
		{0, 1, 6, 1, 3, 9, 0, 2, 5, 1}, // rejected operations interleaved with accepted ones
		{255, 255},                     // the top of both byte ranges
		bytes.Repeat([]byte{0, 1}, 64), // deep: 64 contributions onto a single anchor id
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, program []byte) {
		// The empty id is in the alphabet deliberately: the insert
		// helpers reject an empty anchor outright, where the naive
		// model would match it against every unstamped contribution.
		anchors := []string{"", "a", "b", "c"}

		slot := &emit.Slot{SlotName: "prebody", ElemKind: emit.KindStmt}
		ref := &naiveSlot{}

		for i := 0; i+1 < len(program); i += 2 {
			op, arg := int(program[i])%7, int(program[i+1])
			text := strconv.Itoa(i)
			id := anchors[arg%len(anchors)]
			stmt := emit.NewRawStmt(text)
			prov := emit.Provenance{SetBy: "fuzz", ID: id}

			switch op {
			case 0:
				if err := slot.Append(stmt, prov); err != nil {
					t.Fatalf("Append(%q) rejected a correctly-kinded item: %v", text, err)
				}
				ref.insertAt(len(ref.texts), text, id)
			case 1:
				if err := slot.Prepend(stmt, prov); err != nil {
					t.Fatalf("Prepend(%q) rejected a correctly-kinded item: %v", text, err)
				}
				ref.insertAt(0, text, id)
			case 2:
				at := arg % (len(ref.texts) + 1)
				if err := slot.InsertAt(at, stmt, prov); err != nil {
					t.Fatalf("InsertAt(%d) on a slot of %d rejected: %v", at, len(ref.texts), err)
				}
				ref.insertAt(at, text, id)
			case 3:
				// Both ends of the range. A rejected insert that had
				// already mutated one of the two slices would show up
				// in the lock-step check below.
				for _, at := range []int{-1 - arg%3, len(ref.texts) + 1 + arg%3} {
					if err := slot.InsertAt(at, stmt, prov); err == nil {
						t.Fatalf("InsertAt(%d) accepted an out-of-range index on a slot of %d", at, slot.Len())
					}
				}
			case 4:
				want := -1
				if id != "" {
					want = ref.indexOf(id)
				}
				err := slot.InsertBefore(id, stmt, prov)
				if want < 0 {
					if !errors.Is(err, emit.ErrProvenanceNotFound) {
						t.Fatalf("InsertBefore(%q) against a slot without that anchor returned %v", id, err)
					}
					break
				}
				if err != nil {
					t.Fatalf("InsertBefore(%q) failed with the anchor at index %d: %v", id, want, err)
				}
				ref.insertAt(want, text, id)
			case 5:
				want := -1
				if id != "" {
					want = ref.indexOf(id)
				}
				err := slot.InsertAfter(id, stmt, prov)
				if want < 0 {
					if !errors.Is(err, emit.ErrProvenanceNotFound) {
						t.Fatalf("InsertAfter(%q) against a slot without that anchor returned %v", id, err)
					}
					break
				}
				if err != nil {
					t.Fatalf("InsertAfter(%q) failed with the anchor at index %d: %v", id, want, err)
				}
				ref.insertAt(want+1, text, id)
			case 6:
				// A kind mismatch must be rejected before either slice
				// is touched; a half-applied insert would attribute
				// every later contribution to the wrong plugin. Routed
				// through all five mutators in turn, because each does
				// its own bounds or anchor work first and could land
				// the item before the kind check runs. The anchor may
				// legitimately be absent, so the assertion is that the
				// call failed and the slot is unchanged — the specific
				// sentinel is not the property under test here.
				bad := &emit.Field{Name: text}
				var err error
				var via string
				switch arg % 5 {
				case 0:
					via, err = "Append", slot.Append(bad, prov)
				case 1:
					via, err = "Prepend", slot.Prepend(bad, prov)
				case 2:
					via, err = "InsertAt", slot.InsertAt(arg%(len(ref.texts)+1), bad, prov)
				case 3:
					via, err = "InsertBefore", slot.InsertBefore(id, bad, prov)
				case 4:
					via, err = "InsertAfter", slot.InsertAfter(id, bad, prov)
				}
				if err == nil {
					t.Fatalf("%s accepted an *emit.Field into a slot declared as %s", via, emit.KindStmt)
				}
			}

			if slot.Len() != len(slot.ProvenanceList) {
				t.Fatalf("step %d (op %d) desynchronised the parallel slices: %d items, %d provenance entries",
					i/2, op, slot.Len(), len(slot.ProvenanceList))
			}
			// Compared after every step rather than only at the end so
			// the reported step is the one that diverged. This is also
			// what makes the rejected-operation cases meaningful: a
			// failed call that had already mutated one slice shows up
			// here, on the step that made it.
			assertSlotMatchesModel(t, i/2, op, slot, ref)
		}

		assertSlotMatchesModel(t, len(program)/2, -1, slot, ref)
	})
}

// assertSlotMatchesModel compares a live slot against the reference
// model element by element, in both the item and the provenance
// dimension. step and op are echoed into the failure so a divergence
// names the operation that caused it; op is -1 for the final
// end-of-program comparison.
func assertSlotMatchesModel(t *testing.T, step, op int, slot *emit.Slot, ref *naiveSlot) {
	t.Helper()

	if slot.Len() != len(ref.texts) {
		t.Fatalf("step %d (op %d): slot holds %d items, reference model holds %d",
			step, op, slot.Len(), len(ref.texts))
	}
	for i, wantText := range ref.texts {
		got, ok := slot.At(i).(*emit.Stmt)
		if !ok {
			t.Fatalf("step %d (op %d): item %d is %T, want *emit.Stmt", step, op, i, slot.At(i))
		}
		if got.RawText != wantText {
			t.Fatalf("step %d (op %d): item %d is %q, reference model says %q (full order %q vs %q)",
				step, op, i, got.RawText, wantText, slotTexts(slot), ref.texts)
		}
		if gotID := slot.ProvenanceAt(i).ID; gotID != ref.ids[i] {
			t.Fatalf("step %d (op %d): provenance %d is %q, reference model says %q",
				step, op, i, gotID, ref.ids[i])
		}
	}
}

// slotTexts renders a slot's items as their raw statement text, for
// the differential failure message — the whole ordering is what makes
// a misplacement diagnosable, not the one index that differs.
func slotTexts(s *emit.Slot) []string {
	out := make([]string, s.Len())
	for i := range s.Len() {
		if stmt, ok := s.At(i).(*emit.Stmt); ok {
			out[i] = stmt.RawText
		}
	}
	return out
}

// BenchmarkSlot_Append measures composing one slot out of n
// independent contributions — the shape a heavily cross-cut host
// takes when several plugins each append to the same "prebody".
//
// The whole population is one operation, so cost per operation is
// expected to grow linearly with n; the scaling axis exists to prove
// that it does, since a slot that reallocated on every append rather
// than amortising would show up as quadratic here.
//
// Deliberately outside the timed region: the statement and provenance
// values, which are hoisted and shared across every append. The slot
// itself is allocated inside the loop because a fresh slot is the
// subject of the measurement — reusing one would measure appends into
// an already-grown backing array instead.
func BenchmarkSlot_Append(b *testing.B) {
	for _, n := range []int{1, 10, 100, 1000} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()

			stmt := emit.NewRawStmt("audit.Log(ctx)")
			prov := emit.Provenance{SetBy: "audit"}

			for b.Loop() {
				slot := &emit.Slot{SlotName: "prebody", ElemKind: emit.KindStmt}
				for range n {
					if err := slot.Append(stmt, prov); err != nil {
						b.Fatalf("Append: %v", err)
					}
				}
			}
		})
	}
}

// BenchmarkSlot_InsertBefore measures a single anchored insert into a
// slot that already holds n contributions.
//
// [emit.Slot.InsertBefore] resolves its anchor with a linear scan of
// the provenance list. The anchor is deliberately the last-appended
// contribution, so the scan runs its worst case while the subsequent
// shift moves exactly one element: the measurement isolates the scan.
// That is the number that matters, because k plugins each anchoring
// into the same slot pay it k times over a slot that is itself
// growing — the accidental quadratic this scaling axis exists to
// expose.
//
// Deliberately outside the timed region: building the n-item fixture,
// and the inserted statement and provenance values.
//
// Inside the timed region, and deliberately so: the two reslice
// statements that truncate the slot back to n items. The fixture is
// allocated with capacity n+1 so the truncation never triggers a
// reallocation, which keeps the reset at two slice-header writes —
// negligible against an n-element scan, and the only alternative to
// stopping the timer on every iteration. The inserted contribution
// reuses the anchor's own id, so the truncated slot is structurally
// identical at the head of every iteration.
func BenchmarkSlot_InsertBefore(b *testing.B) {
	const anchor = "host.anchor"

	for _, n := range []int{1, 10, 100, 1000} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()

			slot := &emit.Slot{
				SlotName:       "prebody",
				ElemKind:       emit.KindStmt,
				Items:          make([]emit.Node, 0, n+1),
				ProvenanceList: make([]emit.Provenance, 0, n+1),
			}
			for i := range n {
				id := strconv.Itoa(i)
				if i == n-1 {
					id = anchor
				}
				if err := slot.Append(emit.NewRawStmt(id), emit.Provenance{SetBy: "host", ID: id}); err != nil {
					b.Fatalf("fixture Append: %v", err)
				}
			}

			stmt := emit.NewRawStmt("start := time.Now()")
			prov := emit.Provenance{SetBy: "metrics", ID: anchor}

			for b.Loop() {
				slot.Items = slot.Items[:n]
				slot.ProvenanceList = slot.ProvenanceList[:n]
				if err := slot.InsertBefore(anchor, stmt, prov); err != nil {
					b.Fatalf("InsertBefore: %v", err)
				}
			}
		})
	}
}
