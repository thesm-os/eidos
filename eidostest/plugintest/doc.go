// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package plugintest ships the framework-conformance suite plugin
// authors run against their plugin instances. The suite is split
// into a universal framework check ([RunSuite]) and per-role
// contract checks the plugin author invokes for whichever roles
// the plugin implements ([RunAnnotatorSuite], [RunGeneratorSuite],
// [RunBackendSuite], [RunFrontendSuite]) plus an options
// round-trip suite ([RunOptionsSuite]) for plugins implementing
// [plugin.OptionsProvider].
//
// The framework emits no implicit calls: an author invokes whichever
// suites apply. [RunSuite] therefore logs each role it detects and
// names the per-role suite covering it, so a run that checked
// declarations alone is distinguishable in the output from one that
// checked behaviour. [RunSuiteFor] takes the backend languages to
// probe, for plugins targeting anything other than Go.
//
// # Universal contracts
//
// [RunSuite] is appropriate for every plugin and pins the
// invariants the pipeline relies on at registration / build time:
// stable [plugin.Plugin.Name], at-least-one role-interface
// compliance, deterministic [plugin.CapabilityProvider] ordering,
// unique [plugin.DirectiveProvider] schema names, well-formed
// [plugin.EmitVersioned] entries, stable [plugin.NodesOnly]
// declaration, stable [plugin.FilenameProvider.Outputs] per
// language, and a well-formed Outputs slice (non-empty suffixes,
// unique tags, at-most-one empty-tag output at index 0). Each
// contract is its own subtest so a single regression surfaces
// with the specific contract that broke.
//
// # Per-role contracts
//
// Per-role suites accept a per-role fixture describing the input
// the suite drives the plugin against. The fixtures are the
// surface the plugin author tailors to the plugin's domain —
// e.g. an annotator's [AnnotatorFixture] supplies a store builder
// that lays down whatever source-side decls the annotator's
// stamping logic needs. The suite drives the plugin against the
// fixture twice (or against two equivalent inputs, for the cases
// where re-running on the same state is not meaningful) and
// asserts the two passes produce identical output — the
// determinism contract every pipeline phase relies on for
// reproducible builds, byte-stable goldens, and cache
// composability.
//
// # Diagnostic discipline
//
// Every role suite drives the plugin against a diagnostic sink it
// then reads, and holds three separate properties:
//
//   - No panic, ever. Every probe recovers, so one crashing plugin
//     reports a failure instead of taking the run down.
//   - No arbitrary error return. The generator and annotator suites
//     fail on any non-nil return; the frontend role reserves the
//     return value for failures the suite cannot classify and is
//     exempt by design.
//   - No Error-severity diagnostic on a fixture. A fixture is the
//     author's own declaration that this input is handled, and
//     [pipeline.Pipeline.Run] turns any Error diagnostic into a
//     non-zero exit — so a fixture that produces one describes a
//     plugin nobody can run.
//
// The generator, annotator and frontend suites add a fourth: every
// diagnostic emitted on a fixture carries a source position, because
// a zero one renders as a dash where the file and line belong. It is
// waivable per fixture through AllowsPositionlessDiagnostics, for the
// run- and configuration-level complaints that genuinely name no
// source construct. The backend suite has no counterpart — its
// fixtures are emit graphs assembled in memory, with no source
// construct behind them to point at.
//
// One probe stands outside the diagnostic checks. The generator and
// annotator empty-store probes are inside them — a plugin with
// nothing to do has nothing to complain about — but the frontend's
// empty-pattern probe is not, because rejecting an empty pattern is
// the conforming behaviour rather than a defect.
//
// # Where this sits — the five-rung harness ladder
//
// The eidostest surface is five layers, each proving something the
// one below it cannot. Choosing wrongly is the most common reason a
// conformance run is green over a plugin that does not work.
//
//   - plugintest (this package) — one plugin, in isolation.
//     Declarations, determinism, diagnostic discipline. **Renders
//     nothing**: the emit graph is the last artifact any check here
//     inspects, so a template that parses but cannot execute passes
//     every check in this package.
//   - [backendtest] — one backend over a hand-built emit graph.
//     Rendered bytes, but no generator's templates are merged.
//   - [pipelinetest] — several plugins, a synthetic frontend, a real
//     backend, rendered files. The first rung that proves a generator
//     and a backend agree.
//   - [frontendtest] — a real frontend over real source, with
//     annotators and generators composed behind it.
//   - [acceptancetest] — the built binary, in-tree only. Exit codes,
//     flags, and the only place that proves generated Go compiles.
//
// What this package deliberately does not prove, so it is not
// mistaken for proven: that any template executes, that generated
// output compiles, that two plugins compose without destroying each
// other's output, or that a plugin behaves under a real backend.
// Those belong to the rungs above.
//
// # Running the suite
//
// Run with `-count=2` at minimum. Two of the determinism checks
// compare passes inside one process, and Go randomises map-iteration
// order per range statement, so a defect whose observable effect is
// "first key wins" over a k-entry map is caught with probability
// roughly 1-1/k per invocation — a coin flip at k=2. This repository's
// own gate runs `count: 3`, which is a compensation the suite does not
// otherwise disclose.
//
// Run with `-race` if the plugin holds any state: the pipeline
// dispatches frontends concurrently and generator buckets in
// parallel, and nothing in this package exercises that.
//
// # Typical usage
//
//	func TestMyPlugin(t *testing.T) {
//	    p := myplugin.New()
//	    plugintest.RunSuite(t, p)
//	    plugintest.RunGeneratorSuite(t, p, []plugintest.GeneratorFixture{{
//	        Name: "single struct with +gen:my",
//	        BuildStore: func(t *testing.T) *store.Store {
//	            return storeWithAnnotatedUser(t)
//	        },
//	    }})
//	    plugintest.RunOptionsSuite(t, p, plugintest.OptionsFixture{
//	        Valid: map[string]string{"output_package": "main"},
//	        UnknownKey: "no_such_key",
//	    })
//	}
//
// BuildStore returns a populated [store.Store] and the suite asks
// nothing about how it was built — which is what keeps these checks
// usable by a plugin targeting any language. A Go plugin populates
// one through `lang/golang/golangtest/gofixture`; ExampleRunSuite
// builds one from [node] values directly, so the neutral path is the
// one a reader sees first.
//
// Plugin authors invoke whichever suites apply; the framework
// emits no implicit calls. Subtests scope the suite's checks so
// `go test -run` patterns isolate a single contract.
package plugintest
