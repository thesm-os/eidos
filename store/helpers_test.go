// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store_test

import (
	"strconv"
	"testing"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// assertNoError fails the test if err is non-nil.
func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// directiveAt builds a directive instance with a name and position.
func directiveAt(name directive.Name, pos position.Pos) *directive.Directive {
	return &directive.Directive{Name: name, Pos: pos, KV: map[string]string{}}
}

// builtinTypeRef builds a Named TypeRef with no package — the
// frontend-conventional shape for primitive types.
func builtinTypeRef(name string) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefNamed, Name: name}
}

// makeUserPackage builds a representative [node.Package] with one
// file, two structs (one with directives + fields + methods, one
// plain), an interface, function, variable, constant, enum, and
// alias. Used by store tests that need a populated source package.
func makeUserPackage() *node.Package {
	dir := directiveAt("repo", position.At("user.go", 1, 1))
	user := &node.Struct{
		BaseNode: node.BaseNode{DirectiveList: []*directive.Directive{dir}},
		Name:     "User",
		Package:  "github.com/example/users",
		Fields: []*node.Field{
			{Name: "ID", Type: builtinTypeRef("string")},
			{Name: "Email", Type: builtinTypeRef("string")},
		},
		Methods: []*node.Method{
			{Name: "Validate"},
		},
	}
	addr := &node.Struct{
		Name:    "Address",
		Package: "github.com/example/users",
		Fields: []*node.Field{
			{Name: "City", Type: builtinTypeRef("string")},
		},
	}
	repo := &node.Interface{
		Name:    "Repo",
		Package: "github.com/example/users",
		Methods: []*node.Method{{Name: "Get"}, {Name: "Save"}},
	}
	open := &node.Function{Name: "Open", Package: "github.com/example/users"}
	def := &node.Variable{Name: "Default", Package: "github.com/example/users"}
	pi := &node.Constant{Name: "Pi", Package: "github.com/example/users"}
	status := &node.Enum{
		Name:    "Status",
		Package: "github.com/example/users",
		Variants: []*node.EnumVariant{
			{Name: "Active"},
			{Name: "Inactive"},
		},
	}
	id := &node.Alias{Name: "ID", Package: "github.com/example/users"}
	file := &node.File{Name: "user.go", Path: "github.com/example/users/user.go"}
	return &node.Package{
		Name:       "users",
		Path:       "github.com/example/users",
		Files:      []*node.File{file},
		Imports:    []*node.Import{{Path: "context"}},
		Structs:    []*node.Struct{user, addr},
		Interfaces: []*node.Interface{repo},
		Functions:  []*node.Function{open},
		Variables:  []*node.Variable{def},
		Constants:  []*node.Constant{pi},
		Enums:      []*node.Enum{status},
		Aliases:    []*node.Alias{id},
	}
}

// makeBenchPackage builds a source package of n structs, each with
// three fields and one method, with every fourth struct carrying a
// "repo" directive. It is the shared fixture behind the query and
// reader benchmarks.
//
// The directive spread matters: a predicate benchmark over a fixture
// where every value matches (or none does) measures the scan, not
// the predicate, because the branch predictor sees a single outcome
// and the result slice never grows partially. One in four keeps both
// arms live.
//
// Allocation note: this allocates 1 package + n structs + 4n children
// on the heap. It is a setup helper — benchmarks must call it above
// their timed loop, never inside it.
func makeBenchPackage(n int) *node.Package {
	pkg := &node.Package{
		Name:    "bench",
		Path:    "github.com/example/bench",
		Files:   []*node.File{{Name: "bench.go", Path: "github.com/example/bench/bench.go"}},
		Structs: make([]*node.Struct, 0, n),
	}
	for i := range n {
		s := &node.Struct{
			Name:    "Entity" + strconv.Itoa(i),
			Package: pkg.Path,
			Fields: []*node.Field{
				{Name: "ID", Type: builtinTypeRef("string")},
				{Name: "Count", Type: builtinTypeRef("int")},
				{Name: "Label", Type: builtinTypeRef("string")},
			},
			Methods: []*node.Method{{Name: "Validate"}},
		}
		if i%4 == 0 {
			s.DirectiveList = []*directive.Directive{
				directiveAt("repo", position.At("bench.go", i+1, 1)),
			}
		}
		pkg.Structs = append(pkg.Structs, s)
	}
	return pkg
}
