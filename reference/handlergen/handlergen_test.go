// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package handlergen_test

import (
	"strings"
	"testing"

	backendgolang "go.thesmos.sh/eidos/backend/golang"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/reference/auditgen"
	"go.thesmos.sh/eidos/reference/authgen"
	"go.thesmos.sh/eidos/reference/errorgen"
	"go.thesmos.sh/eidos/reference/handlergen"
	"go.thesmos.sh/eidos/reference/metricgen"
	"go.thesmos.sh/eidos/reference/middlewaregen"
	"go.thesmos.sh/eidos/reference/tracegen"
	"go.thesmos.sh/eidos/reference/validategen"
	"go.thesmos.sh/eidos/store"
)

// TestConformance runs the framework conformance suites against
// the handlergen plugin: [plugintest.RunSuite] for the stability, role and
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
		plugintest.RunSuite(t, handlergen.New())
	})

	t.Run("generator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunGeneratorSuite(t, handlergen.New(), []plugintest.GeneratorFixture{
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

func handlerPkg(t *testing.T) *node.Package {
	t.Helper()
	return storefixture.New().
		Struct("Orders", func(s *storefixture.StructBuilder) {
			s.Pos(position.Pos{File: "orders.go", Line: 1})
			s.Directive(storefixture.Directive(handlergen.DirectiveName))
		}).PackageNode()
}

// TestEnsemble_EveryPluginRendersThroughItsOwnTemplate drives all
// eight plugins in one run.
//
// Every one declares its own emit kind and ships the template that
// renders it. No plugin renders another's content, and no plugin knows
// what lands in the slots it exposes — the backend dispatches each
// item to its owner by Kind.
func TestEnsemble_EveryPluginRendersThroughItsOwnTemplate(t *testing.T) {
	t.Parallel()

	// Registered in an order unrelated to the rendered one, so a pass
	// cannot be an accident of registration sequence.
	p := pipelinetest.New(t).
		WithFrontend(pipelinetest.FromNodes(handlerPkg(t))).
		WithGenerator(auditgen.New()).
		WithGenerator(tracegen.New()).
		WithGenerator(errorgen.New()).
		WithGenerator(middlewaregen.New()).
		WithGenerator(metricgen.New()).
		WithGenerator(handlergen.New()).
		WithGenerator(validategen.New()).
		WithGenerator(authgen.New()).
		WithBackend(backendgolang.New()).
		Build()
	p.Run("./...")

	t.Run("the handler file carries every contributor's entry", func(t *testing.T) {
		t.Parallel()
		body := p.AssertFile("orders_handler.go").String()
		for _, want := range []string{
			"OrdersHandler", "ValidateOrders", "errors.Recover", "audit.Record",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("handler file is missing %q:\n%s", want, body)
			}
		}
	})

	t.Run("the middleware chain renders its own three contributors", func(t *testing.T) {
		t.Parallel()
		body := p.AssertFile("orders_middleware.go").String()
		for _, want := range []string{"RequireAuth", "RecordLatency", "StartSpan"} {
			if !strings.Contains(body, want) {
				t.Errorf("chain file is missing %q:\n%s", want, body)
			}
		}
	})

	// validategen owns a file of its own while also contributing into
	// handlergen's. Outputs and templates are independent capabilities:
	// a plugin needs an Output only for the decls it owns.
	t.Run("validategen owns a file while also contributing to another", func(t *testing.T) {
		t.Parallel()
		p.AssertFile("orders_validate.go").Contains("func ValidateOrders")
	})
}
