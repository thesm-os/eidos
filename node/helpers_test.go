// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/kind"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// assertEqualString fails the test if got and want differ.
func assertEqualString(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("string mismatch:\n got:  %q\n want: %q", got, want)
	}
}

// directiveAt builds a directive instance with a name and position.
// Used across tests that need to populate BaseNode.DirectiveList.
func directiveAt(name directive.Name, pos position.Pos) *directive.Directive {
	return &directive.Directive{Name: name, Pos: pos, KV: map[string]string{}}
}

// namedRef builds a Named TypeRef. Used by tests that need a quick
// reference to a type without full backstory.
func namedRef(pkg, name string) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefNamed, Package: pkg, Name: name}
}

// constraintFrom builds a [node.Constraint] embedding refs as named
// bounds. Used by tests that need a quick generic-constraint instance
// without manual struct-literal noise.
func constraintFrom(refs ...*node.TypeRef) *node.Constraint {
	return &node.Constraint{Embedded: refs}
}

// recordingVisitor collects the [kind.Kind] of every node Walk
// visits, in visit order. Tests assert on the resulting slice.
type recordingVisitor struct {
	kinds *[]kind.Kind
}

// Visit appends n.Kind() to the underlying slice and continues
// descent.
func (r recordingVisitor) Visit(n node.Node) node.Visitor {
	*r.kinds = append(*r.kinds, n.Kind())
	return r
}

// recordWalk runs [node.Walk] starting from n and returns the
// [kind.Kind] of every visited node in visit order.
func recordWalk(n node.Node) []kind.Kind {
	var kinds []kind.Kind
	node.Walk(n, recordingVisitor{kinds: &kinds})
	return kinds
}

// byteSliceRef returns a slice-of-byte [node.TypeRef], a common
// helper across anonymous-interface and signature-comparison tests.
func byteSliceRef() *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: namedRef("", "byte")}
}

// anonInterfaceWithMethod returns a [node.TypeRefAnonInterface] ref
// carrying a single method with the supplied shape.
func anonInterfaceWithMethod(
	name string,
	params []*node.Param,
	returns []*node.TypeRef,
	variadic bool,
) *node.TypeRef {
	if variadic && len(params) > 0 {
		params[len(params)-1].Variadic = true
	}
	return &node.TypeRef{
		TypeKind: node.TypeRefAnonInterface,
		Methods:  []*node.Method{{Name: name, Params: params, Returns: node.AnonReturns(returns...)}},
	}
}

// anonInterfaceWithRead is an [anonInterfaceWithMethod] shorthand for
// the canonical "Read" method shape used across anon-interface
// equality tests.
func anonInterfaceWithRead(
	params []*node.Param,
	returns []*node.TypeRef,
	variadic bool,
) *node.TypeRef {
	return anonInterfaceWithMethod("Read", params, returns, variadic)
}

// makeRichPackage builds a [node.Package] populated with every kind
// of declaration the model supports, including back-pointers between
// the host and child nodes. Used by tests that exercise traversal
// helpers and JSON round-trip + rewire flows.
func makeRichPackage() *node.Package {
	pkg := &node.Package{
		Name: "users",
		Path: "example.com/users",
		Files: []*node.File{
			{
				Name:    "user.go",
				Path:    "example.com/users/user.go",
				Imports: []*node.Import{{Path: "context"}},
			},
		},
		Imports: []*node.Import{{Path: "context"}},
	}

	s := &node.Struct{
		Name:       "User",
		Package:    pkg.Path,
		TypeParams: []*node.TypeParam{{Name: "T", Constraint: constraintFrom(namedRef("", "comparable"))}},
		Fields:     []*node.Field{{Name: "ID", Type: namedRef("", "string")}},
		Embeds:     []*node.Embed{{Type: namedRef("io", "Reader")}},
		Methods: []*node.Method{{
			Name:       "Save",
			Receiver:   &node.TypeRef{TypeKind: node.TypeRefPointer, Elem: namedRef(pkg.Path, "User")},
			Params:     []*node.Param{{Name: "ctx", Type: namedRef("context", "Context")}},
			Returns:    node.AnonReturns(namedRef("", "error")),
			TypeParams: []*node.TypeParam{{Name: "M"}},
		}},
	}
	pkg.Structs = []*node.Struct{s}

	i := &node.Interface{
		Name:       "Repo",
		Package:    pkg.Path,
		TypeParams: []*node.TypeParam{{Name: "U", Constraint: constraintFrom(namedRef("fmt", "Stringer"))}},
		Methods: []*node.Method{{
			Name:    "Get",
			Returns: node.AnonReturns(namedRef(pkg.Path, "User"), namedRef("", "error")),
		}},
		Embeds: []*node.Embed{{Type: namedRef("io", "Closer")}},
	}
	pkg.Interfaces = []*node.Interface{i}

	// Function with composite-shape Params exercising every TypeRef
	// variant the rewire walker has to descend into.
	sliceOf := func(elem *node.TypeRef) *node.TypeRef {
		return &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: elem}
	}
	arrayOf := func(elem *node.TypeRef, n int) *node.TypeRef {
		return &node.TypeRef{TypeKind: node.TypeRefArray, Elem: elem, ArrayLen: n}
	}
	mapOf := func(k, v *node.TypeRef) *node.TypeRef {
		return &node.TypeRef{TypeKind: node.TypeRefMap, MapKey: k, MapValue: v}
	}
	funcOf := func(params, returns []*node.TypeRef) *node.TypeRef {
		return &node.TypeRef{TypeKind: node.TypeRefFunc, FuncParams: params, FuncReturns: returns}
	}
	genericRef := func(pkgPath, name string, args ...*node.TypeRef) *node.TypeRef {
		return &node.TypeRef{TypeKind: node.TypeRefNamed, Package: pkgPath, Name: name, TypeArgs: args}
	}

	fn := &node.Function{
		Name:    "Open",
		Package: pkg.Path,
		Params: []*node.Param{
			{Name: "addr", Type: namedRef("", "string")},
			{Name: "items", Type: sliceOf(namedRef("", "int"))},
			{Name: "buf", Type: arrayOf(namedRef("", "byte"), 16)},
			{Name: "by", Type: mapOf(namedRef("", "string"), namedRef("", "int"))},
			{Name: "fn", Type: funcOf([]*node.TypeRef{namedRef("", "int")}, []*node.TypeRef{namedRef("", "error")})},
			{Name: "gen", Type: genericRef(pkg.Path, "User", namedRef("", "string"))},
			{Name: "t", Type: &node.TypeRef{TypeKind: node.TypeRefTypeParam, Name: "K"}},
		},
		Returns:    node.AnonReturns(namedRef("", "error")),
		TypeParams: []*node.TypeParam{{Name: "K"}},
	}
	pkg.Functions = []*node.Function{fn}

	enum := &node.Enum{
		Name:       "Status",
		Package:    pkg.Path,
		Underlying: namedRef("", "int"),
		Variants:   []*node.EnumVariant{{Name: "Active", Value: "0"}},
	}
	pkg.Enums = []*node.Enum{enum}

	alias := &node.Alias{
		Name:       "ID",
		Package:    pkg.Path,
		Target:     namedRef("", "string"),
		TypeParams: []*node.TypeParam{{Name: "T"}},
	}
	pkg.Aliases = []*node.Alias{alias}

	pkg.Variables = []*node.Variable{{Name: "Default", Package: pkg.Path, Type: namedRef(pkg.Path, "User")}}
	pkg.Constants = []*node.Constant{{Name: "Limit", Package: pkg.Path, Value: "100"}}

	return pkg
}
