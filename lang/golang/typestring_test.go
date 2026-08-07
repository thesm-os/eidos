// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"errors"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

func TestTypeString(t *testing.T) {
	t.Parallel()

	ptr := func(e *node.TypeRef) *node.TypeRef {
		return &node.TypeRef{TypeKind: node.TypeRefPointer, Elem: e}
	}

	t.Run("spells each composite shape", func(t *testing.T) {
		t.Parallel()
		for want, ref := range map[string]*node.TypeRef{
			"string":   builtinRef("string"),
			"*string":  ptr(builtinRef("string")),
			"[]string": sliceRef(builtinRef("string")),
			"[4]byte": {
				TypeKind: node.TypeRefArray, ArrayLen: 4, Elem: builtinRef("byte"),
			},
			"map[string]int": mapRef(builtinRef("string"), builtinRef("int")),
			"**string":       ptr(ptr(builtinRef("string"))),
		} {
			if got := golang.TypeString(ref); got != want {
				t.Errorf("TypeString = %q, want %q", got, want)
			}
		}
	})

	t.Run("qualifies by the last path segment", func(t *testing.T) {
		t.Parallel()
		// For reading: the short form is what appears in the author's
		// own file, and the full path is what they wrote once in an
		// import block.
		got := golang.TypeString(namedTypeRef("example.com/store", "User"))
		if got != "store.User" {
			t.Fatalf("TypeString = %q, want store.User", got)
		}
	})

	t.Run("the qualified form carries the whole path", func(t *testing.T) {
		t.Parallel()
		// For a message that has to be unambiguous, where two
		// `store.User` types would otherwise read as one.
		got := golang.TypeStringQualified(namedTypeRef("example.com/store", "User"))
		if got != "example.com/store.User" {
			t.Fatalf("TypeStringQualified = %q", got)
		}
	})

	t.Run("spells generic instantiation", func(t *testing.T) {
		t.Parallel()
		box := namedTypeRef("example.com/x", "Box")
		box.TypeArgs = []*node.TypeRef{builtinRef("string"), builtinRef("int")}
		if got := golang.TypeString(box); got != "x.Box[string, int]" {
			t.Fatalf("TypeString = %q", got)
		}
	})

	t.Run("spells a function type", func(t *testing.T) {
		t.Parallel()
		fn := &node.TypeRef{
			TypeKind:    node.TypeRefFunc,
			FuncParams:  []*node.TypeRef{ctxRef(), builtinRef("string")},
			FuncReturns: []*node.TypeRef{builtinRef("int"), errorRef()},
		}
		want := "func(context.Context, string) (int, error)"
		if got := golang.TypeString(fn); got != want {
			t.Fatalf("TypeString = %q, want %q", got, want)
		}
	})

	t.Run("a single result renders bare", func(t *testing.T) {
		t.Parallel()
		// Which is what Go's own grammar prefers.
		fn := &node.TypeRef{
			TypeKind:    node.TypeRefFunc,
			FuncReturns: []*node.TypeRef{errorRef()},
		}
		if got := golang.TypeString(fn); got != "func() error" {
			t.Fatalf("TypeString = %q", got)
		}
	})

	t.Run("spells a channel with its direction", func(t *testing.T) {
		t.Parallel()
		// The model has no channel variant, so the spelling is
		// reassembled from the stamp rather than read off the shape.
		ch := namedTypeRef("go.chan", "chan")
		golang.MetaIsChannel.Set(ch.EnsureMeta(), true, "test")
		golang.MetaChanDir.Set(ch.EnsureMeta(), string(golang.ChanRecv), "test")
		ch.TypeArgs = []*node.TypeRef{builtinRef("int")}
		if got := golang.TypeString(ch); got != "<-chan int" {
			t.Fatalf("TypeString = %q, want <-chan int", got)
		}
	})

	t.Run("an anonymous composite is named rather than reproduced", func(t *testing.T) {
		t.Parallel()
		// Reproducing its fields makes a diagnostic longer than the
		// declaration it is about.
		st := &node.TypeRef{TypeKind: node.TypeRefAnonStruct, Fields: []*node.Field{{Name: "ID"}}}
		if got := golang.TypeString(st); got != "struct{…}" {
			t.Fatalf("TypeString = %q", got)
		}
		iface := &node.TypeRef{
			TypeKind: node.TypeRefAnonInterface,
			Methods:  []*node.Method{{Name: "Read"}},
		}
		if got := golang.TypeString(iface); got != "interface{…}" {
			t.Fatalf("TypeString = %q", got)
		}
	})

	t.Run("the empty interface spells any", func(t *testing.T) {
		t.Parallel()
		bare := &node.TypeRef{TypeKind: node.TypeRefAnonInterface}
		if got := golang.TypeString(bare); got != "any" {
			t.Fatalf("TypeString = %q, want any", got)
		}
	})

	t.Run("a type parameter names itself", func(t *testing.T) {
		t.Parallel()
		p := &node.TypeRef{TypeKind: node.TypeRefTypeParam, Name: "T"}
		if got := golang.TypeString(p); got != "T" {
			t.Fatalf("TypeString = %q", got)
		}
	})

	t.Run("nil spells nothing", func(t *testing.T) {
		t.Parallel()
		// An empty type in a message is visibly wrong, where a partial
		// one would read as correct.
		if got := golang.TypeString(nil); got != "" {
			t.Fatalf("TypeString(nil) = %q", got)
		}
	})
}

func TestParseTypeRef(t *testing.T) {
	t.Parallel()

	t.Run("resolves a bare name against the source package", func(t *testing.T) {
		t.Parallel()
		got, err := golang.ParseTypeRef("User", "example.com/x")
		if err != nil {
			t.Fatalf("ParseTypeRef: %v", err)
		}
		ext, ok := got.(*emit.ExternalRef)
		if !ok || ext.Package != "example.com/x" || ext.Name != "User" {
			t.Fatalf("ParseTypeRef = %#v", got)
		}
	})

	t.Run("a builtin stays bare", func(t *testing.T) {
		t.Parallel()
		got, err := golang.ParseTypeRef("string", "example.com/x")
		if err != nil {
			t.Fatalf("ParseTypeRef: %v", err)
		}
		if _, ok := got.(*emit.BuiltinRef); !ok {
			t.Fatalf("ParseTypeRef = %T, want a builtin", got)
		}
	})

	t.Run("parses a pointer", func(t *testing.T) {
		t.Parallel()
		got, err := golang.ParseTypeRef("*User", "example.com/x")
		if err != nil {
			t.Fatalf("ParseTypeRef: %v", err)
		}
		c, ok := got.(*emit.CompositeRef)
		if !ok || c.Shape != emit.ShapePointer {
			t.Fatalf("ParseTypeRef = %#v, want a pointer", got)
		}
	})

	t.Run("parses a slice of pointers", func(t *testing.T) {
		t.Parallel()
		got, err := golang.ParseTypeRef("[]*User", "example.com/x")
		if err != nil {
			t.Fatalf("ParseTypeRef: %v", err)
		}
		outer := got.(*emit.CompositeRef)
		if outer.Shape != emit.ShapeSlice {
			t.Fatalf("outer = %v, want a slice", outer.Shape)
		}
		if inner, ok := outer.Elem.(*emit.CompositeRef); !ok || inner.Shape != emit.ShapePointer {
			t.Fatalf("element = %#v, want a pointer", outer.Elem)
		}
	})

	t.Run("parses a map with both halves", func(t *testing.T) {
		t.Parallel()
		got, err := golang.ParseTypeRef("map[string]*User", "example.com/x")
		if err != nil {
			t.Fatalf("ParseTypeRef: %v", err)
		}
		m := got.(*emit.CompositeRef)
		if m.Shape != emit.ShapeMap {
			t.Fatalf("ParseTypeRef = %v, want a map", m.Shape)
		}
		if b, ok := m.MapKey.(*emit.BuiltinRef); !ok || b.Name != "string" {
			t.Fatalf("key = %#v", m.MapKey)
		}
	})

	t.Run("finds the bracket that closes a nested key", func(t *testing.T) {
		t.Parallel()
		// Taking the first `]` produces a key of `map[string`, which
		// resolves to nothing.
		got, err := golang.ParseTypeRef("map[map[string]int]bool", "example.com/x")
		if err != nil {
			t.Fatalf("ParseTypeRef: %v", err)
		}
		m := got.(*emit.CompositeRef)
		key, ok := m.MapKey.(*emit.CompositeRef)
		if !ok || key.Shape != emit.ShapeMap {
			t.Fatalf("key = %#v, want a map", m.MapKey)
		}
	})

	t.Run("a full import path names itself", func(t *testing.T) {
		t.Parallel()
		got, err := golang.ParseTypeRef("[]example.com/seed.Config", "example.com/x")
		if err != nil {
			t.Fatalf("ParseTypeRef: %v", err)
		}
		elem := got.(*emit.CompositeRef).Elem.(*emit.ExternalRef)
		if elem.Package != "example.com/seed" {
			t.Fatalf("element package = %q", elem.Package)
		}
	})

	t.Run("tolerates surrounding space", func(t *testing.T) {
		t.Parallel()
		if _, err := golang.ParseTypeRef("  []User  ", "example.com/x"); err != nil {
			t.Fatalf("ParseTypeRef: %v", err)
		}
	})

	t.Run("refuses what it cannot parse rather than half-parsing", func(t *testing.T) {
		t.Parallel()
		// Each needs syntax a directive value has no room for, and a
		// caller wanting one declares a named type instead.
		for _, expr := range []string{
			"", "func() error", "chan int", "<-chan int",
			"struct{ID int}", "[4]byte", "map[string", "Box[T]",
		} {
			if _, err := golang.ParseTypeRef(expr, "example.com/x"); err == nil {
				t.Errorf("ParseTypeRef(%q) succeeded; want a refusal", expr)
			}
		}
	})

	t.Run("the refusal carries the sentinel", func(t *testing.T) {
		t.Parallel()
		_, err := golang.ParseTypeRef("chan int", "example.com/x")
		if !errors.Is(err, golang.ErrBadTypeExpr) {
			t.Fatalf("err = %v, want ErrBadTypeExpr", err)
		}
	})

	t.Run("a malformed inner half fails the whole expression", func(t *testing.T) {
		t.Parallel()
		// A map with one usable half is not a usable map.
		for _, expr := range []string{"map[chan int]bool", "map[string]chan int", "*"} {
			if _, err := golang.ParseTypeRef(expr, "example.com/x"); err == nil {
				t.Errorf("ParseTypeRef(%q) succeeded", expr)
			}
		}
	})
}

func TestTypeStringEdges(t *testing.T) {
	t.Parallel()

	t.Run("spells the other channel directions", func(t *testing.T) {
		t.Parallel()
		for dir, want := range map[golang.ChanDirection]string{
			golang.ChanSend: "chan<- int",
			golang.ChanBoth: "chan int",
		} {
			ch := namedTypeRef("go.chan", "chan")
			golang.MetaIsChannel.Set(ch.EnsureMeta(), true, "test")
			golang.MetaChanDir.Set(ch.EnsureMeta(), string(dir), "test")
			ch.TypeArgs = []*node.TypeRef{builtinRef("int")}
			if got := golang.TypeString(ch); got != want {
				t.Errorf("TypeString = %q, want %q", got, want)
			}
		}
	})

	t.Run("a channel with no recorded element spells the direction alone", func(t *testing.T) {
		t.Parallel()
		ch := namedTypeRef("go.chan", "chan")
		golang.MetaIsChannel.Set(ch.EnsureMeta(), true, "test")
		if got := golang.TypeString(ch); got != "chan " {
			t.Fatalf("TypeString = %q", got)
		}
	})

	t.Run("a function with no results renders bare", func(t *testing.T) {
		t.Parallel()
		fn := &node.TypeRef{TypeKind: node.TypeRefFunc}
		if got := golang.TypeString(fn); got != "func()" {
			t.Fatalf("TypeString = %q", got)
		}
	})

	t.Run("a leading digit is not a type name", func(t *testing.T) {
		t.Parallel()
		// A path never starts with one and an identifier cannot open
		// with one.
		if _, err := golang.ParseTypeRef("1x", "example.com/x"); err == nil {
			t.Fatalf("ParseTypeRef(1x) succeeded")
		}
	})

	t.Run("a nesting past the budget is refused", func(t *testing.T) {
		t.Parallel()
		expr := strings.Repeat("[]", 20) + "User"
		if _, err := golang.ParseTypeRef(expr, "example.com/x"); err == nil {
			t.Fatalf("ParseTypeRef accepted a type past the nesting budget")
		}
	})
}
