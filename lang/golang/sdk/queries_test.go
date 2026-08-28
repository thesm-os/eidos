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
		// Display drops the path where QName keeps it — the pair a
		// diagnostic and a comparison want respectively, and the
		// reason forwarding one alone was a gap.
		qualified := named("example.com/store", "User")
		if got, want := sdkgo.Display(qualified), golang.Display(qualified); got != want {
			t.Errorf("Display = %q, want %q", got, want)
		}
		if got := sdkgo.Display(qualified); got != "store.User" {
			t.Errorf("Display = %q, want the author's spelling store.User", got)
		}
		if sdkgo.QName(qualified) == sdkgo.Display(qualified) {
			t.Error("Display and QName agree, so the forwarded pair proves nothing")
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

// TestSamplePartForwards pins the façade against the language package
// for the element-position sample rule.
func TestSamplePartForwards(t *testing.T) {
	t.Parallel()

	// One sample per arm, each pinned to the kind its arm produces —
	// so a forwarder wired to something that happens to agree on one
	// shape still fails on another, and agreement on a wrong answer
	// cannot pass as forwarding.
	for name, tc := range map[string]struct {
		s    sdk.Sample
		kind sdk.ExprKind
	}{
		"typed text": {sdk.Sample{Ref: sdk.Builtin("int64"), Text: "42"}, sdk.ExprRaw},
		"composite":  {sdk.Sample{Ref: sdk.Builtin("Point"), Text: "{X: 42}", Composite: true}, sdk.ExprComposite},
		"bare text":  {sdk.Sample{Text: `"reader"`}, sdk.ExprRaw},
	} {
		got := sdkgo.SamplePart(tc.s)
		if want := golang.SamplePart(tc.s); !reflect.DeepEqual(got, want) {
			t.Errorf("SamplePart(%s) = %+v, want %+v", name, got, want)
		}
		if got.ExprKind != tc.kind {
			t.Errorf("SamplePart(%s).ExprKind = %v, want %v", name, got.ExprKind, tc.kind)
		}
	}
}

// TestMemberWalkForwards pins the forwarders that complete the walk
// a generator makes from a struct to a composed literal: pick the
// members a generated file can name, bind a generic field at this
// reference's arguments, sample it. StructOf and SamplePart — the
// steps either side — are pinned above, and the gap this closes is
// that three of the five steps were reachable through the façade and
// two were not.
func TestMemberWalkForwards(t *testing.T) {
	t.Parallel()

	t.Run("IsExported answers per identifier", func(t *testing.T) {
		t.Parallel()
		for name, want := range map[string]bool{"Lifetime": true, "secret": false} {
			if got := sdkgo.IsExported(name); got != want || got != golang.IsExported(name) {
				t.Errorf("IsExported(%q) = %v, want %v and agreement", name, got, want)
			}
		}
	})

	t.Run("BindTypeArgs binds a reference's arguments", func(t *testing.T) {
		t.Parallel()
		// Filter[string] naming `type Filter[T any] func(T) bool`:
		// the body's T becomes string, which is what a walk into a
		// generic declaration reads instead of a parameter that only
		// exists inside it.
		param := &sdk.TypeParam{Name: "T"}
		body := &sdk.TypeRef{
			TypeKind:    sdk.TypeRefFunc,
			FuncParams:  []*sdk.TypeRef{{TypeKind: sdk.TypeRefTypeParam, Name: "T"}},
			FuncReturns: []*sdk.TypeRef{{TypeKind: sdk.TypeRefNamed, Name: "bool"}},
		}
		args := []*sdk.TypeRef{{TypeKind: sdk.TypeRefNamed, Name: "string"}}

		got := sdkgo.BindTypeArgs(body, []*sdk.TypeParam{param}, args)
		want := golang.BindTypeArgs(body, []*sdk.TypeParam{param}, args)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("BindTypeArgs = %+v, want %+v", got, want)
		}
		if len(got.FuncParams) != 1 || got.FuncParams[0].Name != "string" {
			t.Fatalf("bound body = %+v, want the T parameter bound to string", got)
		}
	})

	t.Run("SampleRefFor answers the pair SamplePart consumes", func(t *testing.T) {
		t.Parallel()
		s, a := sdkgo.SampleRefFor(named("", "int64"), "Lifetime", resolverTable{})
		ws, wa := golang.SampleRefFor(named("", "int64"), "Lifetime", resolverTable{})
		if !reflect.DeepEqual(s, ws) || !reflect.DeepEqual(a, wa) {
			t.Fatalf("SampleRefFor = %+v/%+v, want %+v/%+v", s, a, ws, wa)
		}
		if !s.OK() || !a.OK() || s.Text == a.Text {
			t.Fatalf("pair = %q/%q, want two distinct usable samples", s.Text, a.Text)
		}
		// And the composition the issue named: the façade now carries
		// both halves, producer to part.
		if part := sdkgo.SamplePart(s); part.ExprKind != sdk.ExprRaw {
			t.Fatalf("SamplePart(sample) = %+v, want the raw element form", part)
		}
	})

	t.Run("Source is the language's rules value", func(t *testing.T) {
		t.Parallel()
		// The alias makes `Source{}` the whole spelling; what matters
		// is that the value satisfies the neutral contract a test
		// passes it as.
		var rules sdk.SourceRules = sdkgo.Source{}
		if _, ok := rules.(golang.Source); !ok {
			t.Fatal("Source{} is not the language package's rules value")
		}
	})
}
