// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/emit"
)

// The witness accessors are written by an annotator and read by a
// language, neither of which is this module — the same position the
// authored-sample accessors are in, and the same reason nothing else
// here exercises them.
//
// What they decide is whether the rendered file imports anything. A
// witness is written into an instantiation, so one that lost its
// package renders a bare name in a file that never imported it, and
// one that gained a package it should not have qualifies `int`.

func TestAuthoredWitness(t *testing.T) {
	t.Parallel()

	t.Run("reports nothing on a bag carrying none", func(t *testing.T) {
		t.Parallel()
		if _, _, ok := emit.AuthoredWitness(meta.NewBag()); ok {
			t.Error("an unstamped bag reported a witness")
		}
	})

	t.Run("reports nothing on a nil bag", func(t *testing.T) {
		t.Parallel()
		// The shape a caller walking parameters hands over: a
		// parameter nothing has stamped has no bag at all.
		if _, _, ok := emit.AuthoredWitness(nil); ok {
			t.Error("a nil bag reported a witness")
		}
	})

	t.Run("returns the symbol and package it was stamped with", func(t *testing.T) {
		t.Parallel()
		bag := bagWith(t, emit.MetaWitness, emit.MetaWitnessPackage, "Duration", "time")
		pkg, symbol, ok := emit.AuthoredWitness(bag)
		if !ok {
			t.Fatal("a stamped bag reported no witness")
		}
		if symbol != "Duration" || pkg != "time" {
			t.Errorf("got %q in %q, want Duration in time", symbol, pkg)
		}
	})
}

func TestAuthoredWitnessRef(t *testing.T) {
	t.Parallel()

	t.Run("an unqualified witness is a builtin reference", func(t *testing.T) {
		t.Parallel()
		// The distinction that decides imports: a builtin's rendered
		// form is its name, and nothing has to be registered for it.
		bag := bagWith(t, emit.MetaWitness, emit.MetaWitnessPackage, "int", "")
		ref, ok := emit.AuthoredWitnessRef(bag)
		if !ok {
			t.Fatal("a stamped bag reported no witness")
		}
		builtin, isBuiltin := ref.(*emit.BuiltinRef)
		if !isBuiltin {
			t.Fatalf("got %T, want *emit.BuiltinRef", ref)
		}
		if builtin.Name != "int" {
			t.Errorf("name = %q, want int", builtin.Name)
		}
	})

	t.Run("a qualified witness is an external reference", func(t *testing.T) {
		t.Parallel()
		// The other half: rendering this registers the import, which a
		// builtin reference would not have asked for.
		bag := bagWith(t, emit.MetaWitness, emit.MetaWitnessPackage, "Duration", "time")
		ref, ok := emit.AuthoredWitnessRef(bag)
		if !ok {
			t.Fatal("a stamped bag reported no witness")
		}
		ext, isExternal := ref.(*emit.ExternalRef)
		if !isExternal {
			t.Fatalf("got %T, want *emit.ExternalRef", ref)
		}
		if ext.Package != "time" || ext.Name != "Duration" {
			t.Errorf("got %q.%q, want time.Duration", ext.Package, ext.Name)
		}
	})

	t.Run("reports nothing where nothing was stamped", func(t *testing.T) {
		t.Parallel()
		if _, ok := emit.AuthoredWitnessRef(meta.NewBag()); ok {
			t.Error("an unstamped bag produced a reference")
		}
	})
}
