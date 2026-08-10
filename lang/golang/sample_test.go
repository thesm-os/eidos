// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// TestSampleRefFor_Composites pins the two shapes a string-returning
// sample could never carry: a value written as a composite literal,
// which needs both the type and the braces.
func TestSampleRefFor_Composites(t *testing.T) {
	t.Parallel()

	t.Run("a struct samples its first settable field", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"example.com/x.Point": &node.Struct{
			Name: "Point", Package: "example.com/x",
			Fields: []*node.Field{{Name: "X", Type: builtinRef("int")}},
		}}
		s, a := golang.SampleRefFor(namedTypeRef("example.com/x", "Point"), "P", r)
		if !s.OK() || !a.OK() {
			t.Fatalf("SampleRefFor derived nothing for a resolvable struct")
		}
		if !s.Composite {
			t.Errorf("a struct renders as a composite literal, not a conversion")
		}
		if s.Text != "{X: 42}" || a.Text != "{X: 7}" {
			t.Errorf("Text = %q, %q; want the field set to distinct values", s.Text, a.Text)
		}
		if s.Ref == nil {
			t.Errorf("sample carries no ref, so the import would go unregistered")
		}
	})

	t.Run("a struct skips a field it cannot sample", func(t *testing.T) {
		t.Parallel()
		// Refusing on the first field would lose a sample a later one
		// supplies, and a struct leading with an opaque type is
		// ordinary rather than exceptional.
		r := mapResolver{"example.com/x.Mixed": &node.Struct{
			Name: "Mixed", Package: "example.com/x",
			Fields: []*node.Field{
				{Name: "Opaque", Type: namedTypeRef("example.com/y", "Absent")},
				{Name: "Code", Type: builtinRef("int")},
			},
		}}
		s, _ := golang.SampleRefFor(namedTypeRef("example.com/x", "Mixed"), "M", r)
		if s.Text != "{Code: 42}" {
			t.Errorf("Text = %q, want the first field that has a sample", s.Text)
		}
	})

	t.Run("an unexported field is never sampled", func(t *testing.T) {
		t.Parallel()
		// A composite literal cannot set it from another package, so a
		// sample naming one does not compile where it lands.
		r := mapResolver{"example.com/x.Hidden": &node.Struct{
			Name: "Hidden", Package: "example.com/x",
			Fields: []*node.Field{{Name: "secret", Type: builtinRef("int")}},
		}}
		if s, _ := golang.SampleRefFor(namedTypeRef("example.com/x", "Hidden"), "H", r); s.OK() {
			t.Errorf("SampleRefFor = %+v, want nothing derivable", s)
		}
	})

	t.Run("an array samples one element", func(t *testing.T) {
		t.Parallel()
		// Enough to distinguish it from the zero array, and short
		// enough to stay readable in the generated source.
		arr := &node.TypeRef{TypeKind: node.TypeRefArray, ArrayLen: 3, Elem: builtinRef("int")}
		s, a := golang.SampleRefFor(arr, "Buf", nil)
		if !s.OK() || !a.OK() {
			t.Fatalf("SampleRefFor derived nothing for an array of builtins")
		}
		if !s.Composite || s.Text != "{42}" || a.Text != "{7}" {
			t.Errorf("SampleRefFor = %+v / %+v, want one-element composites", s, a)
		}
	})

	t.Run("an array of an unsampleable element yields nothing", func(t *testing.T) {
		t.Parallel()
		arr := &node.TypeRef{
			TypeKind: node.TypeRefArray, ArrayLen: 2,
			Elem: namedTypeRef("example.com/y", "Absent"),
		}
		if s, _ := golang.SampleRefFor(arr, "Buf", mapResolver{}); s.OK() {
			t.Errorf("SampleRefFor = %+v, want nothing derivable", s)
		}
	})

	t.Run("a nil ref and an exhausted budget yield nothing", func(t *testing.T) {
		t.Parallel()
		if s, _ := golang.SampleRefFor(nil, "F", nil); s.OK() {
			t.Errorf("a nil ref must derive nothing")
		}
		// A builtin the sample table has no entry for.
		if s, _ := golang.SampleRefFor(builtinRef("chan"), "C", nil); s.OK() {
			t.Errorf("a type admitting no distinguishable values must derive nothing")
		}
	})
}
