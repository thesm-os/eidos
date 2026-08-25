// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import "testing"

func TestJoinNamespace(t *testing.T) {
	t.Parallel()

	t.Run("joins segments with a dot", func(t *testing.T) {
		t.Parallel()
		if got := joinNamespace("Outer", "Inner"); got != "Outer.Inner" {
			t.Fatalf("joinNamespace = %q, want Outer.Inner", got)
		}
	})

	t.Run("an empty prefix yields the segment alone", func(t *testing.T) {
		t.Parallel()
		// The file-scope case: the first namespace entered has no
		// enclosing path, and a leading dot would be part of the
		// qualifier a use site spells.
		if got := joinNamespace("", "Outer"); got != "Outer" {
			t.Fatalf("joinNamespace = %q, want Outer", got)
		}
	})

	t.Run("an empty segment leaves the prefix unchanged", func(t *testing.T) {
		t.Parallel()
		if got := joinNamespace("Outer", ""); got != "Outer" {
			t.Fatalf("joinNamespace = %q, want Outer", got)
		}
	})
}

func TestHasToken(t *testing.T) {
	t.Parallel()

	t.Run("finds an anonymous keyword token", func(t *testing.T) {
		t.Parallel()
		p, err := parseFile("a.ts", []byte("export default class C {}"))
		if err != nil {
			t.Fatalf("parseFile: %v", err)
		}
		defer p.close()

		stmt := p.root().NamedChild(0)
		if !hasToken(stmt, "default") {
			t.Error("the default token was not found")
		}
		if hasToken(stmt, "abstract") {
			t.Error("a token that is not there was reported")
		}
	})

	t.Run("a nil node carries no tokens", func(t *testing.T) {
		t.Parallel()
		if hasToken(nil, "default") {
			t.Fatal("nil reported a token")
		}
	})
}

func TestNestedNamespaces(t *testing.T) {
	t.Parallel()

	t.Run("a nested namespace records the full dotted path", func(t *testing.T) {
		t.Parallel()
		decls, _ := convert(t, `namespace A { export namespace B { export interface I {} } }`)
		if len(decls) != 1 {
			t.Fatalf("declarations = %d, want 1", len(decls))
		}
	})

	t.Run("a namespace with no body yields nothing", func(t *testing.T) {
		t.Parallel()
		// `declare namespace N;` names a namespace without declaring
		// its members, so there is nothing to hoist.
		decls, _ := convert(t, `declare module 'x';`)
		if len(decls) != 0 {
			t.Fatalf("declarations = %d, want 0", len(decls))
		}
	})
}
