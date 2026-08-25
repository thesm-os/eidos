// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
)

// TestTypeRef_BasicForms covers every TypeRefKind variant the
// converter produces through the public Load surface — one test
// per kind so failures point at the offending shape.
func TestTypeRef_BasicForms(t *testing.T) {
	t.Parallel()
	t.Run("named in-package ref carries Package and Name", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Inner struct{}\n\ntype Outer struct{ I Inner }\n",
		})
		f := pkg.StructByName("Outer").FieldByName("I")
		if f.Type.TypeKind != node.TypeRefNamed {
			t.Fatalf("kind = %v, want named", f.Type.TypeKind)
		}
		if f.Type.Name != "Inner" {
			t.Fatalf("Name = %q", f.Type.Name)
		}
		if f.Type.Package == "" {
			t.Fatalf("Package must be set for in-package named ref")
		}
	})

	t.Run("basic builtin ref carries no Package", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ N int }\n",
		})
		f := pkg.StructByName("S").FieldByName("N")
		if !f.Type.IsBuiltin() {
			t.Fatalf("expected builtin int, got Package=%q Name=%q", f.Type.Package, f.Type.Name)
		}
	})

	t.Run("pointer ref carries Elem", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ P *int }\n",
		})
		f := pkg.StructByName("S").FieldByName("P")
		if !f.Type.IsPointer() || f.Type.Elem == nil {
			t.Fatalf("expected pointer ref, got %+v", f.Type)
		}
	})

	t.Run("slice ref carries Elem", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ Items []int }\n",
		})
		f := pkg.StructByName("S").FieldByName("Items")
		if !f.Type.IsSlice() || f.Type.Elem == nil || f.Type.Elem.Name != "int" {
			t.Fatalf("expected []int, got %+v", f.Type)
		}
	})

	t.Run("array ref carries ArrayLen and Elem", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ Buf [16]byte }\n",
		})
		f := pkg.StructByName("S").FieldByName("Buf")
		if !f.Type.IsArray() || f.Type.ArrayLen != 16 {
			t.Fatalf("expected [16]byte, got %+v", f.Type)
		}
	})

	t.Run("map ref carries MapKey and MapValue", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ M map[string]int }\n",
		})
		f := pkg.StructByName("S").FieldByName("M")
		if !f.Type.IsMap() || f.Type.MapKey == nil || f.Type.MapValue == nil {
			t.Fatalf("expected map, got %+v", f.Type)
		}
		if f.Type.MapKey.Name != "string" || f.Type.MapValue.Name != "int" {
			t.Fatalf("map K/V = %q/%q, want string/int", f.Type.MapKey.Name, f.Type.MapValue.Name)
		}
	})

	t.Run("func ref carries FuncParams and FuncReturns", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ Fn func(int, string) (bool, error) }\n",
		})
		f := pkg.StructByName("S").FieldByName("Fn")
		if !f.Type.IsFunc() {
			t.Fatalf("expected func, got %+v", f.Type)
		}
		if len(f.Type.FuncParams) != 2 || len(f.Type.FuncReturns) != 2 {
			t.Fatalf("expected 2 params + 2 returns, got %d/%d", len(f.Type.FuncParams), len(f.Type.FuncReturns))
		}
	})

	t.Run("type-parameter ref produces TypeRefTypeParam", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Box[T any] struct{ V T }\n",
		})
		f := pkg.StructByName("Box").FieldByName("V")
		if !f.Type.IsTypeParam() || f.Type.Name != "T" {
			t.Fatalf("expected type-param T, got %+v", f.Type)
		}
	})

	t.Run("generic instantiation carries TypeArgs", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Box[T any] struct{}\n\ntype Holder struct{ B Box[int] }\n",
		})
		f := pkg.StructByName("Holder").FieldByName("B")
		if !f.Type.IsGeneric() {
			t.Fatalf("expected generic instantiation, got %+v", f.Type)
		}
		if len(f.Type.TypeArgs) != 1 || f.Type.TypeArgs[0].Name != "int" {
			t.Fatalf("TypeArgs = %+v, want [int]", f.Type.TypeArgs)
		}
	})

	t.Run("channel ref models as a Named go.chan ref", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{ Ch chan int }\n",
		})
		f := pkg.StructByName("S").FieldByName("Ch")
		if f.Type.TypeKind != node.TypeRefNamed || f.Type.Package != "go" || f.Type.Name != "chan" {
			t.Fatalf("expected go.chan ref, got Package=%q Name=%q", f.Type.Package, f.Type.Name)
		}
		if len(f.Type.TypeArgs) != 1 || f.Type.TypeArgs[0].Name != "int" {
			t.Fatalf("chan type-arg = %+v, want [int]", f.Type.TypeArgs)
		}
	})
}

// TestTypeRef_AnonymousForms covers anonymous-struct / anonymous-
// interface refs produced for inline type expressions.
func TestTypeRef_AnonymousForms(t *testing.T) {
	t.Parallel()
	t.Run("anonymous struct field carries inline Fields", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Wrap struct{ Inner struct{ Name string } }\n",
		})
		f := pkg.StructByName("Wrap").FieldByName("Inner")
		if !f.Type.IsAnonStruct() {
			t.Fatalf("expected anon struct, got %v", f.Type.TypeKind)
		}
		if len(f.Type.Fields) != 1 || f.Type.Fields[0].Name != "Name" {
			t.Fatalf("anon fields = %+v", f.Type.Fields)
		}
	})

	t.Run("anonymous interface field carries inline Methods", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Wrap struct{ I interface{ Foo() } }\n",
		})
		f := pkg.StructByName("Wrap").FieldByName("I")
		if !f.Type.IsAnonInterface() {
			t.Fatalf("expected anon interface, got %v", f.Type.TypeKind)
		}
		if len(f.Type.Methods) != 1 || f.Type.Methods[0].Name != "Foo" {
			t.Fatalf("anon methods = %+v", f.Type.Methods)
		}
	})

	t.Run("anonymous struct with an embed carries the embed in Embeds", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype Base struct{ ID string }\n\ntype Wrap struct{ Inner struct{ Base; Extra int } }\n",
		})
		f := pkg.StructByName("Wrap").FieldByName("Inner")
		if !f.Type.IsAnonStruct() {
			t.Fatalf("expected anon struct, got %v", f.Type.TypeKind)
		}
		if len(f.Type.Embeds) != 1 || f.Type.Embeds[0].Type == nil || f.Type.Embeds[0].Type.Name != "Base" {
			t.Fatalf("expected one Base embed, got %+v", f.Type.Embeds)
		}
		if len(f.Type.Fields) != 1 || f.Type.Fields[0].Name != "Extra" {
			t.Fatalf("expected one Extra field, got %+v", f.Type.Fields)
		}
	})

	t.Run("anonymous interface with an embed carries the embed in Embeds", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nimport \"io\"\n\ntype Wrap struct{ R interface{ io.Reader; Close() error } }\n",
		})
		f := pkg.StructByName("Wrap").FieldByName("R")
		if !f.Type.IsAnonInterface() {
			t.Fatalf("expected anon interface, got %v", f.Type.TypeKind)
		}
		if len(f.Type.Embeds) != 1 {
			t.Fatalf("expected one embed, got %d", len(f.Type.Embeds))
		}
	})
}

// TestTypeRef_CrossPackage verifies cross-package named refs carry
// the originating package path.
func TestTypeRef_CrossPackage(t *testing.T) {
	t.Parallel()
	t.Run("imported type ref carries the foreign package path", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nimport \"time\"\n\ntype S struct{ T time.Time }\n",
		})
		f := pkg.StructByName("S").FieldByName("T")
		if f.Type.Package != "time" || f.Type.Name != "Time" {
			t.Fatalf("expected time.Time ref, got Package=%q Name=%q", f.Type.Package, f.Type.Name)
		}
	})
}
