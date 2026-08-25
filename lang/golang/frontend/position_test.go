// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"strings"
	"testing"
)

// TestPosOf exercises the AST → position.Pos conversion through the
// public conversion path: a Load over a single-file source map yields
// a struct whose position must carry a non-zero line and column for a
// real source location, and must zero out for the zero token.Pos
// (covered by anonymous types whose origin is a synthesized
// declaration without a real position).
func TestPosOf(t *testing.T) {
	t.Parallel()
	t.Run("populates file:line:column for a declared struct", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Foo struct{ Name string }\n",
		})
		s := pkg.StructByName("Foo")
		if s == nil {
			t.Fatalf("Foo struct missing")
		}
		pos := s.Pos()
		if pos.Line == 0 || pos.Column == 0 {
			t.Fatalf("expected populated position, got %+v", pos)
		}
		if !strings.HasSuffix(pos.File, "a.go") {
			t.Fatalf("expected position file to end in a.go, got %q", pos.File)
		}
	})

	t.Run("returns a zero Pos for an absent source location", func(t *testing.T) {
		t.Parallel()
		// An anonymous struct's inline fields have no NamePos in the
		// AST for a field declared from a synthesised type-only
		// projection; the converter still gives them a populated
		// Pos via the type-checker's Field.Pos(), so the zero-Pos
		// edge surfaces through the package-level Pos which the
		// converter never explicitly sets.
		pkg := requirePackage(t, map[string]string{
			"b.go": "package b\n",
		})
		if !pkg.Pos().IsZero() {
			t.Fatalf("package Pos = %v, expected zero", pkg.Pos())
		}
	})
}
