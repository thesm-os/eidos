// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// marker builds a structural marker ref of the given name.
func marker(name string, args ...*node.TypeRef) *node.TypeRef {
	return &node.TypeRef{
		TypeKind: node.TypeRefNamed,
		Package:  typescript.RefPackage,
		Name:     name,
		TypeArgs: args,
	}
}

// named builds an ordinary named ref.
func named(name string) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefNamed, Name: name}
}

func TestIsMarker(t *testing.T) {
	t.Parallel()

	t.Run("recognises a ref qualified by the marker package", func(t *testing.T) {
		t.Parallel()
		if !typescript.IsMarker(marker(typescript.RefUnion)) {
			t.Fatal("a marker ref was not recognised")
		}
	})

	t.Run("an ordinary named ref is not a marker", func(t *testing.T) {
		t.Parallel()
		if typescript.IsMarker(named("User")) {
			t.Fatal("a plain named ref reported as a marker")
		}
	})

	t.Run("a type genuinely named ts is not a marker", func(t *testing.T) {
		t.Parallel()
		// The qualifier is what decides, not the name. A user type
		// called `ts` carries no package and must not be mistaken for
		// one of these.
		if typescript.IsMarker(named(typescript.RefPackage)) {
			t.Fatal("a type named ts reported as a marker")
		}
	})

	t.Run("a non-named ref is not a marker", func(t *testing.T) {
		t.Parallel()
		slice := &node.TypeRef{TypeKind: node.TypeRefSlice, Package: typescript.RefPackage}
		if typescript.IsMarker(slice) {
			t.Fatal("a slice carrying the qualifier reported as a marker")
		}
	})

	t.Run("nil is not a marker", func(t *testing.T) {
		t.Parallel()
		if typescript.IsMarker(nil) {
			t.Fatal("nil reported as a marker")
		}
	})
}

func TestMarkerPredicates(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ref  *node.TypeRef
		is   func(*node.TypeRef) bool
	}{
		{"union", marker(typescript.RefUnion), typescript.IsUnion},
		{"intersection", marker(typescript.RefIntersection), typescript.IsIntersection},
		{"tuple", marker(typescript.RefTuple), typescript.IsTuple},
		{"operator", marker(typescript.RefOperator), typescript.IsOperator},
	}

	t.Run("each predicate matches only its own marker", func(t *testing.T) {
		t.Parallel()
		for _, subject := range cases {
			for _, probe := range cases {
				got := probe.is(subject.ref)
				want := probe.name == subject.name
				if got != want {
					t.Errorf("Is%s(%s marker) = %v, want %v",
						probe.name, subject.name, got, want)
				}
			}
		}
	})

	t.Run("no predicate matches a plain named ref", func(t *testing.T) {
		t.Parallel()
		for _, probe := range cases {
			if probe.is(named("User")) {
				t.Errorf("Is%s matched a plain named ref", probe.name)
			}
		}
	})

	t.Run("no predicate matches nil", func(t *testing.T) {
		t.Parallel()
		for _, probe := range cases {
			if probe.is(nil) {
				t.Errorf("Is%s matched nil", probe.name)
			}
		}
	})
}

func TestMembers(t *testing.T) {
	t.Parallel()

	t.Run("returns a union's members in order", func(t *testing.T) {
		t.Parallel()
		// Members ride on TypeArgs rather than on metadata so a
		// generic walker reaches them; this is the named way to read
		// them, because TypeArgs means generic arguments everywhere
		// else.
		u := marker(typescript.RefUnion, named("A"), named("B"), named("C"))
		got := typescript.Members(u)
		if len(got) != 3 {
			t.Fatalf("Members = %d, want 3", len(got))
		}
		for i, want := range []string{"A", "B", "C"} {
			if got[i].Name != want {
				t.Fatalf("member %d = %q, want %q", i, got[i].Name, want)
			}
		}
	})

	t.Run("returns an intersection's and a tuple's members", func(t *testing.T) {
		t.Parallel()
		for _, name := range []string{typescript.RefIntersection, typescript.RefTuple} {
			m := marker(name, named("A"), named("B"))
			if got := len(typescript.Members(m)); got != 2 {
				t.Errorf("%s members = %d, want 2", name, got)
			}
		}
	})

	t.Run("an operator marker has no members", func(t *testing.T) {
		t.Parallel()
		// Its content is source text on ts.typeText, not a member
		// list, so reporting TypeArgs here would invent structure.
		op := marker(typescript.RefOperator, named("A"))
		if got := typescript.Members(op); got != nil {
			t.Fatalf("Members(operator) = %+v, want nil", got)
		}
	})

	t.Run("a plain named ref has no members even when generic", func(t *testing.T) {
		t.Parallel()
		// `Promise<T>` carries a type argument, which is not a member
		// of a union and must not be read as one.
		g := &node.TypeRef{
			TypeKind: node.TypeRefNamed,
			Name:     "Promise",
			TypeArgs: []*node.TypeRef{named("T")},
		}
		if got := typescript.Members(g); got != nil {
			t.Fatalf("Members(Promise<T>) = %+v, want nil", got)
		}
	})

	t.Run("nil has no members", func(t *testing.T) {
		t.Parallel()
		if got := typescript.Members(nil); got != nil {
			t.Fatalf("Members(nil) = %+v, want nil", got)
		}
	})
}
