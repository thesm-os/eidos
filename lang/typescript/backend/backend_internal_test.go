// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
)

func TestPluginIdentity(t *testing.T) {
	t.Parallel()

	t.Run("reports a stable name, language and version", func(t *testing.T) {
		t.Parallel()
		b := New()
		if b.Name() != Name {
			t.Errorf("Name = %q, want %q", b.Name(), Name)
		}
		if b.Language() != typescript.Language {
			t.Errorf("Language = %q, want %q", b.Language(), typescript.Language)
		}
		if b.Version() != BackendVersion || b.Version() == "" {
			t.Errorf("Version = %q, want %q", b.Version(), BackendVersion)
		}
	})

	t.Run("EmitVersions returns a slice the caller may keep", func(t *testing.T) {
		t.Parallel()
		// The pipeline holds what it is handed; a shared backing array
		// would let one caller's edit reach another's copy.
		b := New()
		first := b.EmitVersions()
		if len(first) == 0 {
			t.Fatal("EmitVersions is empty")
		}
		first[0] = "mutated"

		if second := b.EmitVersions(); second[0] == "mutated" {
			t.Fatal("EmitVersions handed out its own backing array")
		}
	})

	t.Run("the canonical templates parse", func(t *testing.T) {
		t.Parallel()
		// New keeps the parse error rather than panicking, so a broken
		// embed surfaces as a non-nil field rather than a crash in
		// every consumer.
		if b := New(); b.tmplErr != nil {
			t.Fatalf("canonical templates do not parse: %v", b.tmplErr)
		}
	})

	t.Run("every renderable kind has a template", func(t *testing.T) {
		t.Parallel()
		// A kind listed as renderable with no template would reach
		// dispatch and fail per file at render time rather than here.
		b := New()
		for kind := range renderableKinds {
			if b.tmpl.Lookup(kind) == nil {
				t.Errorf("kind %q is renderable but has no template", kind)
			}
		}
	})
}
