// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package handlergen_test

import (
	"strings"
	"testing"

	backendgolang "go.thesmos.sh/eidos/backend/golang"
	"go.thesmos.sh/eidos/eidostest/golangtest"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/reference/auditgen"
	"go.thesmos.sh/eidos/reference/authgen"
	"go.thesmos.sh/eidos/reference/errorgen"
	"go.thesmos.sh/eidos/reference/handlergen"
	"go.thesmos.sh/eidos/reference/metricgen"
	"go.thesmos.sh/eidos/reference/middlewaregen"
	"go.thesmos.sh/eidos/reference/tracegen"
	"go.thesmos.sh/eidos/reference/validategen"
	"go.thesmos.sh/eidos/sdk"
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

// handlerBuilder is the annotated struct every render fixture drives,
// and — through [storefixture.Builder.GoSource] — the declaration the
// generated handler is compiled against.
func handlerBuilder() *storefixture.Builder {
	return storefixture.New().
		Struct("Orders", func(s *storefixture.StructBuilder) {
			s.Docs("Orders is the annotated struct the fixture stamps.")
			s.Pos(sdk.Pos{File: "orders.go", Line: 1})
			s.Directive(storefixture.Directive(handlergen.DirectiveName))
		})
}

// handlerSource pairs the projected subject with the hand-written
// half of its package.
//
// The hand-written half is required, not decoration. handlergen emits
// a `ServeHTTP` that delegates to `h.serve` and deliberately does not
// emit `serve` — the skeleton is generated, the request handling is
// written by hand. So the generated file does not compile alone by
// design, and only a fixture supplying the hand-written half can prove
// the contract holds in the direction that matters: that what
// handlergen emits builds against a consumer who wrote what it asked
// for. Neither declaration is projectable: `serve` has a body and
// hangs off a type the fixture never describes.
//
// `Handler` is a type alias rather than a plain use of
// [net/http.Handler]: [golangtest.Generated.AssertSatisfies] writes its
// assertion into a file with no import block, so the interface it names
// has to be resolvable unqualified in the generated package.
func handlerSource() []golangtest.File {
	return []golangtest.File{
		golangtest.GoFile(handlerBuilder().GoSource()),
		golangtest.GoFile("handler_support.go", `package test

import "net/http"

// Handler names http.Handler unqualified for the satisfaction assertion.
type Handler = http.Handler

// serve is the body handlergen leaves to the consumer.
func (h *OrdersHandler) serve(w http.ResponseWriter, r *http.Request) {}
`),
	}
}

// TestGenerate_HandlerFile drives handlergen alone and asserts on the
// Go it produced.
//
// [golangtest.Generated.AssertCompiles] is the assertion every
// structural one below is a proxy for, and it is paid once: a
// substring check passes just as well against a template that renders
// a call at the wrong arity or a receiver the type does not have.
func TestGenerate_HandlerFile(t *testing.T) {
	t.Parallel()

	gen := golangtest.Render(t, backendgolang.New(), handlerBuilder().PackageNode(), handlergen.New()).
		WithSource(handlerSource()...)

	t.Run("emits Go the consumer can build", func(t *testing.T) {
		t.Parallel()
		gen.AssertCompiles(t)
	})

	// The one assertion a compile does not make: a handler whose
	// ServeHTTP lost a parameter, gained one, or landed on the value
	// receiver still builds and serves nothing.
	t.Run("the handler satisfies http.Handler", func(t *testing.T) {
		t.Parallel()
		gen.AssertSatisfies(t, "OrdersHandler", "Handler")
	})

	t.Run("declares the handler type and its method", func(t *testing.T) {
		t.Parallel()
		src := gen.Suffixed(t, handlergen.GoSuffix)
		src.AssertPackage(t, "test")
		src.AssertType(t, "OrdersHandler")
		src.AssertMethod(t, "OrdersHandler", "ServeHTTP").
			Signature(t, "(w http.ResponseWriter, r *http.Request)").
			AssertPointerReceiver(t, true)
	})

	// The refs are held as [sdk.Ref] so the backend registers the
	// imports; pinning the whole set is what makes a template that
	// starts spelling a type inline, or reaching for a second package,
	// a one-line diff rather than an invisible one.
	t.Run("imports only what the signature needs", func(t *testing.T) {
		t.Parallel()
		gen.Suffixed(t, handlergen.GoSuffix).AssertImportsOnly(t, "net/http")
	})

	t.Run("delegates the body to the hand-written half", func(t *testing.T) {
		t.Parallel()
		gen.Suffixed(t, handlergen.GoSuffix).
			InMethod(t, "OrdersHandler", "ServeHTTP").
			AssertContains(t, "h.serve(w, r)")
	})

	t.Run("is publishable generated Go", func(t *testing.T) {
		t.Parallel()
		src := gen.Suffixed(t, handlergen.GoSuffix)
		src.AssertGeneratedHeader(t)
		src.AssertFormatted(t)
		src.AssertDocumented(t)
	})
}

// ensembleSource is the hand-written package tree the ensemble's output
// references: handlergen's own consumer half plus the five middleware
// packages the contributing plugins import.
//
// Written as Go rather than described, because that is the form the
// consumer writes it in — and because the import paths the plugins emit
// (`example.com/httpmw/...`) only resolve against a module laid out
// this way, which is the check.
func ensembleSource() []golangtest.File {
	pkg := func(path, body string) golangtest.File {
		return golangtest.File{Path: path, Src: []byte(body)}
	}
	return append(handlerSource(),
		pkg("audit/audit.go", `package audit

import "net/http"

// Record writes an audit entry for one request.
func Record(w http.ResponseWriter, r *http.Request) {}
`),
		pkg("errors/errors.go", `package errors

import "net/http"

// Recover turns a panic into a response.
func Recover(w http.ResponseWriter, r *http.Request) {}
`),
		pkg("auth/auth.go", `package auth

import "net/http"

// RequireAuth rejects an unauthenticated request.
func RequireAuth(next http.Handler) http.Handler { return next }
`),
		pkg("metrics/metrics.go", `package metrics

import "net/http"

// RecordLatency times one request.
func RecordLatency(next http.Handler) http.Handler { return next }
`),
		pkg("trace/trace.go", `package trace

import "net/http"

// StartSpan opens a span around one request.
func StartSpan(next http.Handler) http.Handler { return next }
`),
	)
}

// TestEnsemble_EveryPluginRendersThroughItsOwnTemplate drives all
// eight plugins in one run.
//
// Every one declares its own emit kind and ships the template that
// renders it. No plugin renders another's content, and no plugin knows
// what lands in the slots it exposes — the backend dispatches each
// item to its owner by Kind.
//
// The build is the assertion with teeth. Four plugins write into one
// function body through two slots they do not own; a contribution
// landing at the wrong arity, naming a package the run never imported,
// or rendering outside the braces is a compile error and nothing less
// than a compiler sees it.
func TestEnsemble_EveryPluginRendersThroughItsOwnTemplate(t *testing.T) {
	t.Parallel()

	// Registered in an order unrelated to the rendered one, so a pass
	// cannot be an accident of registration sequence.
	gen := golangtest.Render(t, backendgolang.New(), handlerBuilder().PackageNode(),
		auditgen.New(),
		tracegen.New(),
		errorgen.New(),
		middlewaregen.New(),
		metricgen.New(),
		handlergen.New(),
		validategen.New(),
		authgen.New(),
	).
		// The module path is the root of the tree the contributing
		// plugins emit imports under, so `example.com/httpmw/audit`
		// resolves to the hand-written package beside the output
		// rather than to nothing.
		WithModulePath("example.com/httpmw").
		WithSource(ensembleSource()...)

	t.Run("the assembled output builds", func(t *testing.T) {
		t.Parallel()
		gen.AssertCompiles(t)
	})

	t.Run("the handler file carries every contributor's entry", func(t *testing.T) {
		t.Parallel()
		body := gen.Suffixed(t, handlergen.GoSuffix).
			InMethod(t, "OrdersHandler", "ServeHTTP")
		for _, want := range []string{
			"ValidateOrders", "h.serve(w, r)", "errors.Recover", "audit.Record",
		} {
			body.AssertContains(t, want)
		}
	})

	// Slot ordering is otherwise invisible: a contribution rendered on
	// the wrong side of the owner's own statements compiles perfectly.
	t.Run("the prebody slot renders before the postbody slot", func(t *testing.T) {
		t.Parallel()
		body := gen.Suffixed(t, handlergen.GoSuffix).
			InMethod(t, "OrdersHandler", "ServeHTTP").Body()
		pre := indexOf(t, body, "ValidateOrders")
		own := indexOf(t, body, "h.serve(w, r)")
		post := indexOf(t, body, "errors.Recover")
		if pre >= own || own >= post {
			t.Errorf("ServeHTTP renders prebody/own/postbody at %d/%d/%d, want ascending\n%s",
				pre, own, post, body)
		}
	})

	t.Run("the middleware chain renders its own three contributors", func(t *testing.T) {
		t.Parallel()
		// middlewaregen's output is a package-level var holding a
		// composite literal, which golangtest has no scope for — see the
		// note in AssertContains' neighbours. The file-level check stands
		// in until one exists.
		chain := string(gen.Suffixed(t, "_middleware.go").Bytes())
		for _, want := range []string{"auth.RequireAuth", "metrics.RecordLatency", "trace.StartSpan"} {
			if !strings.Contains(chain, want) {
				t.Errorf("chain file is missing %q:\n%s", want, chain)
			}
		}
	})

	// validategen owns a file of its own while also contributing into
	// handlergen's. Outputs and templates are independent capabilities:
	// a plugin needs an Output only for the decls it owns.
	t.Run("validategen owns a file while also contributing to another", func(t *testing.T) {
		t.Parallel()
		gen.Suffixed(t, "_validate.go").
			AssertFunc(t, "ValidateOrders").
			Signature(t, "(v *Orders) error")
	})
}

// indexOf reports where substr starts, failing the test when it is
// absent so an ordering comparison never runs against -1.
func indexOf(t *testing.T, body, substr string) int {
	t.Helper()
	i := strings.Index(body, substr)
	if i < 0 {
		t.Fatalf("body does not contain %q:\n%s", substr, body)
	}
	return i
}
