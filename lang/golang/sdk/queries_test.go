// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk_test

import (
	"reflect"
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
	sdkgo "go.thesmos.sh/eidos/lang/golang/sdk"
	"go.thesmos.sh/eidos/sdk"
)

// named builds a package-qualified type reference.
func named(pkg, name string) *sdk.TypeRef {
	return &sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Package: pkg, Name: name}
}

// builtin builds an unqualified type reference.
func builtin(name string) *sdk.TypeRef { return named("", name) }

// TestQueriesForward pins every forwarder against the language
// package it forwards to.
//
// A forwarder bound to the wrong function is invisible: both spellings
// compile and both answer, so only holding one against the other says
// which. Asserted per name rather than in a loop, because the
// signatures differ and a table would have to erase them.
func TestQueriesForward(t *testing.T) {
	t.Parallel()

	ptr := &sdk.TypeRef{TypeKind: sdk.TypeRefPointer, Elem: named("example.com/x", "User")}
	ctx := named("context", "Context")

	t.Run("type identity and shape", func(t *testing.T) {
		t.Parallel()
		if got, want := sdkgo.QName(ctx), golang.QName(ctx); got != want || got == "" {
			t.Errorf("QName = %q, want %q", got, want)
		}
		if got, want := sdkgo.LocalName("time.Time"), golang.LocalName("time.Time"); got != want {
			t.Errorf("LocalName = %q, want %q", got, want)
		}
		if got, want := sdkgo.Deref(ptr), golang.Deref(ptr); got != want || got == ptr {
			t.Errorf("Deref = %v, want %v (and not the pointer itself)", got, want)
		}
		// Structurally: a Ref is an interface over a freshly built
		// value, so identity would compare the two allocations rather
		// than the two answers.
		if got, want := sdkgo.ElemType(ptr), golang.ElemType(ptr); !reflect.DeepEqual(got, want) {
			t.Errorf("ElemType = %v, want %v", got, want)
		}
		if got, want := sdkgo.FromNode(ctx), golang.FromNode(ctx); !reflect.DeepEqual(got, want) {
			t.Errorf("FromNode = %v, want %v", got, want)
		}
	})

	t.Run("the predicates answer per kind", func(t *testing.T) {
		t.Parallel()
		// Each is checked on a type it holds for and one it does not,
		// so a forwarder wired to a neighbouring predicate fails rather
		// than agreeing by accident on a single input.
		for _, c := range []struct {
			name    string
			fn, ref func(*sdk.TypeRef) bool
			yes, no *sdk.TypeRef
		}{
			{"IsContext", sdkgo.IsContext, golang.IsContext, ctx, builtin("string")},
			{"IsString", sdkgo.IsString, golang.IsString, builtin("string"), builtin("int")},
			{"IsInteger", sdkgo.IsInteger, golang.IsInteger, builtin("int"), builtin("string")},
			{"IsFloat", sdkgo.IsFloat, golang.IsFloat, builtin("float64"), builtin("int")},
			{"Nilable", sdkgo.Nilable, golang.Nilable, ptr, builtin("int")},
		} {
			if !c.fn(c.yes) {
				t.Errorf("%s did not hold for the type it should", c.name)
			}
			if c.fn(c.no) {
				t.Errorf("%s held for the type it should not", c.name)
			}
			if c.fn(c.yes) != c.ref(c.yes) || c.fn(c.no) != c.ref(c.no) {
				t.Errorf("%s disagrees with the language package", c.name)
			}
		}
	})

	t.Run("the comparability walk carries its vocabulary", func(t *testing.T) {
		t.Parallel()
		// Without the re-exported reasons a caller reads the walk's
		// answer by naming golang.NotLoaded, and the import it was
		// spared comes straight back.
		got, problems := sdkgo.ComparableDeep(named("elsewhere", "Opaque"), resolverTable{})
		want, wantProblems := golang.ComparableDeep(named("elsewhere", "Opaque"), resolverTable{})
		if got != want || len(problems) != len(wantProblems) {
			t.Fatalf("ComparableDeep = %v/%d, want %v/%d",
				got, len(problems), want, len(wantProblems))
		}
		if len(problems) == 0 {
			t.Fatal("the unresolvable type reported no problem, so nothing was proved")
		}
		if problems[0].Reason != sdkgo.NotLoaded {
			t.Errorf("Reason = %q, want %q", problems[0].Reason, sdkgo.NotLoaded)
		}
	})

	t.Run("the sequence projection carries its vocabulary", func(t *testing.T) {
		t.Parallel()
		m := &sdk.Method{Name: "All"}
		if got, want := sdkgo.SequenceOf(m), golang.SequenceOf(m); got != want {
			t.Fatalf("SequenceOf = %+v, want %+v", got, want)
		}
		if sdkgo.SequenceOf(m).Kind != sdkgo.NotIterator {
			t.Errorf("a method returning nothing is not NotIterator")
		}
	})

	t.Run("the conventions forward", func(t *testing.T) {
		t.Parallel()
		subject, prefixed := sdkgo.SentinelSubject("ErrNotFound")
		wantSubject, wantPrefixed := golang.SentinelSubject("ErrNotFound")
		if subject != wantSubject || prefixed != wantPrefixed {
			t.Errorf("SentinelSubject = %q, %v; want %q, %v",
				subject, prefixed, wantSubject, wantPrefixed)
		}
		if !prefixed {
			t.Error("ErrNotFound did not read as prefixed, so the comparison proved little")
		}
		returns := []*sdk.Return{{Name: "out", Type: builtin("string")}}
		if got, want := sdkgo.NamedReturnsUsable(returns), golang.NamedReturnsUsable(returns); got != want {
			t.Errorf("NamedReturnsUsable = %v, want %v", got, want)
		}
	})
}
