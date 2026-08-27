// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package gofixture_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
	"go.thesmos.sh/eidos/node"
)

func TestBuilder_Interface(t *testing.T) {
	t.Parallel()

	t.Run("creates an interface with the configured name and package", func(t *testing.T) {
		t.Parallel()
		b := gofixture.New().Package("users", "example.com/users").
			Interface("Repo", nil)
		i := b.PackageNode().InterfaceByName("Repo")
		if i == nil {
			t.Fatalf("Interface should be reachable by name")
		}
		requireQName(t, i.QName(), "example.com/users.Repo")
	})

	t.Run("invokes the configuration callback exactly once", func(t *testing.T) {
		t.Parallel()
		var calls int
		gofixture.New().Interface("I", func(*gofixture.InterfaceBuilder) { calls++ })
		if calls != 1 {
			t.Fatalf("callback invocation count wrong: got %d, want 1", calls)
		}
	})
}

func TestInterfaceBuilder_Node(t *testing.T) {
	t.Parallel()

	t.Run("returns the interface backing the builder", func(t *testing.T) {
		t.Parallel()
		var captured *node.Interface
		gofixture.New().Interface("I", func(b *gofixture.InterfaceBuilder) {
			captured = b.Node()
		})
		if captured == nil || captured.Name != "I" {
			t.Fatalf("Node returned wrong interface: %+v", captured)
		}
	})
}

func TestInterfaceBuilder_Pos(t *testing.T) {
	t.Parallel()

	t.Run("records the supplied position", func(t *testing.T) {
		t.Parallel()
		pos := position.At("repo.go", 3, 1)
		var captured *node.Interface
		gofixture.New().Interface("I", func(b *gofixture.InterfaceBuilder) {
			b.Pos(pos)
			captured = b.Node()
		})
		if !captured.SourcePos.Equal(pos) {
			t.Fatalf("Pos not applied: got %v, want %v", captured.SourcePos, pos)
		}
	})
}

func TestInterfaceBuilder_Docs(t *testing.T) {
	t.Parallel()

	t.Run("appends doc-comment lines in order", func(t *testing.T) {
		t.Parallel()
		var captured *node.Interface
		gofixture.New().Interface("I", func(b *gofixture.InterfaceBuilder) {
			b.Docs("one").Docs("two")
			captured = b.Node()
		})
		got := captured.Docs()
		if len(got) != 2 || got[0] != "one" || got[1] != "two" {
			t.Fatalf("Docs order wrong: %+v", got)
		}
	})
}

func TestInterfaceBuilder_Directive(t *testing.T) {
	t.Parallel()

	t.Run("attaches the directive", func(t *testing.T) {
		t.Parallel()
		d := gofixture.Directive("mock")
		var captured *node.Interface
		gofixture.New().Interface("I", func(b *gofixture.InterfaceBuilder) {
			b.Directive(d)
			captured = b.Node()
		})
		if !captured.HasDirective("mock") {
			t.Fatalf("HasDirective should return true for mock")
		}
	})
}

func TestInterfaceBuilder_TypeParam(t *testing.T) {
	t.Parallel()

	t.Run("declares a generic type parameter with owner wired", func(t *testing.T) {
		t.Parallel()
		var captured *node.Interface
		gofixture.New().Interface("I", func(b *gofixture.InterfaceBuilder) {
			b.TypeParam("T", gofixture.Constraint(gofixture.Named("comparable")))
			captured = b.Node()
		})
		tp := captured.TypeParams[0]
		if tp.Name != "T" || tp.Owner != captured {
			t.Fatalf("TypeParam wiring wrong: %+v", tp)
		}
		if !tp.Constraint.IsComparable() {
			t.Fatalf("constraint should reflect comparable bound")
		}
	})
}

func TestInterfaceBuilder_Embed(t *testing.T) {
	t.Parallel()

	t.Run("records an embed with owner wired", func(t *testing.T) {
		t.Parallel()
		typ := gofixture.PkgNamed("io", "Reader")
		var captured *node.Interface
		gofixture.New().Interface("I", func(b *gofixture.InterfaceBuilder) {
			b.Embed(typ)
			captured = b.Node()
		})
		if len(captured.Embeds) != 1 || captured.Embeds[0].Type != typ || captured.Embeds[0].Owner != captured {
			t.Fatalf("Embed wiring wrong: %+v", captured.Embeds)
		}
	})
}

// TestInterfaceBuilder_TypeSet pins the half a fixture could not
// spell: the marker separating a constraint from a method-set
// contract. The model records both as embeds, so without the stamp
// every reader takes the type set for embedded interfaces the run
// failed to load — and a test over such a fixture exercises the wrong
// branch of whatever it drives.
func TestInterfaceBuilder_TypeSet(t *testing.T) {
	t.Parallel()

	t.Run("records the terms and stamps the constraint marker", func(t *testing.T) {
		t.Parallel()
		var captured *node.Interface
		gofixture.New().Interface("Numeric", func(b *gofixture.InterfaceBuilder) {
			b.TypeSet(gofixture.Named("int"), gofixture.Named("float64"))
			captured = b.Node()
		})
		if len(captured.Embeds) != 2 {
			t.Fatalf("recorded %d embeds, want the two terms", len(captured.Embeds))
		}
		// Read through the accessor every consumer reads through, so
		// the fixture and the readers cannot agree on a wrong key.
		if !golang.IsConstraintInterface(captured) {
			t.Fatal("a type set was declared and the constraint marker was not stamped")
		}
	})

	t.Run("terms carry the owner like any embed", func(t *testing.T) {
		t.Parallel()
		var captured *node.Interface
		gofixture.New().Interface("Numeric", func(b *gofixture.InterfaceBuilder) {
			b.TypeSet(gofixture.Named("int"))
			captured = b.Node()
		})
		if captured.Embeds[0].Owner != captured {
			t.Fatal("a term's Owner must point back at the interface")
		}
	})

	t.Run("a plain embed stamps nothing", func(t *testing.T) {
		t.Parallel()
		// The distinction is the method's whole reason to exist:
		// `interface{ io.Reader }` embeds and is not a constraint, and
		// only the author knows which of the two shapes they meant.
		var captured *node.Interface
		gofixture.New().Interface("I", func(b *gofixture.InterfaceBuilder) {
			b.Embed(gofixture.PkgNamed("io", "Reader"))
			captured = b.Node()
		})
		if golang.IsConstraintInterface(captured) {
			t.Fatal("an ordinary embed must not read as a type-set term")
		}
	})

	t.Run("an empty type set panics", func(t *testing.T) {
		t.Parallel()
		// Go's grammar has no empty union, so this is fixture misuse —
		// and stamping the marker over no terms would build a
		// declaration that visibly lacks the fact it claims.
		defer func() {
			if recover() == nil {
				t.Fatal("TypeSet() with no terms must panic")
			}
		}()
		gofixture.New().Interface("I", func(b *gofixture.InterfaceBuilder) {
			b.TypeSet()
		})
	})
}

func TestInterfaceBuilder_Method(t *testing.T) {
	t.Parallel()

	t.Run("declares a method with nil receiver and interface owner", func(t *testing.T) {
		t.Parallel()
		var captured *node.Interface
		gofixture.New().Interface("Repo", func(b *gofixture.InterfaceBuilder) {
			b.Method("Get", nil)
			captured = b.Node()
		})
		m := captured.Methods[0]
		if m.HasReceiver() {
			t.Fatalf("interface method must have a nil receiver; got %+v", m.Receiver)
		}
		if m.Owner != captured {
			t.Fatalf("Method owner should be the interface; got %+v", m.Owner)
		}
	})

	t.Run("invokes the method configuration callback exactly once", func(t *testing.T) {
		t.Parallel()
		var calls int
		gofixture.New().Interface("I", func(b *gofixture.InterfaceBuilder) {
			b.Method("M", func(*gofixture.MethodBuilder) { calls++ })
		})
		if calls != 1 {
			t.Fatalf("method callback should run exactly once; got %d", calls)
		}
	})
}
