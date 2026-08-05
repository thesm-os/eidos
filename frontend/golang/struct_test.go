// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"slices"
	"strings"
	"testing"
)

// TestConvertStruct covers the per-struct conversion path: name,
// package, generic params, fields, embeds, methods.
func TestConvertStruct(t *testing.T) {
	t.Parallel()
	t.Run("struct carries name and package path", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype User struct{}\n",
		})
		s := pkg.StructByName("User")
		if s == nil || s.Name != "User" {
			t.Fatalf("User struct missing")
		}
		if s.Package == "" {
			t.Fatalf("expected non-empty package path")
		}
	})

	t.Run("generic struct carries type parameters", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Box[T any] struct{ V T }\n",
		})
		s := pkg.StructByName("Box")
		if !s.IsGeneric() || len(s.TypeParams) != 1 {
			t.Fatalf("Box must declare a single type parameter")
		}
	})

	t.Run("multi-name fields share docs and tag", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{\n\t// Coords.\n\tX, Y int `json:\"coord\"`\n}\n",
		})
		s := pkg.StructByName("S")
		x := s.FieldByName("X")
		y := s.FieldByName("Y")
		if x == nil || y == nil {
			t.Fatalf("X / Y fields missing")
		}
		if !slices.Equal(x.DocLines, y.DocLines) {
			t.Fatalf("multi-name fields must share docs, got X=%v Y=%v", x.DocLines, y.DocLines)
		}
		if x.Tag != y.Tag || x.Tag != `json:"coord"` {
			t.Fatalf("tags must match and survive verbatim, got X=%q Y=%q", x.Tag, y.Tag)
		}
	})

	t.Run("embedded type surfaces as Embed", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype I interface{ M() }\n\ntype S struct{ I }\n",
		})
		s := pkg.StructByName("S")
		if len(s.Embeds) != 1 || s.Embeds[0].Type.Name != "I" {
			t.Fatalf("expected one embed of I, got %+v", s.Embeds)
		}
	})

	t.Run("methods attach to the struct via attachMethods", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{}\n\nfunc (s S) Do() {}\nfunc (s *S) Touch() {}\n",
		})
		s := pkg.StructByName("S")
		if len(s.Methods) != 2 {
			t.Fatalf("expected 2 methods (value+pointer), got %d", len(s.Methods))
		}
		// Both methods share a method set so they attach to S regardless of receiver form.
		names := []string{s.Methods[0].Name, s.Methods[1].Name}
		slices.Sort(names)
		if !slices.Equal(names, []string{"Do", "Touch"}) {
			t.Fatalf("methods = %v, want [Do Touch]", names)
		}
	})

	t.Run("field positions are accurate to file:line", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{\n\tName string\n}\n",
		})
		f := pkg.StructByName("S").FieldByName("Name")
		pos := f.Pos()
		if pos.Line == 0 || !strings.HasSuffix(pos.File, "a.go") {
			t.Fatalf("field position not populated: %+v", pos)
		}
	})

	t.Run("alias of a struct populates fields via the type-only path", func(t *testing.T) {
		t.Parallel()
		// `type Alias = Original` of a struct surfaces as a fresh
		// Struct whose body is built by [populateStructFromTypeOnly]
		// because the AST type expression for Alias is an Ident
		// rather than a StructType.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Original struct{ Name string }\n\ntype Alias = Original\n",
		})
		alias := pkg.StructByName("Alias")
		if alias == nil {
			t.Fatalf("alias of struct did not surface; the type-only path is what this test exists to exercise")
		}
		if len(alias.Fields) == 0 {
			t.Fatalf("Alias must carry fields populated by the type-only path")
		}
	})

	t.Run("anonymous-struct field tag round-trips through backticks", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ Inner struct{ X int `json:\"x\"` } }\n",
		})
		f := pkg.StructByName("S").FieldByName("Inner")
		if !f.Type.IsAnonStruct() {
			t.Fatalf("expected anon struct")
		}
		if len(f.Type.Fields) != 1 || f.Type.Fields[0].Tag != `json:"x"` {
			t.Fatalf("anon-struct tag missing, got %+v", f.Type.Fields)
		}
	})
}
