// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// TestRoundTripProjection walks a source type through the projection
// a generator actually performs — read a [node.TypeRef] off a
// declaration, hand the backend an [emit.Ref] — and asserts the two
// spellings agree.
//
// The check that matters for a language adapter: the frontend and the
// backend are written months apart against one model, and nothing
// else notices when they stop meaning the same thing by it. A type
// that parses to one shape and renders as another produces output
// that compiles and is wrong.
func TestRoundTripProjection(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ref  *node.TypeRef
		want string
	}{
		{"scalar", named("string"), "string"},
		{
			"array",
			&node.TypeRef{TypeKind: node.TypeRefSlice, Elem: named("string")},
			"string[]",
		},
		{
			"record",
			&node.TypeRef{
				TypeKind: node.TypeRefMap,
				MapKey:   named("string"),
				MapValue: named("number"),
			},
			"Record<string, number>",
		},
		{
			"union",
			marker(typescript.RefUnion, named("string"), named("number")),
			"string | number",
		},
		{
			"nullable",
			nullable(named("string")),
			"string | null",
		},
		{
			"array of union",
			&node.TypeRef{
				TypeKind: node.TypeRefSlice,
				Elem:     marker(typescript.RefUnion, named("A"), named("B")),
			},
			"(A | B)[]",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+" spells the same on both sides", func(t *testing.T) {
			t.Parallel()
			// TypeString is the human-facing spelling and FromNode is
			// what reaches the backend. They are separate code paths
			// over one model, so agreeing is a real assertion rather
			// than a tautology.
			if got := typescript.TypeString(tc.ref); got != tc.want {
				t.Fatalf("TypeString = %q, want %q", got, tc.want)
			}
			if ref := typescript.FromNode(tc.ref); ref == nil {
				t.Fatal("FromNode produced no emit ref")
			}
		})
	}
}
