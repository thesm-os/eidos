// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
)

func TestRenderableKinds(t *testing.T) {
	t.Parallel()

	t.Run("renderable reports what the canonical set covers", func(t *testing.T) {
		t.Parallel()
		if !renderable(&emit.Interface{Name: "I"}) {
			t.Error("an interface is not reported renderable")
		}
		if renderable(&emit.Method{Name: "m"}) {
			t.Error("a method is reported renderable")
		}
		if renderable(nil) {
			t.Error("nil is reported renderable")
		}
	})

	t.Run("every renderable kind has a template", func(t *testing.T) {
		t.Parallel()
		// The two lists are maintained by hand: a kind added to one
		// and not the other fails at dispatch with ErrTemplateMissing,
		// on a run rather than under test.
		tmpl, err := loadTemplates()
		if err != nil {
			t.Fatalf("loadTemplates: %v", err)
		}
		for kind := range renderableKinds {
			if tmpl.Lookup(kind) == nil {
				t.Errorf("kind %s is renderable but has no template", kind)
			}
		}
	})
}
