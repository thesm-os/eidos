// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline_test

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
)

// TestCoreDirectives_OutRegistered pins the core-set contract: a
// freshly-built pipeline always exposes the `out` directive
// through its [directive.Registry], even when the caller supplies
// no schemas via [pipeline.Builder.WithDirective]. The
// registration is what enables source authors to annotate a node
// with `+gen:out filename.go` and have the Layout phase pick it
// up — without a registry entry, the directive parser rejects the
// comment with an unknown-directive error.
func TestCoreDirectives_OutRegistered(t *testing.T) {
	t.Parallel()

	t.Run("out directive is in the core set after Build", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		schema, ok := p.DirectiveRegistry().Lookup(pipeline.OutDirective)
		if !ok {
			t.Fatalf("OutDirective should be registered in the core set; got missing")
		}
		if schema.Name != pipeline.OutDirective {
			t.Fatalf("schema name = %q, want %q", schema.Name, pipeline.OutDirective)
		}
		if schema.AllowNegated {
			t.Fatalf("OutDirective should reject the negated form")
		}
	})

	t.Run("out directive declares exactly one positional argument", func(t *testing.T) {
		t.Parallel()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		schema, ok := p.DirectiveRegistry().Lookup(pipeline.OutDirective)
		if !ok {
			t.Fatalf("OutDirective missing from registry")
		}
		if got := len(schema.PositionalArgs); got != 1 {
			t.Fatalf("PositionalArgs = %d, want 1 (the filename arg)", got)
		}
	})
}

// restrictiveGen is a generator declaring one directive with a
// closed key surface — the shape every in-tree generator except
// stubgen uses (`builder` whitelists `defaults`, `sentinel`
// whitelists `prefix`).
type restrictiveGen struct{ name string }

// Name returns the configured plugin identifier.
func (g *restrictiveGen) Name() string { return g.name }

// Generate emits nothing; the schema is what this fixture is for.
func (*restrictiveGen) Generate(*plugin.GeneratorContext) error { return nil }

// Directives declares a single option key and nothing else.
func (g *restrictiveGen) Directives() []directive.Schema {
	return []directive.Schema{
		directive.NewSchema(directive.Name(g.name)).
			AllowedKeys("defaults").
			Build(),
	}
}

// TestCoreDirectives_RoutingKeysAlwaysAllowed pins that a plugin
// restricting its own key surface does not thereby disable the
// framework's routing overrides.
//
// The Layout phase reads `out=` / `pkg=` / `tag=` from any
// plugin-owned directive, so a schema that rejects them puts the
// validator and the router in disagreement: the run reports "does
// not accept key out" and then honours it anyway, producing output
// nobody asked for alongside an error nobody can act on. A plugin
// author writing AllowedKeys is describing their options, not
// opting out of redirection.
func TestCoreDirectives_RoutingKeysAlwaysAllowed(t *testing.T) {
	t.Parallel()

	lookup := func(t *testing.T) directive.Schema {
		t.Helper()
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&restrictiveGen{name: "restrictive"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		schema, ok := p.DirectiveRegistry().Lookup("restrictive")
		if !ok {
			t.Fatalf("plugin directive not registered")
		}
		return schema
	}

	for _, key := range pipeline.RoutingKeys {
		t.Run("a restricted schema still accepts "+key, func(t *testing.T) {
			t.Parallel()
			if got := lookup(t).AllowedKeys; !slices.Contains(got, key) {
				t.Fatalf("AllowedKeys = %v, want it to include %q", got, key)
			}
		})
	}

	t.Run("the plugin's own key survives the widening", func(t *testing.T) {
		t.Parallel()
		// Widening must append, not replace. Losing the declared key
		// would trade one false rejection for another.
		if got := lookup(t).AllowedKeys; !slices.Contains(got, "defaults") {
			t.Fatalf("AllowedKeys = %v, want it to retain %q", got, "defaults")
		}
	})

	t.Run("an unrelated key is still rejected", func(t *testing.T) {
		t.Parallel()
		// The widening is a fixed reserved set, not an escape hatch
		// that turns every restricted schema into an open one.
		if got := lookup(t).AllowedKeys; slices.Contains(got, "nonesuch") {
			t.Fatalf("AllowedKeys = %v, want it to stay closed", got)
		}
	})

	t.Run("an open schema is left open rather than closed to the routing set", func(t *testing.T) {
		t.Parallel()
		// An empty AllowedKeys already accepts everything. Appending
		// to it would invert the meaning — from "any key" to
		// "exactly out, pkg, tag".
		if got := directive.NewSchema("open").Build(); len(got.AllowedKeys) != 0 {
			t.Fatalf("baseline open schema has AllowedKeys %v, want empty", got.AllowedKeys)
		}
		p, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&openGen{name: "open"}).
			WithBackend(&stubBE{name: "be"}).
			WithSink(sink.NewMemory()).
			Build()
		if err != nil {
			t.Fatalf("Build: %v", err)
		}
		schema, ok := p.DirectiveRegistry().Lookup("open")
		if !ok {
			t.Fatalf("plugin directive not registered")
		}
		if len(schema.AllowedKeys) != 0 {
			t.Fatalf("AllowedKeys = %v, want it left empty", schema.AllowedKeys)
		}
	})
}

// openGen declares a directive with no key restriction.
type openGen struct{ name string }

// Name returns the configured plugin identifier.
func (g *openGen) Name() string { return g.name }

// Generate emits nothing.
func (*openGen) Generate(*plugin.GeneratorContext) error { return nil }

// Directives declares one schema with an open key surface.
func (g *openGen) Directives() []directive.Schema {
	return []directive.Schema{directive.NewSchema(directive.Name(g.name)).Build()}
}
