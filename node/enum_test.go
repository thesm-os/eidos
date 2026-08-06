// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
)

func makeEnum() *node.Enum {
	return &node.Enum{
		Name:       "Status",
		Package:    "github.com/example/status",
		Underlying: namedRef("", "int"),
		Variants: []*node.EnumVariant{
			{Name: "StatusActive", Value: "1"},
			{Name: "StatusPaused", Value: "2"},
		},
	}
}

func TestEnum_QName(t *testing.T) {
	t.Parallel()

	t.Run("includes package when present", func(t *testing.T) {
		t.Parallel()
		assertEqualString(t, makeEnum().QName(), "github.com/example/status.Status")
	})

	t.Run("returns just the name when package is empty", func(t *testing.T) {
		t.Parallel()
		e := &node.Enum{Name: "Status"}
		assertEqualString(t, e.QName(), "Status")
	})
}

func TestEnum_OwnerContract(t *testing.T) {
	t.Parallel()

	t.Run("OwnerName returns the bare identifier", func(t *testing.T) {
		t.Parallel()
		assertEqualString(t, makeEnum().OwnerName(), "Status")
	})

	t.Run("OwnerQName mirrors QName", func(t *testing.T) {
		t.Parallel()
		e := makeEnum()
		assertEqualString(t, e.OwnerQName(), e.QName())
	})
}

func TestEnum_VariantByName(t *testing.T) {
	t.Parallel()

	t.Run("returns the matching variant", func(t *testing.T) {
		t.Parallel()
		got := makeEnum().VariantByName("StatusActive")
		if got == nil || got.Name != "StatusActive" {
			t.Fatalf("VariantByName mismatch: %+v", got)
		}
	})

	t.Run("returns nil for an unknown name", func(t *testing.T) {
		t.Parallel()
		if got := makeEnum().VariantByName("missing"); got != nil {
			t.Fatalf("VariantByName(unknown) = %+v, want nil", got)
		}
	})
}

func TestEnum_VariantsWith(t *testing.T) {
	t.Parallel()

	t.Run("filters variants by predicate", func(t *testing.T) {
		t.Parallel()
		got := makeEnum().VariantsWith(func(v *node.EnumVariant) bool { return v.Value == "1" })
		if len(got) != 1 || got[0].Name != "StatusActive" {
			t.Fatalf("VariantsWith filter mismatch: %+v", got)
		}
	})
}

func TestEnum_HasUnderlying(t *testing.T) {
	t.Parallel()

	t.Run("returns true when Underlying is set", func(t *testing.T) {
		t.Parallel()
		if !makeEnum().HasUnderlying() {
			t.Fatalf("HasUnderlying should be true when Underlying is set")
		}
	})

	t.Run("returns false when Underlying is nil", func(t *testing.T) {
		t.Parallel()
		var e node.Enum
		if e.HasUnderlying() {
			t.Fatalf("HasUnderlying should be false when Underlying is nil")
		}
	})
}

// enumWithMethods returns an enum carrying a two-method set, the
// shape a typed-iota group with `String` / `MarshalText` produces.
func enumWithMethods() *node.Enum {
	e := makeEnum()
	e.Methods = []*node.Method{
		{Name: "String"},
		{Name: "MarshalText"},
	}
	return e
}

// TestEnum_MethodByName covers the accessor a generator uses to
// decide what it may assert.
//
// The method set is the whole point of the field: String plus Parse
// admits a round-trip law, String alone admits only distinctness, and
// neither leaves just the constant set. A generator that cannot ask
// this question emits the weakest form for every enum.
func TestEnum_MethodByName(t *testing.T) {
	t.Parallel()

	t.Run("returns the matching method", func(t *testing.T) {
		t.Parallel()
		got := enumWithMethods().MethodByName("String")
		if got == nil || got.Name != "String" {
			t.Fatalf("MethodByName mismatch: %+v", got)
		}
	})

	t.Run("returns nil for an unknown name", func(t *testing.T) {
		t.Parallel()
		if got := enumWithMethods().MethodByName("Parse"); got != nil {
			t.Fatalf("MethodByName(unknown) = %+v, want nil", got)
		}
	})

	t.Run("returns nil on a methodless enum", func(t *testing.T) {
		t.Parallel()
		if got := makeEnum().MethodByName("String"); got != nil {
			t.Fatalf("MethodByName on methodless enum = %+v, want nil", got)
		}
	})
}

func TestEnum_MethodsWith(t *testing.T) {
	t.Parallel()

	t.Run("filters methods by predicate", func(t *testing.T) {
		t.Parallel()
		got := enumWithMethods().MethodsWith(func(m *node.Method) bool { return m.Name == "String" })
		if len(got) != 1 || got[0].Name != "String" {
			t.Fatalf("MethodsWith filter mismatch: %+v", got)
		}
	})

	t.Run("preserves declaration order", func(t *testing.T) {
		t.Parallel()
		got := enumWithMethods().MethodsWith(func(*node.Method) bool { return true })
		if len(got) != 2 || got[0].Name != "String" || got[1].Name != "MarshalText" {
			t.Fatalf("MethodsWith must preserve source order; got %+v", got)
		}
	})

	t.Run("returns an empty slice on a methodless enum", func(t *testing.T) {
		t.Parallel()
		got := makeEnum().MethodsWith(func(*node.Method) bool { return true })
		if len(got) != 0 {
			t.Fatalf("expected no methods; got %+v", got)
		}
	})
}
