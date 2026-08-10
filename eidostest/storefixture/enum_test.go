// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package storefixture_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/node"
)

func TestBuilder_Enum(t *testing.T) {
	t.Parallel()

	t.Run("creates an enum with the configured name and package", func(t *testing.T) {
		t.Parallel()
		b := storefixture.New().Package("users", "example.com/users").
			Enum("Status", nil)
		e := b.PackageNode().EnumByName("Status")
		if e == nil {
			t.Fatalf("Enum should be reachable by name")
		}
		requireQName(t, e.QName(), "example.com/users.Status")
	})

	t.Run("invokes the configuration callback exactly once", func(t *testing.T) {
		t.Parallel()
		var calls int
		storefixture.New().Enum("E", func(*storefixture.EnumBuilder) { calls++ })
		if calls != 1 {
			t.Fatalf("callback invocation count wrong: got %d, want 1", calls)
		}
	})
}

func TestEnumBuilder_Node(t *testing.T) {
	t.Parallel()

	t.Run("returns the enum backing the builder", func(t *testing.T) {
		t.Parallel()
		got := captureFirstEnum(t, func(*storefixture.EnumBuilder) {})
		if got == nil || got.Name != "E" {
			t.Fatalf("Node returned wrong enum: %+v", got)
		}
	})
}

func TestEnumBuilder_Pos(t *testing.T) {
	t.Parallel()

	t.Run("records the supplied position", func(t *testing.T) {
		t.Parallel()
		pos := position.At("e.go", 1, 1)
		got := captureFirstEnum(t, func(b *storefixture.EnumBuilder) { b.Pos(pos) })
		if !got.SourcePos.Equal(pos) {
			t.Fatalf("Pos not applied: %v", got.SourcePos)
		}
	})
}

func TestEnumBuilder_Docs(t *testing.T) {
	t.Parallel()

	t.Run("appends doc-comment lines in order", func(t *testing.T) {
		t.Parallel()
		got := captureFirstEnum(t, func(b *storefixture.EnumBuilder) {
			b.Docs("one").Docs("two")
		})
		if d := got.Docs(); len(d) != 2 || d[0] != "one" || d[1] != "two" {
			t.Fatalf("Docs order wrong: %+v", d)
		}
	})
}

func TestEnumBuilder_Directive(t *testing.T) {
	t.Parallel()

	t.Run("attaches the directive", func(t *testing.T) {
		t.Parallel()
		d := storefixture.Directive("expose")
		got := captureFirstEnum(t, func(b *storefixture.EnumBuilder) { b.Directive(d) })
		if !got.HasDirective("expose") {
			t.Fatalf("HasDirective should return true for expose")
		}
	})
}

func TestEnumBuilder_Underlying(t *testing.T) {
	t.Parallel()

	t.Run("records the underlying type", func(t *testing.T) {
		t.Parallel()
		typ := storefixture.Named("int")
		got := captureFirstEnum(t, func(b *storefixture.EnumBuilder) { b.Underlying(typ) })
		if !got.HasUnderlying() || got.Underlying != typ {
			t.Fatalf("Underlying not applied: %+v", got.Underlying)
		}
	})
}

func TestEnumBuilder_Variant(t *testing.T) {
	t.Parallel()

	t.Run("appends variants with owner wired in declaration order", func(t *testing.T) {
		t.Parallel()
		got := captureFirstEnum(t, func(b *storefixture.EnumBuilder) {
			b.Variant("Active", "1").Variant("Inactive", "2")
		})
		if len(got.Variants) != 2 {
			t.Fatalf("expected 2 variants; got %d", len(got.Variants))
		}
		first := got.VariantByName("Active")
		if first == nil || first.Value != "1" || first.Owner != got {
			t.Fatalf("first variant wiring wrong: %+v", first)
		}
		second := got.VariantByName("Inactive")
		if second == nil || second.Value != "2" || second.Owner != got {
			t.Fatalf("second variant wiring wrong: %+v", second)
		}
	})
}

// TestEnumBuilder_VariantBuilder covers the per-variant callback,
// whose reason for existing is the authored text override.
func TestEnumBuilder_VariantBuilder(t *testing.T) {
	t.Parallel()

	build := func(fn func(*storefixture.VariantBuilder)) *node.EnumVariant {
		return storefixture.New().
			Package("shop", "example.com/shop").
			Enum("Region", func(e *storefixture.EnumBuilder) {
				e.Underlying(storefixture.Named("string"))
				e.Variant("RegionUS", `"us-east"`, fn)
			}).
			PackageNode().Enums[0].Variants[0]
	}

	t.Run("attaches a directive to the variant", func(t *testing.T) {
		t.Parallel()
		// The highest-precedence layer of the rule deciding a
		// variant's textual form is authored on the variant, and no
		// fixture could spell one — so that layer was reachable only
		// through a real frontend, where every other layer is already
		// covered.
		v := build(func(vb *storefixture.VariantBuilder) {
			vb.Directive(storefixture.Directive("value", storefixture.Arg("americas")))
		})
		if !v.HasDirective("value") {
			t.Fatalf("variant carries no value directive: %+v", v.Directives())
		}
	})

	t.Run("appends doc lines", func(t *testing.T) {
		t.Parallel()
		v := build(func(vb *storefixture.VariantBuilder) { vb.Docs("RegionUS is the US.") })
		if len(v.Docs()) != 1 {
			t.Fatalf("Docs = %v", v.Docs())
		}
	})

	t.Run("inherits the enum's source file", func(t *testing.T) {
		t.Parallel()
		// A frontend records every member of a declaration at the file
		// the declaration was parsed from, and Layout routes from
		// whichever member a plugin picks as its origin — so a
		// positionless variant routes output to a basename the
		// toolchain discards.
		if got := build(nil).Pos().File; got != "shop/region.go" {
			t.Fatalf("variant file = %q, want the enum's own", got)
		}
	})

	t.Run("an explicit position wins", func(t *testing.T) {
		t.Parallel()
		v := build(func(vb *storefixture.VariantBuilder) {
			vb.Pos(position.Pos{File: "shop/other.go", Line: 9})
		})
		if got := v.Pos().File; got != "shop/other.go" {
			t.Fatalf("variant file = %q, want the pinned one", got)
		}
	})

	t.Run("a nil callback is accepted", func(t *testing.T) {
		t.Parallel()
		// The callback is variadic so every existing two-argument call
		// keeps working, and a nil one passed explicitly must not
		// panic on the way through.
		if v := build(nil); v.Owner == nil || v.Owner.Name != "Region" {
			t.Fatalf("Owner = %+v, want the enum", v.Owner)
		}
	})
}

// TestEnumBuilder_Method covers the hook StructBuilder and
// InterfaceBuilder already carried and EnumBuilder did not.
func TestEnumBuilder_Method(t *testing.T) {
	t.Parallel()

	pkg := storefixture.New().
		Package("shop", "example.com/shop").
		Enum("Region", func(e *storefixture.EnumBuilder) {
			e.Underlying(storefixture.Named("string"))
			e.Variant("RegionUS", `"us-east"`)
			e.Method("String", func(m *storefixture.MethodBuilder) {
				m.Return(storefixture.Named("string"))
			})
		}).
		PackageNode()

	t.Run("declares the method on the enum's type", func(t *testing.T) {
		t.Parallel()
		// A generator's first question about an enum is often which
		// conventional methods the author already wrote: an existing
		// String is what stops it shadowing one, and a fixture that
		// could not declare one could not reach that branch.
		if got := pkg.Enums[0].MethodByName("String"); got == nil {
			t.Fatalf("MethodByName(String) found nothing in %+v", pkg.Enums[0].Methods)
		}
	})

	t.Run("binds the receiver to the enum's own type", func(t *testing.T) {
		t.Parallel()
		recv := pkg.Enums[0].MethodByName("String").Receiver
		if recv == nil || recv.Name != "Region" || recv.Package != "example.com/shop" {
			t.Fatalf("Receiver = %+v, want example.com/shop.Region", recv)
		}
	})

	t.Run("inherits the enum's source file", func(t *testing.T) {
		t.Parallel()
		if got := pkg.Enums[0].MethodByName("String").Pos().File; got != "shop/region.go" {
			t.Fatalf("method file = %q, want the enum's own", got)
		}
	})
}
