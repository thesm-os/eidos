// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/emit"
)

func makeInterface() *emit.Interface {
	return &emit.Interface{
		Name:    "Repo",
		Package: "users",
		Methods: []*emit.Method{
			{Name: "Get"},
			{Name: "Save"},
		},
	}
}

func TestInterface_Kind(t *testing.T) {
	t.Parallel()

	t.Run("reports KindInterface", func(t *testing.T) {
		t.Parallel()
		var i emit.Interface
		if i.Kind() != emit.KindInterface {
			t.Fatalf("Kind = %s, want %s", i.Kind(), emit.KindInterface)
		}
	})
}

func TestInterface_QName(t *testing.T) {
	t.Parallel()

	t.Run("includes package when present", func(t *testing.T) {
		t.Parallel()
		assertEqualString(t, makeInterface().QName(), "users.Repo")
	})

	t.Run("returns just the name when package is empty", func(t *testing.T) {
		t.Parallel()
		assertEqualString(t, (&emit.Interface{Name: "Foo"}).QName(), "Foo")
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

func TestInterface_IsGeneric(t *testing.T) {
	t.Parallel()

	t.Run("reports true when type params declared", func(t *testing.T) {
		t.Parallel()
		i := makeInterface()
		i.TypeParams = []*emit.TypeParam{{Name: "T"}}
		if !i.IsGeneric() {
			t.Fatalf("generic interface should report IsGeneric true")
		}
	})

	t.Run("reports false when no type params declared", func(t *testing.T) {
		t.Parallel()
		if makeInterface().IsGeneric() {
			t.Fatalf("non-generic interface should report IsGeneric false")
		}
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
		if makeInterface().MethodByName("missing") != nil {
			t.Fatalf("MethodByName(unknown) should be nil")
		}
	})
}

func TestInterface_MethodsWith(t *testing.T) {
	t.Parallel()

	t.Run("filters methods by predicate", func(t *testing.T) {
		t.Parallel()
		got := makeInterface().MethodsWith(func(m *emit.Method) bool { return m.Name == "Get" })
		if len(got) != 1 || got[0].Name != "Get" {
			t.Fatalf("MethodsWith mismatch: %+v", got)
		}
	})
}

func TestInterface_Slots(t *testing.T) {
	t.Parallel()

	t.Run("MethodsSlot, EmbedsSlot, and Slot are distinct and idempotent", func(t *testing.T) {
		t.Parallel()
		i := makeInterface()
		m1, m2 := i.MethodsSlot(), i.MethodsSlot()
		e1, e2 := i.EmbedsSlot(), i.EmbedsSlot()
		c1, c2 := i.Slot("custom"), i.Slot("custom")
		if m1 != m2 || e1 != e2 || c1 != c2 {
			t.Fatalf("slot lookups should be idempotent")
		}
		if m1 == e1 || m1 == c1 || e1 == c1 {
			t.Fatalf("slots must be distinct instances")
		}
	})
}

func TestInterface_Fields(t *testing.T) {
	t.Parallel()

	// An interface carrying data members: what a generator emitting
	// TypeScript produces, and why emit.Interface mirrors
	// node.Interface's field list.
	withFields := func() *emit.Interface {
		return &emit.Interface{
			Name:    "User",
			Package: "users",
			Fields: []*emit.Field{
				{Name: "id", Type: emit.Builtin("string")},
				{Name: "age", Type: emit.Builtin("number")},
			},
			Methods: []*emit.Method{{Name: "greet"}},
		}
	}

	t.Run("FieldByName returns the matching field", func(t *testing.T) {
		t.Parallel()
		got := withFields().FieldByName("age")
		if got == nil || got.Name != "age" {
			t.Fatalf("FieldByName mismatch: %+v", got)
		}
	})

	t.Run("FieldByName returns nil for an unknown name", func(t *testing.T) {
		t.Parallel()
		if got := withFields().FieldByName("missing"); got != nil {
			t.Fatalf("FieldByName(unknown) = %+v, want nil", got)
		}
	})

	t.Run("FieldByName returns nil on a method-set interface", func(t *testing.T) {
		t.Parallel()
		if got := makeInterface().FieldByName("anything"); got != nil {
			t.Fatalf("FieldByName = %+v, want nil", got)
		}
	})

	t.Run("FieldsWith returns matches in declaration order", func(t *testing.T) {
		t.Parallel()
		i := &emit.Interface{Fields: []*emit.Field{
			{Name: "id"}, {Name: "name"}, {Name: "id2"},
		}}
		got := i.FieldsWith(func(f *emit.Field) bool {
			return strings.HasPrefix(f.Name, "id")
		})
		if len(got) != 2 || got[0].Name != "id" || got[1].Name != "id2" {
			t.Fatalf("FieldsWith = %+v, want id then id2", got)
		}
	})

	t.Run("FieldsWith returns empty when nothing matches", func(t *testing.T) {
		t.Parallel()
		got := withFields().FieldsWith(func(*emit.Field) bool { return false })
		if len(got) != 0 {
			t.Fatalf("FieldsWith(never) = %+v, want empty", got)
		}
	})
}

func TestInterface_FieldsSlot(t *testing.T) {
	t.Parallel()

	t.Run("is idempotent and distinct from the other slots", func(t *testing.T) {
		t.Parallel()
		i := makeInterface()
		f1, f2 := i.FieldsSlot(), i.FieldsSlot()
		if f1 != f2 {
			t.Fatal("FieldsSlot lookups should be idempotent")
		}
		if f1 == i.MethodsSlot() || f1 == i.EmbedsSlot() {
			t.Fatal("the fields slot must be distinct from methods and embeds")
		}
	})

	t.Run("carries the Field element kind", func(t *testing.T) {
		t.Parallel()
		// The kind is a property of the slot NAME, so the typed
		// accessor and Slot("fields") must agree — otherwise which
		// constraint survives depends on which plugin ran first.
		i := makeInterface()
		if err := i.FieldsSlot().Append(&emit.Field{Name: "id"}, emit.Provenance{SetBy: "t"}); err != nil {
			t.Fatalf("appending a Field to the fields slot: %v", err)
		}
		if err := i.FieldsSlot().Append(&emit.Method{Name: "m"}, emit.Provenance{SetBy: "t"}); err == nil {
			t.Fatal("the fields slot accepted a Method")
		}
	})

	t.Run("Slot(fields) resolves to the same constrained slot", func(t *testing.T) {
		t.Parallel()
		i := makeInterface()
		if i.Slot("fields") != i.FieldsSlot() {
			t.Fatal("Slot(\"fields\") and FieldsSlot disagree")
		}
		if err := i.Slot("fields").Append(&emit.Method{Name: "m"}, emit.Provenance{SetBy: "t"}); err == nil {
			t.Fatal("Slot(\"fields\") is unconstrained where FieldsSlot is not")
		}
	})
}
