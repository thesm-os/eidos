// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"errors"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

func TestQName(t *testing.T) {
	t.Parallel()

	t.Run("qualifies with the full import path", func(t *testing.T) {
		t.Parallel()
		// The form the shape vocabulary records, so a stamp written by
		// one plugin and read by another agrees.
		got := golang.QName(namedTypeRef("example.com/store", "User"))
		if got != "example.com/store.User" {
			t.Fatalf("QName = %q", got)
		}
	})

	t.Run("a builtin needs no qualifier", func(t *testing.T) {
		t.Parallel()
		if got := golang.QName(builtinRef("string")); got != "string" {
			t.Fatalf("QName = %q, want string", got)
		}
	})

	t.Run("nil renders nothing rather than panicking", func(t *testing.T) {
		t.Parallel()
		if got := golang.QName(nil); got != "" {
			t.Fatalf("QName(nil) = %q, want empty", got)
		}
	})
}

func TestDisplay(t *testing.T) {
	t.Parallel()

	t.Run("names the type the way the author wrote it", func(t *testing.T) {
		t.Parallel()
		// A message using the last segment names something the author
		// can search for; the full path names something they wrote
		// once in an import block.
		got := golang.Display(namedTypeRef("example.com/store", "User"))
		if got != "store.User" {
			t.Fatalf("Display = %q, want store.User", got)
		}
	})

	t.Run("differs from QName, which is for stamps", func(t *testing.T) {
		t.Parallel()
		r := namedTypeRef("example.com/store", "User")
		if golang.Display(r) == golang.QName(r) {
			t.Fatalf("Display and QName must not agree on a qualified type")
		}
	})
}

func TestMethodQName(t *testing.T) {
	t.Parallel()

	t.Run("composes the store's canonical key", func(t *testing.T) {
		t.Parallel()
		got := golang.MethodQName("example.com/x.Repo", "Get")
		if got != "example.com/x.Repo.Get" {
			t.Fatalf("MethodQName = %q", got)
		}
	})

	t.Run("an ownerless method is its own key", func(t *testing.T) {
		t.Parallel()
		if got := golang.MethodQName("", "Get"); got != "Get" {
			t.Fatalf("MethodQName = %q, want Get", got)
		}
	})
}

func TestLocalName(t *testing.T) {
	t.Parallel()

	t.Run("takes the trailing identifier", func(t *testing.T) {
		t.Parallel()
		// The resolver rewrites a sibling reference into the qualified
		// form, and that form is exactly what a call cannot use.
		if got := golang.LocalName("example.com/x.Repo.Get"); got != "Get" {
			t.Fatalf("LocalName = %q, want Get", got)
		}
	})

	t.Run("an unqualified name is returned unchanged", func(t *testing.T) {
		t.Parallel()
		// An unresolved reference has already been diagnosed by the
		// run that produced it; failing twice for one cause helps
		// nobody.
		if got := golang.LocalName("Get"); got != "Get" {
			t.Fatalf("LocalName = %q, want Get", got)
		}
	})
}

func TestRefFor(t *testing.T) {
	t.Parallel()

	t.Run("a predeclared name renders bare", func(t *testing.T) {
		t.Parallel()
		if _, ok := golang.RefFor("string", "example.com/x").(*emit.BuiltinRef); !ok {
			t.Fatalf("RefFor(string) must be a builtin")
		}
	})

	t.Run("anything else qualifies against the source package", func(t *testing.T) {
		t.Parallel()
		// What makes a directive argument usable from a generated file
		// routed somewhere else.
		got, ok := golang.RefFor("User", "example.com/x").(*emit.ExternalRef)
		if !ok || got.Package != "example.com/x" || got.Name != "User" {
			t.Fatalf("RefFor = %#v", got)
		}
	})

	t.Run("no source package falls back to bare", func(t *testing.T) {
		t.Parallel()
		// emit.External rejects an empty path, so the two cases cannot
		// share a construction.
		if _, ok := golang.RefFor("User", "").(*emit.BuiltinRef); !ok {
			t.Fatalf("RefFor with no package must render bare")
		}
	})
}

func TestRefForQualified(t *testing.T) {
	t.Parallel()

	t.Run("a bare name resolves against the source package", func(t *testing.T) {
		t.Parallel()
		got, err := golang.RefForQualified("Validate", "example.com/x")
		if err != nil {
			t.Fatalf("RefForQualified: %v", err)
		}
		ext, ok := got.(*emit.ExternalRef)
		if !ok || ext.Package != "example.com/x" {
			t.Fatalf("RefForQualified = %#v", got)
		}
	})

	t.Run("a full path names itself", func(t *testing.T) {
		t.Parallel()
		// An import written only to feed a directive is an unused
		// import, which does not compile — this notation is what makes
		// a cross-package value expressible at all.
		got, err := golang.RefForQualified("example.com/seed.Defaults", "example.com/x")
		if err != nil {
			t.Fatalf("RefForQualified: %v", err)
		}
		ext, _ := got.(*emit.ExternalRef)
		if ext.Package != "example.com/seed" || ext.Name != "Defaults" {
			t.Fatalf("RefForQualified = %#v", ext)
		}
	})

	t.Run("splits on the last dot so a versioned path survives", func(t *testing.T) {
		t.Parallel()
		got, err := golang.RefForQualified("gopkg.in/yaml.v3.Marshal", "example.com/x")
		if err != nil {
			t.Fatalf("RefForQualified: %v", err)
		}
		ext, _ := got.(*emit.ExternalRef)
		if ext.Package != "gopkg.in/yaml.v3" || ext.Name != "Marshal" {
			t.Fatalf("RefForQualified = %#v", ext)
		}
	})

	t.Run("a dangling dot is rejected rather than half-resolved", func(t *testing.T) {
		t.Parallel()
		// A reference to the empty string renders as a bare `.`, which
		// the consumer's compiler reports against generated code.
		for _, raw := range []string{".Validate", "example.com/x.", ""} {
			if _, err := golang.RefForQualified(raw, "p"); !errors.Is(err, golang.ErrBadSymbol) {
				t.Errorf("RefForQualified(%q) err = %v, want ErrBadSymbol", raw, err)
			}
		}
	})
}

func TestRefLists(t *testing.T) {
	t.Parallel()

	t.Run("lifts a type list", func(t *testing.T) {
		t.Parallel()
		got := golang.RefsOf([]*node.TypeRef{builtinRef("int"), builtinRef("string")})
		if len(got) != 2 {
			t.Fatalf("RefsOf = %d, want 2", len(got))
		}
	})

	t.Run("an empty list lifts to nil", func(t *testing.T) {
		t.Parallel()
		// So a caller appending the result emits nothing rather than
		// an empty bracket list.
		if golang.RefsOf(nil) != nil || golang.ParamRefs(nil) != nil || golang.ReturnRefs(nil) != nil {
			t.Fatalf("an empty list must lift to nil")
		}
	})

	t.Run("parameter and return lifting drop the names", func(t *testing.T) {
		t.Parallel()
		// A function type names no parameters, so the identifiers a
		// body would bind are not part of the type.
		p := golang.ParamRefs([]*node.Param{{Name: "id", Type: builtinRef("string")}})
		r := golang.ReturnRefs([]*node.Return{{Name: "err", Type: errorRef()}})
		if len(p) != 1 || len(r) != 1 {
			t.Fatalf("ParamRefs = %v, ReturnRefs = %v", p, r)
		}
	})
}

func TestPkgPathOf(t *testing.T) {
	t.Parallel()

	t.Run("reads the path off a kind that carries one", func(t *testing.T) {
		t.Parallel()
		// The node interface declares no package accessor — a field
		// and a package both satisfy it and only one has a path — so
		// every caller wanting this wrote the assertion inline.
		s := &node.Struct{Name: "User", Package: "example.com/x"}
		if got := golang.PkgPathOf(s); got != "example.com/x" {
			t.Fatalf("PkgPathOf = %q", got)
		}
	})

	t.Run("a kind with no package answers empty", func(t *testing.T) {
		t.Parallel()
		if got := golang.PkgPathOf(&node.Field{Name: "ID"}); got != "" {
			t.Fatalf("PkgPathOf(field) = %q, want empty", got)
		}
	})

	t.Run("SubjectRef qualifies through the origin", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{Name: "User", Package: "example.com/x"}
		got, ok := golang.SubjectRef(s, "User").(*emit.ExternalRef)
		if !ok || got.Package != "example.com/x" {
			t.Fatalf("SubjectRef = %#v", got)
		}
	})

	t.Run("SubjectRef falls back to bare with no package", func(t *testing.T) {
		t.Parallel()
		// Correct for a declaration landing beside its source, where
		// the backend's same-package elision would drop the qualifier
		// anyway.
		if _, ok := golang.SubjectRef(&node.Field{Name: "ID"}, "User").(*emit.BuiltinRef); !ok {
			t.Fatalf("SubjectRef with no package must render bare")
		}
	})
}

func TestPkgPathOfEveryKind(t *testing.T) {
	t.Parallel()

	t.Run("reads the path off every kind that declares one", func(t *testing.T) {
		t.Parallel()
		// The switch is the whole implementation, and an arm left out
		// is silent: the reference stays unqualified and the generated
		// file binds to whatever else is in scope.
		for name, n := range map[string]node.Node{
			"Package":   &node.Package{Path: "example.com/x"},
			"Struct":    &node.Struct{Package: "example.com/x"},
			"Interface": &node.Interface{Package: "example.com/x"},
			"Function":  &node.Function{Package: "example.com/x"},
			"Alias":     &node.Alias{Package: "example.com/x"},
			"Enum":      &node.Enum{Package: "example.com/x"},
			"Variable":  &node.Variable{Package: "example.com/x"},
			"Constant":  &node.Constant{Package: "example.com/x"},
			"TypeRef":   namedTypeRef("example.com/x", "User"),
		} {
			if got := golang.PkgPathOf(n); got != "example.com/x" {
				t.Errorf("PkgPathOf(%s) = %q", name, got)
			}
		}
	})

	t.Run("a method walks up to its owner", func(t *testing.T) {
		t.Parallel()
		// A method carries no package of its own; walking up is what
		// makes one usable as an origin.
		owner := &node.Struct{Name: "Repo", Package: "example.com/x"}
		m := &node.Method{Name: "Get", Owner: owner}
		if got := golang.PkgPathOf(m); got != "example.com/x" {
			t.Fatalf("PkgPathOf(method) = %q", got)
		}
	})

	t.Run("a field walks up to its owner", func(t *testing.T) {
		t.Parallel()
		owner := &node.Struct{Name: "User", Package: "example.com/x"}
		f := &node.Field{Name: "ID", Owner: owner}
		if got := golang.PkgPathOf(f); got != "example.com/x" {
			t.Fatalf("PkgPathOf(field) = %q", got)
		}
	})

	t.Run("an ownerless method answers empty rather than recursing", func(t *testing.T) {
		t.Parallel()
		if got := golang.PkgPathOf(&node.Method{Name: "Get"}); got != "" {
			t.Fatalf("PkgPathOf = %q, want empty", got)
		}
	})

	t.Run("nil answers empty", func(t *testing.T) {
		t.Parallel()
		if got := golang.PkgPathOf(nil); got != "" {
			t.Fatalf("PkgPathOf(nil) = %q", got)
		}
	})
}

func TestDisplayEdges(t *testing.T) {
	t.Parallel()

	t.Run("a builtin displays bare", func(t *testing.T) {
		t.Parallel()
		if got := golang.Display(builtinRef("string")); got != "string" {
			t.Fatalf("Display = %q", got)
		}
	})

	t.Run("nil displays as nothing", func(t *testing.T) {
		t.Parallel()
		if got := golang.Display(nil); got != "" {
			t.Fatalf("Display(nil) = %q", got)
		}
	})
}
