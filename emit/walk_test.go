// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit_test

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/core/kind"
	"go.thesmos.sh/eidos/emit"
)

func TestVisitorFunc_Visit(t *testing.T) {
	t.Parallel()

	t.Run("forwards Visit calls to the underlying function", func(t *testing.T) {
		t.Parallel()
		var seen emit.Node
		f := emit.VisitorFunc(func(n emit.Node) emit.Visitor {
			seen = n
			return nil
		})
		s := &emit.Struct{Name: "X"}
		emit.Walk(s, f)
		if seen != s {
			t.Fatalf("VisitorFunc should forward to the wrapped function")
		}
	})
}

func TestWalk_NilGuards(t *testing.T) {
	t.Parallel()

	t.Run("nil node yields no visits", func(t *testing.T) {
		t.Parallel()
		var kinds []kind.Kind
		emit.Walk(nil, recordingVisitor{kinds: &kinds})
		if len(kinds) != 0 {
			t.Fatalf("nil node should not be visited; got %v", kinds)
		}
	})

	t.Run("typed-nil node is treated as nil", func(t *testing.T) {
		t.Parallel()
		var s *emit.Struct
		var kinds []kind.Kind
		emit.Walk(s, recordingVisitor{kinds: &kinds})
		if len(kinds) != 0 {
			t.Fatalf("typed-nil node should not be visited; got %v", kinds)
		}
	})

	t.Run("nil visitor returns immediately", func(t *testing.T) {
		t.Parallel()
		emit.Walk(&emit.Struct{}, nil)
	})
}

func TestWalk_PrunedSubtree(t *testing.T) {
	t.Parallel()

	t.Run("returning nil from Visit stops descent", func(t *testing.T) {
		t.Parallel()
		var visited int
		pruning := emit.VisitorFunc(func(_ emit.Node) emit.Visitor {
			visited++
			return nil
		})
		s := &emit.Struct{
			Fields:  []*emit.Field{{Name: "ID", Type: builtinRef("string")}},
			Methods: []*emit.Method{{Name: "Save"}},
		}
		emit.Walk(s, pruning)
		if visited != 1 {
			t.Fatalf("expected 1 visit (root only); got %d", visited)
		}
	})
}

func TestWalk_PackageDescentOrder(t *testing.T) {
	t.Parallel()

	t.Run("visits files, imports, then declarations of every kind in order", func(t *testing.T) {
		t.Parallel()
		p := &emit.Package{
			Files:      []*emit.File{{Name: "user.go"}},
			Imports:    []*emit.Import{{Path: "context"}},
			Structs:    []*emit.Struct{{Name: "User"}},
			Interfaces: []*emit.Interface{{Name: "Repo"}},
			Functions:  []*emit.Function{{Name: "Open"}},
			Variables:  []*emit.Variable{{Name: "Default"}},
			Constants:  []*emit.Constant{{Name: "Pi"}},
			Enums:      []*emit.Enum{{Name: "Status"}},
			Aliases:    []*emit.Alias{{Name: "ID"}},
		}
		got := recordWalk(p)
		want := []kind.Kind{
			emit.KindPackage,
			emit.KindFile,
			emit.KindImport,
			emit.KindStruct,
			emit.KindInterface,
			emit.KindFunction,
			emit.KindVariable,
			emit.KindConstant,
			emit.KindEnum,
			emit.KindAlias,
		}
		if !slices.Equal(got, want) {
			t.Fatalf("visit order = %v, want %v", got, want)
		}
	})

	t.Run("Package-level slot contributions are visited", func(t *testing.T) {
		t.Parallel()
		p := &emit.Package{}
		assertNoError(t, p.Slot("registry").Append(&emit.Variable{Name: "X"}, emit.Provenance{}))
		got := recordWalk(p)
		if !slices.Contains(got, emit.KindSlot) || !slices.Contains(got, emit.KindVariable) {
			t.Fatalf("expected Slot+Variable in visit list; got %v", got)
		}
	})
}

func TestWalk_FileDescent(t *testing.T) {
	t.Parallel()

	t.Run("File visits Imports and slots", func(t *testing.T) {
		t.Parallel()
		f := &emit.File{Imports: []*emit.Import{{Path: "context"}, {Path: "fmt"}}}
		got := recordWalk(f)
		if got[0] != emit.KindFile {
			t.Fatalf("first visit should be the File itself; got %v", got)
		}
		count := 0
		for _, k := range got {
			if k == emit.KindImport {
				count++
			}
		}
		if count != 2 {
			t.Fatalf("expected 2 imports visited; got %d", count)
		}
	})
}

func TestWalk_StructDescent(t *testing.T) {
	t.Parallel()

	t.Run("visits TypeParams, Fields, Embeds, Methods", func(t *testing.T) {
		t.Parallel()
		s := &emit.Struct{
			TypeParams: []*emit.TypeParam{{Name: "T"}},
			Fields:     []*emit.Field{{Name: "ID", Type: builtinRef("string")}},
			Embeds:     []*emit.Embed{{Type: builtinRef("Base")}},
			Methods:    []*emit.Method{{Name: "Save"}},
		}
		got := recordWalk(s)
		want := []kind.Kind{
			emit.KindStruct, emit.KindTypeParam,
			emit.KindField, emit.KindBuiltinRef,
			emit.KindEmbed, emit.KindBuiltinRef,
			emit.KindMethod,
		}
		if !slices.Equal(got, want) {
			t.Fatalf("visit order = %v, want %v", got, want)
		}
	})
}

func TestWalk_InterfaceDescent(t *testing.T) {
	t.Parallel()

	t.Run("visits TypeParams, Fields, Methods, Embeds", func(t *testing.T) {
		t.Parallel()
		i := &emit.Interface{
			TypeParams: []*emit.TypeParam{{Name: "T"}},
			Fields:     []*emit.Field{{Name: "id", Type: builtinRef("string")}},
			Methods:    []*emit.Method{{Name: "Get"}},
			Embeds:     []*emit.Embed{{Type: builtinRef("Base")}},
		}
		got := recordWalk(i)
		want := []kind.Kind{
			emit.KindInterface, emit.KindTypeParam,
			emit.KindField, emit.KindBuiltinRef,
			emit.KindMethod, emit.KindEmbed, emit.KindBuiltinRef,
		}
		if !slices.Equal(got, want) {
			t.Fatalf("visit order = %v, want %v", got, want)
		}
	})

	t.Run("a field-only interface is walked in full", func(t *testing.T) {
		t.Parallel()
		// A walk that skipped Fields would visit the declaration and
		// nothing inside it, which reads as an empty interface — the
		// shape a TypeScript-emitting generator produces most often.
		i := &emit.Interface{
			Name:   "User",
			Fields: []*emit.Field{{Name: "id", Type: builtinRef("string")}},
		}
		got := recordWalk(i)
		want := []kind.Kind{
			emit.KindInterface, emit.KindField, emit.KindBuiltinRef,
		}
		if !slices.Equal(got, want) {
			t.Fatalf("visit order = %v, want %v", got, want)
		}
	})
}

func TestWalk_MethodDescent(t *testing.T) {
	t.Parallel()

	t.Run("visits TypeParams, Receiver, Params, Returns, Body", func(t *testing.T) {
		t.Parallel()
		m := &emit.Method{
			TypeParams: []*emit.TypeParam{{Name: "T"}},
			Receiver:   builtinRef("Repo"),
			Params:     []*emit.Param{{Name: "x", Type: builtinRef("int")}},
			Returns:    emit.AnonReturns(builtinRef("error")),
			Body:       []*emit.Stmt{emit.NewReturn()},
		}
		got := recordWalk(m)
		if got[0] != emit.KindMethod {
			t.Fatalf("root visit should be Method; got %v", got)
		}
		if !slices.Contains(got, emit.KindStmt) {
			t.Fatalf("Body should be visited; got %v", got)
		}
	})
}

func TestWalk_FunctionDescent(t *testing.T) {
	t.Parallel()

	t.Run("visits TypeParams, Params, Returns, Body", func(t *testing.T) {
		t.Parallel()
		f := &emit.Function{
			TypeParams: []*emit.TypeParam{{Name: "T"}},
			Params:     []*emit.Param{{Name: "x", Type: builtinRef("int")}},
			Returns:    emit.AnonReturns(builtinRef("error")),
			Body:       []*emit.Stmt{emit.NewReturn()},
		}
		got := recordWalk(f)
		if got[0] != emit.KindFunction {
			t.Fatalf("root visit should be Function; got %v", got)
		}
		if !slices.Contains(got, emit.KindStmt) {
			t.Fatalf("Body should be visited; got %v", got)
		}
	})
}

func TestWalk_FieldDescent(t *testing.T) {
	t.Parallel()

	t.Run("Field descends into its Type", func(t *testing.T) {
		t.Parallel()
		f := &emit.Field{Name: "X", Type: builtinRef("int")}
		got := recordWalk(f)
		if !slices.Equal(got, []kind.Kind{emit.KindField, emit.KindBuiltinRef}) {
			t.Fatalf("visit order = %v", got)
		}
	})
}

func TestWalk_EnumDescent(t *testing.T) {
	t.Parallel()

	t.Run("visits Underlying then Variants", func(t *testing.T) {
		t.Parallel()
		e := &emit.Enum{
			Underlying: builtinRef("int"),
			Variants:   []*emit.EnumVariant{{Name: "A"}, {Name: "B"}},
		}
		got := recordWalk(e)
		want := []kind.Kind{
			emit.KindEnum, emit.KindBuiltinRef,
			emit.KindEnumVariant, emit.KindEnumVariant,
		}
		if !slices.Equal(got, want) {
			t.Fatalf("visit order = %v, want %v", got, want)
		}
	})
}

func TestWalk_EnumVariantDescent(t *testing.T) {
	t.Parallel()

	t.Run("EnumVariant descends into its Value expression", func(t *testing.T) {
		t.Parallel()
		v := &emit.EnumVariant{Name: "A", Value: emit.NewLiteralInt(1)}
		got := recordWalk(v)
		if !slices.Equal(got, []kind.Kind{emit.KindEnumVariant, emit.KindExpr}) {
			t.Fatalf("visit order = %v", got)
		}
	})
}

func TestWalk_AliasDescent(t *testing.T) {
	t.Parallel()

	t.Run("visits TypeParams then Target", func(t *testing.T) {
		t.Parallel()
		a := &emit.Alias{
			TypeParams: []*emit.TypeParam{{Name: "T"}},
			Target:     builtinRef("int"),
		}
		got := recordWalk(a)
		want := []kind.Kind{emit.KindAlias, emit.KindTypeParam, emit.KindBuiltinRef}
		if !slices.Equal(got, want) {
			t.Fatalf("visit order = %v, want %v", got, want)
		}
	})
}

func TestWalk_VariableConstantEmbedDescent(t *testing.T) {
	t.Parallel()

	t.Run("Variable visits Type and Init", func(t *testing.T) {
		t.Parallel()
		v := &emit.Variable{Type: builtinRef("int"), Init: emit.NewLiteralInt(0)}
		got := recordWalk(v)
		if !slices.Contains(got, emit.KindBuiltinRef) || !slices.Contains(got, emit.KindExpr) {
			t.Fatalf("Variable should visit Type and Init; got %v", got)
		}
	})

	t.Run("Constant visits Type and Value", func(t *testing.T) {
		t.Parallel()
		c := &emit.Constant{Type: builtinRef("int"), Value: emit.NewLiteralInt(0)}
		got := recordWalk(c)
		if !slices.Contains(got, emit.KindBuiltinRef) || !slices.Contains(got, emit.KindExpr) {
			t.Fatalf("Constant should visit Type and Value; got %v", got)
		}
	})

	t.Run("Embed visits Type", func(t *testing.T) {
		t.Parallel()
		e := &emit.Embed{Type: builtinRef("Base")}
		got := recordWalk(e)
		if !slices.Equal(got, []kind.Kind{emit.KindEmbed, emit.KindBuiltinRef}) {
			t.Fatalf("visit order = %v", got)
		}
	})
}

func TestWalk_ParamAndTypeParamDescent(t *testing.T) {
	t.Parallel()

	t.Run("Param descends into Type", func(t *testing.T) {
		t.Parallel()
		p := &emit.Param{Name: "x", Type: builtinRef("int")}
		got := recordWalk(p)
		if !slices.Equal(got, []kind.Kind{emit.KindParam, emit.KindBuiltinRef}) {
			t.Fatalf("visit order = %v", got)
		}
	})

	t.Run("TypeParam descends into Constraint.Embedded entries", func(t *testing.T) {
		t.Parallel()
		tp := &emit.TypeParam{
			Name:       "T",
			Constraint: constraintFrom(externalRef("fmt", "Stringer"), builtinRef("comparable")),
		}
		got := recordWalk(tp)
		want := []kind.Kind{emit.KindTypeParam, emit.KindExternalRef, emit.KindBuiltinRef}
		if !slices.Equal(got, want) {
			t.Fatalf("visit order = %v, want %v", got, want)
		}
	})

	t.Run("TypeParam without Constraint is a leaf", func(t *testing.T) {
		t.Parallel()
		tp := &emit.TypeParam{Name: "T"}
		got := recordWalk(tp)
		if !slices.Equal(got, []kind.Kind{emit.KindTypeParam}) {
			t.Fatalf("unconstrained TypeParam should be a leaf; got %v", got)
		}
	})
}

func TestWalk_TypeRefAndExternalRef(t *testing.T) {
	t.Parallel()

	t.Run("TypeRef visits TypeArgs but does not follow Target", func(t *testing.T) {
		t.Parallel()
		s := &emit.Struct{Name: "Container"}
		r := emit.Internal(s, builtinRef("int"))
		got := recordWalk(r)
		if got[0] != emit.KindTypeRef {
			t.Fatalf("first visit should be TypeRef; got %v", got)
		}
		if slices.Contains(got, emit.KindStruct) {
			t.Fatalf("TypeRef.Target should not be followed; got %v", got)
		}
		if !slices.Contains(got, emit.KindBuiltinRef) {
			t.Fatalf("TypeArgs should be visited; got %v", got)
		}
	})

	t.Run("ExternalRef visits TypeArgs", func(t *testing.T) {
		t.Parallel()
		r := emit.External("sync", "Map", builtinRef("string"), builtinRef("int"))
		got := recordWalk(r)
		if got[0] != emit.KindExternalRef {
			t.Fatalf("first visit should be ExternalRef; got %v", got)
		}
		count := 0
		for _, k := range got {
			if k == emit.KindBuiltinRef {
				count++
			}
		}
		if count != 2 {
			t.Fatalf("expected 2 type-arg visits; got %d", count)
		}
	})
}

func TestWalk_BuiltinRefIsLeaf(t *testing.T) {
	t.Parallel()

	t.Run("BuiltinRef has no children to visit", func(t *testing.T) {
		t.Parallel()
		r := builtinRef("int")
		got := recordWalk(r)
		if !slices.Equal(got, []kind.Kind{emit.KindBuiltinRef}) {
			t.Fatalf("BuiltinRef should be a leaf; got %v", got)
		}
	})
}

func TestWalk_CompositeRefVariants(t *testing.T) {
	t.Parallel()

	t.Run("Pointer/Slice/Array visit Elem", func(t *testing.T) {
		t.Parallel()
		cases := []struct {
			name string
			ref  *emit.CompositeRef
		}{
			{"Pointer", emit.Ptr(builtinRef("int"))},
			{"Slice", emit.SliceOf(builtinRef("int"))},
			{"Array", emit.ArrayOf(builtinRef("int"), 8)},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				got := recordWalk(tc.ref)
				if !slices.Equal(got, []kind.Kind{emit.KindCompositeRef, emit.KindBuiltinRef}) {
					t.Fatalf("composite Elem should be visited; got %v", got)
				}
			})
		}
	})

	t.Run("Map visits MapKey and MapValue", func(t *testing.T) {
		t.Parallel()
		r := emit.MapOf(builtinRef("string"), builtinRef("int"))
		got := recordWalk(r)
		if len(got) != 3 {
			t.Fatalf("expected 3 visits (root + key + value); got %v", got)
		}
	})

	t.Run("Func visits FuncParams and FuncReturns", func(t *testing.T) {
		t.Parallel()
		r := emit.FuncOf(
			[]emit.Ref{builtinRef("int")},
			[]emit.Ref{builtinRef("error")},
		)
		got := recordWalk(r)
		if len(got) != 3 {
			t.Fatalf("expected 3 visits (root + param + return); got %v", got)
		}
	})

	t.Run("Union visits every term's Type ref", func(t *testing.T) {
		t.Parallel()
		r := emit.Union(
			emit.UnionTerm{Type: builtinRef("int")},
			emit.UnionTerm{Type: builtinRef("string"), Approx: true},
		)
		got := recordWalk(r)
		if len(got) != 3 {
			t.Fatalf("expected 3 visits (root + 2 terms); got %v", got)
		}
	})

	// An inline struct's field and embed types are reachable only
	// here. Anything that walks refs to collect imports — and that is
	// how imports are collected — would miss the `time` in
	// `struct{ T time.Time }` entirely if this descent were absent.
	t.Run("AnonStruct visits every field type", func(t *testing.T) {
		t.Parallel()
		r := emit.AnonStructOf([]emit.AnonField{
			{Name: "A", Type: builtinRef("int")},
			{Name: "B", Type: builtinRef("string")},
		}, nil)
		got := recordWalk(r)
		if len(got) != 3 {
			t.Fatalf("expected 3 visits (root + 2 field types); got %v", got)
		}
	})

	t.Run("AnonStruct visits every embed", func(t *testing.T) {
		t.Parallel()
		r := emit.AnonStructOf(nil, []emit.Ref{builtinRef("error")})
		got := recordWalk(r)
		if len(got) != 2 {
			t.Fatalf("expected 2 visits (root + embed); got %v", got)
		}
	})

	t.Run("an empty AnonStruct is a leaf", func(t *testing.T) {
		t.Parallel()
		got := recordWalk(emit.AnonStructOf(nil, nil))
		if !slices.Equal(got, []kind.Kind{emit.KindCompositeRef}) {
			t.Fatalf("struct{} has nothing below it; got %v", got)
		}
	})
}

func TestWalk_SlotItemsAreVisited(t *testing.T) {
	t.Parallel()

	t.Run("slot items appear in visit list", func(t *testing.T) {
		t.Parallel()
		s := &emit.Struct{Name: "X"}
		assertNoError(t, s.FieldsSlot().Append(&emit.Field{Name: "extra", Type: builtinRef("int")}, emit.Provenance{}))
		got := recordWalk(s)
		if !slices.Contains(got, emit.KindSlot) {
			t.Fatalf("slot itself should be visited; got %v", got)
		}
		if !slices.Contains(got, emit.KindField) {
			t.Fatalf("slot item should be visited; got %v", got)
		}
	})
}

func TestWalk_StmtChildren(t *testing.T) {
	t.Parallel()

	t.Run("If visits init, cond, body, and else", func(t *testing.T) {
		t.Parallel()
		init := emit.NewAssign(
			[]*emit.Expr{emit.NewIdent("x")},
			":=",
			[]*emit.Expr{emit.NewLiteralInt(1)},
		)
		s := emit.NewIfInit(
			init,
			emit.NewBinary(emit.NewIdent("x"), ">", emit.NewLiteralInt(0)),
			[]*emit.Stmt{emit.NewReturn()},
			[]*emit.Stmt{emit.NewReturn(emit.NewLiteralNil())},
		)
		got := recordWalk(s)
		// We expect at least the root + nested stmts/exprs to be visited.
		if got[0] != emit.KindStmt {
			t.Fatalf("root should be Stmt; got %v", got)
		}
		if !slices.Contains(got, emit.KindExpr) {
			t.Fatalf("nested expr should be visited; got %v", got)
		}
	})

	// Import collection and provenance reporting both ride on Walk. A
	// render statement whose wrapped node is skipped would leave that
	// node's external references uncollected, so the file would render
	// a qualifier it never imported.
	t.Run("Render visits the wrapped node", func(t *testing.T) {
		t.Parallel()
		s := emit.NewRenderStmt(&emit.Field{Name: "Wrapped"})
		got := recordWalk(s)
		if got[0] != emit.KindStmt {
			t.Fatalf("root should be Stmt; got %v", got)
		}
		if !slices.Contains(got, emit.KindField) {
			t.Fatalf("wrapped node should be visited; got %v", got)
		}
	})

	t.Run("Render with a nil node descends without panicking", func(t *testing.T) {
		t.Parallel()
		got := recordWalk(emit.NewRenderStmt(nil))
		if len(got) != 1 || got[0] != emit.KindStmt {
			t.Fatalf("only the root should be visited; got %v", got)
		}
	})

	t.Run("ForRange visits RangeOver and body", func(t *testing.T) {
		t.Parallel()
		s := emit.NewForRange("k", "v", emit.NewIdent("xs"), []*emit.Stmt{emit.NewBlock()})
		got := recordWalk(s)
		count := 0
		for _, k := range got {
			if k == emit.KindStmt {
				count++
			}
		}
		if count < 2 {
			t.Fatalf("expected at least 2 Stmt visits (root + body); got %v", got)
		}
	})

	t.Run("Switch visits cond and cases", func(t *testing.T) {
		t.Parallel()
		s := emit.NewSwitch(
			emit.NewIdent("x"),
			[]*emit.Stmt{emit.NewCase([]*emit.Expr{emit.NewLiteralInt(1)}, nil), emit.NewDefault(nil)},
		)
		got := recordWalk(s)
		count := 0
		for _, k := range got {
			if k == emit.KindStmt {
				count++
			}
		}
		if count < 3 {
			t.Fatalf("expected at least 3 Stmt visits (root + 2 cases); got %v", got)
		}
	})

	t.Run("Var stmt visits DeclType and Init", func(t *testing.T) {
		t.Parallel()
		s := emit.NewVarStmt("x", builtinRef("int"), emit.NewLiteralInt(0))
		got := recordWalk(s)
		if !slices.Contains(got, emit.KindBuiltinRef) {
			t.Fatalf("DeclType should be visited; got %v", got)
		}
		if !slices.Contains(got, emit.KindExpr) {
			t.Fatalf("initialiser should be visited; got %v", got)
		}
	})

	t.Run("Label visits Inner statement", func(t *testing.T) {
		t.Parallel()
		s := emit.NewLabel("loop", emit.NewBlock())
		got := recordWalk(s)
		count := 0
		for _, k := range got {
			if k == emit.KindStmt {
				count++
			}
		}
		if count != 2 {
			t.Fatalf("expected root + Inner = 2 Stmt visits; got %v", got)
		}
	})
}

func TestWalk_ExprChildren(t *testing.T) {
	t.Parallel()

	t.Run("Call visits callee and args", func(t *testing.T) {
		t.Parallel()
		e := emit.NewCall(emit.NewIdent("f"), emit.NewLiteralInt(1), emit.NewLiteralInt(2))
		got := recordWalk(e)
		count := 0
		for _, k := range got {
			if k == emit.KindExpr {
				count++
			}
		}
		if count != 4 {
			t.Fatalf("expected root + callee + 2 args = 4 Expr visits; got %v", got)
		}
	})

	t.Run("CallGeneric visits TypeArgs", func(t *testing.T) {
		t.Parallel()
		e := emit.NewCallGeneric(emit.NewIdent("f"), []emit.Ref{builtinRef("int")}, emit.NewLiteralInt(1))
		got := recordWalk(e)
		if !slices.Contains(got, emit.KindBuiltinRef) {
			t.Fatalf("TypeArgs should be visited; got %v", got)
		}
	})

	t.Run("Binary visits Left and Right", func(t *testing.T) {
		t.Parallel()
		e := emit.NewBinary(emit.NewIdent("a"), "+", emit.NewIdent("b"))
		got := recordWalk(e)
		count := 0
		for _, k := range got {
			if k == emit.KindExpr {
				count++
			}
		}
		if count != 3 {
			t.Fatalf("expected root + 2 operands = 3 Expr visits; got %v", got)
		}
	})

	t.Run("Slice visits Low, High, Max", func(t *testing.T) {
		t.Parallel()
		e := emit.NewSlice(
			emit.NewIdent("xs"),
			emit.NewLiteralInt(0),
			emit.NewLiteralInt(8),
			emit.NewLiteralInt(16),
		)
		got := recordWalk(e)
		count := 0
		for _, k := range got {
			if k == emit.KindExpr {
				count++
			}
		}
		if count != 5 {
			t.Fatalf("expected root + receiver + 3 bounds = 5 Expr visits; got %v", got)
		}
	})

	t.Run("Index visits IndexExpr", func(t *testing.T) {
		t.Parallel()
		e := emit.NewIndex(emit.NewIdent("xs"), emit.NewLiteralInt(0))
		got := recordWalk(e)
		count := 0
		for _, k := range got {
			if k == emit.KindExpr {
				count++
			}
		}
		if count != 3 {
			t.Fatalf("expected 3 Expr visits; got %v", got)
		}
	})

	t.Run("New visits AsType", func(t *testing.T) {
		t.Parallel()
		e := emit.NewNew(builtinRef("int"))
		got := recordWalk(e)
		if !slices.Contains(got, emit.KindBuiltinRef) {
			t.Fatalf("AsType should be visited; got %v", got)
		}
	})

	t.Run("FuncLit visits params, returns, body", func(t *testing.T) {
		t.Parallel()
		e := emit.NewFuncLit(
			[]*emit.Param{{Name: "x", Type: builtinRef("int")}},
			[]emit.Ref{builtinRef("error")},
			[]*emit.Stmt{emit.NewReturn(emit.NewLiteralNil())},
		)
		got := recordWalk(e)
		if !slices.Contains(got, emit.KindParam) {
			t.Fatalf("FuncParams should be visited; got %v", got)
		}
		if !slices.Contains(got, emit.KindStmt) {
			t.Fatalf("FuncBody should be visited; got %v", got)
		}
	})
}
