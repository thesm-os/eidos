// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// convert parses src as one file and returns the declarations it
// yields, failing the test on a parse error.
func convert(t *testing.T, src string) ([]node.Node, *diag.Sink) {
	t.Helper()
	p, err := parseFile("fixture.ts", []byte(src))
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	t.Cleanup(p.close)

	if p.root().HasError() {
		t.Fatalf("fixture does not parse:\n%s", src)
	}

	sink := diag.New()
	c := newConv(p, "./fixture", sink.For(FrontendName), directive.DefaultParser())
	return c.declarations(p.root()), sink
}

// onlyDecl returns the single declaration the source yields.
func onlyDecl(t *testing.T, src string) node.Node {
	t.Helper()
	decls, _ := convert(t, src)
	if len(decls) != 1 {
		t.Fatalf("expected exactly 1 declaration, got %d", len(decls))
	}
	return decls[0]
}

// onlyStruct returns the single class the source declares.
func onlyStruct(t *testing.T, src string) *node.Struct {
	t.Helper()
	s, ok := onlyDecl(t, src).(*node.Struct)
	if !ok {
		t.Fatalf("expected *node.Struct, got %T", onlyDecl(t, src))
	}
	return s
}

// onlyInterface returns the single interface the source declares.
func onlyInterface(t *testing.T, src string) *node.Interface {
	t.Helper()
	i, ok := onlyDecl(t, src).(*node.Interface)
	if !ok {
		t.Fatalf("expected *node.Interface, got %T", onlyDecl(t, src))
	}
	return i
}

func TestInterfaceDecl(t *testing.T) {
	t.Parallel()

	t.Run("an interface converts to node.Interface, not a Struct", func(t *testing.T) {
		t.Parallel()
		// An interface is a contract rather than a type values are
		// made of, and consumers look for it by kind — a mock
		// generator asks the store for interfaces.
		i := onlyInterface(t, `interface User { id: string; name: string; }`)

		if i.Name != "User" {
			t.Fatalf("Name = %q, want User", i.Name)
		}
		if i.Package != "./fixture" {
			t.Fatalf("Package = %q", i.Package)
		}
	})

	t.Run("properties land in Fields", func(t *testing.T) {
		t.Parallel()
		// The reason node.Interface carries a field list at all: most
		// TypeScript interfaces declare no methods.
		i := onlyInterface(t, `interface User { id: string; name: string; }`)

		if got := len(i.Fields); got != 2 {
			t.Fatalf("Fields = %d, want 2", got)
		}
		if i.Fields[0].Name != "id" || i.Fields[1].Name != "name" {
			t.Fatalf("fields = %q, %q; want id, name", i.Fields[0].Name, i.Fields[1].Name)
		}
		for n, f := range i.Fields {
			if f.Owner != node.Node(i) {
				t.Errorf("field %d Owner not wired to the interface", n)
			}
		}
	})

	t.Run("properties and methods land in separate buckets", func(t *testing.T) {
		t.Parallel()
		i := onlyInterface(t, `interface A { p: string; m(x: number): void; }`)

		if len(i.Fields) != 1 || i.Fields[0].Name != "p" {
			t.Fatalf("Fields = %+v, want one field p", i.Fields)
		}
		if len(i.Methods) != 1 || i.Methods[0].Name != "m" {
			t.Fatalf("Methods = %+v, want one method m", i.Methods)
		}
		if got := len(i.Methods[0].Params); got != 1 {
			t.Fatalf("method params = %d, want 1", got)
		}
	})

	t.Run("FieldByName reaches a property", func(t *testing.T) {
		t.Parallel()
		i := onlyInterface(t, `interface A { p: string; q: number; }`)
		if f := i.FieldByName("q"); f == nil {
			t.Fatal("FieldByName could not reach a declared property")
		}
		if f := i.FieldByName("absent"); f != nil {
			t.Fatal("FieldByName invented a property")
		}
	})

	t.Run("optional and readonly ride on the field", func(t *testing.T) {
		t.Parallel()
		i := onlyInterface(t, `interface A { readonly a: string; b?: number; c: boolean; }`)

		if ro, _ := typescript.MetaReadonly.Get(i.Fields[0].Meta()); !ro {
			t.Error("a: readonly not stamped")
		}
		if opt, _ := typescript.MetaOptional.Get(i.Fields[1].Meta()); !opt {
			t.Error("b: optional not stamped")
		}
		if typescript.MetaOptional.Has(i.Fields[2].Meta()) {
			t.Error("c: optional stamped on a required field")
		}
	})

	t.Run("an extends clause yields extends embeds", func(t *testing.T) {
		t.Parallel()
		i := onlyInterface(t, `interface A extends B, C {}`)
		if got := len(i.Embeds); got != 2 {
			t.Fatalf("Embeds = %d, want 2", got)
		}
		for n, e := range i.Embeds {
			if got, _ := typescript.MetaHeritage.Get(e.Meta()); got != typescript.HeritageExtends {
				t.Errorf("embed %d heritage = %q, want extends", n, got)
			}
			if e.Owner != node.Node(i) {
				t.Errorf("embed %d Owner not wired", n)
			}
		}
	})

	t.Run("index and construct signatures ride on the interface", func(t *testing.T) {
		t.Parallel()
		i := onlyInterface(t, `interface A { [k: string]: unknown; new (): A; }`)

		if got, _ := typescript.MetaIndexSignature.Get(i.Meta()); got != "[k: string]: unknown" {
			t.Errorf("indexSignature = %q", got)
		}
		if got, _ := typescript.MetaConstructSignature.Get(i.Meta()); got != "new (): A" {
			t.Errorf("constructSignature = %q", got)
		}
		if len(i.Fields) != 0 || len(i.Methods) != 0 {
			t.Error("signatures leaked into Fields or Methods")
		}
	})

	t.Run("generics carry bound and default", func(t *testing.T) {
		t.Parallel()
		i := onlyInterface(t, `interface A<T extends object = {}> { v: T; }`)

		if got := len(i.TypeParams); got != 1 {
			t.Fatalf("TypeParams = %d, want 1", got)
		}
		p := i.TypeParams[0]
		if p.Name != "T" {
			t.Fatalf("name = %q, want T", p.Name)
		}
		if p.Constraint == nil || p.Constraint.Raw != "object" {
			t.Fatalf("constraint = %+v, want raw object", p.Constraint)
		}
		if def, _ := typescript.MetaTypeParamDefault.Get(p.Meta()); def != "{}" {
			t.Errorf("default = %q, want {}", def)
		}
		if p.Owner != node.Node(i) {
			t.Error("type param Owner not wired to the interface")
		}
	})

	t.Run("a JSDoc block becomes doc lines", func(t *testing.T) {
		t.Parallel()
		i := onlyInterface(t, "/**\n * A user.\n *\n * Second paragraph.\n */\nexport interface User { id: string; }")

		want := []string{"A user.", "", "Second paragraph."}
		got := i.Docs()
		if len(got) != len(want) {
			t.Fatalf("docs = %q, want %q", got, want)
		}
		for n := range want {
			if got[n] != want[n] {
				t.Fatalf("docs = %q, want %q", got, want)
			}
		}
	})

	t.Run("a namespace flattens and records its path", func(t *testing.T) {
		t.Parallel()
		decls, _ := convert(t, `namespace Outer.Inner { export interface A {} }`)
		if len(decls) != 1 {
			t.Fatalf("expected 1 hoisted declaration, got %d", len(decls))
		}
		i, ok := decls[0].(*node.Interface)
		if !ok {
			t.Fatalf("got %T", decls[0])
		}
		if ns, _ := typescript.MetaNamespace.Get(i.Meta()); ns != "Outer.Inner" {
			t.Errorf("namespace = %q, want Outer.Inner", ns)
		}
	})
}

func TestStructDecl(t *testing.T) {
	t.Parallel()

	t.Run("a class converts to node.Struct", func(t *testing.T) {
		t.Parallel()
		// A class is instantiable, which is what Struct models.
		s := onlyStruct(t, `class User { id: string = ''; }`)
		if s.Name != "User" {
			t.Fatalf("Name = %q, want User", s.Name)
		}
		if got := len(s.Fields); got != 1 {
			t.Fatalf("Fields = %d, want 1", got)
		}
	})

	t.Run("extends and implements are distinguishable embeds", func(t *testing.T) {
		t.Parallel()
		s := onlyStruct(t, `class C extends B implements F, G {}`)

		if got := len(s.Embeds); got != 3 {
			t.Fatalf("Embeds = %d, want 3", got)
		}
		want := []string{
			typescript.HeritageExtends,
			typescript.HeritageImplements,
			typescript.HeritageImplements,
		}
		for i, w := range want {
			got, _ := typescript.MetaHeritage.Get(s.Embeds[i].Meta())
			if got != w {
				t.Errorf("embed %d heritage = %q, want %q", i, got, w)
			}
		}
	})

	t.Run("export and declare are recorded from the wrappers", func(t *testing.T) {
		t.Parallel()
		s := onlyStruct(t, `export declare class C {}`)

		if ex, _ := typescript.MetaExported.Get(s.Meta()); !ex {
			t.Error("exported not stamped")
		}
		if am, _ := typescript.MetaAmbient.Get(s.Meta()); !am {
			t.Error("ambient not stamped")
		}
	})

	t.Run("an abstract class is marked", func(t *testing.T) {
		t.Parallel()
		s := onlyStruct(t, `abstract class C {}`)
		if ab, _ := typescript.MetaAbstract.Get(s.Meta()); !ab {
			t.Error("abstract not stamped")
		}
	})

	t.Run("a constructor parameter property becomes a field too", func(t *testing.T) {
		t.Parallel()
		// The declaration is in the parameter list but the member
		// belongs to the class, so both views have to carry it.
		s := onlyStruct(t, `class C { constructor(public readonly y: string, z: number) {} }`)

		if got := len(s.Fields); got != 1 {
			t.Fatalf("Fields = %d, want 1 (only the parameter property)", got)
		}
		if s.Fields[0].Name != "y" {
			t.Fatalf("field = %q, want y", s.Fields[0].Name)
		}
		if v, _ := typescript.MetaVisibility.Get(s.Fields[0].Meta()); v != typescript.VisibilityPublic {
			t.Errorf("visibility = %q, want public", v)
		}
		if ro, _ := typescript.MetaReadonly.Get(s.Fields[0].Meta()); !ro {
			t.Error("readonly did not carry to the field")
		}
	})

	t.Run("a hash-private field is distinguished from private", func(t *testing.T) {
		t.Parallel()
		// `private` erases at compile time; `#x` is enforced at
		// runtime. Collapsing them would claim a guarantee one of them
		// does not make.
		s := onlyStruct(t, `class C { private a = 1; #b = 2; }`)

		gotA, _ := typescript.MetaVisibility.Get(s.Fields[0].Meta())
		gotB, _ := typescript.MetaVisibility.Get(s.Fields[1].Meta())
		if gotA != typescript.VisibilityPrivate {
			t.Errorf("a visibility = %q, want private", gotA)
		}
		if gotB != typescript.VisibilityHard {
			t.Errorf("#b visibility = %q, want hard-private", gotB)
		}
	})

	t.Run("an anonymous default class is reported not silently dropped", func(t *testing.T) {
		t.Parallel()
		decls, sink := convert(t, `export default class {}`)
		if len(decls) != 0 {
			t.Fatalf("expected no declarations, got %d", len(decls))
		}
		if len(sink.Diagnostics()) == 0 {
			t.Fatal("anonymous class dropped without a diagnostic")
		}
	})
}
