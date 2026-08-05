// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node_test

import (
	"encoding/json"
	"testing"

	"go.thesmos.sh/eidos/node"
)

func TestRewireOwners(t *testing.T) {
	t.Parallel()

	t.Run("nil package is a no-op", func(t *testing.T) {
		t.Parallel()
		node.RewireOwners(nil)
	})

	t.Run("attaches every Owner back-pointer on a deserialized package", func(t *testing.T) {
		t.Parallel()
		pkg := makeRichPackage()
		raw, err := json.Marshal(pkg) //nolint:musttag // tags are present on node types; lint sees them indirectly
		if err != nil {
			t.Fatalf("Marshal returned error: %v", err)
		}

		var roundtrip node.Package
		if err := json.Unmarshal(raw, &roundtrip); err != nil { //nolint:musttag // tags are present on node types
			t.Fatalf("Unmarshal returned error: %v", err)
		}

		// Before RewireOwners every Owner is nil; afterwards every
		// reachable back-pointer is wired to its concrete host.
		s := roundtrip.Structs[0]
		if s.Fields[0].Owner != nil {
			t.Fatalf("expected Owner nil before rewire; got %+v", s.Fields[0].Owner)
		}

		node.RewireOwners(&roundtrip)

		if got := roundtrip.Files[0].Owner; got != &roundtrip {
			t.Fatalf("file Owner not wired: got %+v", got)
		}
		if got := roundtrip.Files[0].Imports[0].Owner; got != roundtrip.Files[0] {
			t.Fatalf("file-import Owner not wired: got %+v", got)
		}
		if got := roundtrip.Imports[0].Owner; got != &roundtrip {
			t.Fatalf("package-import Owner not wired: got %+v", got)
		}
		if got := s.Fields[0].Owner; got != s {
			t.Fatalf("field Owner not wired: got %+v", got)
		}
		if got := s.Methods[0].Owner; got != s {
			t.Fatalf("method Owner not wired: got %+v", got)
		}
		if got := s.Methods[0].Params[0].Owner; got != s.Methods[0] {
			t.Fatalf("method param Owner not wired: got %+v", got)
		}
		if got := s.Methods[0].TypeParams[0].Owner; got != s.Methods[0] {
			t.Fatalf("method type-param Owner not wired: got %+v", got)
		}
		if got := s.Embeds[0].Owner; got != s {
			t.Fatalf("struct embed Owner not wired: got %+v", got)
		}
		if got := s.TypeParams[0].Owner; got != s {
			t.Fatalf("struct type-param Owner not wired: got %+v", got)
		}

		i := roundtrip.Interfaces[0]
		if got := i.Methods[0].Owner; got != i {
			t.Fatalf("interface method Owner not wired: got %+v", got)
		}
		if got := i.Embeds[0].Owner; got != i {
			t.Fatalf("interface embed Owner not wired: got %+v", got)
		}

		fn := roundtrip.Functions[0]
		if got := fn.Params[0].Owner; got != fn {
			t.Fatalf("function param Owner not wired: got %+v", got)
		}
		if got := fn.TypeParams[0].Owner; got != fn {
			t.Fatalf("function type-param Owner not wired: got %+v", got)
		}

		e := roundtrip.Enums[0]
		if got := e.Variants[0].Owner; got != e {
			t.Fatalf("enum variant Owner not wired: got %+v", got)
		}

		a := roundtrip.Aliases[0]
		if got := a.TypeParams[0].Owner; got != a {
			t.Fatalf("alias type-param Owner not wired: got %+v", got)
		}
	})

	t.Run("wires alias.Methods Owner and child params after round-trip", func(t *testing.T) {
		t.Parallel()
		method := &node.Method{
			Name:    "Mul",
			Params:  []*node.Param{{Name: "by", Type: namedRef("", "int")}},
			Returns: node.AnonReturns(namedRef("", "int")),
		}
		alias := &node.Alias{
			Name:    "Seconds",
			Package: "p",
			Target:  namedRef("", "int"),
			Methods: []*node.Method{method},
		}
		pkg := &node.Package{Name: "p", Path: "p", Aliases: []*node.Alias{alias}}
		node.RewireOwners(pkg)
		if method.Owner != alias {
			t.Fatalf("alias method Owner not wired: got %+v", method.Owner)
		}
		if method.Params[0].Owner != method {
			t.Fatalf("alias method param Owner not wired: got %+v", method.Params[0].Owner)
		}
	})

	t.Run("idempotent: calling twice leaves the graph unchanged", func(t *testing.T) {
		t.Parallel()
		pkg := makeRichPackage()
		node.RewireOwners(pkg)
		first := pkg.Structs[0].Fields[0].Owner
		node.RewireOwners(pkg)
		if pkg.Structs[0].Fields[0].Owner != first {
			t.Fatalf("second RewireOwners changed wiring")
		}
	})

	t.Run("wires inline anonymous struct field and embed owners to the enclosing ref", func(t *testing.T) {
		t.Parallel()
		field := &node.Field{Name: "ID", Type: namedRef("", "string")}
		embed := &node.Embed{Type: namedRef("io", "Reader")}
		ref := &node.TypeRef{
			TypeKind: node.TypeRefAnonStruct,
			Fields:   []*node.Field{field},
			Embeds:   []*node.Embed{embed},
		}
		pkg := &node.Package{
			Name: "p", Path: "p",
			Variables: []*node.Variable{{Name: "v", Type: ref}},
		}
		node.RewireOwners(pkg)
		if field.Owner != ref {
			t.Fatalf("anon-struct field Owner not wired to enclosing ref: got %+v", field.Owner)
		}
		if embed.Owner != ref {
			t.Fatalf("anon-struct embed Owner not wired to enclosing ref: got %+v", embed.Owner)
		}
	})

	t.Run("wires inline anonymous interface method and embed owners to the enclosing ref", func(t *testing.T) {
		t.Parallel()
		method := &node.Method{Name: "Read"}
		embed := &node.Embed{Type: namedRef("io", "Closer")}
		ref := &node.TypeRef{
			TypeKind: node.TypeRefAnonInterface,
			Methods:  []*node.Method{method},
			Embeds:   []*node.Embed{embed},
		}
		pkg := &node.Package{
			Name: "p", Path: "p",
			Variables: []*node.Variable{{Name: "v", Type: ref}},
		}
		node.RewireOwners(pkg)
		if method.Owner != ref {
			t.Fatalf("anon-interface method Owner not wired to enclosing ref: got %+v", method.Owner)
		}
		if embed.Owner != ref {
			t.Fatalf("anon-interface embed Owner not wired to enclosing ref: got %+v", embed.Owner)
		}
	})
}
