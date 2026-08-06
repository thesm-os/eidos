// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
	"go.thesmos.sh/eidos/node"
)

// TestContext_Anchor covers the declarative-anchor entry point. The
// Anchor accepts a source node, derives the [emit.Package.Path]
// from it, and stamps the node as the default [emit.BaseEmit.OriginNode]
// on every decl built through the resulting PackageBuilder.
func TestContext_Anchor(t *testing.T) {
	t.Parallel()

	t.Run("Path derives from the anchor node's package", func(t *testing.T) {
		t.Parallel()
		iface := &node.Interface{Name: "Store", Package: "example.com/x/store"}
		pkg := builder.For("mg").Anchor(iface).Node()
		if got, want := pkg.Path, "example.com/x/store"; got != want {
			t.Fatalf("Package.Path = %q, want %q", got, want)
		}
		if got := pkg.Name; got != "" {
			t.Fatalf("Package.Name = %q, want empty (framework fills)", got)
		}
	})

	t.Run("default Origin stamps on decls built via the PackageBuilder", func(t *testing.T) {
		t.Parallel()
		iface := &node.Interface{Name: "Store", Package: "example.com/x/store"}
		var s *emit.Struct
		builder.For("mg").Anchor(iface).Struct("StoreMock", func(sb *builder.StructBuilder) {
			s = sb.Node()
		})
		if s.Origin() != iface {
			t.Fatalf("default Origin = %v, want %v", s.Origin(), iface)
		}
	})

	t.Run("per-decl Origin override wins over the anchor default", func(t *testing.T) {
		t.Parallel()
		anchor := &node.Interface{Name: "A", Package: "x"}
		override := &node.Struct{Name: "Other"}
		var s *emit.Struct
		builder.For("mg").Anchor(anchor).Struct("SMock", func(sb *builder.StructBuilder) {
			sb.Origin(override)
			s = sb.Node()
		})
		if s.Origin() != override {
			t.Fatalf("override Origin = %v, want %v", s.Origin(), override)
		}
	})
}

// TestContext_Anchor_PackageDerivation pins the source-package
// derivation across every node kind the anchor accepts. The kinds
// split three ways: those carrying a package path directly, those
// that walk an owner chain to reach one, and those whose routing
// semantics are undefined and therefore anchor to the empty path.
//
// The empty-path cases matter as much as the resolving ones — an
// unroutable anchor has to produce an empty path the pipeline can
// report as a routing error, not a wrong path it would silently
// honour.
func TestContext_Anchor_PackageDerivation(t *testing.T) {
	t.Parallel()

	const pkgPath = "example.com/x/store"
	host := &node.Struct{Name: "Store", Package: pkgPath}
	hostEnum := &node.Enum{Name: "Status", Package: pkgPath}

	t.Run("derives directly from a kind carrying a package", func(t *testing.T) {
		t.Parallel()
		direct := []struct {
			name string
			n    node.Node
		}{
			{"Package", &node.Package{Name: "store", Path: pkgPath}},
			{"Struct", host},
			{"Interface", &node.Interface{Name: "Repo", Package: pkgPath}},
			{"Function", &node.Function{Name: "Open", Package: pkgPath}},
			{"Variable", &node.Variable{Name: "Default", Package: pkgPath}},
			{"Constant", &node.Constant{Name: "Limit", Package: pkgPath}},
			{"Enum", hostEnum},
			{"Alias", &node.Alias{Name: "ID", Package: pkgPath}},
		}
		for _, tc := range direct {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				if got := builder.For("mg").Anchor(tc.n).Node().Path; got != pkgPath {
					t.Fatalf("Path = %q, want %q", got, pkgPath)
				}
			})
		}
	})

	t.Run("walks the owner chain for a kind that carries no package", func(t *testing.T) {
		t.Parallel()
		owned := []struct {
			name string
			n    node.Node
		}{
			{"Method", &node.Method{Name: "Get", Owner: host}},
			{"Field", &node.Field{Name: "ID", Owner: host}},
			{"File", &node.File{Name: "store.go", Owner: &node.Package{Name: "store", Path: pkgPath}}},
			{"EnumVariant", &node.EnumVariant{Name: "Active", Owner: hostEnum}},
		}
		for _, tc := range owned {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				if got := builder.For("mg").Anchor(tc.n).Node().Path; got != pkgPath {
					t.Fatalf("Path = %q, want %q", got, pkgPath)
				}
			})
		}
	})

	t.Run("walks a multi-hop owner chain", func(t *testing.T) {
		t.Parallel()
		// A field on a method's owner reaches the package in two
		// hops; the walk is a loop rather than a single lookup for
		// exactly this shape.
		f := &node.Field{Name: "ID", Owner: &node.Method{Name: "Get", Owner: host}}
		if got := builder.For("mg").Anchor(f).Node().Path; got != pkgPath {
			t.Fatalf("Path = %q, want %q", got, pkgPath)
		}
	})

	t.Run("anchors to the empty path when the chain reaches no package", func(t *testing.T) {
		t.Parallel()
		unrooted := []struct {
			name string
			n    node.Node
		}{
			{"orphan Method", &node.Method{Name: "Get"}},
			{"orphan Field", &node.Field{Name: "ID"}},
			{"orphan File", &node.File{Name: "store.go"}},
			{"orphan EnumVariant", &node.EnumVariant{Name: "Active"}},
			{"undefined kind", &node.Param{Name: "ctx"}},
		}
		for _, tc := range unrooted {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				if got := builder.For("mg").Anchor(tc.n).Node().Path; got != "" {
					t.Fatalf("Path = %q, want empty", got)
				}
			})
		}
	})

	t.Run("anchors to the empty path for a nil node", func(t *testing.T) {
		t.Parallel()
		if got := builder.For("mg").Anchor(nil).Node().Path; got != "" {
			t.Fatalf("Path = %q, want empty", got)
		}
	})
}
