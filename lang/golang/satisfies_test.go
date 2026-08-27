// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"strconv"
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

func TestSatisfies(t *testing.T) {
	t.Parallel()

	want := []*node.Method{
		method("Get", []*node.TypeRef{builtinRef("string")},
			[]*node.TypeRef{builtinRef("string"), errorRef()}),
		method("Close", nil, []*node.TypeRef{errorRef()}),
	}

	t.Run("a matching set satisfies", func(t *testing.T) {
		t.Parallel()
		ok, missing := golang.Satisfies(want, want)
		if !ok || len(missing) != 0 {
			t.Fatalf("Satisfies = %v, %+v", ok, missing)
		}
	})

	t.Run("names an absent method", func(t *testing.T) {
		t.Parallel()
		ok, missing := golang.Satisfies(want[:1], want)
		if ok || len(missing) != 1 || missing[0].Name != "Close" {
			t.Fatalf("Satisfies = %v, %+v", ok, missing)
		}
		if missing[0].Declared != nil {
			t.Fatalf("an absent method must declare nothing")
		}
	})

	t.Run("distinguishes wrong signature from absent", func(t *testing.T) {
		t.Parallel()
		// A missing method and a method of the wrong signature are
		// different mistakes, and the second is the one an author reads
		// twice before seeing.
		wrong := []*node.Method{
			method("Get", []*node.TypeRef{builtinRef("int")},
				[]*node.TypeRef{builtinRef("string"), errorRef()}),
			want[1],
		}
		ok, missing := golang.Satisfies(wrong, want)
		if ok || len(missing) != 1 {
			t.Fatalf("Satisfies = %v, %+v", ok, missing)
		}
		if missing[0].Declared == nil {
			t.Fatalf("a wrong-signature method must be reported as declared")
		}
		if missing[0].Want == nil {
			t.Fatalf("the wanted declaration must travel with the failure")
		}
	})

	t.Run("extra methods do not prevent satisfaction", func(t *testing.T) {
		t.Parallel()
		// Go's rule: a type satisfies an interface by having at least
		// its method set.
		extra := append([]*node.Method{method("Extra", nil, nil)}, want...)
		if ok, _ := golang.Satisfies(extra, want); !ok {
			t.Fatalf("extra methods must not prevent satisfaction")
		}
	})

	t.Run("an empty interface is satisfied by anything", func(t *testing.T) {
		t.Parallel()
		if ok, _ := golang.Satisfies(nil, nil); !ok {
			t.Fatalf("the empty interface must be satisfied")
		}
	})
}

func TestSameSignature(t *testing.T) {
	t.Parallel()

	t.Run("ignores parameter and return names", func(t *testing.T) {
		t.Parallel()
		// A parameter's identifier and a return's binding name are
		// documentation; two methods differing only in them are the
		// same method as far as satisfaction goes.
		a := &node.Method{
			Name:    "Get",
			Params:  []*node.Param{{Name: "id", Type: builtinRef("string")}},
			Returns: []*node.Return{{Name: "item", Type: builtinRef("string")}},
		}
		b := &node.Method{
			Name:    "Get",
			Params:  []*node.Param{{Type: builtinRef("string")}},
			Returns: []*node.Return{{Type: builtinRef("string")}},
		}
		if !golang.SameSignature(a, b) {
			t.Fatalf("SameSignature = false; names must not matter")
		}
	})

	t.Run("does not ignore the variadic marker", func(t *testing.T) {
		t.Parallel()
		// `Print(...string)` and `Print([]string)` are different
		// methods that a name-and-type comparison would conflate.
		variadic := &node.Method{Name: "Print", Params: []*node.Param{
			{Name: "a", Type: builtinRef("string"), Variadic: true},
		}}
		fixed := &node.Method{Name: "Print", Params: []*node.Param{
			{Name: "a", Type: builtinRef("string")},
		}}
		if golang.SameSignature(variadic, fixed) {
			t.Fatalf("SameSignature ignored the variadic marker")
		}
	})

	t.Run("arity differences are caught", func(t *testing.T) {
		t.Parallel()
		a := method("F", []*node.TypeRef{builtinRef("int")}, nil)
		b := method("F", nil, nil)
		if golang.SameSignature(a, b) {
			t.Fatalf("SameSignature ignored an arity difference")
		}
	})

	t.Run("nil compares only to nil", func(t *testing.T) {
		t.Parallel()
		if !golang.SameSignature(nil, nil) {
			t.Fatalf("SameSignature(nil, nil) = false")
		}
		if golang.SameSignature(nil, method("F", nil, nil)) {
			t.Fatalf("SameSignature(nil, m) = true")
		}
	})
}

func TestUnderlyingOf(t *testing.T) {
	t.Parallel()

	t.Run("resolves a defined type to its base", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"x.Weekday": &node.Alias{
			Name: "Weekday", Package: "x", Target: builtinRef("int"),
		}}
		got := golang.UnderlyingOf(namedTypeRef("x", "Weekday"), r)
		if got == nil || got.Name != "int" {
			t.Fatalf("UnderlyingOf = %v, want int", got)
		}
	})

	t.Run("follows a chain of defined types", func(t *testing.T) {
		t.Parallel()
		// `type A B` with `type B int` underlies int.
		r := mapResolver{
			"x.A": &node.Alias{Name: "A", Package: "x", Target: namedTypeRef("x", "B")},
			"x.B": &node.Alias{Name: "B", Package: "x", Target: builtinRef("int")},
		}
		got := golang.UnderlyingOf(namedTypeRef("x", "A"), r)
		if got == nil || got.Name != "int" {
			t.Fatalf("UnderlyingOf = %v, want int", got)
		}
	})

	t.Run("a struct underlies itself", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"x.User": &node.Struct{Name: "User", Package: "x"}}
		got := golang.UnderlyingOf(namedTypeRef("x", "User"), r)
		if got == nil || got.Name != "User" {
			t.Fatalf("UnderlyingOf = %v, want User", got)
		}
	})

	t.Run("a builtin underlies itself", func(t *testing.T) {
		t.Parallel()
		in := builtinRef("int")
		if golang.UnderlyingOf(in, mapResolver{}) != in {
			t.Fatalf("a builtin must underlie itself")
		}
	})

	t.Run("an unreachable type has no underlying", func(t *testing.T) {
		t.Parallel()
		// A smaller answer rather than a wrong one.
		if got := golang.UnderlyingOf(namedTypeRef("x", "Opaque"), mapResolver{}); got != nil {
			t.Fatalf("UnderlyingOf = %v, want nil", got)
		}
	})

	t.Run("a composite is returned unchanged", func(t *testing.T) {
		t.Parallel()
		s := sliceRef(builtinRef("int"))
		if golang.UnderlyingOf(s, mapResolver{}) != s {
			t.Fatalf("a composite must be returned unchanged")
		}
	})
}

func TestComparableDeep(t *testing.T) {
	t.Parallel()

	t.Run("the never-comparable kinds are refused", func(t *testing.T) {
		t.Parallel()
		for name, ref := range map[string]*node.TypeRef{
			"slice": sliceRef(builtinRef("int")),
			"map":   mapRef(builtinRef("string"), builtinRef("int")),
			"func":  {TypeKind: node.TypeRefFunc},
		} {
			ok, problems := golang.ComparableDeep(ref, mapResolver{})
			if ok || len(problems) != 0 {
				t.Errorf("ComparableDeep(%s) = %v, %v", name, ok, len(problems) == 0)
			}
		}
	})

	t.Run("a pointer compares by identity", func(t *testing.T) {
		t.Parallel()
		// Whatever it points at, so the element is not walked.
		p := &node.TypeRef{TypeKind: node.TypeRefPointer, Elem: sliceRef(builtinRef("int"))}
		if ok, problems := golang.ComparableDeep(p, mapResolver{}); !ok || len(problems) != 0 {
			t.Fatalf("ComparableDeep(pointer) = %v, %v", ok, len(problems) == 0)
		}
	})

	t.Run("sees a slice field inside a struct", func(t *testing.T) {
		t.Parallel()
		// The answer Keyable cannot give, because it reads the
		// reference alone.
		bad := &node.Struct{Name: "Bad", Package: "x", Fields: []*node.Field{
			field("Tags", sliceRef(builtinRef("string"))),
		}}
		ok, problems := golang.ComparableDeep(namedTypeRef("x", "Bad"), mapResolver{"x.Bad": bad})
		if ok || len(problems) != 0 {
			t.Fatalf("ComparableDeep = %v, %v; want a definite refusal", ok, len(problems) == 0)
		}
		if !golang.Keyable(namedTypeRef("x", "Bad")) {
			t.Fatalf("Keyable is expected to be the shallower, permissive answer")
		}
	})

	t.Run("a struct of comparable fields is comparable", func(t *testing.T) {
		t.Parallel()
		good := &node.Struct{Name: "Good", Package: "x", Fields: []*node.Field{
			field("ID", builtinRef("string")), field("N", builtinRef("int")),
		}}
		ok, problems := golang.ComparableDeep(
			namedTypeRef("x", "Good"),
			mapResolver{"x.Good": good},
		)
		if !ok || len(problems) != 0 {
			t.Fatalf("ComparableDeep = %v, %v", ok, len(problems) == 0)
		}
	})

	t.Run("an array follows its element", func(t *testing.T) {
		t.Parallel()
		arr := &node.TypeRef{
			TypeKind: node.TypeRefArray, ArrayLen: 2, Elem: sliceRef(builtinRef("int")),
		}
		if ok, _ := golang.ComparableDeep(arr, mapResolver{}); ok {
			t.Fatalf("an array of slices must not be comparable")
		}
	})

	t.Run("an unreachable type is unknown, not comparable", func(t *testing.T) {
		t.Parallel()
		// A caller must not read the first result: emitting a map keyed
		// on it produces a compile error in the consumer's build.
		ok, problems := golang.ComparableDeep(namedTypeRef("x", "Opaque"), mapResolver{})
		if ok || len(problems) == 0 {
			t.Fatalf("ComparableDeep = %v, %v; want unknown", ok, len(problems) == 0)
		}
	})

	t.Run("a definite refusal beats an unknown sibling", func(t *testing.T) {
		t.Parallel()
		// The stronger answer wins: one uncomparable field settles it
		// whatever the rest resolve to.
		mixed := &node.Struct{Name: "Mixed", Package: "x", Fields: []*node.Field{
			field("Opaque", namedTypeRef("x", "Missing")),
			field("Tags", sliceRef(builtinRef("string"))),
		}}
		ok, problems := golang.ComparableDeep(
			namedTypeRef("x", "Mixed"),
			mapResolver{"x.Mixed": mixed},
		)
		if ok || len(problems) != 0 {
			t.Fatalf("ComparableDeep = %v, %v; want a definite refusal", ok, len(problems) == 0)
		}
	})

	t.Run("a self-referential struct terminates", func(t *testing.T) {
		t.Parallel()
		loop := &node.Struct{Name: "Node", Package: "x"}
		loop.Fields = []*node.Field{
			field(
				"Next",
				&node.TypeRef{TypeKind: node.TypeRefPointer, Elem: namedTypeRef("x", "Node")},
			),
		}
		_, problems := golang.ComparableDeep(namedTypeRef("x", "Node"), mapResolver{"x.Node": loop})
		if len(problems) != 0 {
			t.Fatalf("a self-referential struct must still resolve")
		}
	})

	t.Run("nil is unknown", func(t *testing.T) {
		t.Parallel()
		if ok, problems := golang.ComparableDeep(nil, mapResolver{}); ok || len(problems) == 0 {
			t.Fatalf("ComparableDeep(nil) = %v, %v", ok, len(problems) == 0)
		}
	})
}

func TestRecommendedReceiver(t *testing.T) {
	t.Parallel()

	ptrRecv := &node.Method{Name: "Set", Receiver: &node.TypeRef{
		TypeKind: node.TypeRefPointer, Elem: namedTypeRef("x", "User"),
	}}
	valRecv := &node.Method{Name: "Get", Receiver: namedTypeRef("x", "User")}

	t.Run("one pointer method makes the whole set pointer", func(t *testing.T) {
		t.Parallel()
		// Go's consistency rule: the method set is then the same
		// through both forms, and a value is never a partial
		// implementation.
		if !golang.RecommendedReceiver([]*node.Method{valRecv, ptrRecv}) {
			t.Fatalf("RecommendedReceiver = false")
		}
	})

	t.Run("an all-value set stays value", func(t *testing.T) {
		t.Parallel()
		if golang.RecommendedReceiver([]*node.Method{valRecv}) {
			t.Fatalf("RecommendedReceiver = true for an all-value set")
		}
	})

	t.Run("an empty set stays value", func(t *testing.T) {
		t.Parallel()
		if golang.RecommendedReceiver(nil) {
			t.Fatalf("RecommendedReceiver(nil) = true")
		}
	})

	t.Run("reads the declaration rather than a stamp", func(t *testing.T) {
		t.Parallel()
		// What a graph with no frontend behind it can still answer.
		if !golang.ReceiverIsPointerDecl(ptrRecv) {
			t.Fatalf("ReceiverIsPointerDecl = false")
		}
		if golang.ReceiverIsPointerDecl(valRecv) || golang.ReceiverIsPointerDecl(nil) {
			t.Fatalf("ReceiverIsPointerDecl must require a pointer receiver")
		}
	})
}

func TestSatisfiesEdges(t *testing.T) {
	t.Parallel()

	t.Run("nil entries on either side are skipped", func(t *testing.T) {
		t.Parallel()
		// A malformed graph can carry one, and a nil interface method
		// demands nothing.
		want := []*node.Method{nil, method("Close", nil, []*node.TypeRef{errorRef()})}
		have := []*node.Method{nil, method("Close", nil, []*node.TypeRef{errorRef()})}
		if ok, missing := golang.Satisfies(have, want); !ok {
			t.Fatalf("Satisfies = false, %+v", missing)
		}
	})

	t.Run("a nil parameter compares only to nil", func(t *testing.T) {
		t.Parallel()
		withNil := &node.Method{Name: "F", Params: []*node.Param{nil}}
		withType := &node.Method{Name: "F", Params: []*node.Param{{Type: builtinRef("int")}}}
		if golang.SameSignature(withNil, withType) {
			t.Fatalf("SameSignature matched a nil parameter against a typed one")
		}
		if !golang.SameSignature(withNil, &node.Method{Name: "F", Params: []*node.Param{nil}}) {
			t.Fatalf("two nil parameters must compare equal")
		}
	})

	t.Run("a nil return compares only to nil", func(t *testing.T) {
		t.Parallel()
		withNil := &node.Method{Name: "F", Returns: []*node.Return{nil}}
		withType := &node.Method{Name: "F", Returns: []*node.Return{{Type: errorRef()}}}
		if golang.SameSignature(withNil, withType) {
			t.Fatalf("SameSignature matched a nil return against a typed one")
		}
		if !golang.SameSignature(withNil, &node.Method{Name: "F", Returns: []*node.Return{nil}}) {
			t.Fatalf("two nil returns must compare equal")
		}
	})

	t.Run("a return-count difference is caught", func(t *testing.T) {
		t.Parallel()
		a := method("F", nil, []*node.TypeRef{errorRef()})
		if golang.SameSignature(a, method("F", nil, nil)) {
			t.Fatalf("SameSignature ignored a return-count difference")
		}
	})

	t.Run("nil underlies nothing", func(t *testing.T) {
		t.Parallel()
		if got := golang.UnderlyingOf(nil, mapResolver{}); got != nil {
			t.Fatalf("UnderlyingOf(nil) = %v", got)
		}
	})

	t.Run("an alias with no target underlies itself", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"x.A": &node.Alias{Name: "A", Package: "x"}}
		got := golang.UnderlyingOf(namedTypeRef("x", "A"), r)
		if got == nil || got.Name != "A" {
			t.Fatalf("UnderlyingOf = %v, want A", got)
		}
	})

	t.Run("no resolver leaves the type unchanged", func(t *testing.T) {
		t.Parallel()
		in := namedTypeRef("x", "Weekday")
		if golang.UnderlyingOf(in, nil) != in {
			t.Fatalf("UnderlyingOf with no resolver must return the input")
		}
	})

	t.Run("an interface and an enum compare", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{
			"io.Reader": &node.Interface{Name: "Reader", Package: "io"},
			"x.Status":  &node.Enum{Name: "Status", Package: "x"},
		}
		for name, ref := range map[string]*node.TypeRef{
			"interface": namedTypeRef("io", "Reader"),
			"enum":      namedTypeRef("x", "Status"),
		} {
			if ok, problems := golang.ComparableDeep(ref, r); !ok || len(problems) != 0 {
				t.Errorf("ComparableDeep(%s) = %v, %v", name, ok, len(problems) == 0)
			}
		}
	})

	t.Run("an alias is followed", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"x.Tags": &node.Alias{
			Name: "Tags", Package: "x", Target: sliceRef(builtinRef("string")),
		}}
		if ok, _ := golang.ComparableDeep(namedTypeRef("x", "Tags"), r); ok {
			t.Fatalf("an alias to a slice must not be comparable")
		}
	})

	t.Run("a non-type declaration is unknown", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"x.F": &node.Function{Name: "F", Package: "x"}}
		ok, problems := golang.ComparableDeep(namedTypeRef("x", "F"), r)
		if ok || len(problems) == 0 {
			t.Fatalf("ComparableDeep = %v, %d problem(s); want unknown", ok, len(problems))
		}
	})

	t.Run("no resolver makes a named type unknown", func(t *testing.T) {
		t.Parallel()
		ok, problems := golang.ComparableDeep(namedTypeRef("x", "User"), nil)
		if ok || len(problems) == 0 {
			t.Fatalf("ComparableDeep = %v, %d problem(s); want unknown", ok, len(problems))
		}
	})

	t.Run("an anonymous struct is walked in place", func(t *testing.T) {
		t.Parallel()
		st := &node.TypeRef{TypeKind: node.TypeRefAnonStruct, Fields: []*node.Field{
			nil, field("Tags", sliceRef(builtinRef("string"))),
		}}
		if ok, problems := golang.ComparableDeep(st, mapResolver{}); ok || len(problems) != 0 {
			t.Fatalf("ComparableDeep = %v, %v; want a definite refusal", ok, len(problems) == 0)
		}
	})

	t.Run("a type parameter compares", func(t *testing.T) {
		t.Parallel()
		p := &node.TypeRef{TypeKind: node.TypeRefTypeParam, Name: "T"}
		if ok, problems := golang.ComparableDeep(p, mapResolver{}); !ok || len(problems) != 0 {
			t.Fatalf("ComparableDeep(T) = %v, %v", ok, len(problems) == 0)
		}
	})
}

// TestComparableDeepAnswersForCuratedStdlibTypes pins the table that
// closes the gap the resolver cannot: the standard library is never
// among the declarations a run loaded, so a struct holding one of
// these came back undetermined and every comparison it was party to
// was refused.
func TestComparableDeepAnswersForCuratedStdlibTypes(t *testing.T) {
	t.Parallel()

	t.Run("a struct holding a stdlib scalar is comparable", func(t *testing.T) {
		t.Parallel()
		// The reported case. Entry is comparable in Go — both fields
		// are — and the walk used to stop at time.Duration and report
		// the whole struct undetermined.
		entry := &node.Struct{
			Name: "Entry", Package: "kv",
			Fields: []*node.Field{
				field("Key", builtinRef("string")),
				field("Lifetime", namedTypeRef("time", "Duration")),
			},
		}
		r := mapResolver{"kv.Entry": entry}
		ok, problems := golang.ComparableDeep(namedTypeRef("kv", "Entry"), r)
		if !ok || len(problems) != 0 {
			t.Fatalf("ComparableDeep(kv.Entry) = %v, %d problems; want true and none", ok, len(problems))
		}
	})

	t.Run("a curated non-comparable is a verdict, not a gap", func(t *testing.T) {
		t.Parallel()
		// The direction that matters as much: bytes.Buffer holds a
		// []byte, so the answer is a definite no. Reporting it as
		// undetermined would leave a caller unable to tell a type it
		// must not compare from one it could not see.
		buf := &node.Struct{
			Name: "Buf", Package: "kv",
			Fields: []*node.Field{field("B", namedTypeRef("bytes", "Buffer"))},
		}
		r := mapResolver{"kv.Buf": buf}
		ok, problems := golang.ComparableDeep(namedTypeRef("kv", "Buf"), r)
		if ok || len(problems) != 0 {
			t.Fatalf("ComparableDeep(kv.Buf) = %v, %d problems; want false and none", ok, len(problems))
		}
	})

	t.Run("the table answers without a resolver", func(t *testing.T) {
		t.Parallel()
		// Consulted before the resolver gate, so a caller holding no
		// graph still gets the answer rather than NoResolver.
		if ok, problems := golang.ComparableDeep(namedTypeRef("time", "Time"), nil); !ok || len(problems) != 0 {
			t.Fatalf("ComparableDeep(time.Time, nil) = %v, %d problems", ok, len(problems))
		}
	})

	t.Run("an uncurated stdlib type is still undetermined", func(t *testing.T) {
		t.Parallel()
		// The table is curated, not a claim about the whole standard
		// library: a name nobody checked reports NotLoaded rather than
		// guessing, which is what keeps each entry an assertion.
		ok, problems := golang.ComparableDeep(namedTypeRef("net/url", "URL"), mapResolver{})
		if ok || len(problems) != 1 || problems[0].Reason != golang.NotLoaded {
			t.Fatalf("ComparableDeep(url.URL) = %v, %v; want undetermined", ok, problems)
		}
	})
}

func TestComparableDeepReportsWhatItCouldNotReach(t *testing.T) {
	t.Parallel()

	t.Run("names the type the walk stopped at", func(t *testing.T) {
		t.Parallel()
		// A caller that can only say "comparability is undetermined"
		// leaves the author to find which of a struct's fields was the
		// problem.
		//
		// Named against a package no run loads rather than a
		// standard-library one: the curated table answers for those,
		// and pinning this on an entry it does not carry yet would
		// break the day someone adds it.
		_, problems := golang.ComparableDeep(namedTypeRef("elsewhere", "Opaque"), mapResolver{})
		if len(problems) != 1 || problems[0].Written != "elsewhere.Opaque" {
			t.Fatalf("problems = %+v, want one naming elsewhere.Opaque", problems)
		}
		if problems[0].Reason != golang.NotLoaded {
			t.Fatalf("reason = %q, want not-loaded", problems[0].Reason)
		}
	})

	t.Run("distinguishes a missing resolver from a missing type", func(t *testing.T) {
		t.Parallel()
		// The same graph answers in full once a resolver is passed, so
		// the caller's remedy is different.
		_, problems := golang.ComparableDeep(namedTypeRef("x", "User"), nil)
		if len(problems) != 1 || problems[0].Reason != golang.NoResolver {
			t.Fatalf("problems = %+v, want no-resolver", problems)
		}
	})

	t.Run("collects every unreachable field rather than the first", func(t *testing.T) {
		t.Parallel()
		// The author has to make all of them reachable; reporting one
		// per run turns that into as many runs as there are fields.
		s := &node.Struct{
			Name: "Mixed", Package: "x",
			Fields: []*node.Field{
				{Name: "At", Type: namedTypeRef("elsewhere", "Opaque")},
				{Name: "D", Type: namedTypeRef("faraway", "Other")},
			},
		}
		r := mapResolver{"x.Mixed": s}

		_, problems := golang.ComparableDeep(namedTypeRef("x", "Mixed"), r)
		if len(problems) != 2 {
			t.Fatalf("problems = %+v, want one per unreachable field", problems)
		}
	})

	t.Run("reports a chain past the budget", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{}
		for i := range 14 {
			name, next := "L"+strconv.Itoa(i), "L"+strconv.Itoa(i+1)
			r["x."+name] = &node.Alias{
				Name: name, Package: "x", Target: namedTypeRef("x", next),
			}
		}
		_, problems := golang.ComparableDeep(namedTypeRef("x", "L0"), r)
		if len(problems) != 1 || problems[0].Reason != golang.TooDeep {
			t.Fatalf("problems = %+v, want too-deep", problems)
		}
	})
}

// TestComparableDeep_InterfacesAndCycles covers the two arms that
// answer true without walking further.
func TestComparableDeep_InterfacesAndCycles(t *testing.T) {
	t.Parallel()

	t.Run("an anonymous interface is comparable", func(t *testing.T) {
		t.Parallel()
		// Reported comparable because the code compiles: refusing would
		// rule out every interface-keyed map Go admits, and whether the
		// dynamic type compares is not knowable here.
		iface := &node.TypeRef{TypeKind: node.TypeRefAnonInterface}
		got, problems := golang.ComparableDeep(iface, mapResolver{})
		if len(problems) != 0 || !got {
			t.Errorf("ComparableDeep(interface{}) = %v, %v; want true with no problems",
				got, problems)
		}
	})

	t.Run("a self-referential type terminates and reports comparable", func(t *testing.T) {
		t.Parallel()
		// The cycle guard: revisiting a type already on the walk answers
		// true rather than recursing, so a linked structure does not
		// exhaust the budget on its way to the same answer.
		self := &node.Struct{Name: "Node", Package: "x"}
		self.Fields = []*node.Field{field("Next", namedTypeRef("x", "Node"))}
		got, problems := golang.ComparableDeep(namedTypeRef("x", "Node"),
			mapResolver{"x.Node": self})
		if len(problems) != 0 {
			t.Fatalf("ComparableDeep problems = %v", problems)
		}
		if !got {
			t.Error("a self-referential struct of comparable fields reported not comparable")
		}
	})
}
