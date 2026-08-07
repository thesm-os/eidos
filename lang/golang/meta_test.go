// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"errors"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// TestMetaKeys pins the shared `go.*` vocabulary.
//
// A meta key is interned by name, so two packages declaring the
// same name resolve one singleton and a consumer that cannot import
// the declaration re-declares it by string instead. Three did — the
// Go backend for five keys, a shape detector for one, a downstream
// generator for two — each forfeiting the compile-time link and the
// rename-safety that comes with it. These checks are what make the
// single declaration load-bearing: a renamed key stops matching the
// name its readers expect, and nothing else in the tree would say
// so.
func TestMetaKeys(t *testing.T) {
	t.Parallel()

	// declared pairs each exported key with the name its readers —
	// including ones outside this repository — key on.
	declared := []struct {
		name string
		key  interface{ Name() string }
	}{
		{"go.isChannel", golang.MetaIsChannel},
		{"go.chanDir", golang.MetaChanDir},
		{"go.chanElem", golang.MetaChanElem},
		{"go.isContext", golang.MetaIsContext},
		{"go.isError", golang.MetaIsError},
		{"go.isStringer", golang.MetaIsStringer},
		{"go.isComparable", golang.MetaIsComparable},
		{"go.isInterface", golang.MetaIsInterface},
		{"go.embedsInterface", golang.MetaEmbedsInterface},
		{"go.isEmptyInterface", golang.MetaIsEmptyInterface},
		{"go.isConstraintInterface", golang.MetaIsConstraintInterface},
		{"go.underlyingKind", golang.MetaUnderlyingKind},
		{"go.isIterSeq", golang.MetaIsIterSeq},
		{"go.isIterSeq2", golang.MetaIsIterSeq2},
		{"go.iterKeyType", golang.MetaIterKeyType},
		{"go.iterValueType", golang.MetaIterValueType},
		{"go.iotaValue", golang.MetaIotaValue},
		{"go.receiverIsPointer", golang.MetaReceiverIsPointer},
		{"go.constraintTerms", golang.MetaConstraintTerms},
		{"go.type", golang.MetaGoType},
		{"go.name", golang.MetaGoName},
		{"go.import", golang.MetaGoImport},
	}

	t.Run("every key carries the name its readers expect", func(t *testing.T) {
		t.Parallel()
		for _, d := range declared {
			if got := d.key.Name(); got != d.name {
				t.Errorf("key name = %q, want %q", got, d.name)
			}
		}
	})

	t.Run("every key sits in the go namespace", func(t *testing.T) {
		t.Parallel()
		// The namespace is what lets a consumer walk a meta bag and
		// tell Go-language facts from another producer's.
		for _, d := range declared {
			if !strings.HasPrefix(d.name, "go.") {
				t.Errorf("key %q is outside the go.* namespace", d.name)
			}
		}
	})

	t.Run("no two keys share a name", func(t *testing.T) {
		t.Parallel()
		// Interning means a duplicate silently resolves to whichever
		// registered first, with the second declaration's parser
		// discarded.
		seen := map[string]struct{}{}
		for _, d := range declared {
			if _, dup := seen[d.name]; dup {
				t.Errorf("key %q declared twice", d.name)
			}
			seen[d.name] = struct{}{}
		}
	})
}

// TestMetaConstraintTerms pins the one key carrying a structured
// value rather than a scalar.
//
// The terms ride through meta as JSON so a directive override can
// supply them as text, and the parser is what turns that text back
// into the shape a generator branches on.
func TestMetaConstraintTerms(t *testing.T) {
	t.Parallel()

	t.Run("round-trips a stamped term list", func(t *testing.T) {
		t.Parallel()
		bag := meta.NewBag()
		want := []golang.ConstraintTerm{
			{Type: &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "int"}, Approximate: true},
			{Type: &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "string"}},
		}
		golang.MetaConstraintTerms.Set(bag, want, "test")
		got, ok := golang.MetaConstraintTerms.Get(bag)
		if !ok {
			t.Fatalf("stamped terms did not read back")
		}
		if len(got) != 2 || got[0].Type.Name != "int" || !got[0].Approximate {
			t.Fatalf("terms = %+v, want the stamped pair", got)
		}
	})

	t.Run("preserves the approximation flag", func(t *testing.T) {
		t.Parallel()
		// `~int` and `int` are different constraints: the first
		// admits any type whose underlying type is int. Dropping the
		// flag would silently narrow a generated bound.
		bag := meta.NewBag()
		golang.MetaConstraintTerms.Set(bag, []golang.ConstraintTerm{
			{Type: &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "int"}, Approximate: false},
		}, "test")
		got, _ := golang.MetaConstraintTerms.Get(bag)
		if got[0].Approximate {
			t.Fatalf("exact term read back as approximate")
		}
	})

	t.Run("parses a JSON term list supplied as text", func(t *testing.T) {
		t.Parallel()
		// The directive-override path supplies the value as a string,
		// so the parser is the only route from source text to terms.
		bag := meta.NewBag()
		if err := golang.MetaConstraintTerms.SetDirectiveFromString(
			bag, `[{"type":{"type_kind":0,"name":"int"},"approximate":true}]`, position.Pos{},
		); err != nil {
			t.Fatalf("SetDirectiveFromString: %v", err)
		}
		got, ok := golang.MetaConstraintTerms.Get(bag)
		if !ok || len(got) != 1 || !got[0].Approximate {
			t.Fatalf("parsed terms = %+v, want one approximate term", got)
		}
	})

	t.Run("an empty value parses to no terms", func(t *testing.T) {
		t.Parallel()
		bag := meta.NewBag()
		if err := golang.MetaConstraintTerms.SetDirectiveFromString(bag, "", position.Pos{}); err != nil {
			t.Fatalf("SetDirectiveFromString(\"\"): %v", err)
		}
		if got, _ := golang.MetaConstraintTerms.Get(bag); len(got) != 0 {
			t.Fatalf("terms = %+v, want none", got)
		}
	})

	t.Run("malformed JSON is rejected naming this package", func(t *testing.T) {
		t.Parallel()
		// A bare encoding/json message leaves a user guessing which
		// of a run's parsers rejected their override.
		bag := meta.NewBag()
		err := golang.MetaConstraintTerms.SetDirectiveFromString(bag, "{not json", position.Pos{})
		if err == nil {
			t.Fatalf("malformed JSON accepted")
		}
		if !strings.Contains(err.Error(), "lang/golang") {
			t.Fatalf("error %q does not name the rejecting package", err)
		}
	})

	t.Run("the parse error wraps the decoder's own", func(t *testing.T) {
		t.Parallel()
		bag := meta.NewBag()
		err := golang.MetaConstraintTerms.SetDirectiveFromString(bag, "{not json", position.Pos{})
		if errors.Unwrap(err) == nil {
			t.Fatalf("error %q discards the decoder's cause", err)
		}
	})
}
