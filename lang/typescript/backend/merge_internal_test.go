// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"errors"
	"testing"

	"go.thesmos.sh/eidos/emit"
)

// prov is the provenance a test contribution carries.
var prov = emit.Provenance{SetBy: "test"}

func TestMergedFields(t *testing.T) {
	t.Parallel()

	t.Run("typed fields come before slot contributions", func(t *testing.T) {
		t.Parallel()
		// What the generator declared outright is what a reader
		// expects at the top; a cross-cutting plugin adds below it.
		s := &emit.Struct{Name: "S", Fields: []*emit.Field{{Name: "declared"}}}
		if err := s.FieldsSlot().Append(&emit.Field{Name: "injected"}, prov); err != nil {
			t.Fatalf("append: %v", err)
		}

		got, err := mergedFields(s)
		if err != nil {
			t.Fatalf("mergedFields: %v", err)
		}
		if len(got) != 2 || got[0].Name != "declared" || got[1].Name != "injected" {
			t.Fatalf("fields = %+v, want declared then injected", got)
		}
	})

	t.Run("an interface merges the same way", func(t *testing.T) {
		t.Parallel()
		i := &emit.Interface{Name: "I", Fields: []*emit.Field{{Name: "a"}}}
		if err := i.FieldsSlot().Append(&emit.Field{Name: "b"}, prov); err != nil {
			t.Fatalf("append: %v", err)
		}
		got, err := mergedFields(i)
		if err != nil {
			t.Fatalf("mergedFields: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("fields = %d, want 2", len(got))
		}
	})

	t.Run("two contributions of one name are rejected", func(t *testing.T) {
		t.Parallel()
		// One would be dropped, and which one is decided by plugin
		// registration order — so a run would silently emit what a
		// different ordering of the same plugins would not.
		s := &emit.Struct{Name: "S", Fields: []*emit.Field{{Name: "dup"}}}
		if err := s.FieldsSlot().Append(&emit.Field{Name: "dup"}, prov); err != nil {
			t.Fatalf("append: %v", err)
		}
		if _, err := mergedFields(s); !errors.Is(err, ErrDuplicateEntity) {
			t.Fatalf("mergedFields = %v, want ErrDuplicateEntity", err)
		}
	})

	t.Run("a host with no field list yields nothing", func(t *testing.T) {
		t.Parallel()
		got, err := mergedFields(&emit.Function{Name: "F"})
		if err != nil || got != nil {
			t.Fatalf("mergedFields = %+v, %v; want nothing", got, err)
		}
	})
}

func TestMergedMethods(t *testing.T) {
	t.Parallel()

	t.Run("typed methods come before slot contributions", func(t *testing.T) {
		t.Parallel()
		i := &emit.Interface{Name: "I", Methods: []*emit.Method{{Name: "declared"}}}
		if err := i.MethodsSlot().Append(&emit.Method{Name: "injected"}, prov); err != nil {
			t.Fatalf("append: %v", err)
		}
		got, err := mergedMethods(i)
		if err != nil {
			t.Fatalf("mergedMethods: %v", err)
		}
		if len(got) != 2 || got[0].Name != "declared" {
			t.Fatalf("methods = %+v", got)
		}
	})

	t.Run("a struct merges the same way", func(t *testing.T) {
		t.Parallel()
		s := &emit.Struct{Name: "S", Methods: []*emit.Method{{Name: "a"}}}
		if err := s.MethodsSlot().Append(&emit.Method{Name: "b"}, prov); err != nil {
			t.Fatalf("append: %v", err)
		}
		if got, err := mergedMethods(s); err != nil || len(got) != 2 {
			t.Fatalf("mergedMethods = %+v, %v", got, err)
		}
	})

	t.Run("two methods of one name are rejected", func(t *testing.T) {
		t.Parallel()
		s := &emit.Struct{Name: "S", Methods: []*emit.Method{{Name: "dup"}}}
		if err := s.MethodsSlot().Append(&emit.Method{Name: "dup"}, prov); err != nil {
			t.Fatalf("append: %v", err)
		}
		if _, err := mergedMethods(s); !errors.Is(err, ErrDuplicateEntity) {
			t.Fatalf("mergedMethods = %v, want ErrDuplicateEntity", err)
		}
	})

	t.Run("a host with no method list yields nothing", func(t *testing.T) {
		t.Parallel()
		got, err := mergedMethods(&emit.Constant{Name: "K"})
		if err != nil || got != nil {
			t.Fatalf("mergedMethods = %+v, %v; want nothing", got, err)
		}
	})
}

func TestMergedEmbeds(t *testing.T) {
	t.Parallel()

	t.Run("merges typed embeds with slot contributions", func(t *testing.T) {
		t.Parallel()
		i := &emit.Interface{Name: "I", Embeds: []*emit.Embed{{Type: emit.Builtin("A")}}}
		if err := i.EmbedsSlot().Append(&emit.Embed{Type: emit.Builtin("B")}, prov); err != nil {
			t.Fatalf("append: %v", err)
		}
		if got := mergedEmbeds(i); len(got) != 2 {
			t.Fatalf("embeds = %d, want 2", len(got))
		}
	})

	t.Run("a struct merges the same way", func(t *testing.T) {
		t.Parallel()
		s := &emit.Struct{Name: "S", Embeds: []*emit.Embed{{Type: emit.Builtin("A")}}}
		if got := mergedEmbeds(s); len(got) != 1 {
			t.Fatalf("embeds = %d, want 1", len(got))
		}
	})

	t.Run("a repeated embed is allowed", func(t *testing.T) {
		t.Parallel()
		// A class may implement an interface its base already
		// implements, so there is nothing to reject.
		s := &emit.Struct{Name: "S", Embeds: []*emit.Embed{
			{Type: emit.Builtin("A")},
			{Type: emit.Builtin("A")},
		}}
		if got := mergedEmbeds(s); len(got) != 2 {
			t.Fatalf("embeds = %d, want both kept", len(got))
		}
	})

	t.Run("a host with no embed list yields nothing", func(t *testing.T) {
		t.Parallel()
		if got := mergedEmbeds(&emit.Enum{Name: "E"}); got != nil {
			t.Fatalf("embeds = %+v, want nothing", got)
		}
	})
}

func TestMergedVariants(t *testing.T) {
	t.Parallel()

	t.Run("merges typed variants with slot contributions", func(t *testing.T) {
		t.Parallel()
		e := &emit.Enum{Name: "E", Variants: []*emit.EnumVariant{{Name: "A"}}}
		if err := e.VariantsSlot().Append(&emit.EnumVariant{Name: "B"}, prov); err != nil {
			t.Fatalf("append: %v", err)
		}
		got, err := mergedVariants(e)
		if err != nil {
			t.Fatalf("mergedVariants: %v", err)
		}
		if len(got) != 2 || got[0].Name != "A" {
			t.Fatalf("variants = %+v", got)
		}
	})

	t.Run("two members of one name are rejected", func(t *testing.T) {
		t.Parallel()
		e := &emit.Enum{Name: "E", Variants: []*emit.EnumVariant{{Name: "dup"}}}
		if err := e.VariantsSlot().Append(&emit.EnumVariant{Name: "dup"}, prov); err != nil {
			t.Fatalf("append: %v", err)
		}
		if _, err := mergedVariants(e); !errors.Is(err, ErrDuplicateEntity) {
			t.Fatalf("mergedVariants = %v, want ErrDuplicateEntity", err)
		}
	})

	t.Run("a nil enum yields nothing", func(t *testing.T) {
		t.Parallel()
		got, err := mergedVariants(nil)
		if err != nil || got != nil {
			t.Fatalf("mergedVariants(nil) = %+v, %v; want nothing", got, err)
		}
	})
}
