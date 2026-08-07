// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// bounded returns a type-parameter list whose entries carry the
// given constraint spellings.
func bounded(raws ...string) []*node.TypeParam {
	names := []string{"T", "K", "V", "W"}
	out := make([]*node.TypeParam, 0, len(raws))
	for i, raw := range raws {
		out = append(out, &node.TypeParam{
			Name:       names[i],
			Constraint: &node.Constraint{Raw: raw},
		})
	}
	return out
}

func TestWitnesses(t *testing.T) {
	t.Parallel()

	t.Run("derives one concrete type per parameter", func(t *testing.T) {
		t.Parallel()
		got := golang.Witnesses(bounded("any", "comparable"))
		if len(got) != 2 {
			t.Fatalf("Witnesses = %v, want two", got)
		}
	})

	t.Run("the witnesses are pairwise distinct", func(t *testing.T) {
		t.Parallel()
		// So a helper that crossed two parameters produces code that
		// does not compile rather than code that type-checks and
		// asserts the wrong thing.
		got := golang.Witnesses(bounded("any", "any", "any"))
		seen := map[string]bool{}
		for _, r := range got {
			b := r.(*emit.BuiltinRef)
			if seen[b.Name] {
				t.Fatalf("Witnesses reused %q", b.Name)
			}
			seen[b.Name] = true
		}
	})

	t.Run("one unreasonable constraint kills the whole list", func(t *testing.T) {
		t.Parallel()
		// An entry point instantiates the list at once: a witness for
		// one parameter is worth nothing without one for the rest.
		if got := golang.Witnesses(bounded("any", "constraints.Ordered")); got != nil {
			t.Fatalf("Witnesses = %v, want nil", got)
		}
	})

	t.Run("more parameters than palette entries yields none", func(t *testing.T) {
		t.Parallel()
		// Wrapping the list would hand two parameters the same type
		// and lose exactly the distinctness property.
		raws := make([]string, 0, len(golang.WitnessPalette)+1)
		for range len(golang.WitnessPalette) + 1 {
			raws = append(raws, "any")
		}
		bigger := make([]*node.TypeParam, 0, len(raws))
		for i := range raws {
			bigger = append(bigger, &node.TypeParam{
				Name:       "P" + string(rune('a'+i)),
				Constraint: &node.Constraint{Raw: "any"},
			})
		}
		if got := golang.Witnesses(bigger); got != nil {
			t.Fatalf("Witnesses = %v, want nil beyond the palette", got)
		}
	})

	t.Run("a non-generic declaration has none", func(t *testing.T) {
		t.Parallel()
		if got := golang.Witnesses(nil); got != nil {
			t.Fatalf("Witnesses(nil) = %v, want nil", got)
		}
	})

	t.Run("renders the use spelling", func(t *testing.T) {
		t.Parallel()
		if got := golang.WitnessUse(bounded("any", "comparable")); got != "[string, int]" {
			t.Fatalf("WitnessUse = %q", got)
		}
	})

	t.Run("no witnesses render nothing, so a template appends unconditionally", func(t *testing.T) {
		t.Parallel()
		if got := golang.WitnessUse(bounded("constraints.Ordered")); got != "" {
			t.Fatalf("WitnessUse = %q, want empty", got)
		}
	})
}

func TestAdmitsAnyBasicType(t *testing.T) {
	t.Parallel()

	t.Run("accepts the two knowable bounds", func(t *testing.T) {
		t.Parallel()
		// The set is closed to the bounds whose type set is known
		// without loading anything.
		for _, raw := range []string{"any", "interface{}", "comparable"} {
			if !golang.AdmitsAnyBasicType(&node.Constraint{Raw: raw}) {
				t.Errorf("AdmitsAnyBasicType(%q) = false", raw)
			}
		}
	})

	t.Run("an unbounded parameter admits everything", func(t *testing.T) {
		t.Parallel()
		if !golang.AdmitsAnyBasicType(nil) {
			t.Fatalf("AdmitsAnyBasicType(nil) = false")
		}
	})

	t.Run("a named constraint is a reference this cannot read", func(t *testing.T) {
		t.Parallel()
		// It names a package the generator never loaded, so a subject
		// bounded by one takes its witnesses from the source or not at
		// all.
		if golang.AdmitsAnyBasicType(&node.Constraint{Raw: "constraints.Ordered"}) {
			t.Fatalf("a named constraint must not be reasoned about")
		}
	})
}

func TestSubstituteTypeParams(t *testing.T) {
	t.Parallel()

	by := map[string]emit.Ref{"T": emit.Builtin("string")}

	t.Run("rewrites a bare parameter", func(t *testing.T) {
		t.Parallel()
		// A type parameter reaches the emit layer as a bare name,
		// which is the same shape a builtin takes — the binding map is
		// what tells them apart.
		got := golang.SubstituteTypeParams(emit.Builtin("T"), by)
		if b, ok := got.(*emit.BuiltinRef); !ok || b.Name != "string" {
			t.Fatalf("SubstituteTypeParams = %#v", got)
		}
	})

	t.Run("recurses into a composite", func(t *testing.T) {
		t.Parallel()
		got := golang.SubstituteTypeParams(emit.SliceOf(emit.Builtin("T")), by)
		c := got.(*emit.CompositeRef)
		if b, ok := c.Elem.(*emit.BuiltinRef); !ok || b.Name != "string" {
			t.Fatalf("slice element = %#v", c.Elem)
		}
	})

	t.Run("rewrites an external's type arguments", func(t *testing.T) {
		t.Parallel()
		got := golang.SubstituteTypeParams(
			emit.External("example.com/x", "Box", emit.Builtin("T")), by)
		e := got.(*emit.ExternalRef)
		if b, ok := e.TypeArgs[0].(*emit.BuiltinRef); !ok || b.Name != "string" {
			t.Fatalf("type arg = %#v", e.TypeArgs[0])
		}
	})

	t.Run("leaves an unbound parameter visible", func(t *testing.T) {
		t.Parallel()
		// A partial substitution stays visible in the output rather
		// than being silently erased.
		got := golang.SubstituteTypeParams(emit.Builtin("K"), by)
		if b := got.(*emit.BuiltinRef); b.Name != "K" {
			t.Fatalf("SubstituteTypeParams = %q, want K unchanged", b.Name)
		}
	})

	t.Run("does not mutate the input", func(t *testing.T) {
		t.Parallel()
		// A projection is commonly substituted once per instantiation,
		// and a mutating rewrite would corrupt the second.
		src := emit.SliceOf(emit.Builtin("T"))
		golang.SubstituteTypeParams(src, by)
		if b := src.Elem.(*emit.BuiltinRef); b.Name != "T" {
			t.Fatalf("the input was mutated to %q", b.Name)
		}
	})

	t.Run("an empty binding is a no-op", func(t *testing.T) {
		t.Parallel()
		src := emit.Builtin("T")
		if golang.SubstituteTypeParams(src, nil) != emit.Ref(src) {
			t.Fatalf("an empty binding must return the input")
		}
	})
}

func TestWitnessBindings(t *testing.T) {
	t.Parallel()

	t.Run("pairs each parameter with its position", func(t *testing.T) {
		t.Parallel()
		p := bounded("any", "comparable")
		got := golang.WitnessBindings(p, golang.Witnesses(p))
		if len(got) != 2 || got["T"] == nil || got["K"] == nil {
			t.Fatalf("WitnessBindings = %v", got)
		}
	})

	t.Run("mismatched lengths bind nothing", func(t *testing.T) {
		t.Parallel()
		// A partial binding substitutes some positions and leaves
		// others naming a parameter no longer in scope.
		if got := golang.WitnessBindings(bounded("any", "any"), []emit.Ref{emit.Builtin("int")}); got != nil {
			t.Fatalf("WitnessBindings = %v, want nil", got)
		}
	})
}

func TestSubstituteSig(t *testing.T) {
	t.Parallel()

	genericMethod := func() *node.Method {
		return &node.Method{
			Name:       "Get",
			TypeParams: bounded("any"),
			Params:     []*node.Param{{Name: "key", Type: &node.TypeRef{TypeKind: node.TypeRefTypeParam, Name: "T"}}},
			Returns:    []*node.Return{{Type: &node.TypeRef{TypeKind: node.TypeRefTypeParam, Name: "T"}}},
		}
	}

	t.Run("rewrites parameters and returns to the witness", func(t *testing.T) {
		t.Parallel()
		// A check naming `T` in a parameter position does not compile,
		// because a Go test function cannot take type parameters.
		s := golang.SigOf(genericMethod())
		got := golang.SubstituteSig(s, golang.Witnesses(s.TypeParams))
		if b, ok := got.Params[0].Type.(*emit.BuiltinRef); !ok || b.Name != "string" {
			t.Fatalf("substituted param = %#v", got.Params[0].Type)
		}
		if b, ok := got.Returns[0].Type.(*emit.BuiltinRef); !ok || b.Name != "string" {
			t.Fatalf("substituted return = %#v", got.Returns[0].Type)
		}
	})

	t.Run("the rewritten signature declares no parameters", func(t *testing.T) {
		t.Parallel()
		// Every use now names a concrete type; a declaration still
		// carrying the list would declare parameters nothing mentions.
		s := golang.SigOf(genericMethod())
		if got := golang.SubstituteSig(s, golang.Witnesses(s.TypeParams)); got.IsGeneric() {
			t.Fatalf("a substituted signature must not stay generic")
		}
	})

	t.Run("does not mutate the original", func(t *testing.T) {
		t.Parallel()
		s := golang.SigOf(genericMethod())
		golang.SubstituteSig(s, golang.Witnesses(s.TypeParams))
		if !s.IsGeneric() {
			t.Fatalf("the original projection was mutated")
		}
	})

	t.Run("nothing to substitute returns the input", func(t *testing.T) {
		t.Parallel()
		// So the non-generic path allocates nothing.
		s := golang.SigOf(getMethod())
		if golang.SubstituteSig(s, nil) != s {
			t.Fatalf("a non-generic signature must be returned unchanged")
		}
	})
}

func TestEmitSideTypeParams(t *testing.T) {
	t.Parallel()

	params := []*emit.TypeParam{{Name: "K"}, {Name: "V"}}

	t.Run("lifts declaration form", func(t *testing.T) {
		t.Parallel()
		// The constraint already lives on the emit layer and passes
		// through, where the source-side lifter has to convert it.
		got := golang.TypeParamDeclsFromEmit(params)
		if len(got) != 2 || got[0].Name != "K" {
			t.Fatalf("TypeParamDeclsFromEmit = %+v", got)
		}
	})

	t.Run("lifts instantiation arguments", func(t *testing.T) {
		t.Parallel()
		got := golang.TypeParamRefsFromEmit(params)
		if len(got) != 2 {
			t.Fatalf("TypeParamRefsFromEmit = %v", got)
		}
	})

	t.Run("renders the use spelling", func(t *testing.T) {
		t.Parallel()
		if got := golang.TypeParamNamesFromEmit(params); got != "[K, V]" {
			t.Fatalf("TypeParamNamesFromEmit = %q", got)
		}
	})

	t.Run("agrees with the source-side spelling", func(t *testing.T) {
		t.Parallel()
		// The two appear side by side in one generated file, so a
		// difference reads as a bug in the generator.
		src := golang.TypeParamNames([]*node.TypeParam{{Name: "K"}, {Name: "V"}})
		if got := golang.TypeParamNamesFromEmit(params); got != src {
			t.Fatalf("emit = %q, node = %q; the spellings must agree", got, src)
		}
	})

	t.Run("an empty list lifts to nil", func(t *testing.T) {
		t.Parallel()
		if golang.TypeParamDeclsFromEmit(nil) != nil || golang.TypeParamRefsFromEmit(nil) != nil {
			t.Fatalf("an empty list must lift to nil")
		}
		if golang.TypeParamNamesFromEmit(nil) != "" {
			t.Fatalf("an empty list must render nothing")
		}
	})
}

func TestWitnessEdges(t *testing.T) {
	t.Parallel()

	t.Run("a synthetic constraint falls back to the predicates", func(t *testing.T) {
		t.Parallel()
		// A constraint with no printed form is a fixture or a
		// programmatically built value, and the predicates are all
		// there is to read.
		if !golang.AdmitsAnyBasicType(&node.Constraint{}) {
			t.Fatalf("an unbounded synthetic constraint must admit anything")
		}
	})

	t.Run("a nil parameter entry kills the derivation", func(t *testing.T) {
		t.Parallel()
		if got := golang.Witnesses([]*node.TypeParam{nil}); got != nil {
			t.Fatalf("Witnesses = %v, want nil", got)
		}
		if got := golang.WitnessBindings([]*node.TypeParam{nil}, []emit.Ref{emit.Builtin("int")}); got != nil {
			t.Fatalf("WitnessBindings = %v, want nil", got)
		}
	})

	t.Run("substitution leaves an unknown ref kind alone", func(t *testing.T) {
		t.Parallel()
		by := map[string]emit.Ref{"T": emit.Builtin("string")}
		src := emit.External("example.com/x", "User")
		if golang.SubstituteTypeParams(src, by) != emit.Ref(src) {
			t.Fatalf("a reference with no type args must be returned as-is")
		}
		if golang.SubstituteTypeParams(nil, by) != nil {
			t.Fatalf("nil must substitute to nil")
		}
	})

	t.Run("a nil emit parameter entry is skipped", func(t *testing.T) {
		t.Parallel()
		params := []*emit.TypeParam{nil, {Name: "V"}}
		if got := golang.TypeParamDeclsFromEmit(params); len(got) != 2 {
			t.Fatalf("TypeParamDeclsFromEmit = %+v, want two slots", got)
		}
		if got := golang.TypeParamRefsFromEmit(params); len(got) != 2 {
			t.Fatalf("TypeParamRefsFromEmit = %v", got)
		}
		if got := golang.TypeParamNamesFromEmit(params); got != "[, V]" {
			t.Fatalf("TypeParamNamesFromEmit = %q", got)
		}
	})

	t.Run("mismatched witness lengths leave the signature alone", func(t *testing.T) {
		t.Parallel()
		s := golang.SigOf(&node.Method{Name: "F", TypeParams: bounded("any", "any")})
		if golang.SubstituteSig(s, []emit.Ref{emit.Builtin("int")}) != s {
			t.Fatalf("a partial witness list must not substitute")
		}
	})
}
