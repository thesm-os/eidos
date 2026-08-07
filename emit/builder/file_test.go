// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
)

// TestFileBuilder_ImportsAndBlanks covers the file builder's import
// surface, including the BlankImport shorthand.
func TestFileBuilder_ImportsAndBlanks(t *testing.T) {
	t.Parallel()

	t.Run("Import and BlankImport append owner-wired imports", func(t *testing.T) {
		t.Parallel()
		c := builder.For("repogen").WithTarget(defaultTarget)
		var file *emit.File
		c.Package("users", "example.com/users").
			AddFile(emit.Target{}, func(fb *builder.FileBuilder) {
				file = fb.Node()
				fb.Import("fmt", nil)
				fb.BlankImport("embed")
			})
		if len(file.Imports) != 2 {
			t.Fatalf("expected 2 imports; got %d", len(file.Imports))
		}
		if file.Imports[0].Path != "fmt" || file.Imports[0].Alias != "" {
			t.Fatalf("first import wrong; got %+v", file.Imports[0])
		}
		if file.Imports[1].Path != "embed" || file.Imports[1].Alias != "_" {
			t.Fatalf("blank import should carry alias _; got %+v", file.Imports[1])
		}
		for _, imp := range file.Imports {
			if imp.Owner != file {
				t.Fatalf("import Owner not wired on %q", imp.Path)
			}
		}
	})

	t.Run("zero target on AddFile() inherits the Context target", func(t *testing.T) {
		t.Parallel()
		c := builder.For("repogen").WithTarget(defaultTarget)
		var file *emit.File
		c.Package("users", "example.com/users").
			AddFile(emit.Target{}, func(fb *builder.FileBuilder) {
				file = fb.Node()
			})
		if file.Dir != defaultTarget.Dir || file.Name != defaultTarget.Filename {
			t.Fatalf("file target not inherited; got Dir=%q Name=%q", file.Dir, file.Name)
		}
	})
}

// TestFileBuilder_Accessors covers the Pos / Docs accessors on the
// file builder and the nested ImportBuilder accessors.
func TestFileBuilder_Accessors(t *testing.T) {
	t.Parallel()

	t.Run("Pos / Docs thread through; nested Import accessors", func(t *testing.T) {
		t.Parallel()
		c := builder.For("test").WithTarget(defaultTarget)
		other := otherTarget()
		d := fixtureDirective()
		pos := fixturePos()
		var node *emit.File
		c.Package("p", "p").
			AddFile(other, func(b *builder.FileBuilder) {
				node = b.Node()
				b.Pos(pos).Docs("docs").Import("fmt", func(ib *builder.ImportBuilder) {
					ib.Pos(pos).Docs("imp").Directive(d).Alias("fmtalias")
				})
			})
		if node.SourcePos != pos {
			t.Fatalf("file Pos override failed; got %v", node.SourcePos)
		}
		if len(node.DocLines) != 1 {
			t.Fatalf("file docs missing")
		}
		imp := node.Imports[0]
		assertCommon(t, imp.SourcePos, imp.DocLines, imp.DirectiveList, pos, d)
		if imp.Alias != "fmtalias" {
			t.Fatalf("import alias = %q, want fmtalias", imp.Alias)
		}
	})
}
