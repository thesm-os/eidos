// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/core/directive"
)

// TestDocsAndDirectives covers the preferred-doc / spec+block
// directive walk.
func TestDocsAndDirectives(t *testing.T) {
	t.Parallel()
	t.Run("spec doc takes precedence over the block doc", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\n// block doc.\ntype (\n\t// spec doc.\n\tS struct{}\n)\n",
		})
		s := pkg.StructByName("S")
		if s == nil {
			t.Fatalf("S missing")
		}
		want := []string{"spec doc."}
		if !slices.Equal(s.DocLines, want) {
			t.Fatalf("DocLines = %v, want %v", s.DocLines, want)
		}
	})

	t.Run("block doc is used when the spec has no doc", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\n// block doc.\ntype (\n\tS struct{}\n)\n",
		})
		s := pkg.StructByName("S")
		want := []string{"block doc."}
		if !slices.Equal(s.DocLines, want) {
			t.Fatalf("DocLines = %v, want %v", s.DocLines, want)
		}
	})

	t.Run("directives parse off non-directive comments cleanly", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\n// +gen:mock\ntype I interface{ M() }\n",
		})
		i := pkg.InterfaceByName("I")
		if i == nil {
			t.Fatalf("I missing")
		}
		if len(i.DirectiveList) != 1 {
			t.Fatalf("expected 1 directive, got %d", len(i.DirectiveList))
		}
		if i.DirectiveList[0].Name != directive.Name("mock") {
			t.Fatalf("directive name = %q, want %q", i.DirectiveList[0].Name, "mock")
		}
	})
}

// TestParseDirectives covers the directive comment walker on inputs
// with several directives, one valid + one malformed, and a comment
// that simply isn't a directive.
func TestParseDirectives(t *testing.T) {
	t.Parallel()
	t.Run("non-directive comments produce no diagnostics", func(t *testing.T) {
		t.Parallel()
		s, d := loadFromSource(t, map[string]string{
			"a.go": "package a\n\n// A plain doc-comment line.\ntype S struct{}\n",
		})
		_ = s
		for _, dg := range d.Diagnostics() {
			t.Errorf("unexpected diagnostic: %v %v %v", dg.Severity, dg.Pos, dg.Message)
		}
	})

	t.Run("malformed directive emits a positioned diagnostic", func(t *testing.T) {
		t.Parallel()
		_, d := loadFromSource(t, map[string]string{
			"a.go": "package a\n\n// +gen:\ntype S struct{}\n",
		})
		if !d.HasErrors() {
			t.Fatalf("expected an Error diagnostic for empty directive name")
		}
	})

	t.Run("multiple valid directives are recorded in order", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\n// +gen:mock\n// +gen:repo\ntype I interface{ M() }\n",
		})
		i := pkg.InterfaceByName("I")
		names := []directive.Name{i.DirectiveList[0].Name, i.DirectiveList[1].Name}
		want := []directive.Name{"mock", "repo"}
		if !slices.Equal(names, want) {
			t.Fatalf("directives = %v, want %v", names, want)
		}
	})
}

// TestPreferred verifies the [preferred] helper picks the first
// non-empty comment group from its alternatives.
func TestPreferred(t *testing.T) {
	t.Parallel()
	t.Run("missing both yields nil docs", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{}\n",
		})
		if pkg.StructByName("S").DocLines != nil {
			t.Fatalf("expected nil DocLines on undocumented struct")
		}
	})
}

// TestDocsAndDirectives_TrailingComment covers the trailing position
// on the AST nodes that have one — ValueSpec and TypeSpec.
//
// A struct field honoured a trailing directive; a const did not. The
// gap mattered most on enums, where a const block is a table and the
// override belongs on the row: `+gen:value` written the natural way
// was silently ignored, the generator emitted the derived spelling,
// the round-trip test passed against it, and the wrong value reached
// the external protocol with nothing reported at any severity.
//
// Enum variants inherit their BaseNode from the Constant they were
// coalesced from, so the const case carries them too.
func TestDocsAndDirectives_TrailingComment(t *testing.T) {
	t.Parallel()

	t.Run("a trailing directive on a const is attached", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nconst Limit int = 10 //+gen:value ten\n",
		})
		c := pkg.ConstantByName("Limit")
		if c == nil {
			t.Fatalf("Limit missing")
		}
		if got := c.Directive("value"); got == nil {
			t.Fatalf("trailing directive dropped; got %+v", c.DirectiveList)
		}
	})

	t.Run("a trailing directive on an enum variant is attached", func(t *testing.T) {
		t.Parallel()
		// The reported shape: an override on the row rather than
		// above it.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Pill int\n\nconst (\n" +
				"\tPillAspirin Pill = iota // a trailing comment\n" +
				"\tPillIbuprofen //+gen:value ibuprofen-200\n" +
				")\n",
		})
		e := pkg.EnumByName("Pill")
		if e == nil {
			t.Fatalf("Pill not promoted to enum")
		}
		v := e.VariantByName("PillIbuprofen")
		if v == nil {
			t.Fatalf("PillIbuprofen missing; got %+v", e.Variants)
		}
		got := v.Directive("value")
		if got == nil {
			t.Fatalf("trailing directive dropped; got %+v", v.DirectiveList)
		}
		if len(got.Args) != 1 || got.Args[0] != "ibuprofen-200" {
			t.Fatalf("directive args = %+v, want [ibuprofen-200]", got.Args)
		}
	})

	t.Run("a plain trailing comment attaches no directive", func(t *testing.T) {
		t.Parallel()
		// Prose on the line must stay prose. Treating it as a value
		// would mean a clarifying note silently changes what a type
		// marshals to.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Pill int\n\nconst (\n" +
				"\tPillAspirin Pill = iota // a trailing comment\n)\n",
		})
		v := pkg.EnumByName("Pill").VariantByName("PillAspirin")
		if len(v.DirectiveList) != 0 {
			t.Fatalf("prose must not become a directive; got %+v", v.DirectiveList)
		}
	})

	t.Run("a trailing directive on a var is attached", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nvar Registry map[string]int //+gen:value reg\n",
		})
		v := pkg.VariableByName("Registry")
		if v == nil || v.Directive("value") == nil {
			t.Fatalf("trailing directive dropped; got %+v", v)
		}
	})

	t.Run("a trailing directive on a type spec is attached", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Box struct{} //+gen:value boxed\n",
		})
		s := pkg.StructByName("Box")
		if s == nil || s.Directive("value") == nil {
			t.Fatalf("trailing directive dropped; got %+v", s)
		}
	})

	t.Run("leading and trailing directives both attach, in source order", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\n//+gen:value above\nconst Limit int = 10 //+gen:value beside\n",
		})
		c := pkg.ConstantByName("Limit")
		if len(c.DirectiveList) != 2 {
			t.Fatalf("expected both directives; got %+v", c.DirectiveList)
		}
		if c.DirectiveList[0].Args[0] != "above" || c.DirectiveList[1].Args[0] != "beside" {
			t.Fatalf("order should follow the source; got %+v", c.DirectiveList)
		}
	})

	t.Run("a trailing comment does not become a doc line", func(t *testing.T) {
		t.Parallel()
		// A trailing comment is a note on the line, not the entity's
		// doc comment; folding it into Docs would put it in generated
		// godoc where it does not belong.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nconst Limit int = 10 // a trailing note\n",
		})
		if got := pkg.ConstantByName("Limit").DocLines; len(got) != 0 {
			t.Fatalf("DocLines = %v, want none", got)
		}
	})
}
