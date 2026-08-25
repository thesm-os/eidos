// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/emit"
)

// TestRenderVariants_TypedIotaPromotion pins the canonical typed
// iota enum spelling: the first variant carries the enum's named
// type so Go's typed-iota promotion extends to subsequent
// auto-incrementing variants.
func TestRenderVariants_TypedIotaPromotion(t *testing.T) {
	t.Parallel()

	t.Run("first variant carries type name, rest are bare", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Enums: []*emit.Enum{{
				Name: "Status", Package: "x", Target: target,
				Underlying: emit.Builtin("int"),
				Variants: []*emit.EnumVariant{
					{Name: "Pending", Value: &emit.Expr{ExprKind: emit.ExprIdent, Name: "iota"}},
					{Name: "Active"},
					{Name: "Closed"},
				},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		for _, want := range []string{
			"type Status int\n",
			"Pending Status = iota",
			"\n\tActive\n",
			"\n\tClosed\n",
		} {
			if !strings.Contains(string(body), want) {
				t.Fatalf("typed-iota enum should contain %q; got:\n%s", want, body)
			}
		}
	})
}

// TestRenderVariants_ExplicitValuesPerVariant pins the explicit-
// value form: every variant carries its own `Name = Value` line,
// including the first.
func TestRenderVariants_ExplicitValuesPerVariant(t *testing.T) {
	t.Parallel()

	t.Run("each variant emits its declared value", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Enums: []*emit.Enum{{
				Name: "HTTPStatus", Package: "x", Target: target,
				Underlying: emit.Builtin("int"),
				Variants: []*emit.EnumVariant{
					{Name: "OK", Value: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "200"}},
					{
						Name:  "NotFound",
						Value: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "404"},
					},
					{
						Name:  "Server",
						Value: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "500"},
					},
				},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		// First variant carries the type name; subsequent variants
		// inherit the typed promotion via their explicit values.
		for _, want := range []string{
			"OK       HTTPStatus = 200",
			"NotFound            = 404",
			"Server              = 500",
		} {
			if !strings.Contains(string(body), want) {
				t.Fatalf("explicit-value enum should contain %q; got:\n%s", want, body)
			}
		}
	})
}

// TestRenderVariants_UntypedEnum pins the untyped form: no
// `type X Underlying` line and no type prefix on the first
// variant — bare untyped constants.
func TestRenderVariants_UntypedEnum(t *testing.T) {
	t.Parallel()

	t.Run("nil Underlying omits type line and type prefix", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Enums: []*emit.Enum{{
				Name: "Mode", Package: "x", Target: target,
				Variants: []*emit.EnumVariant{
					{Name: "Read", Value: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "1"}},
					{Name: "Write", Value: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "2"}},
				},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if strings.Contains(string(body), "type Mode") {
			t.Fatalf("untyped enum must not emit a type decl; got:\n%s", body)
		}
		// The first variant of an untyped enum is bare `Name = Value`.
		if !strings.Contains(string(body), "Read  = 1") {
			t.Fatalf("untyped first variant should be bare 'Name = Value'; got:\n%s", body)
		}
	})
}

// TestRenderVariants_DocLines pins the per-variant doc placement:
// DocLines render above the variant; a blank line separates
// documented variants from undocumented siblings, mirroring the
// renderFields convention.
func TestRenderVariants_DocLines(t *testing.T) {
	t.Parallel()

	t.Run("documented variant is preceded by its doc lines", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Enums: []*emit.Enum{{
				Name: "Severity", Package: "x", Target: target,
				Underlying: emit.Builtin("int"),
				Variants: []*emit.EnumVariant{
					{
						BaseEmit: emit.BaseEmit{DocLines: []string{"Info is the lowest severity."}},
						Name:     "Info",
						Value:    &emit.Expr{ExprKind: emit.ExprIdent, Name: "iota"},
					},
					{Name: "Warn"},
					{
						BaseEmit: emit.BaseEmit{DocLines: []string{"Error stops the pipeline."}},
						Name:     "Error",
					},
				},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "// Info is the lowest severity.") {
			t.Fatalf("variant doc missing; got:\n%s", body)
		}
		if !strings.Contains(string(body), "// Error stops the pipeline.") {
			t.Fatalf("second variant doc missing; got:\n%s", body)
		}
	})
}
