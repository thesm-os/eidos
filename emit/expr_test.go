// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
)

func TestExprKind_String(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		k    emit.ExprKind
		want string
	}{
		{"Literal", emit.ExprLiteral, "literal"},
		{"Ident", emit.ExprIdent, "ident"},
		{"Field", emit.ExprField, "field"},
		{"Index", emit.ExprIndex, "index"},
		{"IndexList", emit.ExprIndexList, "index_list"},
		{"Slice", emit.ExprSlice, "slice"},
		{"Binary", emit.ExprBinary, "binary"},
		{"Unary", emit.ExprUnary, "unary"},
		{"Call", emit.ExprCall, "call"},
		{"MethodCall", emit.ExprMethodCall, "method_call"},
		{"TypeAssert", emit.ExprTypeAssert, "type_assert"},
		{"New", emit.ExprNew, "new"},
		{"Make", emit.ExprMake, "make"},
		{"Composite", emit.ExprComposite, "composite"},
		{"CompositeKeyed", emit.ExprCompositeKeyed, "composite_keyed"},
		{"FuncLit", emit.ExprFuncLit, "func_lit"},
		{"Paren", emit.ExprParen, "paren"},
		{"Deref", emit.ExprDeref, "deref"},
		{"Addr", emit.ExprAddr, "addr"},
		{"Raw", emit.ExprRaw, "raw"},
		{"External", emit.ExprExternal, "external"},
		{"unknown stringifies with a marker", emit.ExprKind(99), "expr_kind(?)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertEqualString(t, tc.k.String(), tc.want)
		})
	}
}

func TestLiteralKind_String(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		k    emit.LiteralKind
		want string
	}{
		{"String", emit.LitString, "string"},
		{"Int", emit.LitInt, "int"},
		{"Uint", emit.LitUint, "uint"},
		{"Float", emit.LitFloat, "float"},
		{"Bool", emit.LitBool, "bool"},
		{"Nil", emit.LitNil, "nil"},
		{"Rune", emit.LitRune, "rune"},
		{"Raw", emit.LitRaw, "raw"},
		{"unknown stringifies with a marker", emit.LiteralKind(99), "literal_kind(?)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertEqualString(t, tc.k.String(), tc.want)
		})
	}
}

func TestExpr_Kind(t *testing.T) {
	t.Parallel()

	t.Run("reports KindExpr regardless of ExprKind", func(t *testing.T) {
		t.Parallel()
		e := emit.NewIdent("x")
		if e.Kind() != emit.KindExpr {
			t.Fatalf("Kind = %s, want %s", e.Kind(), emit.KindExpr)
		}
	})
}

func TestNewLiteralString(t *testing.T) {
	t.Parallel()

	t.Run("captures unquoted text", func(t *testing.T) {
		t.Parallel()
		e := emit.NewLiteralString("hello")
		if e.LitKind != emit.LitString {
			t.Fatalf("LitKind mismatch: %s", e.LitKind)
		}
		assertEqualString(t, e.RawText, "hello")
	})
}

func TestNewLiteralInt(t *testing.T) {
	t.Parallel()

	t.Run("formats int as decimal text", func(t *testing.T) {
		t.Parallel()
		e := emit.NewLiteralInt(-42)
		assertEqualString(t, e.RawText, "-42")
		if e.LitKind != emit.LitInt {
			t.Fatalf("LitKind = %s, want int", e.LitKind)
		}
	})
}

func TestNewLiteralUint(t *testing.T) {
	t.Parallel()

	t.Run("formats uint as decimal text", func(t *testing.T) {
		t.Parallel()
		e := emit.NewLiteralUint(42)
		assertEqualString(t, e.RawText, "42")
	})
}

func TestNewLiteralFloat(t *testing.T) {
	t.Parallel()

	t.Run("formats float as decimal text", func(t *testing.T) {
		t.Parallel()
		e := emit.NewLiteralFloat(3.5)
		assertEqualString(t, e.RawText, "3.5")
	})
}

func TestNewLiteralBool(t *testing.T) {
	t.Parallel()

	t.Run("formats bool as true/false text", func(t *testing.T) {
		t.Parallel()
		assertEqualString(t, emit.NewLiteralBool(true).RawText, "true")
		assertEqualString(t, emit.NewLiteralBool(false).RawText, "false")
	})
}

func TestNewLiteralNil(t *testing.T) {
	t.Parallel()

	t.Run("renders as 'nil'", func(t *testing.T) {
		t.Parallel()
		e := emit.NewLiteralNil()
		assertEqualString(t, e.RawText, "nil")
		if e.LitKind != emit.LitNil {
			t.Fatalf("LitKind = %s, want nil", e.LitKind)
		}
	})
}

func TestNewLiteralRune(t *testing.T) {
	t.Parallel()

	t.Run("captures unquoted character", func(t *testing.T) {
		t.Parallel()
		e := emit.NewLiteralRune("a")
		assertEqualString(t, e.RawText, "a")
		if e.LitKind != emit.LitRune {
			t.Fatalf("LitKind = %s, want rune", e.LitKind)
		}
	})
}

func TestNewLiteralRaw(t *testing.T) {
	t.Parallel()

	t.Run("captures verbatim text", func(t *testing.T) {
		t.Parallel()
		e := emit.NewLiteralRaw("0x1.5p3")
		assertEqualString(t, e.RawText, "0x1.5p3")
		if e.LitKind != emit.LitRaw {
			t.Fatalf("LitKind = %s, want raw", e.LitKind)
		}
	})
}

func TestNewIdent(t *testing.T) {
	t.Parallel()

	t.Run("captures the identifier name", func(t *testing.T) {
		t.Parallel()
		e := emit.NewIdent("x")
		if e.ExprKind != emit.ExprIdent {
			t.Fatalf("ExprKind mismatch: %s", e.ExprKind)
		}
		assertEqualString(t, e.Name, "x")
	})
}

func TestNewExternal(t *testing.T) {
	t.Parallel()

	t.Run("captures package + symbol name in an ExprExternal", func(t *testing.T) {
		t.Parallel()
		e := emit.NewExternal("example.com/audit", "Record")
		if e.ExprKind != emit.ExprExternal {
			t.Fatalf("ExprKind = %s, want ExprExternal", e.ExprKind)
		}
		assertEqualString(t, e.Pkg, "example.com/audit")
		assertEqualString(t, e.Name, "Record")
	})
}

func TestNewField(t *testing.T) {
	t.Parallel()

	t.Run("captures receiver and field name", func(t *testing.T) {
		t.Parallel()
		recv := emit.NewIdent("u")
		e := emit.NewField(recv, "Email")
		if e.ExprKind != emit.ExprField || e.Receiver != recv {
			t.Fatalf("Field construction mismatch: %+v", e)
		}
		assertEqualString(t, e.Name, "Email")
	})
}

func TestNewIndex(t *testing.T) {
	t.Parallel()

	t.Run("captures receiver and index expressions", func(t *testing.T) {
		t.Parallel()
		e := emit.NewIndex(emit.NewIdent("xs"), emit.NewLiteralInt(0))
		if e.ExprKind != emit.ExprIndex || e.Receiver == nil || e.IndexExpr == nil {
			t.Fatalf("Index construction mismatch: %+v", e)
		}
	})
}

func TestNewSlice(t *testing.T) {
	t.Parallel()

	t.Run("captures bounds with optional nils", func(t *testing.T) {
		t.Parallel()
		e := emit.NewSlice(emit.NewIdent("xs"), emit.NewLiteralInt(1), nil, nil)
		if e.ExprKind != emit.ExprSlice || e.Low == nil {
			t.Fatalf("Slice construction mismatch: %+v", e)
		}
		if e.High != nil || e.Max != nil {
			t.Fatalf("nil bounds should remain nil")
		}
	})
}

func TestNewBinary(t *testing.T) {
	t.Parallel()

	t.Run("captures left, op, and right", func(t *testing.T) {
		t.Parallel()
		e := emit.NewBinary(emit.NewIdent("a"), "+", emit.NewIdent("b"))
		if e.ExprKind != emit.ExprBinary {
			t.Fatalf("ExprKind mismatch: %s", e.ExprKind)
		}
		assertEqualString(t, e.Op, "+")
	})
}

func TestNewUnary(t *testing.T) {
	t.Parallel()

	t.Run("captures op and operand", func(t *testing.T) {
		t.Parallel()
		e := emit.NewUnary("!", emit.NewIdent("ok"))
		if e.ExprKind != emit.ExprUnary || e.Receiver == nil {
			t.Fatalf("Unary construction mismatch: %+v", e)
		}
		assertEqualString(t, e.Op, "!")
	})
}

func TestNewCall(t *testing.T) {
	t.Parallel()

	t.Run("captures callee and variadic args", func(t *testing.T) {
		t.Parallel()
		e := emit.NewCall(emit.NewIdent("f"), emit.NewLiteralInt(1), emit.NewLiteralInt(2))
		if e.ExprKind != emit.ExprCall || e.Callee == nil || len(e.Args) != 2 {
			t.Fatalf("Call construction mismatch: %+v", e)
		}
	})
}

func TestNewCallGeneric(t *testing.T) {
	t.Parallel()

	t.Run("captures type args alongside callee and args", func(t *testing.T) {
		t.Parallel()
		e := emit.NewCallGeneric(
			emit.NewIdent("f"),
			[]emit.Ref{builtinRef("int")},
			emit.NewLiteralInt(1),
		)
		if len(e.TypeArgs) != 1 || len(e.Args) != 1 {
			t.Fatalf("CallGeneric mismatch: %+v", e)
		}
	})
}

func TestNewMethodCall(t *testing.T) {
	t.Parallel()

	t.Run("captures receiver, method name, and args", func(t *testing.T) {
		t.Parallel()
		e := emit.NewMethodCall(emit.NewIdent("u"), "Save", emit.NewIdent("ctx"))
		if e.ExprKind != emit.ExprMethodCall || e.Receiver == nil {
			t.Fatalf("MethodCall construction mismatch: %+v", e)
		}
		assertEqualString(t, e.Name, "Save")
		if len(e.Args) != 1 {
			t.Fatalf("MethodCall args mismatch: %+v", e.Args)
		}
	})
}

func TestNewTypeAssert(t *testing.T) {
	t.Parallel()

	t.Run("captures receiver and target type", func(t *testing.T) {
		t.Parallel()
		e := emit.NewTypeAssert(emit.NewIdent("v"), builtinRef("string"))
		if e.ExprKind != emit.ExprTypeAssert || e.Receiver == nil || e.AsType == nil {
			t.Fatalf("TypeAssert construction mismatch: %+v", e)
		}
	})
}

func TestNewNew(t *testing.T) {
	t.Parallel()

	t.Run("captures the type to allocate", func(t *testing.T) {
		t.Parallel()
		e := emit.NewNew(builtinRef("int"))
		if e.ExprKind != emit.ExprNew || e.AsType == nil {
			t.Fatalf("New construction mismatch: %+v", e)
		}
	})
}

func TestNewMake(t *testing.T) {
	t.Parallel()

	t.Run("captures type and size args", func(t *testing.T) {
		t.Parallel()
		e := emit.NewMake(emit.SliceOf(builtinRef("int")), emit.NewLiteralInt(0), emit.NewLiteralInt(8))
		if e.ExprKind != emit.ExprMake || e.AsType == nil || len(e.Args) != 2 {
			t.Fatalf("Make construction mismatch: %+v", e)
		}
	})
}

func TestNewComposite(t *testing.T) {
	t.Parallel()

	t.Run("captures type and positional elements", func(t *testing.T) {
		t.Parallel()
		e := emit.NewComposite(builtinRef("int"), []*emit.Expr{emit.NewLiteralInt(1), emit.NewLiteralInt(2)})
		if e.ExprKind != emit.ExprComposite || len(e.Args) != 2 {
			t.Fatalf("Composite construction mismatch: %+v", e)
		}
	})
}

func TestNewCompositeKeyed(t *testing.T) {
	t.Parallel()

	t.Run("captures keys and parallel values", func(t *testing.T) {
		t.Parallel()
		e := emit.NewCompositeKeyed(
			builtinRef("User"),
			[]string{"Name", "Email"},
			[]*emit.Expr{emit.NewLiteralString("a"), emit.NewLiteralString("b@x")},
		)
		if e.ExprKind != emit.ExprCompositeKeyed || len(e.Keys) != 2 || len(e.Args) != 2 {
			t.Fatalf("CompositeKeyed construction mismatch: %+v", e)
		}
	})
}

func TestNewFuncLit(t *testing.T) {
	t.Parallel()

	t.Run("captures signature and body", func(t *testing.T) {
		t.Parallel()
		e := emit.NewFuncLit(
			[]*emit.Param{{Name: "x", Type: builtinRef("int")}},
			[]emit.Ref{builtinRef("int")},
			[]*emit.Stmt{emit.NewReturn(emit.NewIdent("x"))},
		)
		if e.ExprKind != emit.ExprFuncLit || len(e.FuncParams) != 1 || len(e.FuncReturns) != 1 || len(e.FuncBody) != 1 {
			t.Fatalf("FuncLit construction mismatch: %+v", e)
		}
	})
}

func TestNewParen(t *testing.T) {
	t.Parallel()

	t.Run("wraps inner expression", func(t *testing.T) {
		t.Parallel()
		inner := emit.NewIdent("x")
		e := emit.NewParen(inner)
		if e.ExprKind != emit.ExprParen || e.Receiver != inner {
			t.Fatalf("Paren construction mismatch: %+v", e)
		}
	})
}

func TestNewDeref(t *testing.T) {
	t.Parallel()

	t.Run("captures the dereferenced expression", func(t *testing.T) {
		t.Parallel()
		e := emit.NewDeref(emit.NewIdent("p"))
		if e.ExprKind != emit.ExprDeref {
			t.Fatalf("ExprKind mismatch: %s", e.ExprKind)
		}
	})
}

func TestNewAddr(t *testing.T) {
	t.Parallel()

	t.Run("captures the address-of operand", func(t *testing.T) {
		t.Parallel()
		e := emit.NewAddr(emit.NewIdent("v"))
		if e.ExprKind != emit.ExprAddr {
			t.Fatalf("ExprKind mismatch: %s", e.ExprKind)
		}
	})
}

func TestNewRawExpr(t *testing.T) {
	t.Parallel()

	t.Run("captures verbatim text", func(t *testing.T) {
		t.Parallel()
		e := emit.NewRawExpr("foo + bar")
		if e.ExprKind != emit.ExprRaw {
			t.Fatalf("ExprKind mismatch: %s", e.ExprKind)
		}
		assertEqualString(t, e.RawText, "foo + bar")
	})
}

// TestNewIndexList covers the constructor for a multi-argument
// instantiation.
//
// Beside [emit.NewIndex] rather than folded into it, because
// `T[A][B]` and `T[A, B]` both parse and mean different things — the
// two forms are one field apart if a single constructor carries both.
func TestNewIndexList(t *testing.T) {
	t.Parallel()

	t.Run("carries every index on Args", func(t *testing.T) {
		t.Parallel()
		got := emit.NewIndexList(
			emit.NewIdent("Pair"), emit.NewIdent("string"), emit.NewIdent("int"),
		)
		if got.ExprKind != emit.ExprIndexList {
			t.Errorf("kind = %v, want index_list", got.ExprKind)
		}
		if len(got.Args) != 2 {
			t.Fatalf("Args = %v, want both indexes", got.Args)
		}
		if got.IndexExpr != nil {
			t.Error("the single-index field was populated too; a reader would see both forms")
		}
	})

	t.Run("keeps the receiver it was given", func(t *testing.T) {
		t.Parallel()
		recv := emit.NewIdent("Pair")
		if got := emit.NewIndexList(recv, emit.NewIdent("string")); got.Receiver != recv {
			t.Error("the receiver was copied rather than kept")
		}
	})
}
