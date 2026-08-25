// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/node"
)

func makeInterface() *node.Interface {
	return &node.Interface{
		Name:    "Repo",
		Package: "github.com/example/repo",
		Methods: []*node.Method{
			{Name: "Get"},
			{Name: "Save"},
		},
	}
}

func TestInterface_QName(t *testing.T) {
	t.Parallel()

	t.Run("includes package when present", func(t *testing.T) {
		t.Parallel()
		assertEqualString(t, makeInterface().QName(), "github.com/example/repo.Repo")
	})

	t.Run("returns just the name when package is empty", func(t *testing.T) {
		t.Parallel()
		i := &node.Interface{Name: "Foo"}
		assertEqualString(t, i.QName(), "Foo")
	})
}

func TestInterface_OwnerContract(t *testing.T) {
	t.Parallel()

	t.Run("OwnerName returns the bare identifier", func(t *testing.T) {
		t.Parallel()
		assertEqualString(t, makeInterface().OwnerName(), "Repo")
	})

	t.Run("OwnerQName mirrors QName", func(t *testing.T) {
		t.Parallel()
		i := makeInterface()
		assertEqualString(t, i.OwnerQName(), i.QName())
	})
}

func TestInterface_MethodByName(t *testing.T) {
	t.Parallel()

	t.Run("returns the matching method", func(t *testing.T) {
		t.Parallel()
		got := makeInterface().MethodByName("Save")
		if got == nil || got.Name != "Save" {
			t.Fatalf("MethodByName mismatch: %+v", got)
		}
	})

	t.Run("returns nil for an unknown name", func(t *testing.T) {
		t.Parallel()
		if got := makeInterface().MethodByName("missing"); got != nil {
			t.Fatalf("MethodByName(unknown) = %+v, want nil", got)
		}
	})
}

func TestInterface_MethodsWith(t *testing.T) {
	t.Parallel()

	t.Run("filters methods by predicate", func(t *testing.T) {
		t.Parallel()
		got := makeInterface().MethodsWith(func(m *node.Method) bool { return m.Name == "Get" })
		if len(got) != 1 || got[0].Name != "Get" {
			t.Fatalf("MethodsWith filter mismatch: %+v", got)
		}
	})
}

func TestInterface_IsGeneric(t *testing.T) {
	t.Parallel()

	t.Run("returns true when type params declared", func(t *testing.T) {
		t.Parallel()
		i := makeInterface()
		i.TypeParams = []*node.TypeParam{{Name: "T"}}
		if !i.IsGeneric() {
			t.Fatalf("generic interface should report IsGeneric true")
		}
	})

	t.Run("returns false when no type params declared", func(t *testing.T) {
		t.Parallel()
		if makeInterface().IsGeneric() {
			t.Fatalf("non-generic interface should report IsGeneric false")
		}
	})
}

func TestInterface_FieldByName(t *testing.T) {
	t.Parallel()

	// An interface with data members: the shape a TypeScript
	// interface takes, and the reason [node.Interface] carries a
	// field list at all.
	withFields := func() *node.Interface {
		return &node.Interface{
			Name:    "User",
			Package: "./users",
			Fields: []*node.Field{
				{Name: "id", Type: &node.TypeRef{Name: "string"}},
				{Name: "age", Type: &node.TypeRef{Name: "number"}},
			},
			Methods: []*node.Method{{Name: "greet"}},
		}
	}

	t.Run("returns the matching field", func(t *testing.T) {
		t.Parallel()
		got := withFields().FieldByName("age")
		if got == nil || got.Name != "age" {
			t.Fatalf("FieldByName mismatch: %+v", got)
		}
	})

	t.Run("returns nil for an unknown name", func(t *testing.T) {
		t.Parallel()
		if got := withFields().FieldByName("missing"); got != nil {
			t.Fatalf("FieldByName(unknown) = %+v, want nil", got)
		}
	})

	t.Run("returns nil when the interface declares no fields", func(t *testing.T) {
		t.Parallel()
		// A Go interface is a method set, so this is the common case
		// for every language whose interfaces carry no data.
		if got := makeInterface().FieldByName("anything"); got != nil {
			t.Fatalf("FieldByName on a method-set interface = %+v, want nil", got)
		}
	})

	t.Run("does not confuse a field with a method of the same name", func(t *testing.T) {
		t.Parallel()
		i := &node.Interface{
			Fields:  []*node.Field{{Name: "value"}},
			Methods: []*node.Method{{Name: "value"}},
		}
		if f := i.FieldByName("value"); f == nil {
			t.Fatal("FieldByName did not find the field")
		}
		if m := i.MethodByName("value"); m == nil {
			t.Fatal("MethodByName did not find the method")
		}
	})
}

func TestInterface_FieldsWith(t *testing.T) {
	t.Parallel()

	i := &node.Interface{
		Fields: []*node.Field{
			{Name: "id"},
			{Name: "name"},
			{Name: "id2"},
		},
	}

	t.Run("returns matching fields in declaration order", func(t *testing.T) {
		t.Parallel()
		got := i.FieldsWith(func(f *node.Field) bool {
			return strings.HasPrefix(f.Name, "id")
		})
		if len(got) != 2 || got[0].Name != "id" || got[1].Name != "id2" {
			t.Fatalf("FieldsWith = %+v, want id then id2", got)
		}
	})

	t.Run("returns an empty slice when nothing matches", func(t *testing.T) {
		t.Parallel()
		got := i.FieldsWith(func(*node.Field) bool { return false })
		if len(got) != 0 {
			t.Fatalf("FieldsWith(never) = %+v, want empty", got)
		}
	})
}
