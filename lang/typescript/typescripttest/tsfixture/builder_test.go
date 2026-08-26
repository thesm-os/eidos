// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tsfixture_test

import (
	"path"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/lang/typescript/typescripttest/tsfixture"
	"go.thesmos.sh/eidos/node"
)

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("seeds a named package with a directory", func(t *testing.T) {
		t.Parallel()
		// A TypeScript frontend names a package after the directory it
		// read, so a fixture spelling them any other way would route
		// its output somewhere a real run never would.
		pkg := tsfixture.New().PackageNode()
		if pkg.Name != "test" || pkg.Path != "src/test" {
			t.Fatalf("package = %q at %q", pkg.Name, pkg.Path)
		}
		if path.Base(pkg.Path) != pkg.Name {
			t.Fatalf("name %q is not the basename of %q", pkg.Name, pkg.Path)
		}
	})

	t.Run("the built package claims TypeScript", func(t *testing.T) {
		t.Parallel()
		// Every plugin dispatching per package reads this marker, and
		// one that finds none skips the package without a diagnostic.
		lang, ok := node.MetaFrontend.Get(built(t).Meta())
		if !ok || lang != typescript.Language {
			t.Fatalf("frontend marker = %q, %v", lang, ok)
		}
	})

	t.Run("Unmarked produces the graph nothing claimed", func(t *testing.T) {
		t.Parallel()
		b := tsfixture.New().Unmarked().Interface("A", nil)
		b.Build()
		if _, ok := node.MetaFrontend.Get(b.PackageNode().Meta()); ok {
			t.Fatal("an unmarked package carries a frontend marker")
		}
	})

	t.Run("Language overrides what the package claims", func(t *testing.T) {
		t.Parallel()
		b := tsfixture.New().Language("protobuf").Interface("A", nil)
		b.Build()
		if lang, _ := node.MetaFrontend.Get(b.PackageNode().Meta()); lang != "protobuf" {
			t.Fatalf("frontend marker = %q, want protobuf", lang)
		}
	})
}

func TestBuilderPackage(t *testing.T) {
	t.Parallel()

	t.Run("renaming retargets every declaration", func(t *testing.T) {
		t.Parallel()
		// A package renamed after its declarations were added would
		// otherwise route its generated output into the old directory.
		b := tsfixture.New().
			Interface("User", func(i *tsfixture.InterfaceBuilder) {
				i.Field("id", tsfixture.Named("string"), nil)
				i.Method("greet", nil)
			}).
			Class("Repo", nil).
			Enum("Role", nil).
			Alias("ID", nil).
			Function("f", nil).
			Variable("v", nil).
			Constant("c", nil).
			Package("api", "src/api")

		pkg := b.PackageNode()
		for _, i := range pkg.Interfaces {
			assertFile(t, "interface", i.Pos().File, "api/user.ts")
			assertFile(t, "property", i.Fields[0].Pos().File, "api/user.ts")
			assertFile(t, "method", i.Methods[0].Pos().File, "api/user.ts")
			if i.Package != "src/api" {
				t.Errorf("interface package = %q", i.Package)
			}
		}
		assertFile(t, "class", pkg.Structs[0].Pos().File, "api/repo.ts")
		assertFile(t, "enum", pkg.Enums[0].Pos().File, "api/role.ts")
		assertFile(t, "alias", pkg.Aliases[0].Pos().File, "api/id.ts")
		assertFile(t, "function", pkg.Functions[0].Pos().File, "api/f.ts")
		assertFile(t, "variable", pkg.Variables[0].Pos().File, "api/v.ts")
		assertFile(t, "constant", pkg.Constants[0].Pos().File, "api/c.ts")
	})

	t.Run("a pinned position survives a rename", func(t *testing.T) {
		t.Parallel()
		// The explicit value always wins: a test that pinned a
		// generated filename did so deliberately.
		b := tsfixture.New().
			Class("Repo", func(c *tsfixture.ClassBuilder) {
				c.Pos(position.Pos{File: "pinned/place.ts"})
			}).
			Package("api", "src/api")
		assertFile(t, "class", b.PackageNode().Structs[0].Pos().File, "pinned/place.ts")
	})

	t.Run("PackageName retargets nothing", func(t *testing.T) {
		t.Parallel()
		// For the case where the directory and the declared name
		// deliberately differ: retargeting would move the declarations
		// into a directory named after the clause, which is the
		// disagreement under test.
		b := tsfixture.New().Class("Repo", nil).PackageName("api")
		if got := b.PackageNode().Name; got != "api" {
			t.Fatalf("name = %q", got)
		}
		if got := b.PackageNode().Path; got != "src/test" {
			t.Fatalf("path = %q, want the original", got)
		}
		assertFile(t, "class", b.PackageNode().Structs[0].Pos().File, "test/repo.ts")
	})
}

func TestBuilderDeclarations(t *testing.T) {
	t.Parallel()

	t.Run("every declaration reaches the store", func(t *testing.T) {
		t.Parallel()
		pkg := tsfixture.New().
			Interface("A", nil).Class("B", nil).Enum("C", nil).
			Alias("D", func(a *tsfixture.AliasBuilder) { a.Target(tsfixture.Named("string")) }).
			Function("e", nil).Variable("f", nil).Constant("g", nil).
			PackageNode()

		counts := map[string]int{
			"interfaces": len(pkg.Interfaces), "classes": len(pkg.Structs),
			"enums": len(pkg.Enums), "aliases": len(pkg.Aliases),
			"functions": len(pkg.Functions), "variables": len(pkg.Variables),
			"constants": len(pkg.Constants),
		}
		for kind, n := range counts {
			if n != 1 {
				t.Errorf("%s = %d, want 1", kind, n)
			}
		}
	})

	t.Run("an alias is always a true alias", func(t *testing.T) {
		t.Parallel()
		// TypeScript has no counterpart to Go's type definition:
		// `type X = Y` introduces a name for Y and nothing more.
		b := tsfixture.New().Alias("ID", func(a *tsfixture.AliasBuilder) {
			a.Target(tsfixture.Named("string"))
		})
		if !b.PackageNode().Aliases[0].IsAlias {
			t.Fatal("the alias was built as a type definition")
		}
	})

	t.Run("a nil callback declares the bare shape", func(t *testing.T) {
		t.Parallel()
		b := tsfixture.New().Interface("A", nil).Class("B", nil)
		if len(b.PackageNode().Interfaces) != 1 || len(b.PackageNode().Structs) != 1 {
			t.Fatal("a nil callback declared nothing")
		}
	})

	t.Run("an unnamed declaration still gets a routable file", func(t *testing.T) {
		t.Parallel()
		// Fixture misuse, but it must not reintroduce the empty
		// basename the synthetic position exists to prevent.
		b := tsfixture.New().Interface("", nil)
		assertFile(t, "interface", b.PackageNode().Interfaces[0].Pos().File, "test/decl.ts")
	})

	t.Run("Build returns a fresh store each call", func(t *testing.T) {
		t.Parallel()
		b := tsfixture.New().Interface("A", nil)
		first, second := b.Build(), b.Build()
		if first == second {
			t.Fatal("two calls returned one store")
		}
	})

	t.Run("a duplicate name panics rather than building half a graph", func(t *testing.T) {
		t.Parallel()
		defer func() {
			if recover() == nil {
				t.Fatal("a duplicated name built successfully")
			}
		}()
		tsfixture.New().Interface("A", nil).Interface("A", nil).Build()
	})
}

func TestBuilderImports(t *testing.T) {
	t.Parallel()

	t.Run("a package-level import is recorded", func(t *testing.T) {
		t.Parallel()
		b := tsfixture.New().Import("./user").ImportAs("pb", "./gen")
		imports := b.PackageNode().Imports
		if len(imports) != 2 {
			t.Fatalf("imports = %d, want 2", len(imports))
		}
		if imports[1].Alias != "pb" || imports[1].Path != "./gen" {
			t.Fatalf("aliased import = %+v", imports[1])
		}
	})

	t.Run("a module's imports reach the package union", func(t *testing.T) {
		t.Parallel()
		// A consumer reading Package.Imports would otherwise see a
		// package importing nothing while its modules import plenty.
		b := tsfixture.New().File("user.ts", func(f *tsfixture.FileBuilder) {
			f.Import("./a").ImportAs("b", "./b")
		})
		if got := len(b.PackageNode().Imports); got != 2 {
			t.Fatalf("union = %d, want 2", got)
		}
	})

	t.Run("one specifier under two names stays two facts", func(t *testing.T) {
		t.Parallel()
		// Collapsing them loses the one a resolver needs.
		b := tsfixture.New().
			File("a.ts", func(f *tsfixture.FileBuilder) { f.ImportAs("x", "./m") }).
			File("b.ts", func(f *tsfixture.FileBuilder) { f.ImportAs("y", "./m") })
		if got := len(b.PackageNode().Imports); got != 2 {
			t.Fatalf("union = %d, want both bindings", got)
		}
	})

	t.Run("the same import twice is deduped", func(t *testing.T) {
		t.Parallel()
		b := tsfixture.New().
			File("a.ts", func(f *tsfixture.FileBuilder) { f.Import("./m") }).
			File("b.ts", func(f *tsfixture.FileBuilder) { f.Import("./m") })
		if got := len(b.PackageNode().Imports); got != 1 {
			t.Fatalf("union = %d, want 1", got)
		}
	})

	t.Run("naming a file twice accumulates into one module", func(t *testing.T) {
		t.Parallel()
		// The alternative is two files of one name, a shape no
		// filesystem produces.
		b := tsfixture.New().
			File("user.ts", func(f *tsfixture.FileBuilder) { f.Import("./a") }).
			File("user.ts", func(f *tsfixture.FileBuilder) { f.Import("./b") })
		files := b.PackageNode().Files
		if len(files) != 1 {
			t.Fatalf("files = %d, want 1", len(files))
		}
		if len(files[0].Imports) != 2 {
			t.Fatalf("imports on the module = %d, want 2", len(files[0].Imports))
		}
	})

	t.Run("a type-only import is told apart from a value one", func(t *testing.T) {
		t.Parallel()
		// A type-only import is erased at compile time, so a generated
		// file emitting it for a value it constructs fails at run time
		// with the binding undefined and the compiler says nothing.
		b := tsfixture.New().File("user.ts", func(f *tsfixture.FileBuilder) {
			f.ImportType("./user").Import("./other")
		})
		imports := b.PackageNode().Files[0].Imports
		if only, _ := typescript.MetaTypeOnly.Get(imports[0].Meta()); !only {
			t.Error("the type-only import is not marked")
		}
		if only, _ := typescript.MetaTypeOnly.Get(imports[1].Meta()); only {
			t.Error("a value import is marked type-only")
		}
	})

	t.Run("a re-export names what it forwards", func(t *testing.T) {
		t.Parallel()
		b := tsfixture.New().File("index.ts", func(f *tsfixture.FileBuilder) {
			f.ReExport("./user", "User", "Role").ReExport("./all")
		})
		imports := b.PackageNode().Files[0].Imports
		if re, _ := typescript.MetaReExport.Get(imports[0].Meta()); !re {
			t.Fatal("the re-export is not marked")
		}
		names, _ := typescript.MetaReExportNames.Get(imports[0].Meta())
		if strings.Join(names, ",") != "User,Role" {
			t.Fatalf("forwarded names = %v", names)
		}
		if names, ok := typescript.MetaReExportNames.Get(imports[1].Meta()); ok {
			t.Fatalf("the star form named %v", names)
		}
	})

	t.Run("a module records its specifier verbatim", func(t *testing.T) {
		t.Parallel()
		// `./user` and `user` resolve differently and neither is
		// normalised.
		b := tsfixture.New().File("a.ts", func(f *tsfixture.FileBuilder) {
			f.Import("../lib/user")
		})
		imp := b.PackageNode().Files[0].Imports[0]
		if spec, _ := typescript.MetaModuleSpecifier.Get(imp.Meta()); spec != "../lib/user" {
			t.Fatalf("specifier = %q", spec)
		}
	})
}

// built returns the package node of a store built from one
// declaration.
func built(t *testing.T) *node.Package {
	t.Helper()
	b := tsfixture.New().Interface("A", nil)
	b.Build()
	return b.PackageNode()
}

// assertFile reports a declaration positioned at the wrong file.
func assertFile(t *testing.T, what, got, want string) {
	t.Helper()
	if got != want {
		t.Errorf("%s file = %q, want %q", what, got, want)
	}
}
