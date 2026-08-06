// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
)

func TestStmtKind_String(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		k    emit.StmtKind
		want string
	}{
		{"Block", emit.StmtBlock, "block"},
		{"Expr", emit.StmtExpr, "expr"},
		{"Assign", emit.StmtAssign, "assign"},
		{"Return", emit.StmtReturn, "return"},
		{"If", emit.StmtIf, "if"},
		{"For", emit.StmtFor, "for"},
		{"ForRange", emit.StmtForRange, "for_range"},
		{"Switch", emit.StmtSwitch, "switch"},
		{"Case", emit.StmtCase, "case"},
		{"Default", emit.StmtDefault, "default"},
		{"Defer", emit.StmtDefer, "defer"},
		{"Go", emit.StmtGo, "go"},
		{"Break", emit.StmtBreak, "break"},
		{"Continue", emit.StmtContinue, "continue"},
		{"Label", emit.StmtLabel, "label"},
		{"Var", emit.StmtVar, "var"},
		{"Const", emit.StmtConst, "const"},
		{"Raw", emit.StmtRaw, "raw"},
		{"Render", emit.StmtRender, "render"},
		{"unknown stringifies with a marker", emit.StmtKind(99), "stmt_kind(?)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertEqualString(t, tc.k.String(), tc.want)
		})
	}
}

func TestStmt_Kind(t *testing.T) {
	t.Parallel()

	t.Run("reports KindStmt regardless of StmtKind", func(t *testing.T) {
		t.Parallel()
		s := emit.NewBlock()
		if s.Kind() != emit.KindStmt {
			t.Fatalf("Kind = %s, want %s", s.Kind(), emit.KindStmt)
		}
	})
}

func TestNewBlock(t *testing.T) {
	t.Parallel()

	t.Run("captures variadic statements as the block body", func(t *testing.T) {
		t.Parallel()
		body := []*emit.Stmt{emit.NewExprStmt(emit.NewIdent("x"))}
		s := emit.NewBlock(body...)
		if s.StmtKind != emit.StmtBlock || len(s.Block) != 1 {
			t.Fatalf("Block construction mismatch: %+v", s)
		}
	})
}

func TestNewExprStmt(t *testing.T) {
	t.Parallel()

	t.Run("wraps the expression as a bare-expression statement", func(t *testing.T) {
		t.Parallel()
		e := emit.NewIdent("x")
		s := emit.NewExprStmt(e)
		if s.StmtKind != emit.StmtExpr || s.Call != e {
			t.Fatalf("ExprStmt construction mismatch: %+v", s)
		}
	})
}

func TestNewAssign(t *testing.T) {
	t.Parallel()

	t.Run("captures targets, op, and values", func(t *testing.T) {
		t.Parallel()
		s := emit.NewAssign(
			[]*emit.Expr{emit.NewIdent("x")},
			"=",
			[]*emit.Expr{emit.NewLiteralInt(1)},
		)
		if s.StmtKind != emit.StmtAssign || s.Op != "=" {
			t.Fatalf("Assign construction mismatch: %+v", s)
		}
		if len(s.Targets) != 1 || len(s.Values) != 1 {
			t.Fatalf("Assign targets/values mismatch: %+v", s)
		}
	})
}

func TestNewReturn(t *testing.T) {
	t.Parallel()

	t.Run("captures return values", func(t *testing.T) {
		t.Parallel()
		s := emit.NewReturn(emit.NewIdent("err"))
		if s.StmtKind != emit.StmtReturn || len(s.Returns) != 1 {
			t.Fatalf("Return construction mismatch: %+v", s)
		}
	})

	t.Run("supports bare returns", func(t *testing.T) {
		t.Parallel()
		s := emit.NewReturn()
		if s.StmtKind != emit.StmtReturn || len(s.Returns) != 0 {
			t.Fatalf("bare Return should have no values: %+v", s)
		}
	})
}

func TestNewIf(t *testing.T) {
	t.Parallel()

	t.Run("captures cond and then-block", func(t *testing.T) {
		t.Parallel()
		s := emit.NewIf(emit.NewLiteralBool(true), []*emit.Stmt{emit.NewReturn()})
		if s.StmtKind != emit.StmtIf || s.Cond == nil || len(s.Block) != 1 {
			t.Fatalf("If construction mismatch: %+v", s)
		}
		if s.Else != nil || s.Init != nil {
			t.Fatalf("plain If should have no Else or Init")
		}
	})
}

func TestNewIfElse(t *testing.T) {
	t.Parallel()

	t.Run("captures then and else branches", func(t *testing.T) {
		t.Parallel()
		s := emit.NewIfElse(
			emit.NewLiteralBool(true),
			[]*emit.Stmt{emit.NewReturn()},
			[]*emit.Stmt{emit.NewReturn(emit.NewLiteralNil())},
		)
		if len(s.Block) != 1 || len(s.Else) != 1 {
			t.Fatalf("IfElse branches mismatch: %+v", s)
		}
	})
}

func TestNewIfInit(t *testing.T) {
	t.Parallel()

	t.Run("captures init clause along with cond and branches", func(t *testing.T) {
		t.Parallel()
		init := emit.NewAssign(
			[]*emit.Expr{emit.NewIdent("x")},
			":=",
			[]*emit.Expr{emit.NewCall(emit.NewIdent("f"))},
		)
		s := emit.NewIfInit(init, emit.NewLiteralBool(true), nil, nil)
		if s.Init != init {
			t.Fatalf("IfInit should carry init clause")
		}
	})
}

func TestNewFor(t *testing.T) {
	t.Parallel()

	t.Run("captures cond and body", func(t *testing.T) {
		t.Parallel()
		s := emit.NewFor(emit.NewLiteralBool(true), nil)
		if s.StmtKind != emit.StmtFor || s.Cond == nil {
			t.Fatalf("For construction mismatch: %+v", s)
		}
	})

	t.Run("supports infinite for with nil cond", func(t *testing.T) {
		t.Parallel()
		s := emit.NewFor(nil, nil)
		if s.Cond != nil {
			t.Fatalf("infinite For should have nil cond")
		}
	})
}

func TestNewForFull(t *testing.T) {
	t.Parallel()

	t.Run("captures init, cond, post, and body", func(t *testing.T) {
		t.Parallel()
		init := emit.NewAssign(
			[]*emit.Expr{emit.NewIdent("i")},
			":=",
			[]*emit.Expr{emit.NewLiteralInt(0)},
		)
		post := emit.NewAssign(
			[]*emit.Expr{emit.NewIdent("i")},
			"+=",
			[]*emit.Expr{emit.NewLiteralInt(1)},
		)
		s := emit.NewForFull(init, emit.NewLiteralBool(true), post, nil)
		if s.Init != init || s.Post != post {
			t.Fatalf("ForFull should carry init and post")
		}
	})
}

func TestNewForRange(t *testing.T) {
	t.Parallel()

	t.Run("captures range key, value, source, and body", func(t *testing.T) {
		t.Parallel()
		s := emit.NewForRange("k", "v", emit.NewIdent("xs"), nil)
		if s.StmtKind != emit.StmtForRange {
			t.Fatalf("StmtKind mismatch: %s", s.StmtKind)
		}
		assertEqualString(t, s.RangeKey, "k")
		assertEqualString(t, s.RangeValue, "v")
		if s.RangeOver == nil {
			t.Fatalf("RangeOver should be populated")
		}
	})
}

func TestNewSwitch(t *testing.T) {
	t.Parallel()

	t.Run("captures cond and cases", func(t *testing.T) {
		t.Parallel()
		s := emit.NewSwitch(emit.NewIdent("x"), []*emit.Stmt{emit.NewDefault(nil)})
		if s.StmtKind != emit.StmtSwitch || len(s.Cases) != 1 {
			t.Fatalf("Switch construction mismatch: %+v", s)
		}
	})

	t.Run("supports condless switch", func(t *testing.T) {
		t.Parallel()
		s := emit.NewSwitch(nil, nil)
		if s.Cond != nil {
			t.Fatalf("condless Switch should have nil cond")
		}
	})
}

func TestNewSwitchInit(t *testing.T) {
	t.Parallel()

	t.Run("captures init clause", func(t *testing.T) {
		t.Parallel()
		init := emit.NewAssign(
			[]*emit.Expr{emit.NewIdent("x")},
			":=",
			[]*emit.Expr{emit.NewLiteralInt(1)},
		)
		s := emit.NewSwitchInit(init, emit.NewIdent("x"), nil)
		if s.Init != init {
			t.Fatalf("SwitchInit should carry init clause")
		}
	})
}

func TestNewCase(t *testing.T) {
	t.Parallel()

	t.Run("captures case values and body", func(t *testing.T) {
		t.Parallel()
		s := emit.NewCase([]*emit.Expr{emit.NewLiteralInt(1)}, []*emit.Stmt{emit.NewReturn()})
		if s.StmtKind != emit.StmtCase || len(s.Values) != 1 || len(s.Block) != 1 {
			t.Fatalf("Case construction mismatch: %+v", s)
		}
	})
}

func TestNewDefault(t *testing.T) {
	t.Parallel()

	t.Run("captures default body", func(t *testing.T) {
		t.Parallel()
		s := emit.NewDefault([]*emit.Stmt{emit.NewReturn()})
		if s.StmtKind != emit.StmtDefault || len(s.Block) != 1 {
			t.Fatalf("Default construction mismatch: %+v", s)
		}
	})
}

func TestNewDefer(t *testing.T) {
	t.Parallel()

	t.Run("captures the deferred call", func(t *testing.T) {
		t.Parallel()
		call := emit.NewCall(emit.NewIdent("close"))
		s := emit.NewDefer(call)
		if s.StmtKind != emit.StmtDefer || s.Call != call {
			t.Fatalf("Defer construction mismatch: %+v", s)
		}
	})
}

func TestNewGo(t *testing.T) {
	t.Parallel()

	t.Run("captures the go-launched call", func(t *testing.T) {
		t.Parallel()
		call := emit.NewCall(emit.NewIdent("worker"))
		s := emit.NewGo(call)
		if s.StmtKind != emit.StmtGo || s.Call != call {
			t.Fatalf("Go construction mismatch: %+v", s)
		}
	})
}

func TestNewBreak(t *testing.T) {
	t.Parallel()

	t.Run("captures optional label", func(t *testing.T) {
		t.Parallel()
		s := emit.NewBreak("loop")
		if s.StmtKind != emit.StmtBreak {
			t.Fatalf("StmtKind mismatch: %s", s.StmtKind)
		}
		assertEqualString(t, s.Label, "loop")
	})
}

func TestNewContinue(t *testing.T) {
	t.Parallel()

	t.Run("captures optional label", func(t *testing.T) {
		t.Parallel()
		s := emit.NewContinue("outer")
		if s.StmtKind != emit.StmtContinue {
			t.Fatalf("StmtKind mismatch: %s", s.StmtKind)
		}
		assertEqualString(t, s.Label, "outer")
	})
}

func TestNewLabel(t *testing.T) {
	t.Parallel()

	t.Run("wraps inner statement with a label", func(t *testing.T) {
		t.Parallel()
		inner := emit.NewBlock()
		s := emit.NewLabel("loop", inner)
		if s.StmtKind != emit.StmtLabel || s.Inner != inner {
			t.Fatalf("Label construction mismatch: %+v", s)
		}
		assertEqualString(t, s.Label, "loop")
	})
}

func TestNewVarStmt(t *testing.T) {
	t.Parallel()

	t.Run("captures name, type, and initialiser", func(t *testing.T) {
		t.Parallel()
		s := emit.NewVarStmt("x", builtinRef("int"), emit.NewLiteralInt(0))
		if s.StmtKind != emit.StmtVar {
			t.Fatalf("StmtKind mismatch: %s", s.StmtKind)
		}
		assertEqualString(t, s.DeclName, "x")
		if s.DeclType == nil || s.Call == nil {
			t.Fatalf("VarStmt should carry declared type and initialiser")
		}
	})
}

func TestNewConstStmt(t *testing.T) {
	t.Parallel()

	t.Run("captures name, type, and value expression", func(t *testing.T) {
		t.Parallel()
		s := emit.NewConstStmt("Pi", builtinRef("float64"), emit.NewLiteralFloat(3.14))
		if s.StmtKind != emit.StmtConst {
			t.Fatalf("StmtKind mismatch: %s", s.StmtKind)
		}
		assertEqualString(t, s.DeclName, "Pi")
	})
}

func TestNewRawStmt(t *testing.T) {
	t.Parallel()

	t.Run("captures verbatim text", func(t *testing.T) {
		t.Parallel()
		s := emit.NewRawStmt("// custom")
		if s.StmtKind != emit.StmtRaw {
			t.Fatalf("StmtKind mismatch: %s", s.StmtKind)
		}
		assertEqualString(t, s.RawText, "// custom")
	})
}

func TestNewRenderStmt(t *testing.T) {
	t.Parallel()

	t.Run("wraps the node under the render variant", func(t *testing.T) {
		t.Parallel()
		node := &emit.Field{Name: "Wrapped"}
		s := emit.NewRenderStmt(node)
		if s.StmtKind != emit.StmtRender {
			t.Fatalf("StmtKind mismatch: %s", s.StmtKind)
		}
		if s.Node != emit.Node(node) {
			t.Fatalf("Node should be the wrapped value; got %#v", s.Node)
		}
	})

	// The wrapper's whole purpose is to satisfy a KindStmt-constrained
	// slot on behalf of a node that cannot. A wrapper reporting its own
	// wrapped kind would be rejected by the very slot it exists to
	// enter.
	t.Run("reports KindStmt, not the wrapped node's kind", func(t *testing.T) {
		t.Parallel()
		s := emit.NewRenderStmt(&emit.Field{Name: "Wrapped"})
		if s.Kind() != emit.KindStmt {
			t.Fatalf("Kind = %s, want %s", s.Kind(), emit.KindStmt)
		}
	})

	t.Run("is accepted by a KindStmt-constrained slot", func(t *testing.T) {
		t.Parallel()
		host := &emit.Method{Name: "ServeHTTP"}
		wrapped := emit.NewRenderStmt(&emit.Field{Name: "Wrapped"})
		if err := host.Prebody().Append(wrapped, emit.Provenance{SetBy: "p"}); err != nil {
			t.Fatalf("prebody should accept a render statement; got %v", err)
		}
	})
}
