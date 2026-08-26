// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescripttest

import (
	"testing"

	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// renderPattern is the load pattern [Render] drives the run with.
//
// A [pipelinetest.FromNodes] frontend ignores patterns entirely — the
// test built the nodes, there is nothing to match — so the value is
// arbitrary. It is spelled the way every hand-rolled driver in the
// repo spelled it, so a plugin swapping to Render cannot see a
// difference in what its own frontend was asked for.
const renderPattern = "./..."

// Driver wires a fixture package, its generators and the caller's
// backend onto a pipeline builder, and stops one call short of
// running.
//
// The backend is a parameter, not a default, and that is the point:
// the moment this package *constructs* the TypeScript backend, every
// consumer of typescripttest links it — including a protobuf
// generator's tests, which reach for [Generated.AssertParses] and
// nothing else. One argument at the call site buys that, and the call
// site is the only place that knows it wanted TypeScript.
//
// Reach for Driver over [Render] in two cases: the run needs a
// builder option ([pipelinetest.Builder.WithPluginOptions], a routing
// override, a pinned source root), or the test asserts on diagnostics
// and therefore wants the [pipelinetest.Pipeline] rather than the
// files. Both finish with `.Build().Run("./...")`.
func Driver(
	tb testing.TB,
	backend plugin.Backend,
	pkg *node.Package,
	gens ...plugin.Generator,
) *pipelinetest.Builder {
	tb.Helper()
	return DriverOf(tb, backend, []*node.Package{pkg}, gens...)
}

// DriverOf is [Driver] over more than one fixture package.
//
// Two variadic parameters cannot sit in one signature, so the
// multi-package form takes a slice and the common single-package form
// keeps the ergonomic spelling.
//
// Worth having because a generator whose whole subject is what
// happens *between* modules — a type in one, its guard in another, a
// reference that must import — cannot be exercised by a fixture with
// one package in it.
func DriverOf(
	tb testing.TB,
	backend plugin.Backend,
	pkgs []*node.Package,
	gens ...plugin.Generator,
) *pipelinetest.Builder {
	tb.Helper()
	if len(pkgs) == 0 {
		tb.Fatalf("typescripttest: no fixture packages, so the run would generate nothing " +
			"and every assertion about its output would pass having looked at nothing")
		return nil
	}
	b := pipelinetest.New(tb).WithFrontend(pipelinetest.FromNodes(pkgs...))
	for _, g := range gens {
		// Registered under every role it implements, which is what the
		// CLI does — so a test agrees with a real run by default.
		b = b.WithPlugins(g)
	}
	return b.WithBackend(backend)
}

// Render drives one fixture package end to end and adopts what came
// out — [Driver], Build, Run and [Rendered] in a single call.
//
// The six lines this replaces were not six decisions, they were one,
// and writing them out per generator kept re-offering two mistakes
// that read as generator bugs rather than as harness bugs. Building
// without running leaves an empty sink, so every assertion downstream
// passes having looked at nothing. Registering the backend after
// Build silently registers it nowhere. Neither is visible in a diff
// of the plugin under test.
//
// Configure the result before sharing it — [Generated.WithSource] and
// its siblings — after which its assertions are safe to run from
// parallel subtests, and the project it assembles is built once for
// all of them.
func Render(
	tb testing.TB,
	backend plugin.Backend,
	pkg *node.Package,
	gens ...plugin.Generator,
) *Generated {
	tb.Helper()
	return RenderOf(tb, backend, []*node.Package{pkg}, gens...)
}

// RenderOf is [Render] over more than one fixture package — the
// end-to-end form of [DriverOf].
//
// The cross-module assertions worth making need it: that a generated
// reference to a type in another module renders imported rather than
// bare, and that a generator declining to look outside its own
// package is caught doing so.
func RenderOf(
	tb testing.TB,
	backend plugin.Backend,
	pkgs []*node.Package,
	gens ...plugin.Generator,
) *Generated {
	tb.Helper()
	return Rendered(tb, DriverOf(tb, backend, pkgs, gens...).Build().Run(renderPattern))
}
