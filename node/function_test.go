// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
)

func makeFunction() *node.Function {
	return &node.Function{
		Name:    "Open",
		Package: "github.com/example/db",
		Params: []*node.Param{
			{Name: "ctx", Type: namedRef("context", "Context")},
			{Name: "dsn", Type: namedRef("", "string")},
		},
		Returns: node.AnonReturns(
			&node.TypeRef{TypeKind: node.TypeRefPointer, Elem: namedRef("db", "Conn")},
			namedRef("", "error"),
		),
	}
}

func TestFunction_QName(t *testing.T) {
	t.Parallel()

	t.Run("includes package when present", func(t *testing.T) {
		t.Parallel()
		assertEqualString(t, makeFunction().QName(), "github.com/example/db.Open")
	})

	t.Run("returns just the name when package is empty", func(t *testing.T) {
		t.Parallel()
		f := &node.Function{Name: "Foo"}
		assertEqualString(t, f.QName(), "Foo")
	})
}

func TestFunction_ParamByName(t *testing.T) {
	t.Parallel()

	t.Run("returns the matching param", func(t *testing.T) {
		t.Parallel()
		got := makeFunction().ParamByName("ctx")
		if got == nil || got.Name != "ctx" {
			t.Fatalf("ParamByName mismatch: %+v", got)
		}
	})

	t.Run("returns nil for an empty name", func(t *testing.T) {
		t.Parallel()
		f := makeFunction()
		f.Params = append(f.Params, &node.Param{}) // anonymous param
		if got := f.ParamByName(""); got != nil {
			t.Fatalf("ParamByName(\"\") should return nil; got %+v", got)
		}
	})

	t.Run("returns nil for an unknown name", func(t *testing.T) {
		t.Parallel()
		if got := makeFunction().ParamByName("missing"); got != nil {
			t.Fatalf("ParamByName(unknown) = %+v, want nil", got)
		}
	})
}

func TestFunction_ParamAt(t *testing.T) {
	t.Parallel()

	t.Run("returns the param at the given index", func(t *testing.T) {
		t.Parallel()
		got := makeFunction().ParamAt(0)
		if got == nil || got.Name != "ctx" {
			t.Fatalf("ParamAt(0) = %+v", got)
		}
	})

	t.Run("returns nil for an index out of range", func(t *testing.T) {
		t.Parallel()
		f := makeFunction()
		if f.ParamAt(99) != nil {
			t.Fatalf("ParamAt(99) should be nil")
		}
		if f.ParamAt(-1) != nil {
			t.Fatalf("ParamAt(-1) should be nil")
		}
	})
}

func TestFunction_ReturnAt(t *testing.T) {
	t.Parallel()

	t.Run("returns the return slot at the given index", func(t *testing.T) {
		t.Parallel()
		got := makeFunction().ReturnAt(1)
		if got == nil || got.Type.Name != "error" {
			t.Fatalf("ReturnAt(1) = %+v", got)
		}
	})

	t.Run("returns nil for an index out of range", func(t *testing.T) {
		t.Parallel()
		f := makeFunction()
		if f.ReturnAt(99) != nil {
			t.Fatalf("ReturnAt(99) should be nil")
		}
		if f.ReturnAt(-1) != nil {
			t.Fatalf("ReturnAt(-1) should be nil")
		}
	})
}

func TestFunction_ParamsWith(t *testing.T) {
	t.Parallel()

	t.Run("filters parameters by predicate", func(t *testing.T) {
		t.Parallel()
		got := makeFunction().ParamsWith(func(p *node.Param) bool { return p.Name == "dsn" })
		if len(got) != 1 || got[0].Name != "dsn" {
			t.Fatalf("ParamsWith filter mismatch: %+v", got)
		}
	})
}

func TestFunction_ReturnsWith(t *testing.T) {
	t.Parallel()

	t.Run("filters returns by predicate", func(t *testing.T) {
		t.Parallel()
		got := makeFunction().ReturnsWith(func(r *node.Return) bool { return r.Type.IsPointer() })
		if len(got) != 1 || !got[0].Type.IsPointer() {
			t.Fatalf("ReturnsWith filter mismatch: %+v", got)
		}
	})
}

func TestFunction_IsVariadic(t *testing.T) {
	t.Parallel()

	t.Run("returns false on a non-variadic function", func(t *testing.T) {
		t.Parallel()
		if makeFunction().IsVariadic() {
			t.Fatalf("non-variadic function should report IsVariadic false")
		}
	})

	t.Run("returns true when last param is variadic", func(t *testing.T) {
		t.Parallel()
		f := makeFunction()
		f.Params = append(f.Params, &node.Param{Name: "extra", Variadic: true})
		if !f.IsVariadic() {
			t.Fatalf("trailing variadic param should report IsVariadic true")
		}
	})

	t.Run("returns false on empty parameter list", func(t *testing.T) {
		t.Parallel()
		var f node.Function
		if f.IsVariadic() {
			t.Fatalf("zero-param function should not be variadic")
		}
	})
}

func TestFunction_IsGeneric(t *testing.T) {
	t.Parallel()

	t.Run("returns true when type params declared", func(t *testing.T) {
		t.Parallel()
		f := makeFunction()
		f.TypeParams = []*node.TypeParam{{Name: "T"}}
		if !f.IsGeneric() {
			t.Fatalf("generic function should report IsGeneric true")
		}
	})

	t.Run("returns false when no type params declared", func(t *testing.T) {
		t.Parallel()
		if makeFunction().IsGeneric() {
			t.Fatalf("non-generic function should report IsGeneric false")
		}
	})
}

func TestFunction_Counts(t *testing.T) {
	t.Parallel()

	t.Run("ParamCount and ReturnCount report slice lengths", func(t *testing.T) {
		t.Parallel()
		f := makeFunction()
		if f.ParamCount() != 2 {
			t.Fatalf("ParamCount = %d, want 2", f.ParamCount())
		}
		if f.ReturnCount() != 2 {
			t.Fatalf("ReturnCount = %d, want 2", f.ReturnCount())
		}
	})
}
