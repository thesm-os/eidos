// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package storefixture_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/store"
)

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("seeds a default-named, default-pathed empty package", func(t *testing.T) {
		t.Parallel()
		b := storefixture.New()
		p := b.PackageNode()
		if p == nil {
			t.Fatalf("PackageNode returned nil")
		}
		if p.Name != "test" {
			t.Fatalf("default name should be %q; got %q", "test", p.Name)
		}
		if p.Path != "example.com/test" {
			t.Fatalf("default path should be %q; got %q", "example.com/test", p.Path)
		}
		if len(p.Structs) != 0 || len(p.Interfaces) != 0 || len(p.Functions) != 0 {
			t.Fatalf("fresh builder should hold no declarations")
		}
	})
}

func TestBuilder_Package(t *testing.T) {
	t.Parallel()

	t.Run("overwrites name and path on an empty builder", func(t *testing.T) {
		t.Parallel()
		b := storefixture.New().Package("users", "example.com/users")
		p := b.PackageNode()
		if p.Name != "users" || p.Path != "example.com/users" {
			t.Fatalf("Package did not overwrite identity: %+v", p)
		}
	})

	t.Run("rewrites the qualified-name path on every existing decl", func(t *testing.T) {
		t.Parallel()
		b := storefixture.New().
			Struct("S", nil).
			Interface("I", nil).
			Function("F", nil).
			Variable("V", nil).
			Constant("C", nil).
			Enum("E", nil).
			Alias("A", nil).
			Package("users", "example.com/users")

		p := b.PackageNode()
		requireQName(t, p.Structs[0].QName(), "example.com/users.S")
		requireQName(t, p.Interfaces[0].QName(), "example.com/users.I")
		requireQName(t, p.Functions[0].QName(), "example.com/users.F")
		requireQName(t, p.Variables[0].QName(), "example.com/users.V")
		requireQName(t, p.Constants[0].QName(), "example.com/users.C")
		requireQName(t, p.Enums[0].QName(), "example.com/users.E")
		requireQName(t, p.Aliases[0].QName(), "example.com/users.A")
	})

	t.Run("returns the same builder for chaining", func(t *testing.T) {
		t.Parallel()
		b := storefixture.New()
		if got := b.Package("a", "a"); got != b {
			t.Fatalf("Package should return its receiver for chaining")
		}
	})
}

func TestBuilder_Import(t *testing.T) {
	t.Parallel()

	t.Run("records the import on the package", func(t *testing.T) {
		t.Parallel()
		b := storefixture.New().Import("context").Import("time")
		imps := b.PackageNode().Imports
		if len(imps) != 2 {
			t.Fatalf("expected 2 imports; got %d", len(imps))
		}
		if imps[0].Path != "context" || imps[1].Path != "time" {
			t.Fatalf("imports wrong: %+v", imps)
		}
		if imps[0].Owner == nil {
			t.Fatalf("import owner back-pointer should be wired")
		}
	})
}

func TestBuilder_Build(t *testing.T) {
	t.Parallel()

	t.Run("returns a populated store with the configured package", func(t *testing.T) {
		t.Parallel()
		s := storefixture.New().Struct("User", nil).Build()
		got, ok := s.Nodes().Structs().ByQName("example.com/test.User")
		if !ok {
			t.Fatalf("struct should be indexed by qname")
		}
		if got.Name != "User" {
			t.Fatalf("struct name wrong: %q", got.Name)
		}
	})

	t.Run("returns independent stores across calls", func(t *testing.T) {
		t.Parallel()
		b := storefixture.New().Struct("X", nil)
		s1, s2 := b.Build(), b.Build()
		if s1 == s2 {
			t.Fatalf("Build must return fresh stores per call")
		}
	})

	t.Run("panics on duplicate qualified names", func(t *testing.T) {
		t.Parallel()
		b := storefixture.New().Struct("Dup", nil).Struct("Dup", nil)
		rec := requirePanic(t, func() { b.Build() })
		err, ok := rec.(error)
		if !ok {
			t.Fatalf("recovered value should be error; got %T", rec)
		}
		if !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("expected ErrDuplicateQName; got %v", err)
		}
	})
}

// routedBasename mirrors the Layout phase's basename derivation
// (pipeline.originSourceDirBasename): the origin's Pos.File is
// reduced to its extension-less basename, and the routed filename
// is that basename concatenated with the plugin's declared suffix.
// Reproduced here rather than imported because the derivation is
// unexported — the root module cannot be reached for it and the
// eidostest module must not grow a pipeline dependency to assert a
// fixture invariant.
func routedBasename(file string) string {
	if file == "" {
		return ""
	}
	base := filepath.Base(file)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// declaredPositions returns one labelled position per sub-builder
// that exposes Pos, read off a single fixture package. The nine
// entries are the complete set of fixture nodes the Layout phase
// can route an emitted declaration from.
func declaredPositions(pkg *node.Package) map[string]position.Pos {
	s := pkg.Structs[0]
	i := pkg.Interfaces[0]
	return map[string]position.Pos{
		"struct":    s.SourcePos,
		"field":     s.Fields[0].SourcePos,
		"method":    s.Methods[0].SourcePos,
		"interface": i.SourcePos,
		"function":  pkg.Functions[0].SourcePos,
		"variable":  pkg.Variables[0].SourcePos,
		"constant":  pkg.Constants[0].SourcePos,
		"enum":      pkg.Enums[0].SourcePos,
		"alias":     pkg.Aliases[0].SourcePos,
	}
}

// fixturePackage builds one package holding every declaration kind
// the fixture supports, with no explicit Pos call anywhere.
func fixturePackage() *node.Package {
	return storefixture.New().
		Struct("User", func(s *storefixture.StructBuilder) {
			s.Field("ID", storefixture.Named("string"), nil)
			s.Method("Save", nil)
		}).
		Interface("Store", func(i *storefixture.InterfaceBuilder) {
			i.Method("Get", nil)
		}).
		Function("Run", nil).
		Variable("Default", nil).
		Constant("Max", nil).
		Enum("Colour", nil).
		Alias("Key", nil).
		PackageNode()
}

// TestBuilder_SyntheticPos pins the fixture's synthetic source
// position. Layout composes an output filename as
// `<origin-basename><plugin-suffix>`, so a positionless origin
// routes to the bare suffix — `_repo.go`, which go/packages drops
// before the toolchain ever sees it. The fixture therefore seeds a
// per-declaration position rather than leaving nodes at the zero
// value.
func TestBuilder_SyntheticPos(t *testing.T) {
	t.Parallel()

	t.Run("a fixture node with no explicit position still routes to a named file", func(t *testing.T) {
		t.Parallel()
		for kind, pos := range declaredPositions(fixturePackage()) {
			base := routedBasename(pos.File)
			if base == "" {
				t.Errorf("%s: Pos.File %q yields an empty routed basename", kind, pos.File)
				continue
			}
			if routed := base + "_repo.go"; strings.HasPrefix(routed, "_") {
				t.Errorf("%s: routes to %q, which the Go toolchain ignores", kind, routed)
			}
		}
	})

	t.Run("the synthetic file names the package and the declaration", func(t *testing.T) {
		t.Parallel()
		want := map[string]string{
			"struct":    "test/user.go",
			"field":     "test/user.go",
			"method":    "test/user.go",
			"interface": "test/store.go",
			"function":  "test/run.go",
			"variable":  "test/default.go",
			"constant":  "test/max.go",
			"enum":      "test/colour.go",
			"alias":     "test/key.go",
		}
		for kind, pos := range declaredPositions(fixturePackage()) {
			if pos.File != want[kind] {
				t.Errorf("%s: Pos.File = %q, want %q", kind, pos.File, want[kind])
			}
		}
	})

	t.Run("nested declarations share their enclosing declaration's file", func(t *testing.T) {
		t.Parallel()
		pkg := fixturePackage()
		if got, want := pkg.Interfaces[0].Methods[0].SourcePos.File, "test/store.go"; got != want {
			t.Fatalf("interface method Pos.File = %q, want %q", got, want)
		}
	})

	t.Run("the configured package name drives the synthetic directory", func(t *testing.T) {
		t.Parallel()
		pkg := storefixture.New().
			Package("users", "example.com/users").
			Struct("User", nil).
			PackageNode()
		if got, want := pkg.Structs[0].SourcePos.File, "users/user.go"; got != want {
			t.Fatalf("Pos.File = %q, want %q", got, want)
		}
	})

	t.Run("an explicit Pos call wins over the synthetic default", func(t *testing.T) {
		t.Parallel()
		pos := position.At("hand/written.go", 3, 1)
		pkg := storefixture.New().
			Struct("User", func(s *storefixture.StructBuilder) {
				s.Pos(pos)
				s.Field("ID", storefixture.Named("string"), func(f *storefixture.FieldBuilder) {
					f.Pos(pos)
				})
				s.Method("Save", func(m *storefixture.MethodBuilder) { m.Pos(pos) })
			}).
			Interface("Store", func(i *storefixture.InterfaceBuilder) { i.Pos(pos) }).
			Function("Run", func(f *storefixture.FunctionBuilder) { f.Pos(pos) }).
			Variable("Default", func(v *storefixture.VariableBuilder) { v.Pos(pos) }).
			Constant("Max", func(c *storefixture.ConstantBuilder) { c.Pos(pos) }).
			Enum("Colour", func(e *storefixture.EnumBuilder) { e.Pos(pos) }).
			Alias("Key", func(a *storefixture.AliasBuilder) { a.Pos(pos) }).
			PackageNode()
		for kind, got := range declaredPositions(pkg) {
			if !got.Equal(pos) {
				t.Errorf("%s: explicit Pos overwritten: got %v, want %v", kind, got, pos)
			}
		}
	})
}

func TestBuilder_PackageNode(t *testing.T) {
	t.Parallel()

	t.Run("returns the same aliased package across calls", func(t *testing.T) {
		t.Parallel()
		b := storefixture.New()
		first := b.PackageNode()
		second := b.PackageNode()
		if first != second {
			t.Fatalf("PackageNode must alias the builder's internal pointer")
		}
	})
}

// TestBuilder_PackageName covers the case Go allows and
// [storefixture.Builder.Package] cannot express.
func TestBuilder_PackageName(t *testing.T) {
	t.Parallel()

	fixture := func() *node.Package {
		return storefixture.New().
			Package("v2", "example.com/api/v2").
			Struct("User", nil).
			PackageName("api").
			PackageNode()
	}

	t.Run("sets the declared name", func(t *testing.T) {
		t.Parallel()
		if got := fixture().Name; got != "api" {
			t.Fatalf("Name = %q, want api", got)
		}
	})

	t.Run("leaves the import path alone", func(t *testing.T) {
		t.Parallel()
		// The disagreement under test: `example.com/api/v2` declaring
		// `package api` is legal and common, and a generator composing
		// an alias from one while a file references the other is the
		// bug a fixture wants to reproduce.
		if got := fixture().Path; got != "example.com/api/v2" {
			t.Fatalf("Path = %q, want the path untouched", got)
		}
	})

	t.Run("does not retarget a declaration's source file", func(t *testing.T) {
		t.Parallel()
		// Package retargets, which is right when the fixture is being
		// renamed and wrong here: it would move the declarations into
		// a directory named after the package clause.
		if got := fixture().Structs[0].Pos().File; got != "v2/user.go" {
			t.Fatalf("declaration file = %q, want it left in v2/", got)
		}
	})

	t.Run("Package still retargets, so the two stay distinguishable", func(t *testing.T) {
		t.Parallel()
		pkg := storefixture.New().
			Package("v2", "example.com/api/v2").
			Struct("User", nil).
			Package("api", "example.com/api").
			PackageNode()
		if got := pkg.Structs[0].Pos().File; got != "api/user.go" {
			t.Fatalf("Package left the declaration at %q; it should retarget", got)
		}
	})
}
