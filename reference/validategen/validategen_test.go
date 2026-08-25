// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package validategen_test

import (
	"testing"

	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	backendgolang "go.thesmos.sh/eidos/lang/golang/backend"
	"go.thesmos.sh/eidos/lang/golang/golangtest"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
	"go.thesmos.sh/eidos/reference/handlergen"
	"go.thesmos.sh/eidos/reference/validategen"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// TestConformance runs the framework conformance suites against
// the validategen plugin: [plugintest.RunSuite] for the stability, role and
// capability contracts, and [plugintest.RunGeneratorSuite] for the
// per-fixture generator contracts — determinism across two runs, a
// frozen source graph, truthful NodesOnly, declared output tags, and
// tolerance of a partial output-package dispatch.
//
// The generator suite is the half that reaches behaviour. RunSuite
// alone checks what the plugin declares; only the fixtures exercise
// what Generate does with a store.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, validategen.New())
	})

	t.Run("generator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunGeneratorSuite(t, validategen.New(), []plugintest.GeneratorFixture{
			{
				Name: "empty store",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					return store.New()
				},
			},
			{
				// A generator must decline work it was not asked for
				// without emitting, panicking, or touching the source
				// graph — the path a real run takes for most packages.
				Name: "package with nothing this plugin handles",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					return gofixture.New().Struct("Plain", nil).Build()
				},
			},
		})
	})
}

// subject is the annotated type every fixture here generates from.
const subject = "Orders"

// handlerBuilder builds one struct carrying `+gen:handler`, plus
// whatever routing directives a fixture adds.
//
// The position is load-bearing: Layout composes a generated file's name
// as `<origin-basename><plugin-suffix>`, so `orders.go` is what turns
// this plugin's `_validate.go` suffix into `orders_validate.go`.
func handlerBuilder(extra ...*sdk.Directive) *gofixture.Builder {
	return gofixture.New().
		Struct("Orders", func(s *gofixture.StructBuilder) {
			s.Docs("Orders is the annotated type the ensemble generates from.")
			s.Pos(sdk.Pos{File: "orders.go", Line: 1})
			s.Directive(gofixture.Directive(handlergen.DirectiveName))
			for _, d := range extra {
				s.Directive(d)
			}
		})
}

// render drives handlergen and validategen over pkg through a real
// backend.
//
// handlergen is not optional scenery. validategen keys off the emit
// values handlergen produced, so a run without it produces no
// validator either — see
// TestRender_ContributesNothingWithoutTheHostPlugin.
func render(t *testing.T, pkg *sdk.Package) *pipelinetest.Pipeline {
	t.Helper()
	return golangtest.Driver(t, backendgolang.New(), pkg,
		handlergen.New(), validategen.New()).
		Build().
		Run("./...")
}

// sourcePkg is the package the default fixture's output references:
// the subject projected from the builder that drove the run, plus the
// declarations no fixture describes.
//
// All three are load-bearing rather than scenery. `Orders` is the
// subject the generated validator takes by pointer — projected, so a
// rename in [handlerBuilder] cannot leave a stale copy behind.
// `serve` is the method handlergen's template delegates the request
// to; it has a body and hangs off a generated type, so it stays
// hand-written. `Handler` is there because
// [golangtest.Generated.AssertSatisfies] writes its assertion file
// with no import block, so the interface has to already be nameable
// inside the generated package.
func sourcePkg() []golangtest.File {
	return []golangtest.File{
		golangtest.GoFile(handlerBuilder().GoSource()),
		golangtest.GoFile("support.go", `package test

import "net/http"

// serve is the hand-written body handlergen's ServeHTTP delegates to.
func (h *OrdersHandler) serve(w http.ResponseWriter, r *http.Request) {}

// Handler is a local spelling of http.Handler, for an assertion that
// cannot carry an import of its own.
type Handler = http.Handler
`),
	}
}

// TestRender_EmitsGoTheConsumerCanBuild is the assertion every
// structural one below is a proxy for.
//
// This plugin renders two things into two different files through two
// different mechanisms — a validator it owns, and a call to that
// validator injected into a slot another plugin's template controls —
// and neither half is checked by looking at the other. A substring
// assertion on `func ValidateOrders` passes just as well when the
// injected call names an arity the validator does not have, and a
// substring on the call passes when the validator was never emitted.
// Only a compiler joins them.
func TestRender_EmitsGoTheConsumerCanBuild(t *testing.T) {
	t.Parallel()

	gen := golangtest.Rendered(t, render(t, handlerBuilder().PackageNode())).
		WithSource(sourcePkg()...)

	// Every toolchain assertion shells out to `go`, so both live in one
	// subtest and every other subtest below is structural. The subtests
	// run in parallel and a Generated caches its built module without a
	// lock, which is the second reason not to spread them out.
	t.Run("the whole ensemble compiles and the handler still satisfies its interface", func(t *testing.T) {
		t.Parallel()
		gen.AssertCompiles(t)
		// The prebody entry renders an `if err := …; err != nil { return }`
		// straight into ServeHTTP. A bare `return` is legal there only
		// because the method has no results, so this pins that the
		// injection site and the injected statement still agree.
		gen.AssertSatisfies(t, "OrdersHandler", "Handler")
	})

	t.Run("the validator lands beside its source under this plugin's suffix", func(t *testing.T) {
		t.Parallel()
		gen.Suffixed(t, validategen.GoSuffix).
			AssertPackage(t, "test").
			AssertGeneratedHeader(t).
			AssertFormatted(t).
			AssertDocumented(t)
	})

	t.Run("the validator takes the subject by pointer and returns an error", func(t *testing.T) {
		t.Parallel()
		gen.Suffixed(t, validategen.GoSuffix).
			AssertFunc(t, "Validate"+subject).
			Signature(t, "(v *"+subject+") error")
	})

	// The reference is emitted unqualified on purpose: the validator
	// lands beside its source by default, where a qualified one would be
	// wrong. An import appearing here means the plugin qualified a
	// same-package reference — which compiles only by accident of this
	// module having the path, and is a breaking change for every
	// consumer whose module does not.
	t.Run("the validator imports nothing when it lands beside its source", func(t *testing.T) {
		t.Parallel()
		gen.Suffixed(t, validategen.GoSuffix).AssertImportsOnly(t)
	})

	// The contributed half. Scoped to the method rather than matched
	// against the file, because the file also holds handlergen's doc
	// comment naming the same slot — a whole-file substring cannot tell
	// the rendered call from the prose describing where calls go.
	t.Run("the prebody call renders inside the host's ServeHTTP", func(t *testing.T) {
		t.Parallel()
		gen.Suffixed(t, handlergen.GoSuffix).
			InMethod(t, "OrdersHandler", "ServeHTTP").
			AssertContains(t, "Validate"+subject+"(nil)").
			AssertCount(t, "Validate"+subject, 1)
	})
}

// TestRender_ContributesNothingWithoutTheHostPlugin pins that the
// dependency on handlergen is total, not partial.
//
// Worth pinning because the obvious reading is the other one: this
// plugin declares an [sdk.Output] of its own, so the file it owns looks
// like it should land regardless. It does not. Generate walks the emit
// graph for handlergen's Handler values and emits one validator per
// hit, so no host means no hit, no validator, and no file — not an
// orphan `_validate.go` holding a validator for a handler that was
// never generated.
func TestRender_ContributesNothingWithoutTheHostPlugin(t *testing.T) {
	t.Parallel()

	golangtest.Driver(t, backendgolang.New(), handlerBuilder().PackageNode(), validategen.New()).
		Build().
		Run("./...").
		AssertFileCount(0)
}

// TestRender_RoutedValidatorQualifiesItsSubject covers the path
// `+gen:out` opens: the validator leaves the package its subject lives
// in.
//
// Both outputs are routed, not just this plugin's. Routing the
// validator alone would put the handler's call to it in the package the
// subject is declared in, so the validator would import that package
// and the package would import the validator — a cycle no arrangement
// of qualified references escapes. Moving the handler out too is the
// shape a consumer who routes anything actually ends up with.
func TestRender_RoutedValidatorQualifiesItsSubject(t *testing.T) {
	t.Parallel()

	pkg := handlerBuilder(
		gofixture.RouteTo(validategen.Name, "validation", "validation"),
		gofixture.RouteTo(handlergen.Name, "api", "api"),
	).PackageNode()
	gen := golangtest.Rendered(t, render(t, pkg))

	t.Run("the validator lands in the routed package", func(t *testing.T) {
		t.Parallel()
		gen.Suffixed(t, validategen.GoSuffix).
			AssertPackage(t, "validation").
			AssertGeneratedHeader(t)
	})

	// The half that regressed silently for as long as the plugin
	// existed: the subject's package was looked up through an interface
	// nothing in the node tree implements, so the lookup always missed
	// and the parameter rendered as a bare `*Orders` — a name that is
	// not in scope anywhere but the package the validator just left.
	t.Run("the subject is qualified against the package it stayed in", func(t *testing.T) {
		t.Parallel()
		v := gen.Suffixed(t, validategen.GoSuffix)
		v.AssertImportsOnly(t, "example.com/test")
		v.AssertFunc(t, "Validate"+subject).Signature(t, "(v *test."+subject+") error")
	})

	t.Run("the routed output compiles against the package it left", func(t *testing.T) {
		t.Parallel()
		t.Skip("validategen.Entry.SetOutputPackages is never called: Layout dispatches it " +
			"through emit.Walk, and emit.Walk's walkChildren type-switches on the built-in " +
			"emit kinds, so it does not descend into a plugin-defined SlotHost. The Entry " +
			"lives in handlergen.Handler's prebody slot and is therefore unreachable, " +
			"leaving the routed handler calling a bare ValidateOrders that is no longer in " +
			"its package. Un-skip when emit.Walk can descend into a plugin-defined slot host.")
		golangtest.Rendered(t, render(t, pkg)).
			WithSource(routedSourcePkg()...).
			AssertCompiles(t)
	})
}

// routedSourcePkg is the hand-written source the routed fixture's
// output references, split the way that run splits: the subject stays
// in the package it was declared in, and the handler's hand-written
// body follows the handler into the one it was routed to.
func routedSourcePkg() []golangtest.File {
	return []golangtest.File{
		{
			Path: "orders.go",
			Src: []byte(`package test

// Orders is the annotated type the ensemble generates from.
type Orders struct{}
`),
		},
		{
			Path: "api/serve.go",
			Src: []byte(`package api

import "net/http"

// serve is the hand-written body handlergen's ServeHTTP delegates to.
func (h *OrdersHandler) serve(w http.ResponseWriter, r *http.Request) {}
`),
		},
	}
}
