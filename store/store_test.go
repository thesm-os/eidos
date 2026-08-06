// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/contract"
	"go.thesmos.sh/eidos/core/kind"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/store"
)

func TestNew(t *testing.T) {
	t.Parallel()

	t.Run("returns a Store with non-nil Nodes and Emit views", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		if s.Nodes() == nil {
			t.Fatalf("Nodes view should be non-nil")
		}
		if s.Emit() == nil {
			t.Fatalf("Emit view should be non-nil")
		}
	})
}

func TestStore_Nodes(t *testing.T) {
	t.Parallel()

	t.Run("returns the same NodeView on every call", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		if a, b := s.Nodes(), s.Nodes(); a != b {
			t.Fatalf("Nodes() should return the cached view")
		}
	})
}

func TestStore_Emit(t *testing.T) {
	t.Parallel()

	t.Run("returns the same EmitView on every call", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		if a, b := s.Emit(), s.Emit(); a != b {
			t.Fatalf("Emit() should return the cached view")
		}
	})
}

// TestStore_RewireMethodOwners pins the cache-replay-safety
// invariant for top-level methods: a method whose Owner is nil
// but whose OwnerRef carries a resolvable {Kind, QName} tuple
// reconstructs the live Owner pointer by looking up the
// referenced entity in the appropriate store bucket (Nodes or
// Emit). After the pass, every routed top-level method satisfies
// the framework's "Owner is always populated" invariant the
// downstream layout, render, and plugin-query passes rely on.
func TestStore_RewireMethodOwners(t *testing.T) {
	t.Parallel()

	t.Run("resolves a node-side enum Owner from OwnerRef", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		srcEnum := &node.Enum{Name: "Status", Package: "example.com/store"}
		if err := s.Nodes().AddPackage(&node.Package{
			Name: "store", Path: "example.com/store",
			Enums: []*node.Enum{srcEnum},
		}); err != nil {
			t.Fatalf("Nodes.AddPackage: %v", err)
		}
		m := &emit.Method{
			Name:    "String",
			Package: "example.com/store",
			OwnerRef: contract.OwnerRef{
				Kind:  srcEnum.Kind(),
				QName: srcEnum.QName(),
			},
		}
		if err := s.Emit().AddPackage(&emit.Package{
			Name: "store", Path: "example.com/store",
			Methods: []*emit.Method{m},
		}); err != nil {
			t.Fatalf("Emit.AddPackage: %v", err)
		}
		s.RewireMethodOwners()
		if m.Owner == nil {
			t.Fatalf("Owner not resolved")
		}
		if got, want := m.Owner.OwnerQName(), "example.com/store.Status"; got != want {
			t.Fatalf("Owner.OwnerQName = %q, want %q", got, want)
		}
	})

	t.Run("resolves every owner-eligible kind against its own graph's bucket", func(t *testing.T) {
		t.Parallel()
		// The resolve dispatches on OwnerRef.Kind twice: first on the
		// `emit.` prefix to pick the graph, then on the kind itself to
		// pick the bucket. Seven types are owner-eligible — four
		// source-side, three emit-side — and a ref for any of them has
		// to land on the live entity rather than nil.
		const pkg = "example.com/store"
		s := store.New()

		srcStruct := &node.Struct{Name: "S", Package: pkg}
		srcIface := &node.Interface{Name: "I", Package: pkg}
		srcEnum := &node.Enum{Name: "E", Package: pkg}
		srcAlias := &node.Alias{Name: "A", Package: pkg}
		if err := s.Nodes().AddPackage(&node.Package{
			Name: "store", Path: pkg,
			Structs:    []*node.Struct{srcStruct},
			Interfaces: []*node.Interface{srcIface},
			Enums:      []*node.Enum{srcEnum},
			Aliases:    []*node.Alias{srcAlias},
		}); err != nil {
			t.Fatalf("Nodes.AddPackage: %v", err)
		}

		emStruct := &emit.Struct{Name: "ES", Package: pkg}
		emIface := &emit.Interface{Name: "EI", Package: pkg}
		emAlias := &emit.Alias{Name: "EA", Package: pkg}

		owners := []struct {
			method string
			kind   kind.Kind
			qname  string
		}{
			{"OnSrcStruct", srcStruct.Kind(), srcStruct.QName()},
			{"OnSrcIface", srcIface.Kind(), srcIface.QName()},
			{"OnSrcEnum", srcEnum.Kind(), srcEnum.QName()},
			{"OnSrcAlias", srcAlias.Kind(), srcAlias.QName()},
			{"OnEmitStruct", emStruct.Kind(), emStruct.QName()},
			{"OnEmitIface", emIface.Kind(), emIface.QName()},
			{"OnEmitAlias", emAlias.Kind(), emAlias.QName()},
		}
		methods := make([]*emit.Method, 0, len(owners))
		for _, o := range owners {
			methods = append(methods, &emit.Method{
				Name:     o.method,
				Package:  pkg,
				OwnerRef: contract.OwnerRef{Kind: o.kind, QName: o.qname},
			})
		}
		if err := s.Emit().AddPackage(&emit.Package{
			Name: "store", Path: pkg,
			Structs:    []*emit.Struct{emStruct},
			Interfaces: []*emit.Interface{emIface},
			Aliases:    []*emit.Alias{emAlias},
			Methods:    methods,
		}); err != nil {
			t.Fatalf("Emit.AddPackage: %v", err)
		}

		s.RewireMethodOwners()

		for i, o := range owners {
			if got := methods[i].Owner; got == nil {
				t.Errorf("%s: Owner not resolved for kind %q", o.method, o.kind)
			} else if got.OwnerQName() != o.qname {
				t.Errorf("%s: Owner.OwnerQName = %q, want %q", o.method, got.OwnerQName(), o.qname)
			}
		}
	})

	t.Run("an unresolvable OwnerRef leaves Owner nil on both graphs", func(t *testing.T) {
		t.Parallel()
		// A ref naming a kind the resolve does not dispatch on, or a
		// qname absent from the bucket, must fall through rather than
		// resolve to something adjacent. The surrounding pass reports
		// the gap; a wrong pointer would be silent.
		const pkg = "example.com/store"
		s := store.New()

		misses := []struct {
			method string
			ref    contract.OwnerRef
		}{
			{"NodeKindMiss", contract.OwnerRef{Kind: node.KindStruct, QName: pkg + ".Absent"}},
			{"NodeKindUnknown", contract.OwnerRef{Kind: node.KindFunction, QName: pkg + ".S"}},
			{"EmitKindMiss", contract.OwnerRef{Kind: emit.KindStruct, QName: pkg + ".Absent"}},
			{"EmitKindUnknown", contract.OwnerRef{Kind: emit.KindEnum, QName: pkg + ".EE"}},
		}
		methods := make([]*emit.Method, 0, len(misses))
		for _, m := range misses {
			methods = append(methods, &emit.Method{Name: m.method, Package: pkg, OwnerRef: m.ref})
		}
		if err := s.Emit().AddPackage(&emit.Package{
			Name: "store", Path: pkg, Methods: methods,
		}); err != nil {
			t.Fatalf("Emit.AddPackage: %v", err)
		}

		s.RewireMethodOwners()

		for i, m := range misses {
			if got := methods[i].Owner; got != nil {
				t.Errorf("%s: Owner = %+v, want nil", m.method, got)
			}
		}
	})

	t.Run("zero OwnerRef leaves Owner nil (no rewire)", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		m := &emit.Method{Name: "Standalone"}
		if err := s.Emit().AddPackage(&emit.Package{
			Name: "x", Path: "x",
			Methods: []*emit.Method{m},
		}); err != nil {
			t.Fatalf("Emit.AddPackage: %v", err)
		}
		s.RewireMethodOwners()
		if m.Owner != nil {
			t.Fatalf("Owner should remain nil for empty OwnerRef; got %+v", m.Owner)
		}
	})

	t.Run("existing Owner is preserved (no clobber)", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		srcEnum := &node.Enum{Name: "Status", Package: "example.com/store"}
		preset := &node.Enum{Name: "PreOwner", Package: "example.com/store"}
		if err := s.Nodes().AddPackage(&node.Package{
			Name: "store", Path: "example.com/store",
			Enums: []*node.Enum{srcEnum, preset},
		}); err != nil {
			t.Fatalf("Nodes.AddPackage: %v", err)
		}
		m := &emit.Method{
			Name:    "String",
			Package: "example.com/store",
			Owner:   preset,
			OwnerRef: contract.OwnerRef{
				Kind:  srcEnum.Kind(),
				QName: srcEnum.QName(),
			},
		}
		if err := s.Emit().AddPackage(&emit.Package{
			Name: "store", Path: "example.com/store",
			Methods: []*emit.Method{m},
		}); err != nil {
			t.Fatalf("Emit.AddPackage: %v", err)
		}
		s.RewireMethodOwners()
		if m.Owner != preset {
			t.Fatalf("preset Owner clobbered: got %v want %v", m.Owner, preset)
		}
	})
}
