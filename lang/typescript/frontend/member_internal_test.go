// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

func TestParamConversion(t *testing.T) {
	t.Parallel()

	params := func(t *testing.T, src string) []*node.Param {
		t.Helper()
		decls, _ := convert(t, src)
		fn, ok := decls[0].(*node.Function)
		if !ok {
			t.Fatalf("expected *node.Function, got %T", decls[0])
		}
		return fn.Params
	}

	t.Run("a rest parameter records the element type", func(t *testing.T) {
		t.Parallel()
		// node.Param documents Variadic as carrying the element type:
		// Go's `...int` records `int`, not `[]int`. TypeScript
		// annotates the array, so the annotation is one level out from
		// what the model asks for — and a consumer following the
		// contract would emit `...rest: string[][]`.
		ps := params(t, `function f(...rest: string[]): void {}`)

		if len(ps) != 1 {
			t.Fatalf("params = %d, want 1", len(ps))
		}
		if !ps[0].Variadic {
			t.Fatal("rest parameter not marked variadic")
		}
		if ps[0].Type == nil || ps[0].Type.IsSlice() {
			t.Fatalf("Type = %+v, want the element type not the array", ps[0].Type)
		}
		if ps[0].Type.Name != "string" {
			t.Fatalf("Type.Name = %q, want string", ps[0].Type.Name)
		}
	})

	t.Run("a non-rest parameter keeps its array type", func(t *testing.T) {
		t.Parallel()
		ps := params(t, `function f(all: string[]): void {}`)
		if ps[0].Variadic {
			t.Fatal("plain parameter marked variadic")
		}
		if ps[0].Type == nil || !ps[0].Type.IsSlice() {
			t.Fatalf("Type = %+v, want a slice", ps[0].Type)
		}
	})

	t.Run("a rest parameter of a non-array type is left as written", func(t *testing.T) {
		t.Parallel()
		// There is no element to take, and inventing one would be
		// worse than reporting the declared type.
		ps := params(t, `function f<T extends unknown[]>(...rest: T): void {}`)
		if ps[0].Type == nil || ps[0].Type.Name != "T" {
			t.Fatalf("Type = %+v, want T unchanged", ps[0].Type)
		}
	})

	t.Run("a method records its return type", func(t *testing.T) {
		t.Parallel()
		// A callable's return annotation is under `return_type`; a
		// property's or parameter's type is under `type`. Reading the
		// wrong field dropped every method's return silently — the
		// method converted, its signature was just wrong, and four
		// golden fixtures had been pinning that.
		i := onlyInterface(t, `interface I {
			m(a: number): boolean;
			none(): void;
		}`)

		if got := len(i.Methods[0].Returns); got != 1 {
			t.Fatalf("m returns = %d, want 1", got)
		}
		if name := i.Methods[0].Returns[0].Type.Name; name != "boolean" {
			t.Fatalf("m return type = %q, want boolean", name)
		}
		if got := len(i.Methods[1].Returns); got != 1 {
			t.Fatalf("none returns = %d, want 1 (void is a type)", got)
		}
	})

	t.Run("a constructor has no return type", func(t *testing.T) {
		t.Parallel()
		s := onlyStruct(t, `class C { constructor(a: string) {} }`)
		if got := len(s.Methods[0].Returns); got != 0 {
			t.Fatalf("constructor returns = %d, want 0", got)
		}
	})

	t.Run("every parameter and return carries its owner", func(t *testing.T) {
		t.Parallel()
		// node.Walk does not descend into Params, so nothing else in
		// the suite would notice these going nil.
		decls, _ := convert(t, `function f(a: string, b?: number): boolean { return true; }`)
		fn, _ := decls[0].(*node.Function)

		for i, p := range fn.Params {
			if p.Owner != node.Node(fn) {
				t.Errorf("param %d Owner not wired", i)
			}
		}
		for i, r := range fn.Returns {
			if r.Owner != node.Node(fn) {
				t.Errorf("return %d Owner not wired", i)
			}
		}
	})
}

func TestPropertyName(t *testing.T) {
	t.Parallel()

	t.Run("reads every static member-name spelling", func(t *testing.T) {
		t.Parallel()
		// Four spellings reach the converter; three name a member
		// statically and are kept as written.
		i := onlyInterface(t, `interface A {
			plain: string;
			'quoted-key': string;
			2: string;
		}`)

		want := []string{"plain", "quoted-key", "2"}
		if len(i.Fields) != len(want) {
			t.Fatalf("Fields = %d, want %d", len(i.Fields), len(want))
		}
		for n, w := range want {
			if i.Fields[n].Name != w {
				t.Errorf("field %d = %q, want %q", n, i.Fields[n].Name, w)
			}
		}
	})

	t.Run("a computed key names no member", func(t *testing.T) {
		t.Parallel()
		// `[Symbol.iterator]` is not a name a generator can key on, so
		// it is reported as unnamed rather than given the expression
		// as its name.
		s := onlyStruct(t, `class C { [Symbol.iterator]() {} }`)
		for _, m := range s.Methods {
			if m.Name == "" {
				t.Fatal("an unnamed method reached the graph")
			}
		}
	})
}

func TestBindingName(t *testing.T) {
	t.Parallel()

	t.Run("a rest parameter binds its inner identifier", func(t *testing.T) {
		t.Parallel()
		decls, _ := convert(t, `function f(...rest: string[]): void {}`)
		fn, _ := decls[0].(*node.Function)
		if got := fn.Params[0].Name; got != "rest" {
			t.Fatalf("Name = %q, want rest", got)
		}
	})

	t.Run("a destructuring parameter is kept with no name", func(t *testing.T) {
		t.Parallel()
		// Dropping it would shift every later parameter's index and
		// silently change the signature a generator sees.
		decls, _ := convert(t, `function f({ a, b }: Opts, second: string): void {}`)
		fn, _ := decls[0].(*node.Function)

		if got := len(fn.Params); got != 2 {
			t.Fatalf("params = %d, want 2 — the destructured one must hold its slot", got)
		}
		if fn.Params[0].Name != "" {
			t.Fatalf("destructured param name = %q, want empty", fn.Params[0].Name)
		}
		if fn.Params[1].Name != "second" {
			t.Fatalf("second param = %q, want second", fn.Params[1].Name)
		}
	})
}

func TestCallableModifiers(t *testing.T) {
	t.Parallel()

	t.Run("async, generator and accessor kind are recorded", func(t *testing.T) {
		t.Parallel()
		s := onlyStruct(t, `class C {
			async a(): Promise<void> {}
			*g(): Generator<number> {}
			get w(): number { return 1; }
		}`)

		byName := map[string]*node.Method{}
		for _, m := range s.Methods {
			byName[m.Name] = m
		}

		if got, _ := typescript.MetaAsync.Get(byName["a"].Meta()); !got {
			t.Error("async not stamped")
		}
		if got, _ := typescript.MetaGenerator.Get(byName["g"].Meta()); !got {
			t.Error("generator not stamped")
		}
		if got, _ := typescript.MetaAccessor.Get(byName["w"].Meta()); got != typescript.AccessorGet {
			t.Errorf("accessor = %q, want get", got)
		}
	})

	t.Run("a plain method carries none of them", func(t *testing.T) {
		t.Parallel()
		s := onlyStruct(t, `class C { m(): void {} }`)
		m := s.Methods[0]
		hasModifier := typescript.MetaAsync.Has(m.Meta()) ||
			typescript.MetaGenerator.Has(m.Meta()) ||
			typescript.MetaAccessor.Has(m.Meta())
		if hasModifier {
			t.Fatal("a plain method gained a modifier it does not have")
		}
	})

	t.Run("a static member is marked", func(t *testing.T) {
		t.Parallel()
		s := onlyStruct(t, `class C { static x = 1; y = 2; }`)
		if st, _ := typescript.MetaStatic.Get(s.Fields[0].Meta()); !st {
			t.Error("static not stamped")
		}
		if typescript.MetaStatic.Has(s.Fields[1].Meta()) {
			t.Error("an instance field was marked static")
		}
	})
}
