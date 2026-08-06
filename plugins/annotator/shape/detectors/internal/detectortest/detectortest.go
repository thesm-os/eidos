// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package detectortest provides the shared test scaffolding every
// detector sub-package uses. Each sub-package's tests focus on
// the signature acceptance / rejection table while delegating
// store wiring, plugin construction, and stamp assertions to
// this package.
//
// Internal — importable only by [shape/detectors/...] children;
// not part of the shape library's public API.
package detectortest

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// frontendMarker mirrors the umbrella plugin's package-level
// frontend lookup so fixtures can stamp the marker on the test
// package's meta bag.
//
//nolint:gochecknoglobals // test-side singleton mirroring plugin's lookup
var frontendMarker = meta.EnsureKey("frontend", meta.StringParser)

// RunFn wires fn into a single-function "x" package, stamps the
// "golang" frontend marker, runs the umbrella shape plugin
// configured with det, and returns fn's meta bag for assertion.
func RunFn(t *testing.T, det shape.Detector, fn *node.Function) *meta.Bag {
	t.Helper()
	pkg := &node.Package{
		Name: "x", Path: "x",
		Functions: []*node.Function{fn},
	}
	runUmbrella(t, det, pkg)
	return fn.EnsureMeta()
}

// RunMethod wires s into a single-struct "x" package, runs the
// plugin, and returns m's meta bag (m must be one of s.Methods).
func RunMethod(t *testing.T, det shape.Detector, s *node.Struct, m *node.Method) *meta.Bag {
	t.Helper()
	pkg := &node.Package{
		Name: "x", Path: "x",
		Structs: []*node.Struct{s},
	}
	runUmbrella(t, det, pkg)
	return m.EnsureMeta()
}

// RunInterfaceMethod wires i into a single-interface "x" package,
// runs the plugin, and returns m's meta bag.
func RunInterfaceMethod(t *testing.T, det shape.Detector, i *node.Interface, m *node.Method) *meta.Bag {
	t.Helper()
	pkg := &node.Package{
		Name: "x", Path: "x",
		Interfaces: []*node.Interface{i},
	}
	runUmbrella(t, det, pkg)
	return m.EnsureMeta()
}

// AssertShape fails when bag's shape / key_type / value_type
// stamps do not equal want. Empty wantKey / wantValue mean
// "must be absent".
func AssertShape(t *testing.T, bag *meta.Bag, wantShape, wantKey, wantValue string) {
	t.Helper()
	if got := shape.Get(bag); got != wantShape {
		t.Fatalf("shape = %q, want %q", got, wantShape)
	}
	assertOptional(t, bag, shape.MetaKeyType, "shape.key_type", wantKey)
	assertOptional(t, bag, shape.MetaValueType, "shape.value_type", wantValue)
}

// AssertUnstamped fails when bag carries any structural-shape
// stamp. Used by every detector's negative-table rejections to
// pin the "this signature does NOT match" contract.
func AssertUnstamped(t *testing.T, bag *meta.Bag) {
	t.Helper()
	if shape.IsStamped(bag) {
		t.Fatalf("expected no shape stamp; got shape=%q", shape.Get(bag))
	}
}

// Ctx returns a [node.TypeRef] for `context.Context` — the
// canonical leading parameter type detectors strip via
// [shape.GoStripContext].
func Ctx() *node.TypeRef {
	return &node.TypeRef{Name: "Context", Package: "context"}
}

// Err returns a [node.TypeRef] for the bare builtin `error` —
// the canonical trailing return type detectors strip via
// [shape.GoStripError].
func Err() *node.TypeRef { return &node.TypeRef{Name: "error"} }

// Named returns a [node.TypeRef] for a named type without a
// package qualifier — used for builtin scalars (`string`, `int`,
// `bool`) in test signatures.
func Named(name string) *node.TypeRef { return &node.TypeRef{Name: name} }

// Qualified returns a [node.TypeRef] for a named type with a
// package qualifier — used for user-defined types
// (`x.Article`, `x.Meta`) in test signatures.
func Qualified(pkg, name string) *node.TypeRef {
	return &node.TypeRef{Name: name, Package: pkg}
}

// Slice returns a [node.TypeRef] for a `[]elem` type.
func Slice(elem *node.TypeRef) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: elem}
}

// Pointer returns a [node.TypeRef] for a `*elem` type.
func Pointer(elem *node.TypeRef) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefPointer, Elem: elem}
}

// IterSeq returns a [node.TypeRef] for `iter.Seq[v]`.
func IterSeq(v *node.TypeRef) *node.TypeRef {
	return &node.TypeRef{
		Name: "Seq", Package: "iter",
		TypeArgs: []*node.TypeRef{v},
	}
}

// IterSeq2 returns a [node.TypeRef] for `iter.Seq2[k, v]`.
func IterSeq2(k, v *node.TypeRef) *node.TypeRef {
	return &node.TypeRef{
		Name: "Seq2", Package: "iter",
		TypeArgs: []*node.TypeRef{k, v},
	}
}

// Param builds a [*node.Param] with the supplied name and type.
func Param(name string, t *node.TypeRef) *node.Param {
	return &node.Param{Name: name, Type: t}
}

// Variadic builds a [*node.Param] with the variadic flag set.
// Used for callable signatures with trailing `...T` parameters.
func Variadic(name string, t *node.TypeRef) *node.Param {
	return &node.Param{Name: name, Type: t, Variadic: true}
}

// runUmbrella adds pkg to a fresh store, stamps the "golang"
// frontend marker, and runs the umbrella shape plugin configured
// with det. Fails the test on any returned error.
func runUmbrella(t *testing.T, det shape.Detector, pkg *node.Package) {
	t.Helper()
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")

	p := shape.New().Detectors(det)
	ctx := &sdk.AnnotatorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   diag.New(),
	}
	if err := p.Annotate(ctx); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
}

// assertOptional fails when key on bag does not match want.
// Empty want means "must be absent"; non-empty means "must equal".
func assertOptional(t *testing.T, bag *meta.Bag, key meta.Key[string], label, want string) {
	t.Helper()
	got, ok := key.Get(bag)
	if want == "" {
		if ok {
			t.Fatalf("%s unexpectedly stamped: %q", label, got)
		}
		return
	}
	if !ok {
		t.Fatalf("%s unstamped; want %q", label, want)
	}
	if got != want {
		t.Fatalf("%s = %q, want %q", label, got, want)
	}
}
