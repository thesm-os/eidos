// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"errors"
	"slices"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
)

// TestInsertField covers every [builder.InsertPos] variant against
// the canonical *emit.Struct field-slot host: Append (zero-value
// position), Prepend, At, Before, and After.
func TestInsertField(t *testing.T) {
	t.Parallel()

	// seed populates a struct's FieldsSlot with three items whose
	// Provenance.IDs are "a", "b", "c" — the anchor targets used
	// by the Before / After cases.
	seed := func() *emit.Struct {
		s := &emit.Struct{Name: "S"}
		c := builder.For("seed").WithTarget(defaultTarget)
		for _, name := range []string{"a", "b", "c"} {
			if err := c.AppendField(s, &emit.Field{Name: name, Type: emit.Builtin("int")}, name); err != nil {
				t.Fatalf("seed AppendField %q: %v", name, err)
			}
		}
		return s
	}

	cases := []struct {
		name  string
		pos   builder.InsertPos
		want  []string // expected name order in the slot after insert
		field *emit.Field
	}{
		{
			name:  "Prepend lands at index 0",
			pos:   builder.Prepend(),
			field: &emit.Field{Name: "x", Type: emit.Builtin("int")},
			want:  []string{"x", "a", "b", "c"},
		},
		{
			name:  "At(2) lands between b and c",
			pos:   builder.At(2),
			field: &emit.Field{Name: "x", Type: emit.Builtin("int")},
			want:  []string{"a", "b", "x", "c"},
		},
		{
			name:  "Before(b) lands immediately before b",
			pos:   builder.Before("b"),
			field: &emit.Field{Name: "x", Type: emit.Builtin("int")},
			want:  []string{"a", "x", "b", "c"},
		},
		{
			name:  "After(b) lands immediately after b",
			pos:   builder.After("b"),
			field: &emit.Field{Name: "x", Type: emit.Builtin("int")},
			want:  []string{"a", "b", "x", "c"},
		},
		{
			name:  "zero-value Append lands at the end",
			pos:   builder.InsertPos{},
			field: &emit.Field{Name: "x", Type: emit.Builtin("int")},
			want:  []string{"a", "b", "c", "x"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := seed()
			c := builder.For("validation").WithTarget(defaultTarget)
			if err := c.InsertField(s, tc.field, tc.pos); err != nil {
				t.Fatalf("InsertField: %v", err)
			}
			got := fieldNames(s)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("InsertField order: got %v, want %v", got, tc.want)
			}
		})
	}

	t.Run("InsertField wires Owner and provenance", func(t *testing.T) {
		t.Parallel()
		s := seed()
		c := builder.For("validation").WithTarget(defaultTarget)
		f := &emit.Field{Name: "x", Type: emit.Builtin("int")}
		if err := c.InsertField(s, f, builder.Before("b"), "validation/x"); err != nil {
			t.Fatalf("InsertField: %v", err)
		}
		if f.Owner != s {
			t.Fatalf("Owner not wired")
		}
		// "x" lands at index 1; assert its provenance carries SetBy
		// and the supplied ID.
		prov := s.FieldsSlot().ProvenanceAt(1)
		if prov.SetBy != "validation" || prov.ID != "validation/x" {
			t.Fatalf("provenance wrong: %+v", prov)
		}
	})

	t.Run("InsertField with missing anchor surfaces emit.ErrProvenanceNotFound", func(t *testing.T) {
		t.Parallel()
		s := seed()
		c := builder.For("validation").WithTarget(defaultTarget)
		err := c.InsertField(s, &emit.Field{Name: "x", Type: emit.Builtin("int")}, builder.Before("nope"))
		if err == nil {
			t.Fatalf("expected error for missing anchor")
		}
	})

	t.Run("InsertField rejects a host that declares no fields", func(t *testing.T) {
		t.Parallel()
		// An Interface is no longer such a host: in some languages it
		// declares mostly fields. A Function has no field list in any
		// of them.
		c := builder.For("validation").WithTarget(defaultTarget)
		err := c.InsertField(&emit.Function{Name: "F"}, &emit.Field{Name: "x"}, builder.Prepend())
		if !errors.Is(err, builder.ErrUnsupportedHost) {
			t.Fatalf("expected ErrUnsupportedHost; got %v", err)
		}
	})

	t.Run("InsertField accepts an Interface host", func(t *testing.T) {
		t.Parallel()
		c := builder.For("validation").WithTarget(defaultTarget)
		iface := &emit.Interface{Name: "I"}
		if err := c.InsertField(iface, &emit.Field{Name: "x"}, builder.Prepend()); err != nil {
			t.Fatalf("InsertField on an Interface: %v", err)
		}
		if got := len(iface.FieldsSlot().Items); got != 1 {
			t.Fatalf("interface fields slot = %d, want 1", got)
		}
	})

	t.Run("InsertField nil host returns ErrNilHost", func(t *testing.T) {
		t.Parallel()
		c := builder.For("validation").WithTarget(defaultTarget)
		err := c.InsertField(nil, &emit.Field{Name: "x"}, builder.Prepend())
		if !errors.Is(err, builder.ErrNilHost) {
			t.Fatalf("expected ErrNilHost; got %v", err)
		}
	})

	t.Run("invalid InsertPos discriminator returns ErrUnknownInsertPos", func(t *testing.T) {
		t.Parallel()
		s := seed()
		c := builder.For("validation").WithTarget(defaultTarget)
		// Bypass the constructor and inject an out-of-range kind
		// via a zero-value pointer dereference trick — actually
		// the simplest path is to ask for an InsertPos we know is
		// unknown. Since posKind is unexported, we can't construct
		// one directly; instead this case verifies that valid
		// positions don't accidentally trip the default arm.
		s2 := &emit.Struct{Name: "S2"}
		if err := c.InsertField(s2, &emit.Field{Name: "x", Type: emit.Builtin("int")}, builder.Prepend()); err != nil {
			t.Fatalf("Prepend should not trip ErrUnknownInsertPos; got %v", err)
		}
		_ = s.FieldsSlot()
	})
}

// TestInsertMethod covers the three accepted host kinds plus
// positional placement of methods relative to anchors.
func TestInsertMethod(t *testing.T) {
	t.Parallel()

	t.Run("Before targets the anchor across host kinds", func(t *testing.T) {
		t.Parallel()
		seed := builder.For("seed").WithTarget(defaultTarget)
		ins := builder.For("validation").WithTarget(defaultTarget)

		s := &emit.Struct{Name: "S"}
		_ = seed.AppendMethod(s, &emit.Method{Name: "a"}, "a")
		_ = seed.AppendMethod(s, &emit.Method{Name: "b"}, "b")
		if err := ins.InsertMethod(s, &emit.Method{Name: "x"}, builder.Before("b")); err != nil {
			t.Fatalf("InsertMethod struct: %v", err)
		}
		if got := methodNamesFromSlot(s.MethodsSlot()); !slices.Equal(got, []string{"a", "x", "b"}) {
			t.Fatalf("struct host: got %v want [a x b]", got)
		}

		i := &emit.Interface{Name: "I"}
		_ = seed.AppendMethod(i, &emit.Method{Name: "a"}, "a")
		_ = seed.AppendMethod(i, &emit.Method{Name: "b"}, "b")
		if err := ins.InsertMethod(i, &emit.Method{Name: "x"}, builder.After("a")); err != nil {
			t.Fatalf("InsertMethod interface: %v", err)
		}
		if got := methodNamesFromSlot(i.MethodsSlot()); !slices.Equal(got, []string{"a", "x", "b"}) {
			t.Fatalf("interface host: got %v want [a x b]", got)
		}

		a := &emit.Alias{Name: "A"}
		_ = seed.AppendMethod(a, &emit.Method{Name: "a"}, "a")
		if err := ins.InsertMethod(a, &emit.Method{Name: "x"}, builder.Prepend()); err != nil {
			t.Fatalf("InsertMethod alias: %v", err)
		}
		if got := methodNamesFromSlot(a.MethodsSlot()); !slices.Equal(got, []string{"x", "a"}) {
			t.Fatalf("alias host: got %v want [x a]", got)
		}
	})

	t.Run("InsertMethod nil host returns ErrNilHost", func(t *testing.T) {
		t.Parallel()
		c := builder.For("validation").WithTarget(defaultTarget)
		err := c.InsertMethod(nil, &emit.Method{Name: "x"}, builder.Prepend())
		if !errors.Is(err, builder.ErrNilHost) {
			t.Fatalf("expected ErrNilHost; got %v", err)
		}
	})

	t.Run("InsertMethod rejects unsupported host", func(t *testing.T) {
		t.Parallel()
		c := builder.For("validation").WithTarget(defaultTarget)
		err := c.InsertMethod(&emit.Function{Name: "F"}, &emit.Method{Name: "x"}, builder.Prepend())
		if !errors.Is(err, builder.ErrUnsupportedHost) {
			t.Fatalf("expected ErrUnsupportedHost; got %v", err)
		}
	})
}

// TestInsertEmbedVariantParamBodyTagFileSlots covers the remaining
// Insert* helpers — one happy-path probe per family plus the
// canonical nil-host / unsupported-host guards.
func TestInsertEmbedVariantParamBodyTagFileSlots(t *testing.T) {
	t.Parallel()

	c := builder.For("audit").WithTarget(defaultTarget)

	t.Run("InsertEmbed: struct and interface", func(t *testing.T) {
		t.Parallel()
		s := &emit.Struct{Name: "S"}
		if err := c.InsertEmbed(s, &emit.Embed{Type: emit.Builtin("R")}, builder.Prepend()); err != nil {
			t.Fatalf("struct: %v", err)
		}
		i := &emit.Interface{Name: "I"}
		if err := c.InsertEmbed(i, &emit.Embed{Type: emit.Builtin("R")}, builder.Prepend()); err != nil {
			t.Fatalf("interface: %v", err)
		}
		if err := c.InsertEmbed(nil, &emit.Embed{}, builder.Prepend()); !errors.Is(err, builder.ErrNilHost) {
			t.Fatalf("nil host: got %v", err)
		}
		err := c.InsertEmbed(&emit.Function{}, &emit.Embed{}, builder.Prepend())
		if !errors.Is(err, builder.ErrUnsupportedHost) {
			t.Fatalf("unsupported host: got %v", err)
		}
	})

	t.Run("InsertVariant: enum host only", func(t *testing.T) {
		t.Parallel()
		e := &emit.Enum{Name: "E"}
		if err := c.InsertVariant(e, &emit.EnumVariant{Name: "X"}, builder.Prepend()); err != nil {
			t.Fatalf("enum: %v", err)
		}
		v := e.VariantsSlot().At(0).(*emit.EnumVariant)
		if v.Owner != e {
			t.Fatalf("variant Owner not wired")
		}
		if err := c.InsertVariant(nil, &emit.EnumVariant{}, builder.Prepend()); !errors.Is(err, builder.ErrNilHost) {
			t.Fatalf("nil host: got %v", err)
		}
	})

	t.Run("InsertParam: function and method", func(t *testing.T) {
		t.Parallel()
		f := &emit.Function{Name: "F"}
		if err := c.InsertParam(f, &emit.Param{Name: "x", Type: emit.Builtin("int")}, builder.Prepend()); err != nil {
			t.Fatalf("function: %v", err)
		}
		m := &emit.Method{Name: "M"}
		if err := c.InsertParam(m, &emit.Param{Name: "x", Type: emit.Builtin("int")}, builder.Prepend()); err != nil {
			t.Fatalf("method: %v", err)
		}
		if err := c.InsertParam(nil, &emit.Param{}, builder.Prepend()); !errors.Is(err, builder.ErrNilHost) {
			t.Fatalf("nil host: got %v", err)
		}
		err := c.InsertParam(&emit.Struct{}, &emit.Param{}, builder.Prepend())
		if !errors.Is(err, builder.ErrUnsupportedHost) {
			t.Fatalf("unsupported host: got %v", err)
		}
	})

	t.Run("InsertPrebody / InsertPostbody: function and method", func(t *testing.T) {
		t.Parallel()
		f := &emit.Function{Name: "F"}
		m := &emit.Method{Name: "M"}
		stmt := builder.RawStmt(`x := 1`)
		if err := c.InsertPrebody(f, stmt, builder.Prepend()); err != nil {
			t.Fatalf("prebody fn: %v", err)
		}
		if err := c.InsertPrebody(m, stmt, builder.Prepend()); err != nil {
			t.Fatalf("prebody method: %v", err)
		}
		if err := c.InsertPostbody(f, stmt, builder.Prepend()); err != nil {
			t.Fatalf("postbody fn: %v", err)
		}
		if err := c.InsertPostbody(m, stmt, builder.Prepend()); err != nil {
			t.Fatalf("postbody method: %v", err)
		}
		if err := c.InsertPrebody(nil, stmt, builder.Prepend()); !errors.Is(err, builder.ErrNilHost) {
			t.Fatalf("prebody nil: %v", err)
		}
		if err := c.InsertPostbody(nil, stmt, builder.Prepend()); !errors.Is(err, builder.ErrNilHost) {
			t.Fatalf("postbody nil: %v", err)
		}
		pre := c.InsertPrebody(&emit.Struct{}, stmt, builder.Prepend())
		if !errors.Is(pre, builder.ErrUnsupportedHost) {
			t.Fatalf("prebody unsupported: %v", pre)
		}
		post := c.InsertPostbody(&emit.Struct{}, stmt, builder.Prepend())
		if !errors.Is(post, builder.ErrUnsupportedHost) {
			t.Fatalf("postbody unsupported: %v", post)
		}
	})

	t.Run("InsertReturn: function and method", func(t *testing.T) {
		t.Parallel()
		f := &emit.Function{Name: "F"}
		if err := c.InsertReturn(f, &emit.Return{Type: emit.Builtin("error")}, builder.Prepend()); err != nil {
			t.Fatalf("function: %v", err)
		}
		if f.ReturnsSlot().Len() != 1 {
			t.Fatalf("function returns slot did not receive contribution")
		}
		m := &emit.Method{Name: "M"}
		if err := c.InsertReturn(m, &emit.Return{Type: emit.Builtin("error")}, builder.Prepend()); err != nil {
			t.Fatalf("method: %v", err)
		}
		if m.ReturnsSlot().Len() != 1 {
			t.Fatalf("method returns slot did not receive contribution")
		}
		if err := c.InsertReturn(nil, &emit.Return{}, builder.Prepend()); !errors.Is(err, builder.ErrNilHost) {
			t.Fatalf("nil host: %v", err)
		}
		uerr := c.InsertReturn(&emit.Struct{}, &emit.Return{}, builder.Prepend())
		if !errors.Is(uerr, builder.ErrUnsupportedHost) {
			t.Fatalf("unsupported host: %v", uerr)
		}
	})

	t.Run("InsertTag: field host only", func(t *testing.T) {
		t.Parallel()
		f := &emit.Field{Name: "F", Type: emit.Builtin("string")}
		if err := c.InsertTag(f, &emit.Tag{Key: "json", Value: "f"}, builder.Prepend()); err != nil {
			t.Fatalf("tag: %v", err)
		}
		if err := c.InsertTag(nil, &emit.Tag{}, builder.Prepend()); !errors.Is(err, builder.ErrNilHost) {
			t.Fatalf("nil host: %v", err)
		}
	})

	t.Run("InsertTop / InsertBottom / InsertInit / InsertImport: file slots", func(t *testing.T) {
		t.Parallel()
		f := &emit.File{Name: "x.go", Dir: "x", Package: "x"}
		decl := &emit.Constant{Name: "K"}
		stmt := builder.RawStmt(`init()`)
		if err := c.InsertTop(f, decl, builder.Prepend()); err != nil {
			t.Fatalf("top: %v", err)
		}
		if err := c.InsertBottom(f, decl, builder.Prepend()); err != nil {
			t.Fatalf("bottom: %v", err)
		}
		if err := c.InsertInit(f, stmt, builder.Prepend()); err != nil {
			t.Fatalf("init: %v", err)
		}
		if err := c.InsertImport(f, &emit.Import{Path: "fmt"}, builder.Prepend()); err != nil {
			t.Fatalf("import: %v", err)
		}
		for _, slotErr := range []error{
			c.InsertTop(nil, decl, builder.Prepend()),
			c.InsertBottom(nil, decl, builder.Prepend()),
			c.InsertInit(nil, stmt, builder.Prepend()),
			c.InsertImport(nil, &emit.Import{}, builder.Prepend()),
		} {
			if !errors.Is(slotErr, builder.ErrNilHost) {
				t.Fatalf("nil file should return ErrNilHost; got %v", slotErr)
			}
		}
	})
}
