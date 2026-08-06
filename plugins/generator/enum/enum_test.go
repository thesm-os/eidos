// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package enum_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/plugin"
	enumplugin "go.thesmos.sh/eidos/plugins/generator/enum"
	"go.thesmos.sh/eidos/store"
)

// TestConformance runs the framework conformance suites against
// the enum plugin. The framework checks pin the static contract
// (stable Name, role implementation, deterministic Outputs,
// well-formed shape); the generator suite pins the
// determinism / frozen-source / diagnostic-discipline contracts
// across a representative fixture set.
//
// Every fixture here is an input the plugin handles cleanly, which
// is what a conformance fixture means: the suite fails any fixture
// producing an Error-severity diagnostic, because the pipeline turns
// one into a non-zero exit for the user. The one input that does
// produce a diagnostic — an annotated enum with no variants — is
// pinned directly in [TestGenerateOnAnnotatedEnumWithNoVariants],
// where the assertion is about the diagnostic rather than about the
// contracts a valid input satisfies.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, enumplugin.New())
	})

	t.Run("generator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunGeneratorSuite(
			t,
			enumplugin.New(),
			[]plugintest.GeneratorFixture{
				{
					Name: "empty package",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return storefixture.New().Build()
					},
				},
				{
					Name: "annotated enum with two variants",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return buildEnumStore(t, withoutOverrides)
					},
				},
				{
					Name: "annotated enum with a +gen:value override",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return buildEnumStore(t, withOverride)
					},
				},
			},
		)
	})

	t.Run("options round-trip", func(t *testing.T) {
		t.Parallel()
		plugintest.RunOptionsSuite(t, enumplugin.New(), plugintest.OptionsFixture{
			Valid: map[string]string{
				"strip_prefix":    "true",
				"parse_prefix":    "Parse",
				"sentinel_prefix": "ErrUnknown",
			},
			UnknownKey: "no_such_key",
		})
	})
}

// TestGenerateOnAnnotatedEnumWithNoVariants pins the plugin's only
// diagnostic path: an enum carrying `+gen:enum` with nothing to
// enumerate.
//
// Driven directly rather than through [plugintest.RunGeneratorSuite]
// because the two disagree about what the input is. To the suite a
// fixture is an input the plugin handles; this one it refuses, on
// purpose, and the refusal is the behaviour worth pinning — with its
// severity and its position, neither of which a conformance run
// inspects.
func TestGenerateOnAnnotatedEnumWithNoVariants(t *testing.T) {
	t.Parallel()

	t.Run("the empty enum is reported as an Error-severity diagnostic", func(t *testing.T) {
		t.Parallel()

		_, d := generateEmptyEnum(t)

		if !d.HasErrors() {
			t.Fatalf("an annotated enum with no variants produced no Error diagnostic; got %+v",
				d.Diagnostics())
		}
		if got := d.Diagnostics()[0].Message; !strings.Contains(got, enumplugin.ErrEnumHasNoVariants.Error()) {
			t.Errorf("diagnostic message %q does not name ErrEnumHasNoVariants", got)
		}
	})

	t.Run("the diagnostic carries the enum's own position", func(t *testing.T) {
		t.Parallel()

		_, d := generateEmptyEnum(t)

		got := d.Diagnostics()[0].Pos
		if got.IsZero() {
			t.Fatal("the diagnostic carries no position; it renders as a dash where the file and " +
				"line belong and the user cannot find the enum to fix")
		}
		if want := emptyEnumPos; got != want {
			t.Errorf("diagnostic position = %v; want the enum's own %v", got, want)
		}
	})

	t.Run("the empty enum contributes nothing to the emit graph", func(t *testing.T) {
		t.Parallel()

		s, _ := generateEmptyEnum(t)

		if got := len(s.Emit().PendingOriginSlots()); got != 0 {
			t.Errorf("plugin queued %d contribution(s) for an enum it reported on; want 0 — a "+
				"diagnostic that does not also stop the emit renders a broken file", got)
		}
	})
}

// emptyEnumPos is the position the no-variant fixture declares, so
// the position assertion compares against a value the fixture owns
// rather than a literal repeated on both sides.
var emptyEnumPos = position.At("empty/empty.go", 1, 1)

// generateEmptyEnum drives Generate against a store holding one
// annotated enum with no variants and returns the store together
// with the diagnostics it produced.
//
// The sink is [diag.Capture] rather than [diag.Discard]: the whole
// point of this test is what was emitted.
func generateEmptyEnum(t *testing.T) (*store.Store, *diag.Sink) {
	t.Helper()
	s := storefixture.New().
		Package("empty", "example.com/empty").
		Enum("Empty", func(eb *storefixture.EnumBuilder) {
			eb.Pos(emptyEnumPos)
			eb.Directive(storefixture.Directive(enumplugin.DirectiveName))
		}).
		Build()
	d := diag.Capture()
	if err := enumplugin.New().Generate(&plugin.GeneratorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   d,
	}); err != nil {
		t.Fatalf("Generate returned an error on an enum it should have reported on: %v", err)
	}
	return s, d
}

// buildEnumStore returns a [store.Store] populated with one
// source enum named Status declared in `status/status.go`. The
// configure hook tweaks the per-test variant set / directive
// layout. Used by the generator suite, which expects a full
// store; the backend-driven end-to-end acceptance test lives
// outside this package (plugins cannot import backends).
func buildEnumStore(t *testing.T, configure func(*storefixture.EnumBuilder)) *store.Store {
	t.Helper()
	return storefixture.New().
		Package("status", "example.com/status").
		Enum("Status", func(eb *storefixture.EnumBuilder) {
			eb.Pos(position.At("status/status.go", 1, 1))
			eb.Directive(storefixture.Directive(enumplugin.DirectiveName))
			configure(eb)
		}).
		Build()
}

// withoutOverrides configures the canonical two-variant enum
// (`StatusActive`, `StatusInactive`) with no per-variant
// `+gen:value` directives — the default prefix-stripping rule
// resolves both to `"Active"` / `"Inactive"`.
func withoutOverrides(eb *storefixture.EnumBuilder) {
	eb.Variant("StatusActive", "0")
	eb.Variant("StatusInactive", "1")
}

// withOverride adds a third variant whose rendered string-form
// is pinned via `+gen:value pending_review` — exercising the
// override branch of the variant-name resolver.
func withOverride(eb *storefixture.EnumBuilder) {
	eb.Variant("StatusActive", "0")
	eb.Variant("StatusInactive", "1")
	eb.Variant("StatusPending", "2")
	// Reach into the just-appended variant to attach the
	// override directive — the storefixture's Variant signature
	// is flat (no callback), so the directive list is mutated
	// after construction.
	enum := eb.Node()
	pending := enum.Variants[len(enum.Variants)-1]
	pending.DirectiveList = append(pending.DirectiveList, &directive.Directive{
		Name: "value",
		Args: []string{"pending_review"},
	})
}
