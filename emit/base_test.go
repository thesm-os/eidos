// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

func TestBaseEmit_Pos(t *testing.T) {
	t.Parallel()

	t.Run("returns the SourcePos", func(t *testing.T) {
		t.Parallel()
		pos := position.At("a.go", 12, 1)
		b := &emit.BaseEmit{SourcePos: pos}
		if b.Pos() != pos {
			t.Fatalf("Pos = %+v, want %+v", b.Pos(), pos)
		}
	})
}

func TestBaseEmit_Docs(t *testing.T) {
	t.Parallel()

	t.Run("returns the DocLines slice", func(t *testing.T) {
		t.Parallel()
		b := &emit.BaseEmit{DocLines: []string{"first", "second"}}
		got := b.Docs()
		if len(got) != 2 || got[0] != "first" || got[1] != "second" {
			t.Fatalf("Docs = %v", got)
		}
	})
}

func TestBaseEmit_Directives(t *testing.T) {
	t.Parallel()

	t.Run("returns the DirectiveList slice", func(t *testing.T) {
		t.Parallel()
		d := directiveAt("mock", position.Pos{})
		b := &emit.BaseEmit{DirectiveList: []*directive.Directive{d}}
		got := b.Directives()
		if len(got) != 1 || got[0] != d {
			t.Fatalf("Directives = %v", got)
		}
	})
}

func TestBaseEmit_Directive(t *testing.T) {
	t.Parallel()

	t.Run("returns the first matching directive", func(t *testing.T) {
		t.Parallel()
		first := directiveAt("mock", position.At("a.go", 1, 1))
		second := directiveAt("mock", position.At("a.go", 2, 1))
		b := &emit.BaseEmit{DirectiveList: []*directive.Directive{first, second}}
		if got := b.Directive("mock"); got != first {
			t.Fatalf("Directive returned the wrong instance")
		}
	})

	t.Run("returns nil when no match exists", func(t *testing.T) {
		t.Parallel()
		var b emit.BaseEmit
		if got := b.Directive("mock"); got != nil {
			t.Fatalf("Directive on empty list = %v, want nil", got)
		}
	})
}

func TestBaseEmit_HasDirective(t *testing.T) {
	t.Parallel()

	t.Run("returns true when at least one match exists", func(t *testing.T) {
		t.Parallel()
		b := &emit.BaseEmit{DirectiveList: []*directive.Directive{directiveAt("mock", position.Pos{})}}
		if !b.HasDirective("mock") {
			t.Fatalf("HasDirective should be true")
		}
	})

	t.Run("returns false when no match exists", func(t *testing.T) {
		t.Parallel()
		var b emit.BaseEmit
		if b.HasDirective("mock") {
			t.Fatalf("HasDirective on empty list should be false")
		}
	})
}

func TestBaseEmit_HasPositiveDirective(t *testing.T) {
	t.Parallel()

	t.Run("matches an unnegated entry", func(t *testing.T) {
		t.Parallel()
		b := &emit.BaseEmit{DirectiveList: []*directive.Directive{{Name: "mock"}}}
		if !b.HasPositiveDirective("mock") {
			t.Fatalf("HasPositiveDirective should match the +gen:mock form")
		}
	})

	t.Run("does not match a negated entry", func(t *testing.T) {
		t.Parallel()
		b := &emit.BaseEmit{DirectiveList: []*directive.Directive{{Name: "mock", Negated: true}}}
		if b.HasPositiveDirective("mock") {
			t.Fatalf("HasPositiveDirective should not match the -gen:mock form")
		}
	})
}

func TestBaseEmit_HasNegatedDirective(t *testing.T) {
	t.Parallel()

	t.Run("matches a negated entry", func(t *testing.T) {
		t.Parallel()
		b := &emit.BaseEmit{DirectiveList: []*directive.Directive{{Name: "mock", Negated: true}}}
		if !b.HasNegatedDirective("mock") {
			t.Fatalf("HasNegatedDirective should match the -gen:mock form")
		}
	})

	t.Run("does not match an unnegated entry", func(t *testing.T) {
		t.Parallel()
		b := &emit.BaseEmit{DirectiveList: []*directive.Directive{{Name: "mock"}}}
		if b.HasNegatedDirective("mock") {
			t.Fatalf("HasNegatedDirective should not match the +gen:mock form")
		}
	})
}

// TestBaseEmit_Meta pins the read/write split. Meta answers
// questions and must not touch the receiver; EnsureMeta is the
// accessor that creates.
func TestBaseEmit_Meta(t *testing.T) {
	t.Parallel()

	t.Run("returns nil without allocating on an unwritten value", func(t *testing.T) {
		t.Parallel()
		var b emit.BaseEmit
		if got := b.Meta(); got != nil {
			t.Fatalf("Meta should not allocate; got %p", got)
		}
		if b.MetaBag != nil {
			t.Fatalf("Meta wrote to the receiver: %p", b.MetaBag)
		}
	})

	t.Run("the nil bag reads as the empty bag", func(t *testing.T) {
		t.Parallel()
		// This is what makes the nil return safe to publish: a
		// caller writes n.Meta().Has(k) with no nil check.
		var b emit.BaseEmit
		if b.Meta().Has("anything") {
			t.Fatalf("nil bag should report nothing set")
		}
		if got := b.Meta().Names(); got != nil {
			t.Fatalf("nil bag should report no names; got %v", got)
		}
	})

	t.Run("EnsureMeta creates and caches the bag", func(t *testing.T) {
		t.Parallel()
		var b emit.BaseEmit
		bag := b.EnsureMeta()
		if bag == nil {
			t.Fatalf("EnsureMeta should return a non-nil bag")
		}
		if b.MetaBag != bag {
			t.Fatalf("EnsureMeta should cache the bag on the receiver")
		}
	})

	t.Run("EnsureMeta returns the same bag on subsequent calls", func(t *testing.T) {
		t.Parallel()
		var b emit.BaseEmit
		if first, second := b.EnsureMeta(), b.EnsureMeta(); first != second {
			t.Fatalf("EnsureMeta should return the same instance on every call")
		}
	})

	t.Run("Meta returns what EnsureMeta created", func(t *testing.T) {
		t.Parallel()
		var b emit.BaseEmit
		created := b.EnsureMeta()
		if got := b.Meta(); got != created {
			t.Fatalf("Meta should return the bag EnsureMeta created")
		}
	})

	t.Run("concurrent reads of an unwritten value do not race", func(t *testing.T) {
		t.Parallel()
		// The regression barrier. Under -race the lazy-allocating
		// accessor produced several distinct reports here, the worst
		// being a reader inside Bag.Has holding its own bag's read
		// lock while another goroutine's NewBag initialised a
		// different mutex word — the lock was the object being raced
		// on, so the bag could not defend itself.
		var b emit.BaseEmit
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				_ = b.Meta().Has("validate")
			})
		}
		wg.Wait()
		if b.MetaBag != nil {
			t.Fatalf("concurrent reads created a bag: %p", b.MetaBag)
		}
	})

	t.Run("reading does not change the serialised form", func(t *testing.T) {
		t.Parallel()
		// json:"meta,omitempty" does not omit a non-nil pointer to
		// an empty bag, so the lazy allocation made a read change
		// the document a later marshal produced.
		f := &emit.Field{Name: "F"}
		// Same reason as the sibling marshalling tests: the Type and
		// OriginNode fields are tagged `json:"-"`.
		//
		//nolint:musttag
		before, err := json.Marshal(f)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		_ = f.Meta().Has("k")
		//nolint:musttag
		after, err := json.Marshal(f)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if !bytes.Equal(before, after) {
			t.Fatalf("a read changed the document:\nbefore: %s\nafter:  %s", before, after)
		}
	})
}

// TestBaseEmit_MetaAllocations pins the cost side of the read/write
// split.
//
// Deliberately not parallel, and not a subtest of a parallel test:
// testing.AllocsPerRun panics in either, because a concurrent
// goroutine's allocations would land in the same count.
//
// The allocation this asserts away was one-shot and permanent — one
// bag per node anything ever asked a question about, retained for
// the life of the emit tree. Measured at 2 allocs and 112 B before
// the split: 64 for the Bag, rounded up from 56, plus a 48-byte map
// header for a map that then stays empty because the caller was
// reading. An allocation assertion rather than a timing one, so it
// cannot go flaky.
//
//nolint:paralleltest // testing.AllocsPerRun panics in a parallel test.
func TestBaseEmit_MetaAllocations(t *testing.T) {
	var b emit.BaseEmit
	if got := testing.AllocsPerRun(100, func() { _ = b.Meta().Has("k") }); got != 0 {
		t.Fatalf("Meta().Has on an unwritten value should not allocate; got %v allocs/op", got)
	}
}

// TestSlotsByName_DoesNotMaterialise pins the slot-map accessor.
//
// An allocation assertion cannot carry this one: the lazy map was
// created once and cached, so testing.AllocsPerRun charges it to the
// warm-up run and reports zero for a host that very much did
// allocate. The observable that survives is the returned value —
// nil, repeatedly, for a host with no slots.
//
// nil is a valid empty map here: every consumer in the tree only
// takes len or ranges it, and both are nil-safe.
func TestSlotsByName_DoesNotMaterialise(t *testing.T) {
	t.Parallel()

	t.Run("a slot-less host returns nil", func(t *testing.T) {
		t.Parallel()
		var s emit.Struct
		if got := s.SlotsByName(); got != nil {
			t.Fatalf("SlotsByName materialised a map: %v", got)
		}
	})

	t.Run("repeated reads keep returning nil", func(t *testing.T) {
		t.Parallel()
		// The lazy version answered nil once and non-nil forever
		// after, so a single call could not tell the two apart.
		var s emit.Struct
		_ = s.SlotsByName()
		if got := s.SlotsByName(); got != nil {
			t.Fatalf("a read created the map: %v", got)
		}
	})

	t.Run("len and range tolerate the nil result", func(t *testing.T) {
		t.Parallel()
		var s emit.Struct
		if n := len(s.SlotsByName()); n != 0 {
			t.Fatalf("len = %d, want 0", n)
		}
		for name := range s.SlotsByName() {
			t.Fatalf("ranged an entry %q on a slot-less host", name)
		}
	})

	t.Run("a host with a slot still returns it", func(t *testing.T) {
		t.Parallel()
		// The control: returning m.slots unconditionally must not
		// have broken the case the accessor exists for.
		s := &emit.Struct{Name: "S"}
		s.FieldsSlot()
		if got := s.SlotsByName(); len(got) == 0 {
			t.Fatalf("SlotsByName lost a registered slot")
		}
	})
}

func TestBaseEmit_Origin(t *testing.T) {
	t.Parallel()

	t.Run("returns the OriginNode field", func(t *testing.T) {
		t.Parallel()
		src := &node.Struct{Name: "Source"}
		b := &emit.BaseEmit{OriginNode: src}
		if b.Origin() != src {
			t.Fatalf("Origin should return the configured OriginNode")
		}
	})

	t.Run("returns nil for synthetic emit values", func(t *testing.T) {
		t.Parallel()
		var b emit.BaseEmit
		if b.Origin() != nil {
			t.Fatalf("zero-value BaseEmit should report nil Origin")
		}
	})
}

func TestBaseEmit_OutputTag(t *testing.T) {
	t.Parallel()

	t.Run("zero value carries an empty OutputTag", func(t *testing.T) {
		t.Parallel()
		var b emit.BaseEmit
		if b.OutputTag() != "" {
			t.Fatalf("zero-value OutputTag = %q, want empty", b.OutputTag())
		}
	})

	t.Run("a non-empty OutputTag round-trips through struct literal", func(t *testing.T) {
		t.Parallel()
		b := emit.BaseEmit{OutputTagName: "test"}
		if b.OutputTag() != "test" {
			t.Fatalf("OutputTag = %q, want %q", b.OutputTag(), "test")
		}
	})

	t.Run("empty OutputTag is omitted from JSON output", func(t *testing.T) {
		t.Parallel()
		var b emit.BaseEmit
		// musttag is satisfied: every JSON-exported field on
		// BaseEmit carries a json tag. The interface-typed
		// OriginNode field is tagged `json:"-"` and never
		// serialised; the linter can't statically prove the
		// negative.
		//
		//nolint:musttag
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if strings.Contains(string(raw), "output_tag") {
			t.Fatalf("empty OutputTag should be omitted; got %s", raw)
		}
	})

	t.Run("non-empty OutputTag round-trips through JSON", func(t *testing.T) {
		t.Parallel()
		b := emit.BaseEmit{OutputTagName: "test"}
		//nolint:musttag
		raw, err := json.Marshal(b)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		if !strings.Contains(string(raw), `"output_tag":"test"`) {
			t.Fatalf(`marshalled JSON missing "output_tag":"test"; got %s`, raw)
		}
		var out emit.BaseEmit
		//nolint:musttag
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if out.OutputTag() != "test" {
			t.Fatalf("round-trip OutputTag = %q, want %q", out.OutputTag(), "test")
		}
	})
}

func TestBaseEmit_SetBy(t *testing.T) {
	t.Parallel()

	t.Run("returns the SetByName field", func(t *testing.T) {
		t.Parallel()
		b := &emit.BaseEmit{SetByName: "repogen"}
		if got := b.SetBy(); got != "repogen" {
			t.Fatalf("SetBy = %q, want %q", got, "repogen")
		}
	})

	t.Run("returns the empty string for unattributed entities", func(t *testing.T) {
		t.Parallel()
		var b emit.BaseEmit
		if got := b.SetBy(); got != "" {
			t.Fatalf("zero-value SetBy = %q, want \"\"", got)
		}
	})
}
