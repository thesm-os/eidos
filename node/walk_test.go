// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node_test

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/core/kind"
	"go.thesmos.sh/eidos/node"
)

func TestWalk_NilNodeIsNoop(t *testing.T) {
	t.Parallel()

	t.Run("nil node yields no visits", func(t *testing.T) {
		t.Parallel()
		got := recordWalk(nil)
		if len(got) != 0 {
			t.Fatalf("nil node should not be visited; got %v", got)
		}
	})

	t.Run("nil visitor yields no visits", func(t *testing.T) {
		t.Parallel()
		node.Walk(&node.Struct{}, nil)
	})
}

func TestWalk_PrunedSubtree(t *testing.T) {
	t.Parallel()

	t.Run("returning nil from Visit stops descent", func(t *testing.T) {
		t.Parallel()
		var visited int
		pruning := node.VisitorFunc(func(_ node.Node) node.Visitor {
			visited++
			return nil
		})
		s := &node.Struct{
			Fields:  []*node.Field{{Name: "ID"}},
			Methods: []*node.Method{{Name: "Save"}},
		}
		node.Walk(s, pruning)
		if visited != 1 {
			t.Fatalf("expected 1 visit (root only); got %d", visited)
		}
	})
}

func TestWalk_PackageDescentOrder(t *testing.T) {
	t.Parallel()

	t.Run("visits files, imports, then declarations of every kind in order", func(t *testing.T) {
		t.Parallel()
		p := &node.Package{
			Files:      []*node.File{{Name: "user.go"}},
			Imports:    []*node.Import{{Path: "context"}},
			Structs:    []*node.Struct{{Name: "User"}},
			Interfaces: []*node.Interface{{Name: "Repo"}},
			Functions:  []*node.Function{{Name: "Open"}},
			Variables:  []*node.Variable{{Name: "Default"}},
			Constants:  []*node.Constant{{Name: "Pi"}},
			Enums:      []*node.Enum{{Name: "Status"}},
			Aliases:    []*node.Alias{{Name: "ID"}},
		}
		got := recordWalk(p)
		want := []kind.Kind{
			node.KindPackage,
			node.KindFile,
			node.KindImport,
			node.KindStruct,
			node.KindInterface,
			node.KindFunction,
			node.KindVariable,
			node.KindConstant,
			node.KindEnum,
			node.KindAlias,
		}
		if !slices.Equal(got, want) {
			t.Fatalf("visit order = %v, want %v", got, want)
		}
	})
}

func TestWalk_StructDescent(t *testing.T) {
	t.Parallel()

	t.Run("visits TypeParams, Fields, Embeds, Methods", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{
			TypeParams: []*node.TypeParam{{Name: "T"}},
			Fields:     []*node.Field{{Name: "ID", Type: namedRef("", "string")}},
			Embeds:     []*node.Embed{{Type: namedRef("", "Base")}},
			Methods:    []*node.Method{{Name: "Save"}},
		}
		got := recordWalk(s)
		want := []kind.Kind{
			node.KindStruct, node.KindTypeParam, node.KindField, node.KindTypeRef,
			node.KindEmbed, node.KindTypeRef, node.KindMethod,
		}
		if !slices.Equal(got, want) {
			t.Fatalf("visit order = %v, want %v", got, want)
		}
	})
}

func TestWalk_InterfaceDescent(t *testing.T) {
	t.Parallel()

	t.Run("visits TypeParams, Methods, Embeds", func(t *testing.T) {
		t.Parallel()
		i := &node.Interface{
			TypeParams: []*node.TypeParam{{Name: "T"}},
			Methods:    []*node.Method{{Name: "Get"}},
			Embeds:     []*node.Embed{{Type: namedRef("", "Base")}},
		}
		got := recordWalk(i)
		want := []kind.Kind{
			node.KindInterface, node.KindTypeParam, node.KindMethod, node.KindEmbed, node.KindTypeRef,
		}
		if !slices.Equal(got, want) {
			t.Fatalf("visit order = %v, want %v", got, want)
		}
	})
}

func TestWalk_MethodDescent(t *testing.T) {
	t.Parallel()

	t.Run("visits TypeParams, Params, Returns", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{
			TypeParams: []*node.TypeParam{{Name: "T"}},
			Params:     []*node.Param{{Type: namedRef("", "int")}},
			Returns:    node.AnonReturns(namedRef("", "error")),
		}
		got := recordWalk(m)
		want := []kind.Kind{
			// A return is a node in its own right, visited before its
			// type — symmetric with how a param is visited.
			node.KindMethod, node.KindTypeParam, node.KindParam, node.KindTypeRef,
			node.KindReturn, node.KindTypeRef,
		}
		if !slices.Equal(got, want) {
			t.Fatalf("visit order = %v, want %v", got, want)
		}
	})
}

func TestWalk_FunctionDescent(t *testing.T) {
	t.Parallel()

	t.Run("visits TypeParams, Params, Returns", func(t *testing.T) {
		t.Parallel()
		f := &node.Function{
			TypeParams: []*node.TypeParam{{Name: "T"}},
			Params:     []*node.Param{{Type: namedRef("", "int")}},
			Returns:    node.AnonReturns(namedRef("", "error")),
		}
		got := recordWalk(f)
		want := []kind.Kind{
			node.KindFunction, node.KindTypeParam, node.KindParam, node.KindTypeRef,
			node.KindReturn, node.KindTypeRef,
		}
		if !slices.Equal(got, want) {
			t.Fatalf("visit order = %v, want %v", got, want)
		}
	})
}

func TestWalk_EnumDescent(t *testing.T) {
	t.Parallel()

	t.Run("visits Underlying then Variants", func(t *testing.T) {
		t.Parallel()
		e := &node.Enum{
			Underlying: namedRef("", "int"),
			Variants:   []*node.EnumVariant{{Name: "A"}, {Name: "B"}},
		}
		got := recordWalk(e)
		want := []kind.Kind{
			node.KindEnum, node.KindTypeRef, node.KindEnumVariant, node.KindEnumVariant,
		}
		if !slices.Equal(got, want) {
			t.Fatalf("visit order = %v, want %v", got, want)
		}
	})
}

func TestWalk_AliasDescent(t *testing.T) {
	t.Parallel()

	t.Run("visits TypeParams then Target", func(t *testing.T) {
		t.Parallel()
		a := &node.Alias{
			TypeParams: []*node.TypeParam{{Name: "T"}},
			Target:     namedRef("", "int"),
		}
		got := recordWalk(a)
		want := []kind.Kind{node.KindAlias, node.KindTypeParam, node.KindTypeRef}
		if !slices.Equal(got, want) {
			t.Fatalf("visit order = %v, want %v", got, want)
		}
	})
}

func TestWalk_TypeRefVariants(t *testing.T) {
	t.Parallel()

	t.Run("composite refs descend into their structural children", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			ref  *node.TypeRef
			want int
		}{
			{
				"Pointer visits Elem",
				&node.TypeRef{TypeKind: node.TypeRefPointer, Elem: namedRef("", "int")},
				2,
			},
			{
				"Slice visits Elem",
				&node.TypeRef{TypeKind: node.TypeRefSlice, Elem: namedRef("", "int")},
				2,
			},
			{
				"Array visits Elem",
				&node.TypeRef{TypeKind: node.TypeRefArray, ArrayLen: 3, Elem: namedRef("", "int")},
				2,
			},
			{
				"Map visits MapKey and MapValue",
				&node.TypeRef{
					TypeKind: node.TypeRefMap,
					MapKey:   namedRef("", "string"),
					MapValue: namedRef("", "int"),
				},
				3,
			},
			{
				"Func visits parameters and returns",
				&node.TypeRef{
					TypeKind:    node.TypeRefFunc,
					FuncParams:  []*node.TypeRef{namedRef("", "int")},
					FuncReturns: []*node.TypeRef{namedRef("", "error")},
				},
				3,
			},
			{
				"Named visits TypeArgs",
				&node.TypeRef{
					TypeKind: node.TypeRefNamed,
					Name:     "Container",
					TypeArgs: []*node.TypeRef{namedRef("", "int"), namedRef("", "string")},
				},
				3,
			},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				if got := recordWalk(tc.ref); len(got) != tc.want {
					t.Fatalf("expected %d visits; got %v", tc.want, got)
				}
			})
		}
	})

	t.Run("TypeParam ref is a leaf", func(t *testing.T) {
		t.Parallel()
		got := recordWalk(&node.TypeRef{TypeKind: node.TypeRefTypeParam, Name: "T"})
		if len(got) != 1 || got[0] != node.KindTypeRef {
			t.Fatalf("TypeParam ref should be a leaf; got %v", got)
		}
	})

	t.Run("AnonStruct visits inline Fields then Embeds", func(t *testing.T) {
		t.Parallel()
		r := &node.TypeRef{
			TypeKind: node.TypeRefAnonStruct,
			Fields:   []*node.Field{{Name: "ID", Type: namedRef("", "string")}},
			Embeds:   []*node.Embed{{Type: namedRef("io", "Reader")}},
		}
		got := recordWalk(r)
		want := []kind.Kind{
			node.KindTypeRef, node.KindField, node.KindTypeRef,
			node.KindEmbed, node.KindTypeRef,
		}
		if !slices.Equal(got, want) {
			t.Fatalf("visit order = %v, want %v", got, want)
		}
	})

	t.Run("AnonInterface visits inline Methods then Embeds", func(t *testing.T) {
		t.Parallel()
		r := &node.TypeRef{
			TypeKind: node.TypeRefAnonInterface,
			Methods:  []*node.Method{{Name: "Read"}},
			Embeds:   []*node.Embed{{Type: namedRef("io", "Reader")}},
		}
		got := recordWalk(r)
		want := []kind.Kind{
			node.KindTypeRef, node.KindMethod,
			node.KindEmbed, node.KindTypeRef,
		}
		if !slices.Equal(got, want) {
			t.Fatalf("visit order = %v, want %v", got, want)
		}
	})
}

func TestWalk_LeafKinds(t *testing.T) {
	t.Parallel()

	t.Run("nodes carrying a single Type ref descend into it", func(t *testing.T) {
		t.Parallel()
		ref := namedRef("", "int")
		cases := []struct {
			name string
			n    node.Node
		}{
			{"Variable", &node.Variable{Type: ref}},
			{"Constant", &node.Constant{Type: ref}},
			{"Embed", &node.Embed{Type: ref}},
			{"Field", &node.Field{Type: ref}},
			{"Param", &node.Param{Type: ref}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				got := recordWalk(tc.n)
				if len(got) != 2 {
					t.Fatalf("%s expected to visit itself + Type; got %v", tc.n.Kind(), got)
				}
			})
		}
	})

	t.Run("TypeParam descends into Constraint.Embedded entries", func(t *testing.T) {
		t.Parallel()
		tp := &node.TypeParam{
			Name:       "T",
			Constraint: constraintFrom(namedRef("fmt", "Stringer"), namedRef("", "comparable")),
		}
		got := recordWalk(tp)
		want := []kind.Kind{node.KindTypeParam, node.KindTypeRef, node.KindTypeRef}
		if !slices.Equal(got, want) {
			t.Fatalf("visit order = %v, want %v", got, want)
		}
	})

	t.Run("TypeParam without Constraint is a leaf", func(t *testing.T) {
		t.Parallel()
		got := recordWalk(&node.TypeParam{Name: "T"})
		if len(got) != 1 || got[0] != node.KindTypeParam {
			t.Fatalf("unconstrained TypeParam should be a leaf; got %v", got)
		}
	})

	t.Run("EnumVariant is a leaf node", func(t *testing.T) {
		t.Parallel()
		got := recordWalk(&node.EnumVariant{Name: "Active"})
		if len(got) != 1 || got[0] != node.KindEnumVariant {
			t.Fatalf("EnumVariant should be a leaf; got %v", got)
		}
	})

	t.Run("File visits its Imports", func(t *testing.T) {
		t.Parallel()
		f := &node.File{Imports: []*node.Import{{Path: "context"}, {Path: "fmt"}}}
		got := recordWalk(f)
		want := []kind.Kind{node.KindFile, node.KindImport, node.KindImport}
		if !slices.Equal(got, want) {
			t.Fatalf("visit order = %v, want %v", got, want)
		}
	})
}
