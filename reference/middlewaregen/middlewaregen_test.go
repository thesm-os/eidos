// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package middlewaregen_test

import (
	"fmt"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/eidostest/golangtest"
	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	backendgolang "go.thesmos.sh/eidos/lang/golang/backend"
	"go.thesmos.sh/eidos/reference/authgen"
	"go.thesmos.sh/eidos/reference/metricgen"
	"go.thesmos.sh/eidos/reference/middlewaregen"
	"go.thesmos.sh/eidos/reference/tracegen"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// TestConformance runs the framework conformance suites against
// the middlewaregen plugin: [plugintest.RunSuite] for the stability, role and
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
		plugintest.RunSuite(t, middlewaregen.New())
	})

	t.Run("generator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunGeneratorSuite(t, middlewaregen.New(), []plugintest.GeneratorFixture{
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
					return storefixture.New().Struct("Plain", nil).Build()
				},
			},
		})
	})
}

// handlerPkg builds a package of structs each carrying `+gen:handler`.
//
// Every struct is positioned in one source file on purpose. Layout
// composes <source-basename><suffix>, so a fixture without a position
// yields a file named for the suffix alone, and one position per
// struct would scatter the chains across files the assertions would
// then have to address separately.
func handlerPkg(t *testing.T, names ...string) *sdk.Package {
	t.Helper()
	fixture := storefixture.New()
	for i, name := range names {
		fixture.Struct(name, func(s *storefixture.StructBuilder) {
			s.Pos(sdk.Pos{File: "users.go", Line: i + 1})
			s.Directive(storefixture.Directive(middlewaregen.DirectiveName))
		})
	}
	return fixture.PackageNode()
}

// runPipeline renders one handler package through the given generators
// and the Go backend.
func runPipeline(t *testing.T, pkg *sdk.Package, gens ...sdk.Generator) *pipelinetest.Pipeline {
	t.Helper()
	// Unclaimed directives are permitted because this run is
	// deliberately narrower than its fixture. `handler` is handlergen's
	// directive — this plugin reads it and declares nothing — so a run
	// registering the contributor without its host has a name nothing
	// claims, which is the arrangement several of these subtests exist
	// to exercise.
	return golangtest.Driver(t, backendgolang.New(), pkg, gens...).
		WithUnclaimedDirectives().
		Build().
		Run("./...")
}

// middlewareModule is the module the generated chain is compiled
// inside.
//
// Overridden rather than derived from the run: the contributors name
// packages under example.com/httpmw, and a hand-written package can
// only be handed to the build as a directory of the module under test.
// Declaring that module path is what makes the generated imports
// resolve the way a consumer's will rather than by accident.
const middlewareModule = "example.com/httpmw"

// middlewarePkgs is the hand-written middleware the chain references.
//
// The signatures are the assertion, not scaffolding around it. The
// generated variable is typed `[]func(http.Handler) http.Handler`, so
// a contributor whose constructor takes or returns anything else fails
// the build here — where a substring check spelling its name passes
// regardless.
//
// The import paths are restated from the three contributors, which
// spell them as literals rather than exporting a constant. That
// coupling is the reason this list has to be kept in step by hand.
func middlewarePkgs() []golangtest.File {
	pkg := func(name, fn string) golangtest.File {
		return golangtest.File{
			Path: name + "/" + name + ".go",
			Src: fmt.Appendf(nil,
				"package %s\n\nimport \"net/http\"\n\n"+
					"// %s is a middleware constructor of the shape the chain declares.\n"+
					"func %s(next http.Handler) http.Handler { return next }\n",
				name, fn, fn),
		}
	}
	return []golangtest.File{
		pkg("auth", "RequireAuth"),
		pkg("metrics", "RecordLatency"),
		pkg("trace", "StartSpan"),
	}
}

// TestRenderedGo_TheChainIsValidGoAgainstRealMiddleware compiles the
// emitted chain against hand-written middleware packages.
//
// The assertion every other one in this file is a proxy for. The
// template renders a slice of function values whose element type comes
// from one plugin and whose entries come from three others; nothing
// short of a compiler can say those agree. A run that emitted the
// wrong element type, a stray comma from the slot's range, or an entry
// referencing a package it never imported passes every substring check
// this suite used to make.
//
// # Cost
//
// Two toolchain invocations, some seconds, sharing one built module;
// every fine-grained claim below is structural and free.
func TestRenderedGo_TheChainIsValidGoAgainstRealMiddleware(t *testing.T) {
	t.Parallel()

	gen := golangtest.Rendered(t, runPipeline(t,
		handlerPkg(t, "Users", "Orders"),
		middlewaregen.New(), authgen.New(), metricgen.New(), tracegen.New(),
	)).
		WithModulePath(middlewareModule).
		WithSource(middlewarePkgs()...)

	// Parsed once, up front: a Source is read-only, so every subtest
	// below can share it rather than re-parsing the same bytes.
	src := gen.Primary(t)

	t.Run("builds and vets as a consumer would build it", func(t *testing.T) {
		t.Parallel()
		gen.AssertCompiles(t)
		gen.AssertVets(t)
	})

	t.Run("is the machine-written, gofmt-canonical file tooling expects", func(t *testing.T) {
		t.Parallel()
		src.AssertPackage(t, "test")
		src.AssertGeneratedHeader(t)
		src.AssertFormatted(t)
		src.AssertDocumented(t)
	})

	t.Run("declares one chain per annotated struct, in source order", func(t *testing.T) {
		t.Parallel()
		src.AssertOrder(t, "UsersMiddleware", "OrdersMiddleware")
	})

	// The import set is the generator's API: every path here is one a
	// consumer's module has to require, and a contributor that starts
	// naming a fourth package raises that bar for everyone. Pinning the
	// whole set turns that into a one-line diff — and it is also the
	// structural proof that all three contributions rendered, since an
	// entry that never reached the template registers no import.
	t.Run("forces exactly the imports the chain needs", func(t *testing.T) {
		t.Parallel()
		src.AssertImportsOnly(t,
			"net/http",
			middlewareModule+"/auth",
			middlewareModule+"/metrics",
			middlewareModule+"/trace",
		)
	})
}

// TestRenderedGo_AnUnfilledChainStillCompiles renders the host alone.
//
// The degenerate case the composition tests never reach: with no
// contributor registered the slot's range emits nothing, and a
// template that left a separator behind would produce a file that
// still contains every substring anyone asserts on. Cheap to hold
// because it needs no support package — the empty chain names only
// net/http.
func TestRenderedGo_AnUnfilledChainStillCompiles(t *testing.T) {
	t.Parallel()

	gen := golangtest.Rendered(t, runPipeline(t,
		handlerPkg(t, "Users"), middlewaregen.New(),
	))
	gen.AssertCompiles(t)
	gen.Primary(t).AssertImportsOnly(t, "net/http")
}

// TestComposition_ContributorsFillTheHostsChain pins the whole
// composition pattern end to end: a host plugin declares an emit kind
// carrying a named slot, two unrelated plugins append into it, and the
// host's template renders all three contributions into one file.
func TestComposition_ContributorsFillTheHostsChain(t *testing.T) {
	t.Parallel()

	t.Run("both contributors render inside the host's file", func(t *testing.T) {
		t.Parallel()
		p := runPipeline(t, handlerPkg(t, "Users"),
			middlewaregen.New(), authgen.New(), metricgen.New(), tracegen.New())

		src := golangtest.Rendered(t, p).Suffixed(t, middlewaregen.GoSuffix)
		assertChainOrder(t, src,
			"auth.RequireAuth", "metrics.RecordLatency", "trace.StartSpan")
	})

	// Ordering inside a slot is the pipeline's capability topology,
	// not append order. metricgen names authgen's capability in
	// Requires, so it must render second however the two happened to
	// be registered — which is why this subtest registers them in the
	// opposite order to the one it asserts.
	t.Run("Requires orders the chain, registration order does not", func(t *testing.T) {
		t.Parallel()
		p := runPipeline(t, handlerPkg(t, "Users"),
			middlewaregen.New(), tracegen.New(), metricgen.New(), authgen.New())

		// Registered trace, metric, auth — asserted auth, metric,
		// trace. The chain of Requires reverses the registration
		// order completely, which is the point.
		assertChainOrder(t, golangtest.Rendered(t, p).Suffixed(t, middlewaregen.GoSuffix),
			"auth.RequireAuth", "metrics.RecordLatency", "trace.StartSpan")
	})

	// The dependency is structural, not configured. A contributor
	// appends to a value the host created, so a run without the host
	// finds nothing to append to. Nothing is emitted and — the part
	// worth pinning — no file is produced either, which is the failure
	// a suffix-sharing contributor would have: Layout's FileFor is
	// lookup-or-create, so routing by origin into an absent host's
	// suffix conjures an orphan file containing only the contributor.
	t.Run("contributors emit nothing when the host plugin is absent", func(t *testing.T) {
		t.Parallel()
		p := runPipeline(t, handlerPkg(t, "Users"),
			authgen.New(), metricgen.New(), tracegen.New())

		p.AssertFileCount(0)
	})
}

// assertChainOrder fails unless every entry appears in the file, in
// the order given.
//
// Offsets into the file rather than a structural query, because the
// entries render inside one composite literal and golangtest scopes
// only to a declaration — [golangtest.Source.InFunc] has no counterpart
// for a var's initialiser. The check is still worth more than the
// substring loop it replaces: it runs against a file that
// [golangtest.Parse] accepted and that
// TestRenderedGo_TheChainIsValidGoAgainstRealMiddleware compiled, and it
// names which pair is inverted rather than reporting a missing string.
func assertChainOrder(t *testing.T, src *golangtest.Source, entries ...string) {
	t.Helper()
	body := string(src.Bytes())
	at := make([]int, len(entries))
	for i, e := range entries {
		at[i] = strings.Index(body, e)
		if at[i] < 0 {
			t.Fatalf("%s: chain is missing the entry %q; the whole chain must render\n%s",
				src.Path(), e, body)
		}
	}
	for i := 1; i < len(entries); i++ {
		if at[i-1] >= at[i] {
			t.Errorf("%s: chain renders %q at %d, at or after %q at %d; the order is the "+
				"pipeline's resolved Requires topology, not registration order\n%s",
				src.Path(), entries[i-1], at[i-1], entries[i], at[i], body)
		}
	}
}
