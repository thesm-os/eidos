// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// sliceRef returns a [node.TypeRef] of the given element type —
// the inline equivalent of storefixture.Slice. Kept local so
// the package stays a leaf with no cross-module test
// dependencies.
func sliceRef(elem *node.TypeRef) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: elem}
}

// mapRef returns a [node.TypeRef] of the given key+value types
// — the inline equivalent of storefixture.Map.
func mapRef(k, v *node.TypeRef) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefMap, MapKey: k, MapValue: v}
}

// TestIsExported pins Go's exported-identifier rule. The
// upstream consumers (plugin templates, [ExportedFields])
// route every field-filter / setter-emission decision
// through this helper, so the contract has to be
// unambiguous: first ASCII upper-case rune is exported,
// everything else is not, empty string is not.
func TestIsExported(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		want bool
	}{
		{"Title", true},
		{"ID", true},
		{"internal", false},
		{"", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := golang.IsExported(tc.name); got != tc.want {
				t.Errorf("IsExported(%q) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestIsByteSlice pins the `[]byte` discrimination — the
// only slice shape Go renders idiomatically through a
// bytes-string convenience setter pair. Plugin templates
// branch on this helper to pick the setter shape per field.
func TestIsByteSlice(t *testing.T) {
	t.Parallel()

	t.Run("bytes slice matches", func(t *testing.T) {
		t.Parallel()
		if !golang.IsByteSlice(sliceRef(&node.TypeRef{Name: "byte"})) {
			t.Errorf("[]byte must be recognised as byte slice")
		}
	})

	t.Run("uint8 slice matches", func(t *testing.T) {
		t.Parallel()
		// `byte` and `uint8` are one type and the frontend records
		// whichever the author wrote. Keying on the literal name gave
		// `[]byte` a bytes-string setter pair and the identical
		// `[]uint8` a variadic `...uint8` pair — two builder APIs for
		// one field shape, chosen by spelling.
		if !golang.IsByteSlice(sliceRef(&node.TypeRef{Name: "uint8"})) {
			t.Errorf("[]uint8 must be recognised as byte slice; it is the same type as []byte")
		}
	})

	t.Run("string slice does not match", func(t *testing.T) {
		t.Parallel()
		if golang.IsByteSlice(sliceRef(&node.TypeRef{Name: "string"})) {
			t.Errorf("[]string must not be recognised as byte slice")
		}
	})

	t.Run("nil ref does not match", func(t *testing.T) {
		t.Parallel()
		if golang.IsByteSlice(nil) {
			t.Errorf("nil ref must not be recognised as byte slice")
		}
	})

	t.Run("non-slice does not match", func(t *testing.T) {
		t.Parallel()
		if golang.IsByteSlice(&node.TypeRef{Name: "byte"}) {
			t.Errorf("non-slice byte ref must not be recognised as byte slice")
		}
	})
}

// TestIsSlice / TestIsMap pin the IsByteSlice complement and
// the map-type predicate respectively. The non-byte-slice
// constraint matters because the bytes branch has its own
// setter shape; emitting both would render duplicate
// methods.
func TestIsSlice(t *testing.T) {
	t.Parallel()

	t.Run("string slice matches", func(t *testing.T) {
		t.Parallel()
		if !golang.IsSlice(sliceRef(&node.TypeRef{Name: "string"})) {
			t.Errorf("[]string must be recognised as a (non-byte) slice")
		}
	})

	t.Run("byte slice does not match", func(t *testing.T) {
		t.Parallel()
		if golang.IsSlice(sliceRef(&node.TypeRef{Name: "byte"})) {
			t.Errorf("[]byte must route through IsByteSlice, not IsSlice")
		}
	})

	t.Run("uint8 slice does not match", func(t *testing.T) {
		t.Parallel()
		// The complement of the IsByteSlice widening: a template
		// branching `isSlice`-then-`isByteSlice` must not see `[]uint8`
		// twice, and must not see it on the variadic arm at all.
		if golang.IsSlice(sliceRef(&node.TypeRef{Name: "uint8"})) {
			t.Errorf("[]uint8 must route through IsByteSlice, not IsSlice")
		}
	})

	t.Run("scalar does not match", func(t *testing.T) {
		t.Parallel()
		if golang.IsSlice(&node.TypeRef{Name: "string"}) {
			t.Errorf("scalar must not be recognised as a slice")
		}
	})
}

// TestIsMap covers the map-type predicate paired with the
// map's per-entry setter rendering in plugin templates.
func TestIsMap(t *testing.T) {
	t.Parallel()

	t.Run("map matches", func(t *testing.T) {
		t.Parallel()
		ref := mapRef(&node.TypeRef{Name: "string"}, &node.TypeRef{Name: "int"})
		if !golang.IsMap(ref) {
			t.Errorf("map must be recognised")
		}
	})

	t.Run("slice does not match", func(t *testing.T) {
		t.Parallel()
		if golang.IsMap(sliceRef(&node.TypeRef{Name: "string"})) {
			t.Errorf("slice must not be recognised as a map")
		}
	})

	t.Run("nil ref does not match", func(t *testing.T) {
		t.Parallel()
		if golang.IsMap(nil) {
			t.Errorf("nil ref must not be recognised as a map")
		}
	})
}

// TestExportedFields pins the source-order filter
// [ExportedFields] applies to a struct's full field list.
// Unexported fields drop out; exported fields keep their
// declared order so generated setters mirror the source's
// shape.
func TestExportedFields(t *testing.T) {
	t.Parallel()
	s := &node.Struct{
		Name:    "Article",
		Package: "example.com/blog",
		Fields: []*node.Field{
			{Name: "Title", Type: &node.TypeRef{Name: "string"}},
			{Name: "internal", Type: &node.TypeRef{Name: "string"}},
			{Name: "Body", Type: &node.TypeRef{Name: "string"}},
		},
	}
	got := golang.ExportedFields(s)
	if len(got) != 2 {
		t.Fatalf("ExportedFields = %d entries, want 2", len(got))
	}
	if got[0].Name != "Title" || got[1].Name != "Body" {
		t.Errorf("ExportedFields order = [%s, %s], want [Title, Body]", got[0].Name, got[1].Name)
	}
}

// TestTypeArgs pins the bracketed parameter-name use form
// the per-language template appends to receiver / return /
// composite refs in generic-struct emissions. Non-generic
// structs return the empty string so non-generic templates
// stay free of stray brackets.
func TestTypeArgs(t *testing.T) {
	t.Parallel()

	t.Run("non-generic struct yields empty string", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{Name: "Article", Package: "example.com/blog"}
		if got := golang.TypeArgs(s); got != "" {
			t.Errorf("TypeArgs(non-generic) = %q, want empty", got)
		}
	})

	t.Run("single type parameter yields [T]", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{
			Name:       "Container",
			Package:    "example.com/blog",
			TypeParams: []*node.TypeParam{{Name: "T"}},
		}
		if got := golang.TypeArgs(s); got != "[T]" {
			t.Errorf("TypeArgs = %q, want [T]", got)
		}
	})

	t.Run("two type parameters yield [T, K]", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{
			Name:    "Map",
			Package: "example.com/blog",
			TypeParams: []*node.TypeParam{
				{Name: "T"},
				{Name: "K"},
			},
		}
		if got := golang.TypeArgs(s); got != "[T, K]" {
			t.Errorf("TypeArgs = %q, want [T, K]", got)
		}
	})
}

// TestFieldType pins the field-type lifter templates feed to the
// backend's renderType entry. It is a thin delegation to FromNode,
// so the contract worth pinning is that it delegates the field's
// declared type rather than the field itself.
func TestFieldType(t *testing.T) {
	t.Parallel()

	t.Run("lifts the field's declared type", func(t *testing.T) {
		t.Parallel()
		f := &node.Field{Name: "ID", Type: &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "string"}}
		b, ok := golang.FieldType(f).(*emit.BuiltinRef)
		if !ok || b.Name != "string" {
			t.Fatalf("FieldType = %#v, want BuiltinRef{string}", golang.FieldType(f))
		}
	})

	t.Run("threads the source ref as the lifted ref's origin", func(t *testing.T) {
		t.Parallel()
		typ := &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "string"}
		b, _ := golang.FieldType(&node.Field{Name: "ID", Type: typ}).(*emit.BuiltinRef)
		if b.OriginNode != typ {
			t.Fatalf("OriginNode = %#v, want the source TypeRef", b.OriginNode)
		}
	})
}

// TestElemType and its map siblings pin the composite-projection
// lifters. Each reaches into one sub-ref of a composite type, so
// each also has to answer what happens when that sub-ref is absent
// — templates call these on whatever the model holds.
func TestElemType(t *testing.T) {
	t.Parallel()

	t.Run("lifts the slice element type", func(t *testing.T) {
		t.Parallel()
		b, ok := golang.ElemType(sliceRef(&node.TypeRef{TypeKind: node.TypeRefNamed, Name: "int"})).(*emit.BuiltinRef)
		if !ok || b.Name != "int" {
			t.Fatalf("ElemType = %#v, want BuiltinRef{int}", b)
		}
	})

	t.Run("threads the element ref as the lifted ref's origin", func(t *testing.T) {
		t.Parallel()
		elem := &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "int"}
		b, _ := golang.ElemType(sliceRef(elem)).(*emit.BuiltinRef)
		if b.OriginNode != elem {
			t.Fatalf("OriginNode = %#v, want the element TypeRef", b.OriginNode)
		}
	})
}

func TestMapKeyType(t *testing.T) {
	t.Parallel()

	t.Run("lifts the map key type", func(t *testing.T) {
		t.Parallel()
		m := mapRef(
			&node.TypeRef{TypeKind: node.TypeRefNamed, Name: "string"},
			&node.TypeRef{TypeKind: node.TypeRefNamed, Name: "int"},
		)
		b, ok := golang.MapKeyType(m).(*emit.BuiltinRef)
		if !ok || b.Name != "string" {
			t.Fatalf("MapKeyType = %#v, want BuiltinRef{string}", b)
		}
	})

	t.Run("an external key type lifts to an ExternalRef", func(t *testing.T) {
		t.Parallel()
		m := mapRef(
			&node.TypeRef{TypeKind: node.TypeRefNamed, Package: "time", Name: "Time"},
			&node.TypeRef{TypeKind: node.TypeRefNamed, Name: "int"},
		)
		e, ok := golang.MapKeyType(m).(*emit.ExternalRef)
		if !ok || e.Package != "time" || e.Name != "Time" {
			t.Fatalf("MapKeyType = %#v, want ExternalRef{time, Time}", golang.MapKeyType(m))
		}
	})
}

func TestMapValType(t *testing.T) {
	t.Parallel()

	t.Run("lifts the map value type", func(t *testing.T) {
		t.Parallel()
		m := mapRef(
			&node.TypeRef{TypeKind: node.TypeRefNamed, Name: "string"},
			&node.TypeRef{TypeKind: node.TypeRefNamed, Name: "int"},
		)
		b, ok := golang.MapValType(m).(*emit.BuiltinRef)
		if !ok || b.Name != "int" {
			t.Fatalf("MapValType = %#v, want BuiltinRef{int}", b)
		}
	})

	t.Run("a composite value type lifts to a CompositeRef", func(t *testing.T) {
		t.Parallel()
		m := mapRef(
			&node.TypeRef{TypeKind: node.TypeRefNamed, Name: "string"},
			sliceRef(&node.TypeRef{TypeKind: node.TypeRefNamed, Name: "byte"}),
		)
		c, ok := golang.MapValType(m).(*emit.CompositeRef)
		if !ok || c.Shape != emit.ShapeSlice {
			t.Fatalf("MapValType = %#v, want slice CompositeRef", golang.MapValType(m))
		}
	})
}

// TestTypeParams pins the generic-parameter lifter. The nil return
// for a non-generic struct is load-bearing: the template's
// renderTypeParams call emits no bracket list at all for nil, and
// an empty-but-non-nil slice would render `[]`.
func TestTypeParams(t *testing.T) {
	t.Parallel()

	t.Run("a non-generic struct lifts to nil", func(t *testing.T) {
		t.Parallel()
		if got := golang.TypeParams(&node.Struct{Name: "Container"}); got != nil {
			t.Fatalf("TypeParams(non-generic) = %#v, want nil", got)
		}
	})

	t.Run("lifts each parameter name in declaration order", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{Name: "Container", TypeParams: []*node.TypeParam{{Name: "T"}, {Name: "K"}}}
		got := golang.TypeParams(s)
		if len(got) != 2 || got[0].Name != "T" || got[1].Name != "K" {
			t.Fatalf("TypeParams = %#v, want [T K]", got)
		}
	})

	t.Run("lifts the constraint alongside the name", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{
			Name: "Container",
			TypeParams: []*node.TypeParam{{
				Name: "T",
				Constraint: &node.Constraint{
					Embedded: []*node.TypeRef{{TypeKind: node.TypeRefNamed, Name: "comparable"}},
				},
			}},
		}
		got := golang.TypeParams(s)
		if len(got) != 1 || got[0].Constraint == nil {
			t.Fatalf("TypeParams dropped the constraint: %#v", got)
		}
	})
}

// TestSelfType pins the struct's own-type instantiation. The
// generic arm threads the parameter names back in as type args so
// an emitted helper referring to its host renders `Container[T]`
// rather than the uninstantiated `Container`, which would not
// compile.
func TestSelfType(t *testing.T) {
	t.Parallel()

	t.Run("a non-generic struct is its bare external ref", func(t *testing.T) {
		t.Parallel()
		got, ok := golang.SelfType(&node.Struct{Name: "Container", Package: "example.com/c"}).(*emit.ExternalRef)
		if !ok || got.Name != "Container" || got.Package != "example.com/c" {
			t.Fatalf("SelfType = %#v, want ExternalRef{example.com/c, Container}", got)
		}
	})

	t.Run("a non-generic struct carries no type args", func(t *testing.T) {
		t.Parallel()
		got, _ := golang.SelfType(&node.Struct{Name: "Container", Package: "example.com/c"}).(*emit.ExternalRef)
		if len(got.TypeArgs) != 0 {
			t.Fatalf("SelfType TypeArgs = %#v, want none", got.TypeArgs)
		}
	})

	t.Run("a generic struct threads its parameter names as type args", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{
			Name: "Container", Package: "example.com/c",
			TypeParams: []*node.TypeParam{{Name: "T"}, {Name: "K"}},
		}
		got, ok := golang.SelfType(s).(*emit.ExternalRef)
		if !ok || len(got.TypeArgs) != 2 {
			t.Fatalf("SelfType = %#v, want two type args", got)
		}
		a, ok := got.TypeArgs[0].(*emit.BuiltinRef)
		if !ok || a.Name != "T" {
			t.Fatalf("SelfType first type arg = %#v, want BuiltinRef{T}", got.TypeArgs[0])
		}
	})
}

// TestFuncMap pins the funcmap plugins compose with their own
// entries. The keys are a published contract — a template in a
// downstream plugin references them by string, so a rename here
// breaks that template at render time rather than at build time.
func TestFuncMap(t *testing.T) {
	t.Parallel()

	t.Run("registers every documented key", func(t *testing.T) {
		t.Parallel()
		fm := golang.FuncMap()
		for _, key := range []string{
			"isExported", "exportedFields", "isSlice", "isMap", "isByteSlice",
			"fieldType", "elemType", "mapKeyType", "mapValType",
			"typeParams", "typeArgs", "selfType",
		} {
			if _, ok := fm[key]; !ok {
				t.Errorf("FuncMap missing documented key %q", key)
			}
		}
	})

	t.Run("registers no key beyond the documented set", func(t *testing.T) {
		t.Parallel()
		if got := len(golang.FuncMap()); got != 12 {
			t.Fatalf("FuncMap holds %d keys, want the 12 documented ones", got)
		}
	})

	t.Run("returns a fresh map per call", func(t *testing.T) {
		t.Parallel()
		// Callers merge this into their own funcmap and some
		// delete from it; a shared map would leak that edit into
		// every later plugin.
		first := golang.FuncMap()
		delete(first, "isExported")
		if _, ok := golang.FuncMap()["isExported"]; !ok {
			t.Fatalf("FuncMap returned a shared map; a caller's delete leaked")
		}
	})
}

// TestIsByte pins both spellings of Go's byte.
//
// The frontend records whichever the author wrote, so a predicate
// matching one name turns a `[]byte` convenience setter into a
// `[]uint8` one for half the corpus — which is the edge case a
// private copy of this gets wrong.
func TestIsByte(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name string
		ref  *node.TypeRef
		want bool
	}{
		{"byte", &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "byte"}, true},
		{"uint8", &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "uint8"}, true},
		{"int", &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "int"}, false},
		{"qualified byte", &node.TypeRef{TypeKind: node.TypeRefNamed, Package: "pkg", Name: "byte"}, false},
		{"nil", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := golang.IsByte(tc.ref); got != tc.want {
				t.Fatalf("IsByte(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestIsEmptyStruct pins the shape that makes a map a set.
func TestIsEmptyStruct(t *testing.T) {
	t.Parallel()

	t.Run("the anonymous empty struct matches", func(t *testing.T) {
		t.Parallel()
		if !golang.IsEmptyStruct(&node.TypeRef{TypeKind: node.TypeRefAnonStruct}) {
			t.Fatalf("struct{} must match")
		}
	})

	t.Run("an anonymous struct with a field does not match", func(t *testing.T) {
		t.Parallel()
		r := &node.TypeRef{TypeKind: node.TypeRefAnonStruct, Fields: []*node.Field{{Name: "X"}}}
		if golang.IsEmptyStruct(r) {
			t.Fatalf("a struct with a field must not match")
		}
	})

	t.Run("an anonymous struct with an embed does not match", func(t *testing.T) {
		t.Parallel()
		// Both emptiness tests: an anonymous struct may carry embeds
		// as well as declared fields, and one holding either is a
		// value a caller has something to say about.
		r := &node.TypeRef{TypeKind: node.TypeRefAnonStruct, Embeds: []*node.Embed{{}}}
		if golang.IsEmptyStruct(r) {
			t.Fatalf("a struct with an embed must not match")
		}
	})

	t.Run("a named struct type does not match", func(t *testing.T) {
		t.Parallel()
		if golang.IsEmptyStruct(&node.TypeRef{TypeKind: node.TypeRefNamed, Name: "Empty"}) {
			t.Fatalf("a named type must not match")
		}
	})

	t.Run("nil does not match", func(t *testing.T) {
		t.Parallel()
		if golang.IsEmptyStruct(nil) {
			t.Fatalf("nil must not match")
		}
	})
}

func TestIsVariadic(t *testing.T) {
	t.Parallel()

	t.Run("a variadic parameter matches", func(t *testing.T) {
		t.Parallel()
		if !golang.IsVariadic(&node.Param{Name: "keys", Variadic: true}) {
			t.Fatalf("a variadic param must match")
		}
	})

	t.Run("a fixed parameter does not match", func(t *testing.T) {
		t.Parallel()
		// Forwarding a variadic without its ellipsis passes the slice
		// as one element: it type-checks against `...any` and silently
		// records one argument where the caller passed several.
		if golang.IsVariadic(&node.Param{Name: "id"}) {
			t.Fatalf("a fixed param must not match")
		}
	})

	t.Run("nil does not match", func(t *testing.T) {
		t.Parallel()
		if golang.IsVariadic(nil) {
			t.Fatalf("nil must not match")
		}
	})
}

// TestInstantiation pins the third of Go's three type-parameter
// spellings, alongside TypeParams (declaration) and TypeArgs (use).
//
// Mixing them produces code that either fails to compile or
// compiles and asserts the wrong thing, and naming only two of the
// three is what leaves a consumer inventing a name for the third.
func TestInstantiation(t *testing.T) {
	t.Parallel()

	t.Run("renders a concrete argument list", func(t *testing.T) {
		t.Parallel()
		if got := golang.Instantiation("string", "int"); got != "[string, int]" {
			t.Fatalf("Instantiation = %q, want [string, int]", got)
		}
	})

	t.Run("renders a single argument", func(t *testing.T) {
		t.Parallel()
		if got := golang.Instantiation("string"); got != "[string]" {
			t.Fatalf("Instantiation = %q, want [string]", got)
		}
	})

	t.Run("no arguments render nothing", func(t *testing.T) {
		t.Parallel()
		// A non-generic entry point must emit no bracket list at all;
		// `[]` does not compile.
		if got := golang.Instantiation(); got != "" {
			t.Fatalf("Instantiation() = %q, want empty", got)
		}
	})

	t.Run("agrees with TypeArgs on separator and brackets", func(t *testing.T) {
		t.Parallel()
		// The two forms appear side by side in one generated file —
		// a declaration instantiated at concrete types, and a
		// reference using the parameter names — so a difference in
		// spelling reads as a bug in the generator.
		s := &node.Struct{
			Name: "Container", Package: "x",
			TypeParams: []*node.TypeParam{{Name: "K"}, {Name: "V"}},
		}
		if got, want := golang.Instantiation("K", "V"), golang.TypeArgs(s); got != want {
			t.Fatalf("Instantiation = %q, TypeArgs = %q; the spellings must agree", got, want)
		}
	})
}
