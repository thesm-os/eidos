// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package mixintest provides shared assertions for the per-mixin
// sub-packages — an [AssertIdentity] sibling of contracttest's
// equivalent, plus [RunPipeline] + [AssertAttached] /
// [AssertParam] for integration tests that exercise the umbrella
// plugin's `+gen:mixin` stamping on a specific catalog mixin.
//
// Internal — importable only by [shape/mixins/...] children; not
// part of the shape library's public API.
package mixintest

import (
	"maps"
	"reflect"
	"slices"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// AssertIdentity fails the test when m does not match the
// expected name + params. Use as the canonical body of every
// per-mixin `TestMixin_Identity` test.
func AssertIdentity(t *testing.T, m shape.Mixin, wantName string, wantParams []shape.Param) {
	t.Helper()
	if m.Name != wantName {
		t.Fatalf("Mixin().Name = %q, want %q", m.Name, wantName)
	}
	if len(wantParams) == 0 {
		if len(m.Params) != 0 {
			t.Fatalf("Mixin().Params = %v, want empty", m.Params)
		}
		return
	}
	if !reflect.DeepEqual(m.Params, wantParams) {
		t.Fatalf("Mixin().Params = %v, want %v", m.Params, wantParams)
	}
	assertNoParamRoles(t, m)
}

// assertNoParamRoles fails when a mixin param carries a
// [shape.Param.Role].
//
// The field is a contract concept: a mixin has no role vocabulary, so
// a scoped mixin param can never match and the key it names would
// silently stop being accepted. Rejected rather than ignored, because
// ignoring it is the exact silence the field was added to remove.
func assertNoParamRoles(t *testing.T, m shape.Mixin) {
	t.Helper()
	for _, p := range m.Params {
		if p.Role == "" {
			continue
		}
		t.Errorf("Mixin().Params[%q].Role = %q; mixins have no roles, so a "+
			"scoped param never applies", p.Key, p.Role)
	}
}

// HostDirective builds a `+gen:mixin <name> [<param>=<value>]...`
// directive for test fixtures.
func HostDirective(mixinName string, params map[string]string) *sdk.Directive {
	d := &sdk.Directive{
		Name: shape.MixinDirectiveName,
		Args: []string{mixinName},
	}
	if len(params) > 0 {
		d.KV = make(map[string]string, len(params))
		maps.Copy(d.KV, params)
	}
	return d
}

// RunPipeline wires a single-function package into a fresh store,
// stamps the "golang" frontend marker, and runs the umbrella
// plugin with m as the sole registered mixin. Returns fn's bag.
func RunPipeline(t *testing.T, m shape.Mixin, fn *sdk.Function) *sdk.Bag {
	t.Helper()
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{fn},
	}
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	sdk.MetaFrontend.Set(pkg.EnsureMeta(), golang.Language, "test")

	p := shape.New().Mixins(m)
	ctx := &sdk.AnnotatorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   diag.New(),
	}
	if err := p.Annotate(ctx); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	return fn.EnsureMeta()
}

// RunWithResolver wires pkg into a fresh store, stamps the
// "golang" frontend marker, and runs the umbrella → resolver
// sequence with m as the sole registered mixin. Use for testing
// sibling-param resolution where the test fixture
// needs more than one callable in scope.
func RunWithResolver(t *testing.T, m shape.Mixin, pkg *sdk.Package) {
	t.Helper()
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	sdk.MetaFrontend.Set(pkg.EnsureMeta(), golang.Language, "test")

	p := shape.New().Mixins(m)
	ctx := &sdk.AnnotatorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   diag.New(),
	}
	if err := p.Annotate(ctx); err != nil {
		t.Fatalf("umbrella.Annotate: %v", err)
	}
	if err := p.Resolver().Annotate(ctx); err != nil {
		t.Fatalf("resolver.Annotate: %v", err)
	}
}

// AssertAttached fails when mixinName is not in the [shape.Mixins]
// list on bag.
func AssertAttached(t *testing.T, bag *sdk.Bag, mixinName string) {
	t.Helper()
	got := shape.Mixins(bag)
	if !slices.Contains(got, mixinName) {
		t.Fatalf("Mixins = %v; want list containing %q", got, mixinName)
	}
}

// AssertParam fails when the param stamp for (mixinName, param)
// on bag does not equal want.
func AssertParam(t *testing.T, bag *sdk.Bag, mixinName, param, want string) {
	t.Helper()
	got, ok := shape.MixinParamKey(mixinName, param).Get(bag)
	if !ok {
		t.Fatalf("param %q for %q unstamped; want %q", param, mixinName, want)
	}
	if got != want {
		t.Fatalf("param %q for %q = %q, want %q", param, mixinName, got, want)
	}
}

// RunWithValidator drives the umbrella, the resolver and the
// validator over pkg and returns every diagnostic raised.
//
// The sibling of [RunWithResolver] for a mixin carrying a
// [shape.Mixin.Validate] hook. Resolution runs first because a
// validator reads params after the resolver has rewritten the ones it
// owns, so a test skipping that step would check the wrong values.
func RunWithValidator(t *testing.T, m shape.Mixin, pkg *sdk.Package) []sdk.Diag {
	t.Helper()
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	sdk.MetaFrontend.Set(pkg.EnsureMeta(), golang.Language, "test")

	umbrella := shape.New().Mixins(m)
	sink := diag.New()
	ctx := &sdk.AnnotatorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   sink,
	}
	if err := umbrella.Annotate(ctx); err != nil {
		t.Fatalf("umbrella.Annotate: %v", err)
	}
	if err := umbrella.Resolver().Annotate(ctx); err != nil {
		t.Fatalf("resolver.Annotate: %v", err)
	}
	if err := umbrella.Validator().Annotate(ctx); err != nil {
		t.Fatalf("validator.Annotate: %v", err)
	}
	return sink.Diagnostics()
}
