// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/sdk"
)

// TestRefFactoriesReturnEmitTypes pins each re-exported type-ref
// factory to the concrete emit type it constructs.
//
// Assigning the result into a variable of the underlying type is
// the assertion: a factory rebound to the wrong emit constructor —
// [sdk.Ptr] to SliceOf, say — would keep compiling everywhere it
// is called, and the mistake would surface only as a wrong type in
// a generated file.
//
//nolint:staticcheck // intentional redundant typing — the redundancy is the test
func TestRefFactoriesReturnEmitTypes(t *testing.T) {
	t.Parallel()

	t.Run("leaf refs build their emit shapes", func(t *testing.T) {
		t.Parallel()
		var b *emit.BuiltinRef = sdk.Builtin("string")
		if b == nil || b.Name != "string" {
			t.Fatalf("Builtin(string) = %v, want a builtin ref named string", b)
		}
		var x *emit.ExternalRef = sdk.External("context", "Context")
		if x == nil || x.Package != "context" || x.Name != "Context" {
			t.Fatalf("External(context, Context) = %v", x)
		}
		var i *emit.TypeRef = sdk.Internal(&sdk.EmitStruct{Name: "Repo"})
		if i == nil {
			t.Fatal("Internal returned nil")
		}
	})

	t.Run("composite refs build their emit shapes", func(t *testing.T) {
		t.Parallel()
		elem := sdk.Builtin("byte")
		composites := map[string]*emit.CompositeRef{
			"Ptr":          sdk.Ptr(elem),
			"SliceOf":      sdk.SliceOf(elem),
			"ArrayOf":      sdk.ArrayOf(elem, 4),
			"MapOf":        sdk.MapOf(sdk.Builtin("string"), elem),
			"FuncOf":       sdk.FuncOf([]sdk.Ref{elem}, []sdk.Ref{elem}),
			"Union":        sdk.Union(sdk.UnionTerm{Type: elem}),
			"AnonStructOf": sdk.AnonStructOf([]sdk.AnonField{{Name: "N", Type: elem}}, nil),
		}
		for name, ref := range composites {
			if ref == nil {
				t.Errorf("%s returned nil", name)
			}
		}
	})

	t.Run("the composite shapes stay distinct", func(t *testing.T) {
		t.Parallel()
		// The factories share a return type, so a misbound one is
		// invisible to the assignment check above. The shape
		// discriminator is what actually separates them.
		elem := sdk.Builtin("byte")
		if sdk.Ptr(elem).Shape == sdk.SliceOf(elem).Shape {
			t.Error("Ptr and SliceOf produced the same composite shape")
		}
		if sdk.SliceOf(elem).Shape == sdk.ArrayOf(elem, 4).Shape {
			t.Error("SliceOf and ArrayOf produced the same composite shape")
		}
	})

	t.Run("EmitAnonReturns builds emit-side returns", func(t *testing.T) {
		t.Parallel()
		// Pinned against the source-side [sdk.AnonReturns], which
		// takes and returns different types under the bare name.
		var got []*emit.Return = sdk.EmitAnonReturns(sdk.Builtin("error"))
		if len(got) != 1 {
			t.Fatalf("EmitAnonReturns returned %d entries, want 1", len(got))
		}
	})
}

// TestExprFactoriesReturnEmitExpr pins the expression family. The
// slice literal is the identity assertion — every element must be
// assignable to *emit.Expr — and the nil sweep catches a factory
// re-export that resolved to something that builds nothing.
func TestExprFactoriesReturnEmitExpr(t *testing.T) {
	t.Parallel()

	ident := sdk.NewIdent("x")
	typ := sdk.Builtin("string")

	built := map[string]*emit.Expr{
		"NewIdent":          ident,
		"NewExternal":       sdk.NewExternal("fmt", "Println"),
		"NewField":          sdk.NewField(ident, "Name"),
		"NewIndex":          sdk.NewIndex(ident, sdk.NewLiteralInt(0)),
		"NewSlice":          sdk.NewSlice(ident, nil, nil, nil),
		"NewCall":           sdk.NewCall(ident),
		"NewCallGeneric":    sdk.NewCallGeneric(ident, []sdk.Ref{typ}),
		"NewMethodCall":     sdk.NewMethodCall(ident, "Do"),
		"NewAddr":           sdk.NewAddr(ident),
		"NewDeref":          sdk.NewDeref(ident),
		"NewParen":          sdk.NewParen(ident),
		"NewUnary":          sdk.NewUnary("!", ident),
		"NewBinary":         sdk.NewBinary(ident, "==", ident),
		"NewTypeAssert":     sdk.NewTypeAssert(ident, typ),
		"NewComposite":      sdk.NewComposite(typ, nil),
		"NewCompositeKeyed": sdk.NewCompositeKeyed(typ, []string{"N"}, []*sdk.Expr{ident}),
		"NewFuncLit":        sdk.NewFuncLit(nil, nil, nil),
		"NewMake":           sdk.NewMake(sdk.SliceOf(typ)),
		"NewNew":            sdk.NewNew(typ),
		"NewRawExpr":        sdk.NewRawExpr("raw"),
		"NewLiteralString":  sdk.NewLiteralString("s"),
		"NewLiteralInt":     sdk.NewLiteralInt(1),
		"NewLiteralUint":    sdk.NewLiteralUint(1),
		"NewLiteralFloat":   sdk.NewLiteralFloat(1),
		"NewLiteralBool":    sdk.NewLiteralBool(true),
		"NewLiteralRune":    sdk.NewLiteralRune("a"),
		"NewLiteralNil":     sdk.NewLiteralNil(),
		"NewLiteralRaw":     sdk.NewLiteralRaw("raw"),
	}

	t.Run("every factory builds an expression", func(t *testing.T) {
		t.Parallel()
		for name, e := range built {
			if e == nil {
				t.Errorf("%s returned nil", name)
			}
		}
	})

	t.Run("the literal factories stay distinct", func(t *testing.T) {
		t.Parallel()
		// One shared return type again: only the literal kind
		// separates a string literal from a raw one, and rendering
		// an int as a raw literal is a generated file that does not
		// compile.
		if sdk.NewLiteralString("1").LitKind == sdk.NewLiteralInt(1).LitKind {
			t.Error("NewLiteralString and NewLiteralInt share a literal kind")
		}
		if sdk.NewLiteralNil().LitKind == sdk.NewLiteralBool(false).LitKind {
			t.Error("NewLiteralNil and NewLiteralBool share a literal kind")
		}
	})
}

// TestStmtFactoriesReturnEmitStmt pins the statement family, on the
// same principle as the expression one.
func TestStmtFactoriesReturnEmitStmt(t *testing.T) {
	t.Parallel()

	ident := sdk.NewIdent("x")
	call := sdk.NewCall(ident)
	inner := sdk.NewExprStmt(call)
	typ := sdk.Builtin("string")

	built := map[string]*emit.Stmt{
		"NewBlock":      sdk.NewBlock(inner),
		"NewExprStmt":   inner,
		"NewAssign":     sdk.NewAssign([]*sdk.Expr{ident}, "=", []*sdk.Expr{ident}),
		"NewVarStmt":    sdk.NewVarStmt("v", typ, nil),
		"NewConstStmt":  sdk.NewConstStmt("c", typ, sdk.NewLiteralString("s")),
		"NewReturn":     sdk.NewReturn(ident),
		"NewIf":         sdk.NewIf(ident, []*sdk.Stmt{inner}),
		"NewIfElse":     sdk.NewIfElse(ident, []*sdk.Stmt{inner}, nil),
		"NewIfInit":     sdk.NewIfInit(inner, ident, []*sdk.Stmt{inner}, nil),
		"NewFor":        sdk.NewFor(ident, []*sdk.Stmt{inner}),
		"NewForFull":    sdk.NewForFull(inner, ident, inner, nil),
		"NewForRange":   sdk.NewForRange("k", "v", ident, nil),
		"NewSwitch":     sdk.NewSwitch(ident, nil),
		"NewSwitchInit": sdk.NewSwitchInit(inner, ident, nil),
		"NewCase":       sdk.NewCase([]*sdk.Expr{ident}, nil),
		"NewDefault":    sdk.NewDefault(nil),
		"NewBreak":      sdk.NewBreak(""),
		"NewContinue":   sdk.NewContinue(""),
		"NewLabel":      sdk.NewLabel("loop", inner),
		"NewDefer":      sdk.NewDefer(call),
		"NewGo":         sdk.NewGo(call),
		"NewRenderStmt": sdk.NewRenderStmt(&sdk.EmitStruct{Name: "Repo"}),
		"NewRawStmt":    sdk.NewRawStmt("raw"),
	}

	t.Run("every factory builds a statement", func(t *testing.T) {
		t.Parallel()
		for name, s := range built {
			if s == nil {
				t.Errorf("%s returned nil", name)
			}
		}
	})

	t.Run("the control-flow factories stay distinct", func(t *testing.T) {
		t.Parallel()
		if sdk.NewBreak("").StmtKind == sdk.NewContinue("").StmtKind {
			t.Error("NewBreak and NewContinue share a statement kind")
		}
		if sdk.NewIf(ident, nil).StmtKind == sdk.NewFor(ident, nil).StmtKind {
			t.Error("NewIf and NewFor share a statement kind")
		}
	})
}
