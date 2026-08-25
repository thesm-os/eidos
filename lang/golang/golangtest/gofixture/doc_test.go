// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package gofixture_test

import (
	"fmt"

	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
	"go.thesmos.sh/eidos/node"
)

// Example builds the fixture shape a plugin unit test starts from —
// one package, one annotated struct, two fields — and inspects it
// through [gofixture.Builder.PackageNode].
//
// The example doubles as the compile check the package docblock
// cannot provide. Prose in a docblock is not built: the snippet
// there has previously drifted from the real signatures (a method's
// arity, a type-reference helper that was never added), and nothing
// caught it because no compiler ever read it. This function does the
// same job in code, so the next drift is a build failure.
//
// PackageNode returns the builder's live package — the accessor to
// use when a test needs to stamp typed metadata on a node before
// handing the store to a plugin. Use [gofixture.Builder.Build]
// instead when the test wants a populated [store.Store] and does not
// care about the intermediate node graph.
func Example() {
	pkg := gofixture.New().
		Package("users", "example.com/users").
		Struct("User", func(s *gofixture.StructBuilder) {
			s.Docs("User is the stored account record.")
			s.Directive(gofixture.Directive("repo", gofixture.KV("table", "users")))
			s.Field("ID", gofixture.Named("string"), func(f *gofixture.FieldBuilder) {
				f.Tag(`json:"id"`)
			})
			s.Field("Roles", gofixture.Slice(gofixture.Named("string")), nil)
		}).
		PackageNode()

	user := pkg.Structs[0]
	fmt.Printf("%s (%s)\n", user.QName(), pkg.Name)
	fmt.Printf("docs: %q\n", user.Docs())
	for _, d := range user.Directives() {
		fmt.Printf("directive: %s table=%s\n", d.Name, d.KV["table"])
	}
	for _, f := range user.Fields {
		fmt.Printf("field: %s %s(%s) tag=%q\n", f.Name, f.Type.TypeKind, typeName(f.Type), f.Tag)
	}

	// Output:
	// example.com/users.User (users)
	// docs: ["User is the stored account record."]
	// directive: repo table=users
	// field: ID named(string) tag="json:\"id\""
	// field: Roles slice(string) tag=""
}

// typeName renders the leaf identifier of a [node.TypeRef] built by
// the package's type-reference helpers, descending through the one
// level of nesting the example's `[]string` field introduces.
//
// It exists so the example prints something a reader can check by
// eye. A full renderer belongs to a backend, not to a doc example —
// the node graph is deliberately language-neutral and carries no
// opinion about how a type is spelled in source.
func typeName(t *node.TypeRef) string {
	if t.Elem != nil {
		return typeName(t.Elem)
	}
	return t.Name
}
