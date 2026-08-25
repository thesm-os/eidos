// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/core/directive"
)

// TestDocLinesFromCommentGroup exercises every documented comment
// form via the public Load surface — `//` line comments, multi-line
// `/* … */` block comments, and the leading-space stripping
// convention.
func TestDocLinesFromCommentGroup(t *testing.T) {
	t.Parallel()
	t.Run("line comments preserve content one entry per line", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\n// First line.\n// Second line.\ntype S struct{}\n",
		})
		s := pkg.StructByName("S")
		if s == nil {
			t.Fatalf("S missing")
		}
		want := []string{"First line.", "Second line."}
		if !slices.Equal(s.DocLines, want) {
			t.Fatalf("DocLines = %v, want %v", s.DocLines, want)
		}
	})

	t.Run("block comments split on newlines and strip * continuation", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\n/*\nFirst.\n* Second.\n*/\ntype S struct{}\n",
		})
		s := pkg.StructByName("S")
		if s == nil {
			t.Fatalf("S missing")
		}
		// The block-body splitter trims the closing-marker blank
		// line and the continuation `* ` prefix.
		want := []string{"", "First.", "Second."}
		if !slices.Equal(s.DocLines, want) {
			t.Fatalf("DocLines = %v, want %v", s.DocLines, want)
		}
	})

	t.Run("absent doc yields nil rather than empty slice", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{}\n",
		})
		s := pkg.StructByName("S")
		if s == nil {
			t.Fatalf("S missing")
		}
		if s.DocLines != nil {
			t.Fatalf("expected nil DocLines, got %v", s.DocLines)
		}
	})

	t.Run("standalone star line collapses to empty", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\n/*\nFirst.\n*\nThird.\n*/\ntype S struct{}\n",
		})
		s := pkg.StructByName("S")
		if s == nil {
			t.Fatalf("S missing")
		}
		want := []string{"", "First.", "", "Third."}
		if !slices.Equal(s.DocLines, want) {
			t.Fatalf("DocLines = %v, want %v", s.DocLines, want)
		}
	})
}

// TestPackageDocCollection verifies the converter hoists the first
// non-empty package-level doc comment found across files.
func TestPackageDocCollection(t *testing.T) {
	t.Parallel()
	t.Run("package doc surfaces on the Package node", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"doc.go": "// Package a is the canonical example.\npackage a\n",
			"a.go":   "package a\n\ntype S struct{}\n",
		})
		want := []string{"Package a is the canonical example."}
		if !slices.Equal(pkg.DocLines, want) {
			t.Fatalf("Package DocLines = %v, want %v", pkg.DocLines, want)
		}
	})

	t.Run("package without doc lines yields nil", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n",
		})
		if pkg.DocLines != nil {
			t.Fatalf("expected nil package doc, got %v", pkg.DocLines)
		}
	})
}

// TestPackageDirectives covers the directive surface on
// [node.Package]. The package-level comment block above `package`
// is parsed for `gen:` directives the same way every other decl
// kind is — without this wiring the directive line would be
// preserved as a doc string but never reach the parser.
func TestPackageDirectives(t *testing.T) {
	t.Parallel()
	t.Run("package-level gen: directive surfaces on Package.Directives", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"doc.go": "// Package a is the canonical example.\n" +
				"//\n" +
				"// +gen:scope owner=core\n" +
				"package a\n",
			"a.go": "package a\n\ntype S struct{}\n",
		})
		dirs := pkg.Directives()
		if len(dirs) != 1 {
			t.Fatalf("expected 1 package directive, got %d", len(dirs))
		}
		if dirs[0].Name != directive.Name("scope") {
			t.Fatalf("directive name = %q, want %q", dirs[0].Name, "scope")
		}
		if got := dirs[0].KV["owner"]; got != "core" {
			t.Fatalf("KV[owner] = %q, want %q", got, "core")
		}
	})

	t.Run("package without directives yields no Package directive list", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "// Package a is a plain doc-only package.\npackage a\n\ntype S struct{}\n",
		})
		if len(pkg.Directives()) != 0 {
			t.Fatalf("expected no package directives, got %+v", pkg.Directives())
		}
	})
}

// TestFileDirectivesAreNotRecorded pins the design choice that
// directives in the comment block above `package` belong to the
// package, not the file. Surfacing them on both would double-fire
// every validator that distinguishes node kinds — see
// [converter.convertFiles].
func TestFileDirectivesAreNotRecorded(t *testing.T) {
	t.Parallel()
	t.Run("package-level directive does not appear on the File node", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "// +gen:scope\npackage a\n\ntype S struct{}\n",
		})
		if len(pkg.Files) != 1 {
			t.Fatalf("expected 1 file, got %d", len(pkg.Files))
		}
		if len(pkg.Files[0].Directives()) != 0 {
			t.Fatalf("expected no file directives, got %+v", pkg.Files[0].Directives())
		}
		// And the same directive must still surface on the Package.
		if len(pkg.Directives()) != 1 {
			t.Fatalf("expected 1 package directive, got %d", len(pkg.Directives()))
		}
	})
}
