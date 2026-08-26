// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// method builds a method from parameter names and a single return.
func method(name string, params []*node.Param, returns ...*node.Return) *node.Method {
	return &node.Method{Name: name, Params: params, Returns: returns}
}

// param is a declared parameter of the named type.
func param(name, typeName string) *node.Param {
	return &node.Param{Name: name, Type: named(typeName)}
}

func TestSigOf(t *testing.T) {
	t.Parallel()

	t.Run("a declared parameter keeps its name", func(t *testing.T) {
		t.Parallel()
		got := typescript.SigOf(method("greet", []*node.Param{param("loud", "boolean")}))
		if got.Params[0].Name != "loud" || got.Params[0].Declared != "loud" {
			t.Fatalf("param = %+v", got.Params[0])
		}
		if got.Params[0].Field != "Loud" {
			t.Fatalf("Field = %q, want Loud", got.Params[0].Field)
		}
	})

	t.Run("an unnamed parameter takes the positional fallback", func(t *testing.T) {
		t.Parallel()
		// A generated body that references a parameter has to call it
		// something.
		got := typescript.SigOf(method("f", []*node.Param{{Type: named("string")}}))
		if got.Params[0].Name != "arg0" {
			t.Errorf("Name = %q, want arg0", got.Params[0].Name)
		}
		if got.Params[0].Declared != "" {
			t.Errorf("Declared = %q, want empty", got.Params[0].Declared)
		}
	})

	t.Run("a name colliding with the fallback is made unique", func(t *testing.T) {
		t.Parallel()
		// `(arg0: string, x: string)` names the first parameter exactly
		// what the second would fall back to, and two parameters of one
		// name do not compile.
		got := typescript.SigOf(method("f", []*node.Param{
			param("arg0", "string"),
			{Type: named("string")},
		}))
		if got.Params[0].Name == got.Params[1].Name {
			t.Fatalf("both parameters bound %q", got.Params[0].Name)
		}
	})

	t.Run("a reserved name is made bindable", func(t *testing.T) {
		t.Parallel()
		got := typescript.SigOf(method("f", []*node.Param{param("class", "string")}))
		if got.Params[0].Name != "class_" {
			t.Fatalf("Name = %q, want class_", got.Params[0].Name)
		}
		if got.Params[0].Declared != "class" {
			t.Fatalf("Declared = %q, want the authored name", got.Params[0].Declared)
		}
	})

	t.Run("a rest parameter is marked", func(t *testing.T) {
		t.Parallel()
		// A double that dropped the marker takes one value where the
		// interface takes many and no longer satisfies it.
		p := param("items", "string")
		p.Variadic = true
		got := typescript.SigOf(method("f", []*node.Param{p}))
		if !got.Params[0].Variadic {
			t.Fatal("the rest marker was dropped")
		}
		if !got.Variadic() {
			t.Fatal("the signature does not report itself variadic")
		}
	})

	t.Run("a lone return is Result and never an error", func(t *testing.T) {
		t.Parallel()
		// TypeScript reports failure by throwing, so a returned value
		// is a value — a generator reading one as an error would emit a
		// check against a return the callable never makes.
		got := typescript.SigOf(method("f", nil, &node.Return{Type: named("string")}))
		if got.Returns[0].Field != "Result" {
			t.Errorf("Field = %q, want Result", got.Returns[0].Field)
		}
		if got.Returns[0].Error {
			t.Error("a value return was read as an error")
		}
		if got.ReturnsError() {
			t.Error("the signature reports an error return")
		}
	})

	t.Run("several returns are numbered", func(t *testing.T) {
		t.Parallel()
		got := typescript.SigOf(method("f", nil,
			&node.Return{Type: named("string")},
			&node.Return{Type: named("number")},
		))
		if got.Returns[0].Field != "Result0" || got.Returns[1].Field != "Result1" {
			t.Fatalf("fields = %q, %q", got.Returns[0].Field, got.Returns[1].Field)
		}
	})

	t.Run("a capture local avoids a parameter's identifier", func(t *testing.T) {
		t.Parallel()
		// Shadowing a parameter would capture the wrong value.
		got := typescript.SigOf(method("f",
			[]*node.Param{param("r0", "string")},
			&node.Return{Type: named("string")},
		))
		if got.Returns[0].Local != "_r0" {
			t.Fatalf("Local = %q, want _r0", got.Returns[0].Local)
		}
	})

	t.Run("the receiver is this and returns are never named", func(t *testing.T) {
		t.Parallel()
		// TypeScript binds the receiver as a keyword, so unlike Go's it
		// cannot collide with anything the signature declares — and a
		// signature names its parameters but not its results.
		got := typescript.SigOf(method("f",
			[]*node.Param{param("this", "string")},
			&node.Return{Name: "out", Type: named("string")},
		))
		if got.ReceiverIdent != "this" {
			t.Errorf("ReceiverIdent = %q, want this", got.ReceiverIdent)
		}
		if got.NamedReturns {
			t.Error("the projection carried a return name")
		}
	})

	t.Run("a generic method carries its parameters", func(t *testing.T) {
		t.Parallel()
		m := method("f", nil)
		m.TypeParams = []*node.TypeParam{{Name: "T"}}
		if got := typescript.SigOf(m); !got.IsGeneric() {
			t.Fatal("the type parameters were dropped")
		}
	})

	t.Run("nil projects to nothing", func(t *testing.T) {
		t.Parallel()
		// A caller iterating a resolved method set that holds one skips
		// rather than panics.
		if got := typescript.SigOf(nil); got != nil {
			t.Errorf("SigOf(nil) = %+v", got)
		}
		if got := typescript.SigOfFunc(nil); got != nil {
			t.Errorf("SigOfFunc(nil) = %+v", got)
		}
	})

	t.Run("a nil parameter and a nil return still take a slot", func(t *testing.T) {
		t.Parallel()
		// Dropping one would renumber everything after it, so a
		// generated forwarding call would pass the wrong argument.
		got := typescript.SigOf(&node.Method{
			Name:    "f",
			Params:  []*node.Param{nil, param("x", "string")},
			Returns: []*node.Return{nil},
		})
		if len(got.Params) != 2 || got.Params[0].Name != "arg0" {
			t.Errorf("params = %+v", got.Params)
		}
		if len(got.Returns) != 1 {
			t.Errorf("returns = %+v", got.Returns)
		}
	})
}

func TestSigOfFunc(t *testing.T) {
	t.Parallel()

	t.Run("a function has no receiver", func(t *testing.T) {
		t.Parallel()
		got := typescript.SigOfFunc(&node.Function{
			Name:   "greet",
			Params: []*node.Param{param("loud", "boolean")},
		})
		if got.ReceiverIdent != "" {
			t.Fatalf("ReceiverIdent = %q, want empty", got.ReceiverIdent)
		}
		if got.Name != "greet" || got.Params[0].Name != "loud" {
			t.Fatalf("projection = %+v", got)
		}
	})
}

func TestIsConstraint(t *testing.T) {
	t.Parallel()

	t.Run("no TypeScript interface declares a type set", func(t *testing.T) {
		t.Parallel()
		// A generic parameter is bounded in the declaration that uses
		// it — `<T extends Shape>` — so the shape Go's constraint
		// interface has does not exist to be recognised.
		if typescript.IsConstraint(&node.Interface{Name: "I"}) {
			t.Error("an interface was read as a constraint")
		}
		if typescript.IsConstraint(nil) {
			t.Error("nil was read as a constraint")
		}
	})
}
