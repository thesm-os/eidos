// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
)

func makeMethod() *node.Method {
	return &node.Method{
		Name:     "Get",
		Receiver: namedRef("repo", "User"),
		Params: []*node.Param{
			{Name: "ctx", Type: namedRef("context", "Context")},
			{Name: "id", Type: namedRef("", "string")},
		},
		Returns: node.AnonReturns(
			namedRef("repo", "User"),
			namedRef("", "error"),
		),
	}
}

func TestMethod_ParamByName(t *testing.T) {
	t.Parallel()

	t.Run("returns the matching param", func(t *testing.T) {
		t.Parallel()
		m := makeMethod()
		got := m.ParamByName("id")
		if got == nil || got.Name != "id" {
			t.Fatalf("ParamByName mismatch: %+v", got)
		}
	})

	t.Run("returns nil for an empty name", func(t *testing.T) {
		t.Parallel()
		m := makeMethod()
		m.Params = append(m.Params, &node.Param{}) // anonymous param
		if got := m.ParamByName(""); got != nil {
			t.Fatalf("ParamByName(\"\") should return nil; got %+v", got)
		}
	})

	t.Run("returns nil for an unknown name", func(t *testing.T) {
		t.Parallel()
		if got := makeMethod().ParamByName("missing"); got != nil {
			t.Fatalf("ParamByName(unknown) = %+v, want nil", got)
		}
	})
}

func TestMethod_ParamAt(t *testing.T) {
	t.Parallel()

	t.Run("returns the param at the given index", func(t *testing.T) {
		t.Parallel()
		got := makeMethod().ParamAt(1)
		if got == nil || got.Name != "id" {
			t.Fatalf("ParamAt(1) = %+v", got)
		}
	})

	t.Run("returns nil for an index out of range", func(t *testing.T) {
		t.Parallel()
		m := makeMethod()
		if m.ParamAt(99) != nil {
			t.Fatalf("ParamAt(99) should be nil")
		}
		if m.ParamAt(-1) != nil {
			t.Fatalf("ParamAt(-1) should be nil")
		}
	})
}

func TestMethod_ReturnAt(t *testing.T) {
	t.Parallel()

	t.Run("returns the return slot at the given index", func(t *testing.T) {
		t.Parallel()
		got := makeMethod().ReturnAt(1)
		if got == nil || got.Type.Name != "error" {
			t.Fatalf("ReturnAt(1) = %+v", got)
		}
	})

	t.Run("returns nil for an index out of range", func(t *testing.T) {
		t.Parallel()
		m := makeMethod()
		if m.ReturnAt(99) != nil {
			t.Fatalf("ReturnAt(99) should be nil")
		}
		if m.ReturnAt(-1) != nil {
			t.Fatalf("ReturnAt(-1) should be nil")
		}
	})
}

func TestMethod_ParamsWith(t *testing.T) {
	t.Parallel()

	t.Run("filters parameters by predicate", func(t *testing.T) {
		t.Parallel()
		got := makeMethod().ParamsWith(func(p *node.Param) bool { return p.Name == "ctx" })
		if len(got) != 1 || got[0].Name != "ctx" {
			t.Fatalf("ParamsWith filter mismatch: %+v", got)
		}
	})
}

func TestMethod_ReturnsWith(t *testing.T) {
	t.Parallel()

	t.Run("filters returns by predicate", func(t *testing.T) {
		t.Parallel()
		got := makeMethod().ReturnsWith(func(r *node.Return) bool { return r.Type.Name == "error" })
		if len(got) != 1 || got[0].Type.Name != "error" {
			t.Fatalf("ReturnsWith filter mismatch: %+v", got)
		}
	})
}

func TestMethod_HasReceiver(t *testing.T) {
	t.Parallel()

	t.Run("returns true when receiver is set", func(t *testing.T) {
		t.Parallel()
		if !makeMethod().HasReceiver() {
			t.Fatalf("struct method should report HasReceiver true")
		}
	})

	t.Run("returns false when receiver is nil", func(t *testing.T) {
		t.Parallel()
		var m node.Method
		if m.HasReceiver() {
			t.Fatalf("interface method should report HasReceiver false")
		}
	})
}

func TestMethod_IsVariadic(t *testing.T) {
	t.Parallel()

	t.Run("returns false when no params are variadic", func(t *testing.T) {
		t.Parallel()
		if makeMethod().IsVariadic() {
			t.Fatalf("non-variadic method should report IsVariadic false")
		}
	})

	t.Run("returns true when last param is variadic", func(t *testing.T) {
		t.Parallel()
		m := makeMethod()
		m.Params = append(m.Params, &node.Param{Name: "extra", Variadic: true})
		if !m.IsVariadic() {
			t.Fatalf("trailing variadic param should report IsVariadic true")
		}
	})

	t.Run("returns false on empty parameter list", func(t *testing.T) {
		t.Parallel()
		var m node.Method
		if m.IsVariadic() {
			t.Fatalf("zero-param method should not be variadic")
		}
	})
}

func TestMethod_IsGeneric(t *testing.T) {
	t.Parallel()

	t.Run("returns false when no type params declared", func(t *testing.T) {
		t.Parallel()
		if makeMethod().IsGeneric() {
			t.Fatalf("non-generic method should report IsGeneric false")
		}
	})

	t.Run("returns true when type params declared", func(t *testing.T) {
		t.Parallel()
		m := makeMethod()
		m.TypeParams = []*node.TypeParam{{Name: "T"}}
		if !m.IsGeneric() {
			t.Fatalf("generic method should report IsGeneric true")
		}
	})
}

func TestMethod_Counts(t *testing.T) {
	t.Parallel()

	t.Run("ParamCount and ReturnCount report slice lengths", func(t *testing.T) {
		t.Parallel()
		m := makeMethod()
		if m.ParamCount() != 2 {
			t.Fatalf("ParamCount = %d, want 2", m.ParamCount())
		}
		if m.ReturnCount() != 2 {
			t.Fatalf("ReturnCount = %d, want 2", m.ReturnCount())
		}
	})
}
