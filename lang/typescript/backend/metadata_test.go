// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend_test

// The ts.* vocabulary, rendered. One test per key the backend spells,
// each driven through the real templates rather than a render helper
// in isolation — the keyword order and the ambient interactions are
// template facts, and a unit test over a helper cannot see them.
//
// A standalone file rather than lines in render_members_test because
// the subject is cross-cutting: the keys land on fields, methods,
// classes, enums, functions and bindings alike, and the file is the
// map of which key renders where.

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/backendtest"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/lang/typescript/backend"
)

func TestMemberModifierMetadata(t *testing.T) {
	t.Parallel()

	t.Run("visibility, static and readonly compose in grammar order", func(t *testing.T) {
		t.Parallel()
		f := &emit.Field{Name: "cache", Type: emit.Builtin("string")}
		typescript.MetaVisibility.Set(f.EnsureMeta(), typescript.VisibilityPrivate, "test")
		typescript.MetaStatic.Set(f.EnsureMeta(), true, "test")
		typescript.MetaReadonly.Set(f.EnsureMeta(), true, "test")

		got := render(t, pkgWith(&emit.Struct{
			Name: "Repo", Target: target, Fields: []*emit.Field{f},
		}))
		if !strings.Contains(got, "private static readonly cache: string;") {
			t.Fatalf("member = %q", line(t, got, "cache"))
		}
	})

	t.Run("a stamped public renders, an unstamped one does not", func(t *testing.T) {
		t.Parallel()
		// The key's contract: absent and public are distinguishable
		// precisely so a backend does not invent a keyword the author
		// omitted — which means one that was stamped was written.
		stamped := &emit.Field{Name: "a", Type: emit.Builtin("string")}
		typescript.MetaVisibility.Set(stamped.EnsureMeta(), typescript.VisibilityPublic, "test")
		bare := &emit.Field{Name: "b", Type: emit.Builtin("string")}

		got := render(t, pkgWith(&emit.Struct{
			Name: "Repo", Target: target, Fields: []*emit.Field{stamped, bare},
		}))
		if !strings.Contains(got, "public a: string;") {
			t.Errorf("stamped public dropped: %q", line(t, got, "a:"))
		}
		if strings.Contains(line(t, got, "b:"), "public") {
			t.Errorf("unstamped member gained a keyword: %q", line(t, got, "b:"))
		}
	})

	t.Run("a hard-private member carries the hash on its name", func(t *testing.T) {
		t.Parallel()
		// `#` is part of the name rather than a modifier, and it must
		// bypass the property-key quoting: `'#x'` declares a public
		// property whose name contains a hash.
		f := &emit.Field{Name: "secret", Type: emit.Builtin("string")}
		typescript.MetaVisibility.Set(f.EnsureMeta(), typescript.VisibilityHard, "test")

		got := render(t, pkgWith(&emit.Struct{
			Name: "Repo", Target: target, Fields: []*emit.Field{f},
		}))
		if !strings.Contains(got, "#secret: string;") {
			t.Fatalf("member = %q", line(t, got, "secret"))
		}
		if strings.Contains(got, "'#secret'") {
			t.Fatal("the hash was quoted into a public property name")
		}
	})

	t.Run("an abstract method on an abstract class", func(t *testing.T) {
		t.Parallel()
		m := &emit.Method{Name: "find", Returns: []*emit.Return{{Type: emit.Builtin("string")}}}
		typescript.MetaAbstract.Set(m.EnsureMeta(), true, "test")
		s := &emit.Struct{Name: "Base", Target: target, Methods: []*emit.Method{m}}
		typescript.MetaAbstract.Set(s.EnsureMeta(), true, "test")

		got := render(t, pkgWith(s))
		if !strings.Contains(got, "export declare abstract class Base {") {
			t.Errorf("class head = %q", line(t, got, "class"))
		}
		if !strings.Contains(got, "abstract find(): string;") {
			t.Errorf("method = %q", line(t, got, "find"))
		}
	})

	t.Run("an accessor keeps its keyword", func(t *testing.T) {
		t.Parallel()
		// A getter's use site is a property read; rendered as an
		// ordinary method the declaration would promise `r.size()`
		// where the implementation offers `r.size`.
		m := &emit.Method{Name: "size", Returns: []*emit.Return{{Type: emit.Builtin("int")}}}
		typescript.MetaAccessor.Set(m.EnsureMeta(), typescript.AccessorGet, "test")

		got := render(t, pkgWith(&emit.Struct{
			Name: "Repo", Target: target, Methods: []*emit.Method{m},
		}))
		if !strings.Contains(got, "get size(): number;") {
			t.Fatalf("accessor = %q", line(t, got, "size"))
		}
	})

	t.Run("an optional method carries the marker", func(t *testing.T) {
		t.Parallel()
		m := &emit.Method{Name: "close"}
		typescript.MetaOptional.Set(m.EnsureMeta(), true, "test")

		got := render(t, pkgWith(&emit.Interface{
			Name: "Store", Target: target, Methods: []*emit.Method{m},
		}))
		if !strings.Contains(got, "close?(): void;") {
			t.Fatalf("method = %q", line(t, got, "close"))
		}
	})
}

func TestOverloadMetadata(t *testing.T) {
	t.Parallel()

	t.Run("a method renders its overloads instead of the derived signature", func(t *testing.T) {
		t.Parallel()
		// The overloads are what a caller may use; the implementation
		// signature exists to cover them, which is a body fact. Order
		// is source order, because TypeScript resolves a call top-down.
		m := &emit.Method{Name: "find", Params: []*emit.Param{{Name: "q", Type: emit.Builtin("any")}}}
		typescript.MetaOverloads.Set(m.EnsureMeta(), []typescript.Overload{
			{Text: "find(id: string): User"},
			{Text: "find(ids: string[]): User[]"},
		}, "test")

		got := render(t, pkgWith(&emit.Interface{
			Name: "Store", Target: target, Methods: []*emit.Method{m},
		}))
		hasBoth := strings.Contains(got, "find(id: string): User;") &&
			strings.Contains(got, "find(ids: string[]): User[];")
		if !hasBoth {
			t.Fatalf("overloads missing:\n%s", got)
		}
		if strings.Contains(got, "find(q: any)") {
			t.Fatalf("the implementation signature leaked:\n%s", got)
		}
		first := strings.Index(got, "find(id: string)")
		second := strings.Index(got, "find(ids: string[])")
		if first > second {
			t.Fatal("overload order does not follow source order")
		}
	})

	t.Run("a function renders one declaration per overload", func(t *testing.T) {
		t.Parallel()
		// Function overloads are sibling declarations, each with its
		// own export and declare keywords.
		f := &emit.Function{Name: "parse", Target: target}
		typescript.MetaOverloads.Set(f.EnsureMeta(), []typescript.Overload{
			{Text: "parse(raw: string): Config"},
			{Text: "parse(raw: Buffer): Config"},
		}, "test")

		got := render(t, pkgWith(f))
		hasBoth := strings.Contains(got, "export declare function parse(raw: string): Config;") &&
			strings.Contains(got, "export declare function parse(raw: Buffer): Config;")
		if !hasBoth {
			t.Fatalf("overload declarations missing:\n%s", got)
		}
	})
}

func TestDeclarationMetadata(t *testing.T) {
	t.Parallel()

	t.Run("a const enum keeps its keyword", func(t *testing.T) {
		t.Parallel()
		// Inlined at every use site, no runtime object — a different
		// contract from the same members in an ordinary enum.
		e := &emit.Enum{
			Name: "Level", Target: target,
			Variants: []*emit.EnumVariant{{Name: "Low"}},
		}
		typescript.MetaConstEnum.Set(e.EnsureMeta(), true, "test")

		got := render(t, pkgWith(e))
		if !strings.Contains(got, "export const enum Level {") {
			t.Fatalf("enum head = %q", line(t, got, "enum"))
		}
	})

	t.Run("index and construct signatures open the interface body", func(t *testing.T) {
		t.Parallel()
		i := &emit.Interface{
			Name: "Bag", Target: target,
			Fields: []*emit.Field{{Name: "size", Type: emit.Builtin("int")}},
		}
		typescript.MetaIndexSignature.Set(i.EnsureMeta(), "[key: string]: unknown", "test")
		typescript.MetaConstructSignature.Set(i.EnsureMeta(), "new (size: number): Bag", "test")

		got := render(t, pkgWith(i))
		hasBoth := strings.Contains(got, "[key: string]: unknown;") &&
			strings.Contains(got, "new (size: number): Bag;")
		if !hasBoth {
			t.Fatalf("signatures missing:\n%s", got)
		}
	})

	t.Run("a type parameter carries its default", func(t *testing.T) {
		t.Parallel()
		p := &emit.TypeParam{Name: "T"}
		typescript.MetaTypeParamDefault.Set(p.EnsureMeta(), "string", "test")

		got := render(t, pkgWith(&emit.Interface{
			Name: "Box", Target: target, TypeParams: []*emit.TypeParam{p},
			Fields: []*emit.Field{{Name: "value", Type: emit.Builtin("string")}},
		}))
		if !strings.Contains(got, "interface Box<T = string> {") {
			t.Fatalf("head = %q", line(t, got, "Box"))
		}
	})
}

func TestInitialiserMetadata(t *testing.T) {
	t.Parallel()

	t.Run("a constant with a value drops declare and carries it", func(t *testing.T) {
		t.Parallel()
		// An ambient declaration admits no initialiser, and a value
		// this backend can spell is not the runtime code the ambient
		// spelling exists to avoid.
		got := render(t, pkgWith(&emit.Constant{
			Name: "MAX", Target: target,
			Type:  emit.Builtin("int"),
			Value: emit.NewLiteralInt(100),
		}))
		if !strings.Contains(got, "export const MAX: number = 100;") {
			t.Fatalf("constant = %q", line(t, got, "MAX"))
		}
	})

	t.Run("a variable with an init does the same", func(t *testing.T) {
		t.Parallel()
		got := render(t, pkgWith(&emit.Variable{
			Name: "cache", Target: target,
			Type: emit.Builtin("string"),
			Init: emit.NewLiteralString("warm"),
		}))
		if !strings.Contains(got, "export let cache: string = 'warm';") {
			t.Fatalf("variable = %q", line(t, got, "cache"))
		}
	})

	t.Run("the verbatim stamp serves where no expression was built", func(t *testing.T) {
		t.Parallel()
		// What a bridge copies from source; the structured expression
		// wins where a generator built one.
		c := &emit.Constant{Name: "LIMITS", Target: target}
		typescript.MetaInitialiser.Set(c.EnsureMeta(), "{ max: 10 }", "test")

		got := render(t, pkgWith(c))
		if !strings.Contains(got, "export const LIMITS = { max: 10 };") {
			t.Fatalf("constant = %q", line(t, got, "LIMITS"))
		}
	})

	t.Run("no value keeps the ambient spelling", func(t *testing.T) {
		t.Parallel()
		got := render(t, pkgWith(&emit.Constant{
			Name: "MAX", Target: target, Type: emit.Builtin("int"),
		}))
		if !strings.Contains(got, "export declare const MAX: number;") {
			t.Fatalf("constant = %q", line(t, got, "MAX"))
		}
	})

	t.Run("an inferred type renders no empty annotation", func(t *testing.T) {
		t.Parallel()
		// `export const MAX = 100;` is legal and common; `MAX: = 100`
		// is the bug an unconditional colon writes.
		got := render(t, pkgWith(&emit.Constant{
			Name: "MAX", Target: target, Value: emit.NewLiteralInt(100),
		}))
		if !strings.Contains(got, "export const MAX = 100;") {
			t.Fatalf("constant = %q", line(t, got, "MAX"))
		}
	})

	t.Run("an unrenderable initialiser fails the file rather than lying", func(t *testing.T) {
		t.Parallel()
		// A call is runtime code; dropping it would render a declare
		// const whose declared value the consumer never sees.
		bad := &emit.Constant{
			Name: "K", Target: target,
			Type:  emit.Builtin("string"),
			Value: &emit.Expr{ExprKind: emit.ExprCall},
		}
		assertRenderFails(t, pkgWith(bad))
	})
}

// assertRenderFails drives the backend and requires an error
// diagnostic instead of a written file.
func assertRenderFails(t *testing.T, pkg *emit.Package) {
	t.Helper()
	res := backendtest.Run(t, backendtest.RunOptions{
		Backend:      backend.New(),
		EmitPackages: []*emit.Package{pkg},
	})
	for _, d := range res.Diag.Diagnostics() {
		if d.Severity == diag.Error {
			return
		}
	}
	t.Fatal("the render succeeded; an unrenderable construct was dropped in silence")
}
