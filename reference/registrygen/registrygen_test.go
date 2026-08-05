// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package registrygen_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/reference/registrygen"
	"go.thesmos.sh/eidos/store"
)

// TestConformance runs the framework conformance suites against
// registrygen. Cross-cutting plugins (Generator + emits a
// per-package init-time registration file) verify both the
// universal framework contracts and the per-role determinism /
// frozen-source / diagnostic-discipline contracts.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, registrygen.New())
	})

	t.Run("generator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunGeneratorSuite(
			t,
			registrygen.New(),
			[]plugintest.GeneratorFixture{
				{
					Name: "empty package",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return storefixture.New().Build()
					},
				},
				{
					Name: "package with a struct",
					BuildStore: func(t *testing.T) *store.Store {
						t.Helper()
						return storefixture.New().
							Struct("Plain", nil).
							Build()
					},
				},
			},
		)
	})

	t.Run("options round-trip", func(t *testing.T) {
		t.Parallel()
		plugintest.RunOptionsSuite(t, registrygen.New(), plugintest.OptionsFixture{
			Valid: map[string]string{
				"register_package": "log",
				"register_func":    "Print",
			},
			UnknownKey: "no_such_field",
		})
	})
}

func TestGenerate_AppendsRegistration(t *testing.T) {
	t.Parallel()

	// pending drives Generate over a fixture and returns the
	// origin-anchored contributions it queued for the Layout phase.
	pending := func(t *testing.T, s *store.Store) []store.PendingOriginSlot {
		t.Helper()
		if err := registrygen.New().Generate(&plugin.GeneratorContext{
			Store: s, Reader: store.NewReader(s), Diag: diag.New(),
		}); err != nil {
			t.Fatalf("Generate: %v", err)
		}
		return s.Emit().PendingOriginSlots()
	}

	t.Run("one registration per annotated struct", func(t *testing.T) {
		t.Parallel()
		s := storefixture.New().
			Struct("User", func(b *storefixture.StructBuilder) {
				b.Directive(storefixture.Directive("register"))
			}).
			Struct("Plain", nil).
			Build()

		got := pending(t, s)
		if len(got) != 1 {
			t.Fatalf("only the +gen:register struct should register, got %d contributions", len(got))
		}
		if got[0].SlotName != registrygen.SlotName {
			t.Errorf("contribution slot = %q, want %q", got[0].SlotName, registrygen.SlotName)
		}
		reg, ok := got[0].Item.(*registrygen.Registration)
		if !ok {
			t.Fatalf("contribution item is %T, want *registrygen.Registration", got[0].Item)
		}
		if reg.Name != "User" {
			t.Errorf("registration name = %q, want User", reg.Name)
		}
		// The plugin-defined emit kind is what lets the backend pick
		// this node's template.
		if reg.Kind() != registrygen.Kind {
			t.Errorf("registration kind = %q, want %q", reg.Kind(), registrygen.Kind)
		}
	})

	t.Run("a struct without the directive contributes nothing", func(t *testing.T) {
		t.Parallel()
		s := storefixture.New().Struct("Plain", nil).Build()
		if got := pending(t, s); len(got) != 0 {
			t.Errorf("unannotated struct should contribute nothing, got %d", len(got))
		}
	})
}
