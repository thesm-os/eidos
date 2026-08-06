// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package middlewaregen_test

import (
	"strings"
	"testing"

	backendgolang "go.thesmos.sh/eidos/backend/golang"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/reference/authgen"
	"go.thesmos.sh/eidos/reference/metricgen"
	"go.thesmos.sh/eidos/reference/middlewaregen"
	"go.thesmos.sh/eidos/reference/tracegen"
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
				BuildStore: func(t *testing.T) *store.Store {
					t.Helper()
					return store.New()
				},
			},
			{
				// A generator must decline work it was not asked for
				// without emitting, panicking, or touching the source
				// graph — the path a real run takes for most packages.
				Name: "package with nothing this plugin handles",
				BuildStore: func(t *testing.T) *store.Store {
					t.Helper()
					return storefixture.New().Struct("Plain", nil).Build()
				},
			},
		})
	})
}

// handlerPkg builds one struct carrying +gen:handler.
func handlerPkg(t *testing.T) *node.Package {
	t.Helper()
	return storefixture.New().
		Struct("Users", func(s *storefixture.StructBuilder) {
			// Layout composes <source-basename><suffix>, so a fixture
			// without a position yields a file named for the suffix
			// alone. Setting it is what makes the assertion name a
			// realistic filename rather than "_middleware.go".
			s.Pos(position.Pos{File: "users.go", Line: 1})
			s.Directive(storefixture.Directive(middlewaregen.DirectiveName))
		}).PackageNode()
}

// TestComposition_ContributorsFillTheHostsChain pins the whole
// composition pattern end to end: a host plugin declares an emit kind
// carrying a named slot, two unrelated plugins append into it, and the
// host's template renders all three contributions into one file.
func TestComposition_ContributorsFillTheHostsChain(t *testing.T) {
	t.Parallel()

	t.Run("both contributors render inside the host's file", func(t *testing.T) {
		t.Parallel()
		p := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes(handlerPkg(t))).
			WithGenerator(middlewaregen.New()).
			WithGenerator(authgen.New()).
			WithGenerator(metricgen.New()).
			WithGenerator(tracegen.New()).
			WithBackend(backendgolang.New()).
			Build()
		p.Run("./...")

		body := p.AssertFile("users_middleware.go").String()
		for _, want := range []string{"UsersMiddleware", "RequireAuth", "RecordLatency", "StartSpan"} {
			if !strings.Contains(body, want) {
				t.Errorf("rendered file is missing %q:\n%s", want, body)
			}
		}
	})

	// Ordering inside a slot is the pipeline's capability topology,
	// not append order. metricgen names authgen's capability in
	// Requires, so it must render second however the two happened to
	// be registered — which is why this subtest registers them in the
	// opposite order to the one it asserts.
	t.Run("Requires orders the chain, registration order does not", func(t *testing.T) {
		t.Parallel()
		p := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes(handlerPkg(t))).
			WithGenerator(middlewaregen.New()).
			WithGenerator(tracegen.New()).
			WithGenerator(metricgen.New()).
			WithGenerator(authgen.New()).
			WithBackend(backendgolang.New()).
			Build()
		p.Run("./...")

		body := p.AssertFile("users_middleware.go").String()
		auth := strings.Index(body, "RequireAuth")
		metric := strings.Index(body, "RecordLatency")
		trace := strings.Index(body, "StartSpan")
		if auth < 0 || metric < 0 || trace < 0 {
			t.Fatalf("all three entries must render; got auth=%d metric=%d trace=%d:\n%s",
				auth, metric, trace, body)
		}
		// Registered trace, metric, auth — asserted auth, metric,
		// trace. The chain of Requires reverses the registration
		// order completely, which is the point.
		if auth >= metric || metric >= trace {
			t.Errorf("chain must render auth -> metric -> trace, the order their Requires "+
				"declare, not the order they were registered:\n%s", body)
		}
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
		p := pipelinetest.New(t).
			WithFrontend(pipelinetest.FromNodes(handlerPkg(t))).
			WithGenerator(authgen.New()).
			WithGenerator(metricgen.New()).
			WithGenerator(tracegen.New()).
			WithBackend(backendgolang.New()).
			Build()
		p.Run("./...")

		p.AssertFileCount(0)
	})
}
