// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

func onlyAlias(t *testing.T, src string) *node.Alias {
	t.Helper()
	decls, _ := convert(t, src)
	if len(decls) != 1 {
		t.Fatalf("expected 1 declaration, got %d", len(decls))
	}
	a, ok := decls[0].(*node.Alias)
	if !ok {
		t.Fatalf("expected *node.Alias, got %T", decls[0])
	}
	return a
}

func TestAliasDecl(t *testing.T) {
	t.Parallel()

	t.Run("a type alias is always the alias form", func(t *testing.T) {
		t.Parallel()
		// TypeScript's `type` never creates a nominal type, so the
		// defined-type form Go also has does not arise.
		a := onlyAlias(t, `type A = B;`)
		if !a.IsAlias {
			t.Fatal("IsAlias = false; TypeScript has no defined-type form")
		}
		if a.Target == nil || a.Target.Name != "B" {
			t.Fatalf("Target = %+v, want B", a.Target)
		}
	})

	t.Run("generic parameters are carried and owned", func(t *testing.T) {
		t.Parallel()
		a := onlyAlias(t, `type A<T> = B<T>;`)
		if len(a.TypeParams) != 1 || a.TypeParams[0].Name != "T" {
			t.Fatalf("TypeParams = %+v", a.TypeParams)
		}
		if a.TypeParams[0].Owner != node.Node(a) {
			t.Error("type param Owner not wired")
		}
		if a.Target == nil || len(a.Target.TypeArgs) != 1 {
			t.Fatalf("Target type args = %+v", a.Target)
		}
	})

	t.Run("a union target carries its members on TypeArgs", func(t *testing.T) {
		t.Parallel()
		a := onlyAlias(t, `type A = 'x' | 'y' | number;`)

		if !typescript.IsUnion(a.Target) {
			t.Fatalf("Target is not a union marker: %+v", a.Target)
		}
		if got := len(typescript.Members(a.Target)); got != 3 {
			t.Fatalf("union members = %d, want 3", got)
		}
	})

	t.Run("a nullable union is marked", func(t *testing.T) {
		t.Parallel()
		nullable := onlyAlias(t, `type A = string | null;`)
		plain := onlyAlias(t, `type B = string | number;`)

		if got, _ := typescript.MetaNullable.Get(nullable.Target.Meta()); !got {
			t.Error("string | null not marked nullable")
		}
		if typescript.MetaNullable.Has(plain.Target.Meta()) {
			t.Error("string | number marked nullable")
		}
	})

	t.Run("an intersection is distinct from a union", func(t *testing.T) {
		t.Parallel()
		a := onlyAlias(t, `type A = B & C;`)
		if !typescript.IsIntersection(a.Target) {
			t.Fatalf("Target is not an intersection: %+v", a.Target)
		}
		if typescript.IsUnion(a.Target) {
			t.Error("intersection also reports as a union")
		}
	})

	t.Run("a type with no structured form is carried as text", func(t *testing.T) {
		t.Parallel()
		// The deliberate limit: modelling conditional types
		// structurally means modelling most of TypeScript's type
		// system in a language-agnostic package.
		a := onlyAlias(t, `type A<T> = T extends string ? X : Y;`)

		if !typescript.IsOperator(a.Target) {
			t.Fatalf("Target is not the operator marker: %+v", a.Target)
		}
		got, _ := typescript.MetaTypeText.Get(a.Target.Meta())
		if got != "T extends string ? X : Y" {
			t.Fatalf("typeText = %q", got)
		}
	})

	t.Run("an array target is a slice, not a marker", func(t *testing.T) {
		t.Parallel()
		a := onlyAlias(t, `type A = string[];`)
		if !a.Target.IsSlice() {
			t.Fatalf("Target = %+v, want a slice", a.Target)
		}
		if a.Target.Elem == nil || a.Target.Elem.Name != "string" {
			t.Fatalf("Elem = %+v", a.Target.Elem)
		}
	})

	t.Run("a function type keeps params and return", func(t *testing.T) {
		t.Parallel()
		a := onlyAlias(t, `type A = (x: string, y: number) => boolean;`)

		if !a.Target.IsFunc() {
			t.Fatalf("Target = %+v, want a func", a.Target)
		}
		if got := len(a.Target.FuncParams); got != 2 {
			t.Fatalf("FuncParams = %d, want 2", got)
		}
		if got := len(a.Target.FuncReturns); got != 1 {
			t.Fatalf("FuncReturns = %d, want 1", got)
		}
	})

	t.Run("an inline object type becomes an anonymous struct", func(t *testing.T) {
		t.Parallel()
		a := onlyAlias(t, `type A = { x: string; y?: number };`)

		if !a.Target.IsAnonStruct() {
			t.Fatalf("Target = %+v, want an anon struct", a.Target)
		}
		if got := len(a.Target.Fields); got != 2 {
			t.Fatalf("Fields = %d, want 2", got)
		}
		if opt, _ := typescript.MetaOptional.Get(a.Target.Fields[1].Meta()); !opt {
			t.Error("y? not marked optional")
		}
	})

	t.Run("a mapped type is not mistaken for an index signature", func(t *testing.T) {
		t.Parallel()
		// Both are an index_signature inside an object_type; only the
		// `in` keyword separates them, and it is an anonymous token.
		mapped := onlyAlias(t, `type A<T> = { [K in keyof T]: T[K] };`)
		indexed := onlyAlias(t, `type B = { [k: string]: number };`)

		if got, _ := typescript.MetaMapped.Get(mapped.Target.Meta()); !got {
			t.Error("mapped type not marked")
		}
		if typescript.MetaMapped.Has(indexed.Target.Meta()) {
			t.Error("plain index signature marked as mapped")
		}
		// An index signature is an associative container, not a
		// struct with no members.
		if !indexed.Target.IsMap() {
			t.Errorf("index-signature object = %+v, want a map", indexed.Target)
		}
		if indexed.Target.MapKey == nil || indexed.Target.MapKey.Name != "string" {
			t.Errorf("MapKey = %+v, want string", indexed.Target.MapKey)
		}
		if indexed.Target.MapValue == nil || indexed.Target.MapValue.Name != "number" {
			t.Errorf("MapValue = %+v, want number", indexed.Target.MapValue)
		}
	})

	t.Run("an object with fields and an index signature stays a struct", func(t *testing.T) {
		t.Parallel()
		// It does have those members; the signature says what else is
		// admitted alongside them.
		a := onlyAlias(t, `type A = { x: string; [k: string]: unknown };`)

		if !a.Target.IsAnonStruct() {
			t.Fatalf("Target = %+v, want an anon struct", a.Target)
		}
		if len(a.Target.Fields) != 1 {
			t.Fatalf("Fields = %d, want 1", len(a.Target.Fields))
		}
		if !typescript.MetaIndexSignature.Has(a.Target.Meta()) {
			t.Error("index signature not recorded on the struct")
		}
	})

	t.Run("a tuple carries element modifiers", func(t *testing.T) {
		t.Parallel()
		a := onlyAlias(t, `type A = [a: string, b?: number];`)

		if !typescript.IsTuple(a.Target) {
			t.Fatalf("Target = %+v, want a tuple", a.Target)
		}
		members := typescript.Members(a.Target)
		if len(members) != 2 {
			t.Fatalf("elements = %d, want 2", len(members))
		}
		if opt, _ := typescript.MetaOptional.Get(members[1].Meta()); !opt {
			t.Error("optional tuple element not marked")
		}
	})

	t.Run("a qualified name splits into package and name", func(t *testing.T) {
		t.Parallel()
		a := onlyAlias(t, `type A = ns.Inner.T;`)
		if a.Target.Package != "ns.Inner" || a.Target.Name != "T" {
			t.Fatalf("Target = %q.%q, want ns.Inner.T", a.Target.Package, a.Target.Name)
		}
	})

	t.Run("a literal type is distinguishable from a type of that name", func(t *testing.T) {
		t.Parallel()
		lit := onlyAlias(t, `type A = 'x';`)
		named := onlyAlias(t, `type B = x;`)

		if got, _ := typescript.MetaLiteralType.Get(lit.Target.Meta()); got != "'x'" {
			t.Errorf("literalType = %q, want 'x'", got)
		}
		if typescript.MetaLiteralType.Has(named.Target.Meta()) {
			t.Error("a named type was marked as a literal")
		}
	})
}
