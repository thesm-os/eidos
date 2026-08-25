// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
)

// namedRef builds the reference a field or embed carries for pkg.name.
func namedRef(pkg, name string) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefNamed, Package: pkg, Name: name}
}

// TestReader_Resolve pins the store-backed resolution a generated
// double depends on.
//
// That this Reader satisfies the port `lang/golang` declares is
// asserted from that side — see TestResolverPort_Store there. The
// port is theirs, and testing it from here made the root module
// depend on a language adapter that already depends on it.
func TestReader_Resolve(t *testing.T) {
	t.Parallel()

	const pkg = "example.com/x"

	t.Run("resolves each kind a named reference can denote", func(t *testing.T) {
		t.Parallel()
		r := readerOver(t, &node.Package{
			Name: "x", Path: pkg,
			Structs:    []*node.Struct{{Name: "User", Package: pkg}},
			Interfaces: []*node.Interface{{Name: "Store", Package: pkg}},
			Aliases:    []*node.Alias{{Name: "Weekday", Package: pkg}},
			Enums:      []*node.Enum{{Name: "Status", Package: pkg}},
		})
		for _, name := range []string{"User", "Store", "Weekday", "Status"} {
			got, ok := r.Resolve(namedRef(pkg, name))
			if !ok || got == nil {
				t.Errorf("Resolve(%s) = (%v, %v), want the declaration", name, got, ok)
			}
		}
	})

	t.Run("a type this run never loaded reports not found", func(t *testing.T) {
		t.Parallel()
		// The smaller answer rather than a wrong one: a narrow run
		// legitimately has no declaration for a cross-package name, and
		// a caller acts on that by omitting a check rather than writing
		// one against a type it cannot see.
		r := readerOver(t, &node.Package{Name: "x", Path: pkg})
		if got, ok := r.Resolve(namedRef("example.com/other", "Absent")); ok || got != nil {
			t.Errorf("Resolve of an unloaded type = (%v, %v), want (nil, false)", got, ok)
		}
	})

	t.Run("a nameless reference resolves to nothing", func(t *testing.T) {
		t.Parallel()
		// Guarded because the qualified name of an unnamed ref is the
		// empty string, which would otherwise match whatever the
		// buckets happen to hold under it.
		r := readerOver(t, &node.Package{Name: "x", Path: pkg})
		if _, ok := r.Resolve(&node.TypeRef{TypeKind: node.TypeRefSlice}); ok {
			t.Errorf("a slice ref must not resolve to a declaration")
		}
		if _, ok := r.Resolve(nil); ok {
			t.Errorf("a nil ref must not resolve to a declaration")
		}
	})

	t.Run("PackageAt and FileAt answer from the keyed buckets", func(t *testing.T) {
		t.Parallel()
		// Both buckets were already keyed by exactly these values, so
		// the alternative was a linear scan a generator repeats per
		// declaration — quadratic over a graph that has the index.
		r := readerOver(t, &node.Package{
			Name: "x", Path: pkg,
			Files: []*node.File{{Path: "x/user.go"}},
		})
		if got, ok := r.PackageAt(pkg); !ok || got == nil || got.Path != pkg {
			t.Errorf("PackageAt(%q) = (%v, %v), want the package", pkg, got, ok)
		}
		if got, ok := r.FileAt("x/user.go"); !ok || got == nil || got.Path != "x/user.go" {
			t.Errorf("FileAt = (%v, %v), want the file", got, ok)
		}
	})

	t.Run("PackageAt and FileAt report a miss rather than a zero value", func(t *testing.T) {
		t.Parallel()
		r := readerOver(t, &node.Package{Name: "x", Path: pkg})
		if got, ok := r.PackageAt("example.com/absent"); ok || got != nil {
			t.Errorf("PackageAt of an unloaded path = (%v, %v), want (nil, false)", got, ok)
		}
		if got, ok := r.FileAt("absent.go"); ok || got != nil {
			t.Errorf("FileAt of an unloaded path = (%v, %v), want (nil, false)", got, ok)
		}
	})
}
