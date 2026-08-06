// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
)

func TestReturn_Kind(t *testing.T) {
	t.Parallel()

	t.Run("returns KindReturn", func(t *testing.T) {
		t.Parallel()
		var r node.Return
		if got := r.Kind(); got != node.KindReturn {
			t.Fatalf("Kind() = %v, want %v", got, node.KindReturn)
		}
	})
}

func TestReturnTypes(t *testing.T) {
	t.Parallel()

	t.Run("nil input returns nil", func(t *testing.T) {
		t.Parallel()
		if got := node.ReturnTypes(nil); got != nil {
			t.Fatalf("ReturnTypes(nil) = %v, want nil", got)
		}
	})

	t.Run("empty input returns an empty non-nil slice", func(t *testing.T) {
		t.Parallel()
		got := node.ReturnTypes([]*node.Return{})
		if got == nil || len(got) != 0 {
			t.Fatalf("ReturnTypes(empty) = %v, want empty non-nil slice", got)
		}
	})

	t.Run("projects the declared type out of each slot", func(t *testing.T) {
		t.Parallel()
		str, err := namedRef("", "string"), namedRef("", "error")
		got := node.ReturnTypes([]*node.Return{{Name: "item", Type: str}, {Type: err}})
		if len(got) != 2 || got[0] != str || got[1] != err {
			t.Fatalf("ReturnTypes projected %v, want [%v %v]", got, str, err)
		}
	})

	t.Run("preserves a nil slot as a nil type", func(t *testing.T) {
		t.Parallel()
		str := namedRef("", "string")
		got := node.ReturnTypes([]*node.Return{{Type: str}, nil, {Type: str}})
		if len(got) != 3 || got[1] != nil {
			t.Fatalf("ReturnTypes(nil slot) = %v, want index 1 nil", got)
		}
	})

	t.Run("keeps arity when every slot is nil", func(t *testing.T) {
		t.Parallel()
		if got := node.ReturnTypes([]*node.Return{nil, nil}); len(got) != 2 {
			t.Fatalf("ReturnTypes len = %d, want 2", len(got))
		}
	})

	t.Run("drops the binding name", func(t *testing.T) {
		t.Parallel()
		str := namedRef("", "string")
		got := node.ReturnTypes([]*node.Return{{Name: "item", Type: str}})
		if len(got) != 1 || got[0] != str {
			t.Fatalf("ReturnTypes = %v, want the bare type", got)
		}
	})
}

func TestAnonReturns(t *testing.T) {
	t.Parallel()

	t.Run("no types returns an empty non-nil slice", func(t *testing.T) {
		t.Parallel()
		got := node.AnonReturns()
		if got == nil || len(got) != 0 {
			t.Fatalf("AnonReturns() = %v, want empty non-nil slice", got)
		}
	})

	t.Run("wraps each type as a slot in order", func(t *testing.T) {
		t.Parallel()
		str, err := namedRef("", "string"), namedRef("", "error")
		got := node.AnonReturns(str, err)
		if len(got) != 2 || got[0].Type != str || got[1].Type != err {
			t.Fatalf("AnonReturns wrapped %v, want [%v %v]", got, str, err)
		}
	})

	t.Run("leaves every wrapped slot unnamed", func(t *testing.T) {
		t.Parallel()
		for _, r := range node.AnonReturns(namedRef("", "string"), namedRef("", "error")) {
			if r.Name != "" {
				t.Fatalf("AnonReturns produced named slot %q, want unnamed", r.Name)
			}
		}
	})

	t.Run("round-trips through ReturnTypes", func(t *testing.T) {
		t.Parallel()
		str, err := namedRef("", "string"), namedRef("", "error")
		got := node.ReturnTypes(node.AnonReturns(str, err))
		if len(got) != 2 || got[0] != str || got[1] != err {
			t.Fatalf("round-trip = %v, want [%v %v]", got, str, err)
		}
	})
}
