// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package gofixture_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
	"go.thesmos.sh/eidos/node"
)

func TestBuilder_Struct(t *testing.T) {
	t.Parallel()

	t.Run("creates a struct with the configured name and package", func(t *testing.T) {
		t.Parallel()
		b := gofixture.New().Package("users", "example.com/users").
			Struct("User", nil)
		s := b.PackageNode().StructByName("User")
		if s == nil {
			t.Fatalf("Struct should be reachable by name")
		}
		requireQName(t, s.QName(), "example.com/users.User")
	})

	t.Run("invokes the configuration callback exactly once when provided", func(t *testing.T) {
		t.Parallel()
		var calls int
		gofixture.New().Struct("S", func(*gofixture.StructBuilder) { calls++ })
		if calls != 1 {
			t.Fatalf("callback invocation count wrong: got %d, want 1", calls)
		}
	})

	t.Run("accepts a nil callback as a bare-struct shorthand", func(t *testing.T) {
		t.Parallel()
		b := gofixture.New().Struct("Bare", nil)
		s := b.PackageNode().StructByName("Bare")
		if len(s.Fields) != 0 || len(s.Methods) != 0 {
			t.Fatalf("nil callback should leave the struct empty: %+v", s)
		}
	})
}

func TestStructBuilder_Node(t *testing.T) {
	t.Parallel()

	t.Run("returns the struct backing the builder", func(t *testing.T) {
		t.Parallel()
		var captured *node.Struct
		gofixture.New().Struct("S", func(b *gofixture.StructBuilder) {
			captured = b.Node()
		})
		if captured == nil || captured.Name != "S" {
			t.Fatalf("Node returned wrong struct: %+v", captured)
		}
	})
}

func TestStructBuilder_Pos(t *testing.T) {
	t.Parallel()

	t.Run("records the supplied position on the struct", func(t *testing.T) {
		t.Parallel()
		pos := position.At("user.go", 7, 1)
		var captured *node.Struct
		gofixture.New().Struct("S", func(b *gofixture.StructBuilder) {
			b.Pos(pos)
			captured = b.Node()
		})
		if !captured.SourcePos.Equal(pos) {
			t.Fatalf("Pos not applied: got %v, want %v", captured.SourcePos, pos)
		}
	})
}

func TestStructBuilder_Docs(t *testing.T) {
	t.Parallel()

	t.Run("appends doc-comment lines in order", func(t *testing.T) {
		t.Parallel()
		var captured *node.Struct
		gofixture.New().Struct("S", func(b *gofixture.StructBuilder) {
			b.Docs("first line").Docs("second line", "third line")
			captured = b.Node()
		})
		got := captured.Docs()
		if len(got) != 3 || got[0] != "first line" || got[1] != "second line" || got[2] != "third line" {
			t.Fatalf("Docs order wrong: %+v", got)
		}
	})
}

func TestStructBuilder_Directive(t *testing.T) {
	t.Parallel()

	t.Run("attaches the directive to the struct's directive list", func(t *testing.T) {
		t.Parallel()
		d := gofixture.Directive("repo")
		var captured *node.Struct
		gofixture.New().Struct("S", func(b *gofixture.StructBuilder) {
			b.Directive(d)
			captured = b.Node()
		})
		if !captured.HasDirective("repo") {
			t.Fatalf("HasDirective should return true for repo")
		}
		if captured.Directive("repo") != d {
			t.Fatalf("attached directive should be retrievable verbatim")
		}
	})
}

func TestStructBuilder_TypeParam(t *testing.T) {
	t.Parallel()

	t.Run("declares a generic type parameter with owner wired", func(t *testing.T) {
		t.Parallel()
		constraint := gofixture.Constraint(gofixture.PkgNamed("fmt", "Stringer"))
		var captured *node.Struct
		gofixture.New().Struct("S", func(b *gofixture.StructBuilder) {
			b.TypeParam("T", constraint)
			captured = b.Node()
		})
		if !captured.IsGeneric() {
			t.Fatalf("struct should be generic")
		}
		tp := captured.TypeParams[0]
		if tp.Name != "T" || tp.Constraint != constraint || tp.Owner != captured {
			t.Fatalf("TypeParam wiring wrong: %+v", tp)
		}
	})

	t.Run("accepts nil constraint as an implicit any bound", func(t *testing.T) {
		t.Parallel()
		var captured *node.Struct
		gofixture.New().Struct("S", func(b *gofixture.StructBuilder) {
			b.TypeParam("T", nil)
			captured = b.Node()
		})
		if !captured.IsGeneric() {
			t.Fatalf("struct should be generic even with nil constraint")
		}
		if captured.TypeParams[0].Constraint != nil {
			t.Fatalf("nil constraint should be stored verbatim")
		}
	})
}

func TestStructBuilder_Embed(t *testing.T) {
	t.Parallel()

	t.Run("records an embed with owner wired", func(t *testing.T) {
		t.Parallel()
		typ := gofixture.PkgNamed("io", "Reader")
		var captured *node.Struct
		gofixture.New().Struct("S", func(b *gofixture.StructBuilder) {
			b.Embed(typ)
			captured = b.Node()
		})
		if len(captured.Embeds) != 1 {
			t.Fatalf("expected 1 embed; got %d", len(captured.Embeds))
		}
		e := captured.Embeds[0]
		if e.Type != typ || e.Owner != captured {
			t.Fatalf("embed wiring wrong: %+v", e)
		}
	})
}

func TestStructBuilder_Field(t *testing.T) {
	t.Parallel()

	t.Run("appends a field with the supplied type and owner wired", func(t *testing.T) {
		t.Parallel()
		var captured *node.Struct
		gofixture.New().Struct("S", func(b *gofixture.StructBuilder) {
			b.Field("ID", gofixture.Named("string"), nil)
			captured = b.Node()
		})
		f := captured.FieldByName("ID")
		if f == nil {
			t.Fatalf("Field not found by name")
		}
		if f.Type.Name != "string" {
			t.Fatalf("Field type wrong: %+v", f.Type)
		}
		if f.Owner != captured {
			t.Fatalf("Field owner back-pointer wrong: %+v", f.Owner)
		}
	})

	t.Run("invokes the field configuration callback exactly once", func(t *testing.T) {
		t.Parallel()
		var calls int
		gofixture.New().Struct("S", func(b *gofixture.StructBuilder) {
			b.Field("ID", gofixture.Named("string"), func(*gofixture.FieldBuilder) {
				calls++
			})
		})
		if calls != 1 {
			t.Fatalf("field callback should run exactly once; got %d", calls)
		}
	})
}

func TestStructBuilder_Method(t *testing.T) {
	t.Parallel()

	t.Run("seeds the method with a pointer-receiver to the enclosing struct", func(t *testing.T) {
		t.Parallel()
		var captured *node.Struct
		gofixture.New().Struct("Repo", func(b *gofixture.StructBuilder) {
			b.Method("Get", nil)
			captured = b.Node()
		})
		m := captured.MethodByName("Get")
		if m == nil {
			t.Fatalf("Method not found by name")
		}
		if !m.HasReceiver() {
			t.Fatalf("struct method should have a receiver by default")
		}
		if !m.Receiver.IsPointer() || m.Receiver.Elem.Name != "Repo" {
			t.Fatalf("default receiver should be *Repo; got %+v", m.Receiver)
		}
		if m.Owner != captured {
			t.Fatalf("Method owner back-pointer wrong: %+v", m.Owner)
		}
	})

	t.Run("invokes the method configuration callback exactly once", func(t *testing.T) {
		t.Parallel()
		var calls int
		gofixture.New().Struct("S", func(b *gofixture.StructBuilder) {
			b.Method("M", func(*gofixture.MethodBuilder) { calls++ })
		})
		if calls != 1 {
			t.Fatalf("method callback should run exactly once; got %d", calls)
		}
	})
}
