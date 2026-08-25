// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/emit"
)

// TestTemplates_DispatchByKind verifies the `render` funcmap entry
// dispatches by [emit.Node.Kind], producing the matching kind's
// rendered output inline within a parent template.
func TestTemplates_DispatchByKind(t *testing.T) {
	t.Parallel()

	t.Run("emit.struct rendered through emit.file's render call", func(t *testing.T) {
		t.Parallel()
		body := renderSingleFieldStruct(t, "Name", emit.Builtin("string"))
		// The file template's `package <pkg>` clause should be
		// followed by the struct template's `type X struct { ... }`
		// block. Verifying both fragments are present is the
		// dispatch-level assertion.
		if !strings.Contains(body, "package x") {
			t.Fatalf("body should contain 'package x'; got:\n%s", body)
		}
		if !strings.Contains(body, "type X struct {") {
			t.Fatalf("body should contain 'type X struct {'; got:\n%s", body)
		}
	})
}

// TestTemplates_Interface covers the emit.interface template:
// methods inlined as signatures, embeds rendered as bare type
// references, the empty-interface case.
func TestTemplates_Interface(t *testing.T) {
	t.Parallel()

	t.Run("interface with method signatures renders inline", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Interfaces: []*emit.Interface{{
				Name: "Closer", Package: "x", Target: target,
				Methods: []*emit.Method{
					{Name: "Close", Returns: emit.AnonReturns(emit.Builtin("error"))},
				},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "type Closer interface {") {
			t.Fatalf("interface decl missing; got:\n%s", body)
		}
		if !strings.Contains(string(body), "Close() error") {
			t.Fatalf("method signature missing; got:\n%s", body)
		}
	})

	t.Run("empty interface renders as bare braces", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Interfaces: []*emit.Interface{{Name: "Any", Package: "x", Target: target}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		bareBraces := strings.Contains(string(body), "type Any interface {\n}")
		tightBraces := strings.Contains(string(body), "type Any interface{}")
		if !bareBraces && !tightBraces {
			t.Fatalf("empty interface should render with empty braces; got:\n%s", body)
		}
	})

	t.Run("interface with embeds renders embedded type references", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Interfaces: []*emit.Interface{{
				Name: "Reader", Package: "x", Target: target,
				Embeds: []*emit.Embed{{Type: emit.External("io", "Closer")}},
				Methods: []*emit.Method{
					{
						Name:    "Read",
						Params:  []*emit.Param{{Name: "n", Type: emit.Builtin("int")}},
						Returns: emit.AnonReturns(emit.Builtin("int"), emit.Builtin("error")),
					},
				},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "io.Closer\n") {
			t.Fatalf("embedded type ref missing; got:\n%s", body)
		}
		if !strings.Contains(string(body), "Read(n int) (int, error)") {
			t.Fatalf("method signature with multi-return mismatched; got:\n%s", body)
		}
	})
}

// TestTemplates_Alias covers the emit.alias template for both
// forms: the alias form (`type X = Y`) and the definition form
// (`type X Y`).
func TestTemplates_Alias(t *testing.T) {
	t.Parallel()

	t.Run("type definition form renders without '='", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Aliases: []*emit.Alias{{
				Name: "UserID", Package: "x", File: target,
				Target:  emit.Builtin("int"),
				IsAlias: false,
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "type UserID int\n") {
			t.Fatalf("type-definition form mismatched; got:\n%s", body)
		}
	})

	t.Run("type alias form renders with '='", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Aliases: []*emit.Alias{{
				Name: "Bytes", Package: "x", File: target,
				Target:  emit.Builtin("byte"),
				IsAlias: true,
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "type Bytes = byte\n") {
			t.Fatalf("alias form mismatched; got:\n%s", body)
		}
	})
}

// TestTemplates_Variable covers the emit.variable template across
// the three init combinations: declared type only, init only
// (type inferred), and both.
func TestTemplates_Variable(t *testing.T) {
	t.Parallel()

	t.Run("typed var without initialiser renders as 'var X T'", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Variables: []*emit.Variable{{
				Name: "Counter", Package: "x", Target: target,
				Type: emit.Builtin("int"),
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "var Counter int\n") {
			t.Fatalf("typed-no-init form mismatched; got:\n%s", body)
		}
	})

	t.Run("typed var with literal initialiser", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Variables: []*emit.Variable{{
				Name: "Greeting", Package: "x", Target: target,
				Type: emit.Builtin("string"),
				Init: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitString, RawText: "hello"},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "var Greeting string = \"hello\"\n") {
			t.Fatalf("typed-with-init form mismatched; got:\n%s", body)
		}
	})

	t.Run("inferred-type var renders without type slot", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Variables: []*emit.Variable{{
				Name: "MaxRetries", Package: "x", Target: target,
				Init: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "3"},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "var MaxRetries = 3\n") {
			t.Fatalf("inferred-type form mismatched; got:\n%s", body)
		}
	})
}

// TestTemplates_Constant covers the emit.constant template:
// untyped and typed constants, plus the iota case via ExprIdent.
func TestTemplates_Constant(t *testing.T) {
	t.Parallel()

	t.Run("untyped constant renders as 'const X = V'", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Constants: []*emit.Constant{{
				Name: "Pi", Package: "x", Target: target,
				Value: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitFloat, RawText: "3.14"},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "const Pi = 3.14\n") {
			t.Fatalf("untyped const form mismatched; got:\n%s", body)
		}
	})

	t.Run("typed constant renders with declared type slot", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Constants: []*emit.Constant{{
				Name: "Limit", Package: "x", Target: target,
				Type:  emit.Builtin("int"),
				Value: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "100"},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "const Limit int = 100\n") {
			t.Fatalf("typed const form mismatched; got:\n%s", body)
		}
	})

	t.Run("constant valued at iota uses ExprIdent", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Constants: []*emit.Constant{{
				Name: "First", Package: "x", Target: target,
				Value: &emit.Expr{ExprKind: emit.ExprIdent, Name: "iota"},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "const First = iota\n") {
			t.Fatalf("iota constant form mismatched; got:\n%s", body)
		}
	})
}

// TestTemplates_Function covers the emit.function template across
// the documented shape combinations — bare signature with empty
// body, signature with params + returns + body, and the generic
// form (`func F[T any](...)`).
func TestTemplates_Function(t *testing.T) {
	t.Parallel()

	t.Run("bare function with empty body renders 'func F() {}'", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Functions: []*emit.Function{{Name: "F", Package: "x", Target: target}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		// gofmt collapses an empty body to `func F() {\n}`.
		if !strings.Contains(string(body), "func F() {") {
			t.Fatalf("function signature missing; got:\n%s", body)
		}
	})

	t.Run("function with params, returns, and body", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Functions: []*emit.Function{{
				Name: "Add", Package: "x", Target: target,
				Params: []*emit.Param{
					{Name: "a", Type: emit.Builtin("int")},
					{Name: "b", Type: emit.Builtin("int")},
				},
				Returns: emit.AnonReturns(emit.Builtin("int")),
				Body: []*emit.Stmt{emit.NewReturn(&emit.Expr{
					ExprKind: emit.ExprBinary, Op: "+",
					Left:  &emit.Expr{ExprKind: emit.ExprIdent, Name: "a"},
					Right: &emit.Expr{ExprKind: emit.ExprIdent, Name: "b"},
				})},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "func Add(a int, b int) int {") {
			t.Fatalf("function signature mismatched; got:\n%s", body)
		}
		if !strings.Contains(string(body), "return a + b") {
			t.Fatalf("function body mismatched; got:\n%s", body)
		}
	})

	t.Run("generic function renders type-param list", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Functions: []*emit.Function{{
				Name: "Identity", Package: "x", Target: target,
				TypeParams: []*emit.TypeParam{{
					Name:       "T",
					Constraint: &emit.Constraint{Embedded: []emit.Ref{emit.Builtin("any")}},
				}},
				Params:  []*emit.Param{{Name: "v", Type: emit.Builtin("int")}},
				Returns: emit.AnonReturns(emit.Builtin("int")),
				Body: []*emit.Stmt{emit.NewReturn(&emit.Expr{
					ExprKind: emit.ExprIdent, Name: "v",
				})},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "func Identity[T any](v int) int {") {
			t.Fatalf("generic function signature mismatched; got:\n%s", body)
		}
	})
}

// TestTemplates_Method covers the emit.method template — receiver
// rendering for both pointer and value receivers, the body
// dispatch, and the anonymous-receiver form (`func (Type) M()`).
func TestTemplates_Method(t *testing.T) {
	t.Parallel()

	t.Run("value-receiver method renders 'func (r T) M() { ... }'", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		holder := &emit.Struct{Name: "Counter", Package: "x", Target: target}
		holder.Methods = []*emit.Method{{
			Name:         "Inc",
			Receiver:     emit.Internal(holder),
			ReceiverName: "c",
			Body: []*emit.Stmt{emit.NewAssign(
				[]*emit.Expr{
					{ExprKind: emit.ExprField, Receiver: &emit.Expr{ExprKind: emit.ExprIdent, Name: "c"}, Name: "n"},
				},
				"+=",
				[]*emit.Expr{{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "1"}},
			)},
		}}
		addEmitPackage(t, ctx, emitPackage("x", holder))
		out := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(out), "func (c Counter) Inc() {") {
			t.Fatalf("method receiver clause mismatched; got:\n%s", out)
		}
		if !strings.Contains(string(out), "c.n += 1") {
			t.Fatalf("method body mismatched; got:\n%s", out)
		}
	})

	t.Run("pointer-receiver method renders '*T'", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		holder := &emit.Struct{Name: "Repo", Package: "x", Target: target}
		holder.Methods = []*emit.Method{{
			Name:         "Save",
			Receiver:     emit.Ptr(emit.Internal(holder)),
			ReceiverName: "r",
			Returns:      emit.AnonReturns(emit.Builtin("error")),
			Body:         []*emit.Stmt{emit.NewReturn(&emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitNil})},
		}}
		addEmitPackage(t, ctx, emitPackage("x", holder))
		out := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(out), "func (r *Repo) Save() error {") {
			t.Fatalf("pointer-receiver method mismatched; got:\n%s", out)
		}
	})

	t.Run("anonymous-receiver method renders '(T)'", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		holder := &emit.Struct{Name: "Marker", Package: "x", Target: target}
		holder.Methods = []*emit.Method{{
			Name:     "Marker",
			Receiver: emit.Internal(holder),
		}}
		addEmitPackage(t, ctx, emitPackage("x", holder))
		out := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(out), "func (Marker) Marker() {") {
			t.Fatalf("anonymous-receiver method mismatched; got:\n%s", out)
		}
	})
}

// TestTemplates_Enum covers the emit.enum template — typed iota
// promotion, untyped enums, and explicit-value variants.
func TestTemplates_Enum(t *testing.T) {
	t.Parallel()

	t.Run("typed iota enum renders 'type Color int' + const block", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Enums: []*emit.Enum{{
				Name: "Color", Package: "x", Target: target,
				Underlying: emit.Builtin("int"),
				Variants: []*emit.EnumVariant{
					{Name: "Red", Value: &emit.Expr{ExprKind: emit.ExprIdent, Name: "iota"}},
					{Name: "Green"},
					{Name: "Blue"},
				},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if !strings.Contains(string(body), "type Color int\n") {
			t.Fatalf("enum type decl mismatched; got:\n%s", body)
		}
		if !strings.Contains(string(body), "Red Color = iota") {
			t.Fatalf("first variant should declare type + iota; got:\n%s", body)
		}
		if !strings.Contains(string(body), "Green") || !strings.Contains(string(body), "Blue") {
			t.Fatalf("subsequent variants missing; got:\n%s", body)
		}
	})

	t.Run("untyped enum with explicit values omits 'type' line", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, &emit.Package{
			Name: "x", Path: "x",
			Enums: []*emit.Enum{{
				Name: "Flag", Package: "x", Target: target,
				Variants: []*emit.EnumVariant{
					{Name: "A", Value: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "1"}},
					{Name: "B", Value: &emit.Expr{ExprKind: emit.ExprLiteral, LitKind: emit.LitInt, RawText: "2"}},
				},
			}},
		})
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		if strings.Contains(string(body), "type Flag") {
			t.Fatalf("untyped enum must not emit a type decl; got:\n%s", body)
		}
		if !strings.Contains(string(body), "A = 1") || !strings.Contains(string(body), "B = 2") {
			t.Fatalf("explicit-value variants mismatched; got:\n%s", body)
		}
	})
}
