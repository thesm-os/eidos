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
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// frontendMarker mirrors the umbrella plugin's package-level
// frontend lookup so fixtures can stamp the marker on the test
// package's meta bag.
//
//nolint:gochecknoglobals // test-side singleton mirroring plugin's lookup
var frontendMarker = sdk.EnsureKey("frontend", sdk.StringParser)

// RunFn wires fn into a single-function "x" package, stamps the
// "golang" frontend marker, runs the umbrella shape plugin
// configured with det, and returns fn's meta bag for assertion.
func RunFn(t *testing.T, det shape.Detector, fn *sdk.Function) *sdk.Bag {
	t.Helper()
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{fn},
	}
	runUmbrella(t, det, pkg)
	return fn.EnsureMeta()
}

// RunMethod wires s into a single-struct "x" package, runs the
// plugin, and returns m's meta bag (m must be one of s.Methods).
func RunMethod(t *testing.T, det shape.Detector, s *sdk.Struct, m *sdk.Method) *sdk.Bag {
	t.Helper()
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Structs: []*sdk.Struct{s},
	}
	runUmbrella(t, det, pkg)
	return m.EnsureMeta()
}

// RunInterfaceMethod wires i into a single-interface "x" package,
// runs the plugin, and returns m's meta bag.
func RunInterfaceMethod(t *testing.T, det shape.Detector, i *sdk.Interface, m *sdk.Method) *sdk.Bag {
	t.Helper()
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Interfaces: []*sdk.Interface{i},
	}
	runUmbrella(t, det, pkg)
	return m.EnsureMeta()
}

// AssertShape fails when bag's shape / key_type / value_type
// stamps do not equal want. Empty wantKey / wantValue mean
// "must be absent".
func AssertShape(t *testing.T, bag *sdk.Bag, wantShape, wantKey, wantValue string) {
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
func AssertUnstamped(t *testing.T, bag *sdk.Bag) {
	t.Helper()
	if shape.IsStamped(bag) {
		t.Fatalf("expected no shape stamp; got shape=%q", shape.Get(bag))
	}
}

// Ctx returns a [sdk.TypeRef] for `context.Context` — the
// canonical leading parameter type detectors strip via
// [shape.GoStripContext].
func Ctx() *sdk.TypeRef {
	return &sdk.TypeRef{Name: "Context", Package: "context"}
}

// Err returns a [sdk.TypeRef] for the bare builtin `error` —
// the canonical trailing return type detectors strip via
// [shape.GoStripError].
func Err() *sdk.TypeRef { return &sdk.TypeRef{Name: "error"} }

// Named returns a [sdk.TypeRef] for a named type without a
// package qualifier — used for builtin scalars (`string`, `int`,
// `bool`) in test signatures.
func Named(name string) *sdk.TypeRef { return &sdk.TypeRef{Name: name} }

// Qualified returns a [sdk.TypeRef] for a named type with a
// package qualifier — used for user-defined types
// (`x.Article`, `x.Meta`) in test signatures.
func Qualified(pkg, name string) *sdk.TypeRef {
	return &sdk.TypeRef{Name: name, Package: pkg}
}

// Slice returns a [sdk.TypeRef] for a `[]elem` type.
func Slice(elem *sdk.TypeRef) *sdk.TypeRef {
	return &sdk.TypeRef{TypeKind: sdk.TypeRefSlice, Elem: elem}
}

// Pointer returns a [sdk.TypeRef] for a `*elem` type.
func Pointer(elem *sdk.TypeRef) *sdk.TypeRef {
	return &sdk.TypeRef{TypeKind: sdk.TypeRefPointer, Elem: elem}
}

// IterSeq returns a [sdk.TypeRef] for `iter.Seq[v]`.
func IterSeq(v *sdk.TypeRef) *sdk.TypeRef {
	return &sdk.TypeRef{
		Name: "Seq", Package: "iter",
		TypeArgs: []*sdk.TypeRef{v},
	}
}

// IterSeq2 returns a [sdk.TypeRef] for `iter.Seq2[k, v]`.
func IterSeq2(k, v *sdk.TypeRef) *sdk.TypeRef {
	return &sdk.TypeRef{
		Name: "Seq2", Package: "iter",
		TypeArgs: []*sdk.TypeRef{k, v},
	}
}

// Param builds a [*sdk.Param] with the supplied name and type.
func Param(name string, t *sdk.TypeRef) *sdk.Param {
	return &sdk.Param{Name: name, Type: t}
}

// Variadic builds a [*sdk.Param] with the variadic flag set.
// Used for callable signatures with trailing `...T` parameters.
func Variadic(name string, t *sdk.TypeRef) *sdk.Param {
	return &sdk.Param{Name: name, Type: t, Variadic: true}
}

// runUmbrella adds pkg to a fresh store, stamps the "golang"
// frontend marker, and runs the umbrella shape plugin configured
// with det. Fails the test on any returned error.
func runUmbrella(t *testing.T, det shape.Detector, pkg *sdk.Package) {
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
func assertOptional(t *testing.T, bag *sdk.Bag, key sdk.Key[string], label, want string) {
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
