// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

func TestFromNode(t *testing.T) {
	t.Parallel()

	t.Run("projects each shape onto its emit counterpart", func(t *testing.T) {
		t.Parallel()
		for name, ref := range map[string]*node.TypeRef{
			"scalar":    named("string"),
			"slice":     {TypeKind: node.TypeRefSlice, Elem: named("string")},
			"map":       {TypeKind: node.TypeRefMap, MapKey: named("string"), MapValue: named("number")},
			"pointer":   {TypeKind: node.TypeRefPointer, Elem: named("string")},
			"func":      {TypeKind: node.TypeRefFunc},
			"union":     marker(typescript.RefUnion, named("A")),
			"tuple":     marker(typescript.RefTuple, named("A")),
			"qualified": {TypeKind: node.TypeRefNamed, Package: "./m", Name: "User"},
		} {
			if got := typescript.FromNode(ref); got == nil {
				t.Errorf("%s projected to nothing", name)
			}
		}
	})

	t.Run("nil projects to nothing", func(t *testing.T) {
		t.Parallel()
		if got := typescript.FromNode(nil); got != nil {
			t.Fatalf("FromNode(nil) = %+v, want nil", got)
		}
	})

	t.Run("an operator type carries its text across", func(t *testing.T) {
		t.Parallel()
		op := marker(typescript.RefOperator)
		typescript.MetaTypeText.Set(op.EnsureMeta(), "keyof T", "test")

		ref := typescript.FromNode(op)
		if ref == nil {
			t.Fatal("operator projected to nothing")
		}
		// It reaches the backend as a builtin, which the backend
		// spells verbatim — which is exactly what a type carried as
		// text needs.
		if got := typescript.TypeString(op); got != "keyof T" {
			t.Fatalf("TypeString = %q", got)
		}
	})
}
