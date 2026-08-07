// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// printSig projects a variadic method — the shape both reference
// generators get wrong today.
func printSig() *golang.Sig {
	return golang.SigOf(&node.Method{
		Name: "Print",
		Params: []*node.Param{
			{Name: "prefix", Type: builtinRef("string")},
			{Name: "args", Type: builtinRef("string"), Variadic: true},
		},
	})
}

func TestFuncTypeOf(t *testing.T) {
	t.Parallel()

	t.Run("builds the func type behind a configurable field", func(t *testing.T) {
		t.Parallel()
		got, ok := golang.FuncTypeOf(golang.SigOf(getMethod())).(*emit.CompositeRef)
		if !ok {
			t.Fatalf("FuncTypeOf = %T, want a composite ref", got)
		}
		if len(got.FuncParams) != 2 || len(got.FuncReturns) != 2 {
			t.Fatalf("FuncTypeOf = %d params, %d returns; want 2, 2",
				len(got.FuncParams), len(got.FuncReturns))
		}
	})

	t.Run("a nil projection yields an empty func type", func(t *testing.T) {
		t.Parallel()
		if golang.FuncTypeOf(nil) == nil {
			t.Fatalf("FuncTypeOf(nil) must still yield a type")
		}
	})
}

func TestEmitParams(t *testing.T) {
	t.Parallel()

	t.Run("carries the variadic marker onto the declaration", func(t *testing.T) {
		t.Parallel()
		// Dropping it declares `Print(args string)`, which takes one
		// value where the interface takes many and fails to compile
		// at the consumer's assignment rather than here.
		got := golang.EmitParams(printSig())
		if len(got) != 2 || !got[1].Variadic {
			t.Fatalf("EmitParams = %+v, want a variadic tail", got)
		}
	})

	t.Run("binds the resolved identifiers", func(t *testing.T) {
		t.Parallel()
		got := golang.EmitParams(golang.SigOf(getMethod()))
		if got[0].Name != "ctx" || got[1].Name != "id" {
			t.Fatalf("EmitParams names = %q, %q", got[0].Name, got[1].Name)
		}
	})
}

func TestEmitReturns(t *testing.T) {
	t.Parallel()

	t.Run("carries names only when the whole list is usable", func(t *testing.T) {
		t.Parallel()
		// Go requires results to be all named or all anonymous, and
		// the emit layer rejects the mixed slice.
		got := golang.EmitReturns(golang.SigOf(getMethod()))
		if got[0].Name != "item" || got[1].Name != "err" {
			t.Fatalf("EmitReturns names = %q, %q; want item, err", got[0].Name, got[1].Name)
		}
	})

	t.Run("drops names when the signature falls back", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Name: "F", Returns: []*node.Return{
			{Type: builtinRef("string")}, {Name: "err", Type: errorRef()},
		}}
		for i, r := range golang.EmitReturns(golang.SigOf(m)) {
			if r.Name != "" {
				t.Fatalf("EmitReturns[%d].Name = %q, want empty", i, r.Name)
			}
		}
	})
}

func TestCallArgs(t *testing.T) {
	t.Parallel()

	t.Run("spreads a variadic tail", func(t *testing.T) {
		t.Parallel()
		// Forwarding without the ellipsis passes the slice as a single
		// element, which type-checks against `...any` and silently
		// records one argument where the caller passed several.
		got := golang.CallArgs(printSig())
		if len(got) != 2 || got[1].RawText != "args..." {
			t.Fatalf("CallArgs = %+v, want the tail spread", got)
		}
	})

	t.Run("a fixed parameter forwards as a bare identifier", func(t *testing.T) {
		t.Parallel()
		got := golang.CallArgs(golang.SigOf(getMethod()))
		if got[0].Name != "ctx" || got[0].ExprKind != emit.ExprIdent {
			t.Fatalf("CallArgs[0] = %+v, want the ident ctx", got[0])
		}
	})
}

func TestDelegateBody(t *testing.T) {
	t.Parallel()

	t.Run("returns the delegate's result", func(t *testing.T) {
		t.Parallel()
		got := golang.DelegateBody("m", "GetFunc", golang.SigOf(getMethod()))
		if len(got) != 1 || got[0].StmtKind != emit.StmtReturn {
			t.Fatalf("DelegateBody = %+v, want a return", got)
		}
	})

	t.Run("a void method drops the return", func(t *testing.T) {
		t.Parallel()
		// `return f()` on a void call does not compile — the branch
		// every hand-rolled copy has to remember.
		got := golang.DelegateBody("m", "CloseFunc", golang.SigOf(&node.Method{Name: "Close"}))
		if len(got) != 1 || got[0].StmtKind != emit.StmtExpr {
			t.Fatalf("DelegateBody = %+v, want a bare call", got)
		}
	})
}

func TestCaptureAssign(t *testing.T) {
	t.Parallel()

	call := emit.NewCall(emit.NewIdent("f"))

	t.Run("assigns to named results rather than redeclaring", func(t *testing.T) {
		t.Parallel()
		// Named results are already declared by the signature; `:=`
		// would redeclare them.
		got := golang.CaptureAssign(golang.SigOf(getMethod()), call)
		if got.Op != "=" {
			t.Fatalf("Op = %q, want =", got.Op)
		}
	})

	t.Run("declares fresh locals for anonymous results", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{Name: "F", Returns: []*node.Return{{Type: builtinRef("string")}}}
		if got := golang.CaptureAssign(golang.SigOf(m), call); got.Op != ":=" {
			t.Fatalf("Op = %q, want :=", got.Op)
		}
	})

	t.Run("a void call binds nothing", func(t *testing.T) {
		t.Parallel()
		got := golang.CaptureAssign(golang.SigOf(&node.Method{Name: "Close"}), call)
		if got.StmtKind != emit.StmtExpr {
			t.Fatalf("CaptureAssign = %+v, want a bare call", got)
		}
	})
}

func TestReturnLocals(t *testing.T) {
	t.Parallel()

	t.Run("returns explicitly even where results are named", func(t *testing.T) {
		t.Parallel()
		// A naked return in generated code reads as an omission, and
		// a reader cannot tell it from a body that forgot to assign.
		got := golang.ReturnLocals(golang.SigOf(getMethod()))
		if len(got.Returns) != 2 {
			t.Fatalf("ReturnLocals = %d values, want 2", len(got.Returns))
		}
	})

	t.Run("a void method returns nothing", func(t *testing.T) {
		t.Parallel()
		got := golang.ReturnLocals(golang.SigOf(&node.Method{Name: "Close"}))
		if len(got.Returns) != 0 {
			t.Fatalf("ReturnLocals = %d values, want 0", len(got.Returns))
		}
	})
}

func TestRecordFields(t *testing.T) {
	t.Parallel()

	t.Run("one field per parameter and per return", func(t *testing.T) {
		t.Parallel()
		got := golang.RecordFields(golang.SigOf(getMethod()))
		if len(got) != 4 {
			t.Fatalf("RecordFields = %d, want 4", len(got))
		}
		if got[1].Name != "ID" || got[2].Name != "Item" {
			t.Fatalf("RecordFields names = %q, %q", got[1].Name, got[2].Name)
		}
	})

	t.Run("a variadic parameter records as a slice", func(t *testing.T) {
		t.Parallel()
		// The method takes many values and the record has to keep all
		// of them — the one place the field's type differs from the
		// parameter's.
		got := golang.RecordFields(printSig())
		if _, ok := got[1].Type.(*emit.CompositeRef); !ok {
			t.Fatalf("variadic field type = %T, want a slice", got[1].Type)
		}
	})
}

func TestRecordCall(t *testing.T) {
	t.Parallel()

	callType := emit.External("example.com/x", "StoreGetCall")

	t.Run("records the arguments", func(t *testing.T) {
		t.Parallel()
		got := golang.RecordCall("s", "GetCalls", callType, golang.SigOf(getMethod()), false)
		lit := got.Values[0].Args[1]
		if len(lit.Keys) != 2 {
			t.Fatalf("recorded keys = %v, want the two parameters", lit.Keys)
		}
	})

	t.Run("records the results once captured", func(t *testing.T) {
		t.Parallel()
		// A generator recording before it invokes anything has no
		// results to store, which is why the caller says.
		got := golang.RecordCall("s", "GetCalls", callType, golang.SigOf(getMethod()), true)
		lit := got.Values[0].Args[1]
		if len(lit.Keys) != 4 {
			t.Fatalf("recorded keys = %v, want parameters and returns", lit.Keys)
		}
	})
}

func TestSatisfiesAssertion(t *testing.T) {
	t.Parallel()

	iface := emit.External("example.com/x", "Store")
	impl := emit.External("example.com/x", "StoreStub")

	t.Run("asserts through the blank identifier", func(t *testing.T) {
		t.Parallel()
		got := golang.SatisfiesAssertion(iface, impl, true)
		if got.Name != "_" || got.Type != iface {
			t.Fatalf("SatisfiesAssertion = %+v, want a blank of the interface type", got)
		}
	})

	t.Run("the impl travels as a reference, not as text", func(t *testing.T) {
		t.Parallel()
		// Text cannot ask for an import; a generator that concatenated
		// the name would emit a file naming a package it never
		// imports.
		inner := golang.NilOf(impl, true).Callee.Receiver.Receiver
		if inner.ExprKind != emit.ExprExternal || inner.Pkg != "example.com/x" {
			t.Fatalf("conversion target = %+v, want an external reference", inner)
		}
	})

	t.Run("the pointer form dereferences", func(t *testing.T) {
		t.Parallel()
		// A method set on the pointer receiver is satisfied by *T and
		// not by T.
		if golang.NilOf(impl, true).Callee.Receiver.ExprKind != emit.ExprDeref {
			t.Fatalf("the pointer form must dereference the impl")
		}
	})

	t.Run("the value form does not", func(t *testing.T) {
		t.Parallel()
		if golang.NilOf(impl, false).Callee.Receiver.ExprKind == emit.ExprDeref {
			t.Fatalf("the value form must not dereference the impl")
		}
	})

	t.Run("converts the untyped nil", func(t *testing.T) {
		t.Parallel()
		got := golang.NilOf(impl, true)
		if got.ExprKind != emit.ExprCall || len(got.Args) != 1 {
			t.Fatalf("NilOf = %+v, want a one-argument conversion", got)
		}
	})
}

func TestZeroValueExpr(t *testing.T) {
	t.Parallel()

	t.Run("spells each derivable zero", func(t *testing.T) {
		t.Parallel()
		for name, want := range map[string]emit.LiteralKind{
			"int":    emit.LitInt,
			"bool":   emit.LitBool,
			"string": emit.LitString,
			"error":  emit.LitNil,
		} {
			got, ok := golang.ZeroValueExpr(builtinRef(name))
			if !ok || got.LitKind != want {
				t.Errorf("ZeroValueExpr(%s) = %v, %v", name, got, ok)
			}
		}
	})

	t.Run("refuses what ZeroLiteral refuses", func(t *testing.T) {
		t.Parallel()
		// A caller that cannot derive a zero must omit the field
		// rather than write nil into one that does not accept it.
		if _, ok := golang.ZeroValueExpr(namedTypeRef("time", "Duration")); ok {
			t.Fatalf("ZeroValueExpr(time.Duration) must report not derivable")
		}
	})
}

func TestConstructNilAndEdges(t *testing.T) {
	t.Parallel()

	t.Run("every builder tolerates a nil projection", func(t *testing.T) {
		t.Parallel()
		var s *golang.Sig
		if golang.EmitParams(s) != nil || golang.EmitReturns(s) != nil || golang.CallArgs(s) != nil {
			t.Fatalf("a nil projection must build nothing")
		}
		if golang.RecordFields(s) != nil {
			t.Fatalf("RecordFields(nil) must build nothing")
		}
	})

	t.Run("a nil parameter entry does not crash the lift", func(t *testing.T) {
		t.Parallel()
		// Projected from a method whose list carries a gap, which a
		// bridge or a hand-built fixture can produce.
		s := golang.SigOf(&node.Method{Name: "F", Params: []*node.Param{nil}, Returns: []*node.Return{nil}})
		if len(golang.EmitParams(s)) != 1 || len(golang.EmitReturns(s)) != 1 {
			t.Fatalf("a nil entry must still project to a slot")
		}
	})

	t.Run("delegates to a method on a held value", func(t *testing.T) {
		t.Parallel()
		// The wrapper form: forwarding to the value being wrapped,
		// where the target is a method rather than a stored function.
		got := golang.MethodCall("inner", "Get", golang.SigOf(getMethod()))
		if got.ExprKind != emit.ExprMethodCall || got.Name != "Get" {
			t.Fatalf("MethodCall = %+v", got)
		}
		if len(got.Args) != 2 {
			t.Fatalf("MethodCall args = %d, want 2", len(got.Args))
		}
	})

	t.Run("a ref the model cannot name falls back visibly", func(t *testing.T) {
		t.Parallel()
		// Correct for a same-package reference, and visible in the
		// output when it is not — rather than a silently wrong package.
		got := golang.NilOf(emit.SliceOf(emit.Builtin("int")), false)
		if got.Callee.Receiver.ExprKind != emit.ExprIdent {
			t.Fatalf("an unnameable ref must fall back to an identifier")
		}
	})

	t.Run("a builtin impl names itself", func(t *testing.T) {
		t.Parallel()
		got := golang.NilOf(emit.Builtin("MyStub"), true)
		if got.Callee.Receiver.Receiver.ExprKind != emit.ExprIdent {
			t.Fatalf("a builtin impl must render as a bare identifier")
		}
	})

	t.Run("builds a func type from bare ref lists", func(t *testing.T) {
		t.Parallel()
		got := golang.FuncTypeFrom(
			[]emit.Ref{emit.Builtin("int")},
			[]emit.Ref{emit.Builtin("error")},
		).(*emit.CompositeRef)
		if len(got.FuncParams) != 1 || len(got.FuncReturns) != 1 {
			t.Fatalf("FuncTypeFrom = %+v", got)
		}
	})
}
