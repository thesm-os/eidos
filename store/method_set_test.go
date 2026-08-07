// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/store"
)

// ifaceNamed builds an interface declaring the named methods.
func ifaceNamed(pkg, name string, methods ...string) *node.Interface {
	i := &node.Interface{Name: name, Package: pkg}
	for _, m := range methods {
		i.Methods = append(i.Methods, &node.Method{Name: m})
	}
	return i
}

// structNamed builds a struct declaring the named methods.
func structNamed(pkg, name string, methods ...string) *node.Struct {
	s := &node.Struct{Name: name, Package: pkg}
	for _, m := range methods {
		s.Methods = append(s.Methods, &node.Method{Name: m})
	}
	return s
}

// embedRef appends an embed of pkg.name to i.
func embedRef(i *node.Interface, pkg, name string) {
	i.Embeds = append(i.Embeds, &node.Embed{
		Type:  &node.TypeRef{TypeKind: node.TypeRefNamed, Package: pkg, Name: name},
		Owner: i,
	})
}

// readerOver builds a store holding pkg and returns a reader on it.
func readerOver(t *testing.T, pkg *node.Package) *store.Reader {
	t.Helper()
	s := store.New()
	assertNoError(t, s.Nodes().AddPackage(pkg))
	return store.NewReader(s)
}

// TestReader_MethodSet pins the resolution a generated double
// depends on, against a real graph rather than a hand-supplied
// resolver.
func TestReader_MethodSet(t *testing.T) {
	t.Parallel()

	const pkg = "example.com/x"

	t.Run("resolves an embed declared in the same package", func(t *testing.T) {
		t.Parallel()
		reader := ifaceNamed(pkg, "Reader", "Read")
		store2 := ifaceNamed(pkg, "Store", "Put")
		embedRef(store2, pkg, "Reader")
		r := readerOver(t, &node.Package{
			Name: "x", Path: pkg,
			Interfaces: []*node.Interface{reader, store2},
		})
		got := r.MethodSet(store2)
		if !got.OK() {
			t.Fatalf("Issues = %+v, want none", got.Issues)
		}
		if got.ByName("Read") == nil {
			t.Fatalf("Methods missing the embedded Read")
		}
	})

	t.Run("an embed from a package this run did not load is unresolved", func(t *testing.T) {
		t.Parallel()
		// The honest answer for a narrow run: reporting it lets a
		// generator say so instead of emitting a short method set as
		// though it were complete.
		s := ifaceNamed(pkg, "Store", "Put")
		embedRef(s, "example.com/other", "Reader")
		r := readerOver(t, &node.Package{
			Name: "x", Path: pkg,
			Interfaces: []*node.Interface{s},
		})
		got := r.MethodSet(s)
		if len(got.Issues) != 1 || got.Issues[0].Reason != node.ReasonUnresolved {
			t.Fatalf("Issues = %+v, want one ReasonUnresolved", got.Issues)
		}
	})

	t.Run("an embed naming a struct reports NonInterface", func(t *testing.T) {
		t.Parallel()
		// Distinct from unresolved: a source defect no wider run
		// fixes, so a caller must not send the author looking at
		// their -target filter.
		s := ifaceNamed(pkg, "Store", "Put")
		embedRef(s, pkg, "Config")
		r := readerOver(t, &node.Package{
			Name: "x", Path: pkg,
			Interfaces: []*node.Interface{s},
			Structs:    []*node.Struct{structNamed(pkg, "Config")},
		})
		got := r.MethodSet(s)
		if len(got.Issues) != 1 || got.Issues[0].Reason != node.ReasonNonInterface {
			t.Fatalf("Issues = %+v, want one ReasonNonInterface", got.Issues)
		}
	})

	t.Run("resolution is not narrowed by the scope predicate", func(t *testing.T) {
		t.Parallel()
		// A qualified-name lookup bypasses scope by design: otherwise
		// the method set would change with the user's -target filter,
		// and a double generated under a narrow run would be missing
		// methods the interface has.
		reader := ifaceNamed(pkg, "Reader", "Read")
		s := ifaceNamed(pkg, "Store", "Put")
		embedRef(s, pkg, "Reader")
		st := store.New()
		assertNoError(t, st.Nodes().AddPackage(&node.Package{
			Name: "x", Path: pkg,
			Interfaces: []*node.Interface{reader, s},
		}))
		scoped := store.NewScopedReader(st, func(n node.Node) bool {
			i, ok := n.(*node.Interface)
			return ok && i.Name == "Store"
		})
		got := scoped.MethodSet(s)
		if !got.OK() || got.ByName("Read") == nil {
			t.Fatalf("scoped MethodSet = %+v; the embed must still resolve", got)
		}
	})

	t.Run("records the resolved interface as a read", func(t *testing.T) {
		t.Parallel()
		// The resolved interface is an input to whatever the caller
		// emits, so it belongs in the fingerprint — otherwise a warm
		// cache serves output that predates an edit to the embed.
		reader := ifaceNamed(pkg, "Reader", "Read")
		s := ifaceNamed(pkg, "Store", "Put")
		embedRef(s, pkg, "Reader")
		r := readerOver(t, &node.Package{
			Name: "x", Path: pkg,
			Interfaces: []*node.Interface{reader, s},
		})
		before := r.ReadSet().Len()
		r.MethodSet(s)
		if r.ReadSet().Len() == before {
			t.Fatalf("MethodSet recorded no read; the embed would not reach the fingerprint")
		}
	})
}

// TestReader_Implementers pins the derivation of the concrete types
// a generated suite or double is checked against.
func TestReader_Implementers(t *testing.T) {
	t.Parallel()

	const pkg = "example.com/x"

	// graph holds one interface plus three structs: one satisfying,
	// one short a method, one unrelated.
	graph := func(t *testing.T) (*store.Reader, *node.Interface) {
		t.Helper()
		iface := ifaceNamed(pkg, "Store", "Get", "Put")
		r := readerOver(t, &node.Package{
			Name: "x", Path: pkg,
			Interfaces: []*node.Interface{iface},
			Structs: []*node.Struct{
				structNamed(pkg, "MemStore", "Get", "Put"),
				structNamed(pkg, "ReadOnly", "Get"),
				structNamed(pkg, "Unrelated", "Ping"),
			},
		})
		return r, iface
	}

	t.Run("returns a struct declaring every method", func(t *testing.T) {
		t.Parallel()
		r, iface := graph(t)
		got := r.Implementers(iface)
		if len(got) != 1 || got[0].Name != "MemStore" {
			t.Fatalf("Implementers = %+v, want [MemStore]", got)
		}
	})

	t.Run("omits a struct missing one method", func(t *testing.T) {
		t.Parallel()
		r, iface := graph(t)
		for _, s := range r.Implementers(iface) {
			if s.Name == "ReadOnly" {
				t.Fatalf("a struct short a method must not count as an implementer")
			}
		}
	})

	t.Run("counts a method through an embedded interface", func(t *testing.T) {
		t.Parallel()
		// The required set is the resolved one, so a struct must
		// satisfy the embedded methods too.
		base := ifaceNamed(pkg, "Closer", "Close")
		iface := ifaceNamed(pkg, "Store", "Get")
		embedRef(iface, pkg, "Closer")
		r := readerOver(t, &node.Package{
			Name: "x", Path: pkg,
			Interfaces: []*node.Interface{base, iface},
			Structs: []*node.Struct{
				structNamed(pkg, "Full", "Get", "Close"),
				structNamed(pkg, "Partial", "Get"),
			},
		})
		got := r.Implementers(iface)
		if len(got) != 1 || got[0].Name != "Full" {
			t.Fatalf("Implementers = %+v, want [Full]", got)
		}
	})

	t.Run("returns nil for an interface whose method set did not resolve", func(t *testing.T) {
		t.Parallel()
		// Every struct trivially satisfies an empty set, so answering
		// "all of them" for an interface this run failed to read is
		// worse than answering nothing.
		iface := ifaceNamed(pkg, "Store", "Get")
		embedRef(iface, "example.com/other", "Absent")
		r := readerOver(t, &node.Package{
			Name: "x", Path: pkg,
			Interfaces: []*node.Interface{iface},
			Structs:    []*node.Struct{structNamed(pkg, "MemStore", "Get")},
		})
		if got := r.Implementers(iface); got != nil {
			t.Fatalf("Implementers = %+v, want nil for an unresolved interface", got)
		}
	})

	t.Run("returns nil for an interface declaring no methods", func(t *testing.T) {
		t.Parallel()
		iface := ifaceNamed(pkg, "Any")
		r := readerOver(t, &node.Package{
			Name: "x", Path: pkg,
			Interfaces: []*node.Interface{iface},
			Structs:    []*node.Struct{structNamed(pkg, "MemStore", "Get")},
		})
		if got := r.Implementers(iface); got != nil {
			t.Fatalf("Implementers = %+v, want nil for an empty method set", got)
		}
	})
}
