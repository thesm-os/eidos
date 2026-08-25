// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/emit"
)

// These accessors are written by an annotator and read by a language,
// neither of which is this module — so nothing here exercised them and
// the coverage gate read them as dead. They are not: they are the
// whole channel between an author naming a value and a check writing
// one.

// bagWith returns a bag carrying the given symbol and package.
func bagWith(t *testing.T, symbolKey, pkgKey meta.Key[string], symbol, pkg string) *meta.Bag {
	t.Helper()
	bag := meta.NewBag()
	if symbol != "" {
		symbolKey.Set(bag, symbol, "test")
	}
	if pkg != "" {
		pkgKey.Set(bag, pkg, "test")
	}
	return bag
}

// An authored value reads back as the call an author named.
func TestAuthoredSample(t *testing.T) {
	t.Parallel()

	t.Run("names the function and the package it resolves in", func(t *testing.T) {
		t.Parallel()
		bag := bagWith(t, emit.MetaSample, emit.MetaSamplePackage, "NewTestAccount", "example.com/a")
		pkg, symbol, ok := emit.AuthoredSample(bag)
		if !ok || symbol != "NewTestAccount" || pkg != "example.com/a" {
			t.Errorf("got %q %q ok=%v, want the named function and its package", pkg, symbol, ok)
		}
	})

	t.Run("renders as a call rather than a reference", func(t *testing.T) {
		t.Parallel()
		// What was named is a function, and a consumer writing its
		// identifier where a value belongs emits the function itself.
		bag := bagWith(t, emit.MetaSample, emit.MetaSamplePackage, "NewTestAccount", "example.com/a")
		got, ok := emit.AuthoredSampleOf(bag)
		if !ok {
			t.Fatal("a stamped value did not read back")
		}
		if got.Expr == nil || got.Expr.ExprKind != emit.ExprCall {
			t.Errorf("got %+v, want a call expression", got.Expr)
		}
		if !got.OK() {
			t.Error("an authored value has to report itself usable, or every " +
				"consumer drops the check it was named for")
		}
	})

	t.Run("the second value is read independently", func(t *testing.T) {
		t.Parallel()
		// A type may need one and not the other: a derived first value
		// is often fine where the second has to differ from it in a way
		// the derivation cannot know.
		bag := bagWith(t, emit.MetaAlternate, emit.MetaAlternatePackage, "OtherAccount", "example.com/a")
		if _, ok := emit.AuthoredSampleOf(bag); ok {
			t.Error("naming the second value claimed the first as well")
		}
		if _, ok := emit.AuthoredAlternateOf(bag); !ok {
			t.Error("the second value did not read back")
		}
	})
}

// Nothing named is not an error, and neither is nothing to read.
func TestAuthoredSample_AbsentAnswers(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		bag  *meta.Bag
	}{
		{"a nil bag", nil},
		{"an empty bag", meta.NewBag()},
	} {
		t.Run(tc.name+" names no value", func(t *testing.T) {
			t.Parallel()
			// Every declaration reaches these, and only a few carry the
			// directive — so the absent answer is the common one and has
			// to be quiet.
			if _, _, ok := emit.AuthoredSample(tc.bag); ok {
				t.Error("a value was reported where none was named")
			}
			if _, _, ok := emit.AuthoredAlternate(tc.bag); ok {
				t.Error("a second value was reported where none was named")
			}
			if _, ok := emit.AuthoredSampleOf(tc.bag); ok {
				t.Error("a sample was produced where none was named")
			}
			if _, ok := emit.AuthoredAlternateOf(tc.bag); ok {
				t.Error("an alternate was produced where none was named")
			}
		})
	}
}
