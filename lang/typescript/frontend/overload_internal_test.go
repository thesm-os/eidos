// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

func TestFoldOverloads(t *testing.T) {
	t.Parallel()

	functions := func(t *testing.T, src string) []*node.Function {
		t.Helper()
		decls, _ := convert(t, src)
		var out []*node.Function
		for _, d := range decls {
			if fn, ok := d.(*node.Function); ok {
				out = append(out, fn)
			}
		}
		return out
	}

	t.Run("an overload set collapses to one declaration", func(t *testing.T) {
		t.Parallel()
		// The model keys a declaration by qualified name, so three
		// Functions called `f` are a duplicate the store rejects —
		// which is how this was found, by the golden corpus failing to
		// build a package at all.
		fns := functions(t, `
			export function f(a: string): void;
			export function f(a: number): void;
			export function f(a: any): void {}
		`)
		if len(fns) != 1 {
			t.Fatalf("functions = %d, want 1", len(fns))
		}
	})

	t.Run("the implementation survives as the declaration", func(t *testing.T) {
		t.Parallel()
		// Its signature is the one a body was written against.
		fns := functions(t, `
			function f(a: string): void;
			function f(a: number): void;
			function f(a: any): void {}
		`)
		if got := fns[0].Params[0].Type.Name; got != "any" {
			t.Fatalf("surviving param type = %q, want any (the implementation)", got)
		}
	})

	t.Run("every alternative is carried, in source order", func(t *testing.T) {
		t.Parallel()
		fns := functions(t, `
			function f(a: string): void;
			function f(a: number): void;
			function f(a: any): void {}
		`)
		got, _ := typescript.MetaOverloads.Get(fns[0].Meta())
		if len(got) != 2 {
			t.Fatalf("overloads = %d, want 2", len(got))
		}
		if got[0].Text != "function f(a: string): void" {
			t.Errorf("overloads[0] = %q", got[0].Text)
		}
		if got[1].Text != "function f(a: number): void" {
			t.Errorf("overloads[1] = %q", got[1].Text)
		}
	})

	t.Run("an ambient set keeps the first signature", func(t *testing.T) {
		t.Parallel()
		// No implementation exists, so there is no body to prefer.
		fns := functions(t, `
			declare function f(a: string): void;
			declare function f(a: number): void;
		`)
		if len(fns) != 1 {
			t.Fatalf("functions = %d, want 1", len(fns))
		}
		if got := fns[0].Params[0].Type.Name; got != "string" {
			t.Fatalf("surviving param type = %q, want string (the first)", got)
		}
		if got, _ := typescript.MetaOverloads.Get(fns[0].Meta()); len(got) != 1 {
			t.Fatalf("overloads = %d, want 1", len(got))
		}
	})

	t.Run("the declaration's own signature is not one of its alternatives", func(t *testing.T) {
		t.Parallel()
		// The implementation's spelling is not a way it may be called.
		fns := functions(t, `
			function f(a: string): void;
			function f(a: any): void {}
		`)
		for _, o := range mustOverloads(t, fns[0]) {
			if o.Text == "function f(a: any): void" {
				t.Fatal("the implementation's own signature was listed as an alternative")
			}
		}
	})

	t.Run("distinct functions are left alone", func(t *testing.T) {
		t.Parallel()
		fns := functions(t, `
			function a(): void {}
			function b(): void {}
		`)
		if len(fns) != 2 {
			t.Fatalf("functions = %d, want 2", len(fns))
		}
		for _, fn := range fns {
			if typescript.MetaOverloads.Has(fn.Meta()) {
				t.Errorf("%s gained overloads it does not have", fn.Name)
			}
		}
	})

	t.Run("method overloads collapse too", func(t *testing.T) {
		t.Parallel()
		// The store does not key methods by name, so this does not
		// fail a run — it produces a method list with repeats, and a
		// generator emitting one wrapper per entry emits several for
		// one method.
		s := onlyStruct(t, `class C {
			m(a: string): void;
			m(a: number): void;
			m(a: any): void {}
		}`)

		if len(s.Methods) != 1 {
			t.Fatalf("methods = %d, want 1", len(s.Methods))
		}
		if got, _ := typescript.MetaOverloads.Get(s.Methods[0].Meta()); len(got) != 2 {
			t.Fatalf("method overloads = %d, want 2", len(got))
		}
	})
}

// mustOverloads returns fn's alternatives, failing when it has none.
func mustOverloads(t *testing.T, fn *node.Function) []typescript.Overload {
	t.Helper()
	got, ok := typescript.MetaOverloads.Get(fn.Meta())
	if !ok {
		t.Fatal("no overloads recorded")
	}
	return got
}

func TestOverloadEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("an alternative identical to the declaration is not listed", func(t *testing.T) {
		t.Parallel()
		// The implementation's own spelling is not one of the ways it
		// may be called, so a set whose only alternative repeats it
		// records nothing rather than an empty list.
		decls, _ := convert(t, `
			function f(a: string): void;
			function f(a: string): void {}
		`)
		fn, ok := decls[0].(*node.Function)
		if !ok {
			t.Fatalf("got %T", decls[0])
		}
		if typescript.MetaOverloads.Has(fn.Meta()) {
			got, _ := typescript.MetaOverloads.Get(fn.Meta())
			t.Fatalf("overloads = %+v, want none", got)
		}
	})

	t.Run("a method whose only alternative repeats it records nothing", func(t *testing.T) {
		t.Parallel()
		s := onlyStruct(t, `class C {
			m(a: string): void;
			m(a: string): void {}
		}`)
		if typescript.MetaOverloads.Has(s.Methods[0].Meta()) {
			t.Fatal("a redundant method alternative was recorded")
		}
	})
}
