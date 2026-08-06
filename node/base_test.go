// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node_test

import (
	"sync"
	"testing"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

func TestBaseNode_Pos(t *testing.T) {
	t.Parallel()

	t.Run("returns the SourcePos", func(t *testing.T) {
		t.Parallel()
		pos := position.At("a.go", 12, 1)
		b := &node.BaseNode{SourcePos: pos}
		if b.Pos() != pos {
			t.Fatalf("Pos = %+v, want %+v", b.Pos(), pos)
		}
	})
}

func TestBaseNode_Docs(t *testing.T) {
	t.Parallel()

	t.Run("returns the DocLines slice", func(t *testing.T) {
		t.Parallel()
		b := &node.BaseNode{DocLines: []string{"first", "second"}}
		got := b.Docs()
		if len(got) != 2 || got[0] != "first" || got[1] != "second" {
			t.Fatalf("Docs = %v", got)
		}
	})
}

func TestBaseNode_Directives(t *testing.T) {
	t.Parallel()

	t.Run("returns the DirectiveList slice", func(t *testing.T) {
		t.Parallel()
		d := directiveAt("mock", position.Pos{})
		b := &node.BaseNode{DirectiveList: []*directive.Directive{d}}
		got := b.Directives()
		if len(got) != 1 || got[0] != d {
			t.Fatalf("Directives = %v", got)
		}
	})
}

func TestBaseNode_Directive(t *testing.T) {
	t.Parallel()

	t.Run("returns the first matching directive", func(t *testing.T) {
		t.Parallel()
		first := directiveAt("mock", position.At("a.go", 1, 1))
		second := directiveAt("mock", position.At("a.go", 2, 1))
		b := &node.BaseNode{DirectiveList: []*directive.Directive{first, second}}
		if got := b.Directive("mock"); got != first {
			t.Fatalf("Directive returned the wrong instance")
		}
	})

	t.Run("returns nil when no match exists", func(t *testing.T) {
		t.Parallel()
		var b node.BaseNode
		if got := b.Directive("mock"); got != nil {
			t.Fatalf("Directive on empty list = %v, want nil", got)
		}
	})
}

func TestBaseNode_HasDirective(t *testing.T) {
	t.Parallel()

	t.Run("returns true when at least one match exists", func(t *testing.T) {
		t.Parallel()
		b := &node.BaseNode{DirectiveList: []*directive.Directive{directiveAt("mock", position.Pos{})}}
		if !b.HasDirective("mock") {
			t.Fatalf("HasDirective should be true")
		}
	})

	t.Run("returns false when no match exists", func(t *testing.T) {
		t.Parallel()
		var b node.BaseNode
		if b.HasDirective("mock") {
			t.Fatalf("HasDirective on empty list should be false")
		}
	})
}

func TestBaseNode_HasPositiveDirective(t *testing.T) {
	t.Parallel()

	t.Run("matches an unnegated entry", func(t *testing.T) {
		t.Parallel()
		b := &node.BaseNode{DirectiveList: []*directive.Directive{{Name: "repo"}}}
		if !b.HasPositiveDirective("repo") {
			t.Fatalf("HasPositiveDirective should match the +gen:repo form")
		}
	})

	t.Run("does not match a negated entry", func(t *testing.T) {
		t.Parallel()
		b := &node.BaseNode{DirectiveList: []*directive.Directive{{Name: "repo", Negated: true}}}
		if b.HasPositiveDirective("repo") {
			t.Fatalf("HasPositiveDirective should not match the -gen:repo form")
		}
	})
}

func TestBaseNode_HasNegatedDirective(t *testing.T) {
	t.Parallel()

	t.Run("matches a negated entry", func(t *testing.T) {
		t.Parallel()
		b := &node.BaseNode{DirectiveList: []*directive.Directive{{Name: "repo", Negated: true}}}
		if !b.HasNegatedDirective("repo") {
			t.Fatalf("HasNegatedDirective should match the -gen:repo form")
		}
	})

	t.Run("does not match an unnegated entry", func(t *testing.T) {
		t.Parallel()
		b := &node.BaseNode{DirectiveList: []*directive.Directive{{Name: "repo"}}}
		if b.HasNegatedDirective("repo") {
			t.Fatalf("HasNegatedDirective should not match the +gen:repo form")
		}
	})
}

func TestBaseNode_Meta(t *testing.T) {
	t.Parallel()

	t.Run("returns nil without allocating on an unstamped node", func(t *testing.T) {
		t.Parallel()
		var b node.BaseNode
		if got := b.Meta(); got != nil {
			t.Fatalf("Meta should not allocate; got %p", got)
		}
		if b.MetaBag != nil {
			t.Fatalf("Meta wrote to the receiver: %p", b.MetaBag)
		}
	})

	t.Run("the nil bag reads as the empty bag", func(t *testing.T) {
		t.Parallel()
		var b node.BaseNode
		if b.Meta().Has("anything") {
			t.Fatalf("nil bag should report nothing set")
		}
		if got := b.Meta().Names(); got != nil {
			t.Fatalf("nil bag should report no names; got %v", got)
		}
	})

	t.Run("EnsureMeta creates and caches the bag", func(t *testing.T) {
		t.Parallel()
		var b node.BaseNode
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
		var b node.BaseNode
		if first, second := b.EnsureMeta(), b.EnsureMeta(); first != second {
			t.Fatalf("EnsureMeta should return the same instance on every call")
		}
	})

	t.Run("concurrent reads of an unstamped node do not race", func(t *testing.T) {
		t.Parallel()
		// The node graph is the one a bucket of parallel NodesOnly
		// generators shares, and reading annotator-stamped metadata
		// is exactly what they do — so this path is concurrent by
		// contract rather than by accident.
		var b node.BaseNode
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				_ = b.Meta().Has("shape")
			})
		}
		wg.Wait()
		if b.MetaBag != nil {
			t.Fatalf("concurrent reads created a bag: %p", b.MetaBag)
		}
	})
}

// TestBaseNode_MetaAllocations mirrors the emit-side assertion. Not
// parallel: testing.AllocsPerRun panics inside a parallel test.
//
//nolint:paralleltest // testing.AllocsPerRun panics in a parallel test.
func TestBaseNode_MetaAllocations(t *testing.T) {
	var b node.BaseNode
	if got := testing.AllocsPerRun(100, func() { _ = b.Meta().Has("k") }); got != 0 {
		t.Fatalf("Meta().Has on an unstamped node should not allocate; got %v allocs/op", got)
	}
}
