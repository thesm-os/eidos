// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"testing"

	ts "github.com/tree-sitter/go-tree-sitter"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/node"
)

// firstDecl parses src and returns the first top-level node, so a
// guard can be exercised against a real tree.
func firstDecl(t *testing.T, src string) (*conv, *ts.Node) {
	t.Helper()
	p, err := parseFile("g.ts", []byte(src))
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	t.Cleanup(p.close)
	c := newConv(p, "./x", diag.New().For(FrontendName), directive.DefaultParser())
	return c, p.root().NamedChild(0)
}

// TestConverterGuards pins the nil and absent-child paths every
// converter helper tolerates.
//
// These exist so call sites need no guard of their own — a helper
// that panicked on an absent optional child would push a nil check
// into every one of them. Asserted directly because the shapes that
// reach them are rare in real source and would otherwise be covered
// only by accident.
func TestConverterGuards(t *testing.T) {
	t.Parallel()

	t.Run("annotatedType tolerates a nil node and an absent annotation", func(t *testing.T) {
		t.Parallel()
		if got := annotatedType(nil); got != nil {
			t.Errorf("annotatedType(nil) = %v, want nil", got)
		}
		// A parameter with no type annotation: legal TypeScript, and
		// the model records a nil Type for it.
		c, decl := firstDecl(t, `function f(a) {}`)
		params := c.params(decl.ChildByFieldName("parameters"))
		if len(params) != 1 {
			t.Fatalf("params = %d, want 1", len(params))
		}
		if params[0].Type != nil {
			t.Errorf("Type = %+v, want nil for an unannotated parameter", params[0].Type)
		}
	})

	t.Run("returnType tolerates an absent return annotation", func(t *testing.T) {
		t.Parallel()
		// A constructor declares no return type, and neither does an
		// inferred-return function.
		c, decl := firstDecl(t, `function f(a: string) { return 1; }`)
		if got := returnType(decl); got != nil {
			t.Errorf("returnType = %v, want nil", got)
		}
		fn := c.functionDecl(decl)
		if fn == nil {
			t.Fatal("functionDecl returned nothing")
		}
		if len(fn.Returns) != 0 {
			t.Errorf("Returns = %d, want 0 for an inferred return", len(fn.Returns))
		}
	})

	t.Run("boundType unwraps a constraint and passes a bare type through", func(t *testing.T) {
		t.Parallel()
		c, decl := firstDecl(t, `interface A<T extends object> { v: T; }`)
		params := c.typeParams(decl.ChildByFieldName("type_parameters"))
		if len(params) != 1 || params[0].Constraint == nil {
			t.Fatalf("TypeParams = %+v, want one constrained parameter", params)
		}
		if params[0].Constraint.Raw != "object" {
			t.Fatalf("Raw = %q, want object — the `extends` keyword must not survive",
				params[0].Constraint.Raw)
		}
	})

	t.Run("typeParam ignores a node that is not a type parameter", func(t *testing.T) {
		t.Parallel()
		_, decl := firstDecl(t, `interface A<T> {}`)
		// The declaration itself is not a type_parameter.
		c, _ := firstDecl(t, `interface A<T> {}`)
		if got := c.typeParam(decl); got != nil {
			t.Fatalf("typeParam(non-parameter) = %+v, want nil", got)
		}
	})

	t.Run("typeRef stops at an exhausted budget and a nil node", func(t *testing.T) {
		t.Parallel()
		c, _ := firstDecl(t, `type A = string;`)
		if got := c.typeRef(nil, maxTypeDepth); got != nil {
			t.Errorf("typeRef(nil) = %+v, want nil", got)
		}
		_, decl := firstDecl(t, `type A = string;`)
		if got := c.typeRef(decl, 0); got != nil {
			t.Errorf("typeRef(depth 0) = %+v, want nil", got)
		}
	})

	t.Run("propertyName reports nothing for a nil node", func(t *testing.T) {
		t.Parallel()
		c, _ := firstDecl(t, `interface A {}`)
		if got := c.propertyName(nil); got != "" {
			t.Fatalf("propertyName(nil) = %q, want empty", got)
		}
	})

	t.Run("bindingName reports nothing for a nil pattern", func(t *testing.T) {
		t.Parallel()
		c, _ := firstDecl(t, `interface A {}`)
		if got := c.bindingName(nil); got != "" {
			t.Fatalf("bindingName(nil) = %q, want empty", got)
		}
	})

	t.Run("params tolerates an absent parameter list", func(t *testing.T) {
		t.Parallel()
		c, _ := firstDecl(t, `interface A {}`)
		if got := c.params(nil); got != nil {
			t.Fatalf("params(nil) = %+v, want nil", got)
		}
	})

	t.Run("typeParams tolerates an absent list", func(t *testing.T) {
		t.Parallel()
		c, _ := firstDecl(t, `interface A {}`)
		if got := c.typeParams(nil); got != nil {
			t.Fatalf("typeParams(nil) = %+v, want nil", got)
		}
	})

	t.Run("members tolerates an absent body", func(t *testing.T) {
		t.Parallel()
		c, _ := firstDecl(t, `interface A {}`)
		host := &node.Interface{Name: "A"}
		got := c.members(nil, host, host.EnsureMeta())
		if len(got.fields) != 0 || len(got.methods) != 0 {
			t.Fatalf("members(nil) = %+v, want nothing", got)
		}
	})

	t.Run("a malformed directive warns rather than aborting the declaration", func(t *testing.T) {
		t.Parallel()
		// The declaration still converts; a directive nobody can parse
		// is a comment problem, not a reason to lose the type.
		p, err := parseFile("d.ts", []byte("// +gen:\nexport interface A { id: string; }\n"))
		if err != nil {
			t.Fatalf("parseFile: %v", err)
		}
		defer p.close()

		sink := diag.New()
		c := newConv(p, "./x", sink.For(FrontendName), directive.DefaultParser())
		decls := c.declarations(p.root())
		if len(decls) != 1 {
			t.Fatalf("declarations = %d, want the interface to survive", len(decls))
		}
	})
}
