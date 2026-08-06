// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

import (
	"go.thesmos.sh/eidos/core/kind"
)

// labelRaw is the diagnostic name shared by raw escape-hatch variants
// across the [StmtKind], [ExprKind], and [LiteralKind] discriminators.
const labelRaw = "raw"

// StmtKind discriminates the variant forms a [Stmt] can take. The
// set covers the statement shapes common to systems languages
// (Go-shaped today; the model is intentionally generic enough that
// Rust / TypeScript backends can reuse it via per-kind templates).
// Add new variants at the end so existing ordinals stay stable.
type StmtKind int

// Stmt variants in declaration order.
const (
	// StmtBlock is a sequence of statements rendered as a block
	// ({ ... }). The body lives in [Stmt.Block].
	StmtBlock StmtKind = iota
	// StmtExpr is a bare expression statement (e.g., `foo()`). The
	// expression lives in [Stmt.Call].
	StmtExpr
	// StmtAssign is an assignment ([Stmt.Targets] op [Stmt.Values]).
	// [Stmt.Op] is the operator ("=", ":=", "+=", "-=", …).
	StmtAssign
	// StmtReturn is a return with zero or more values in
	// [Stmt.Returns].
	StmtReturn
	// StmtIf is a conditional with optional init, then-block, and
	// else-block in [Stmt.Init], [Stmt.Cond], [Stmt.Block], and
	// [Stmt.Else].
	StmtIf
	// StmtFor is a loop with optional init / cond / post and a body.
	// All-nil init/cond/post renders as `for { ... }`.
	StmtFor
	// StmtForRange is a range-loop with [Stmt.RangeKey],
	// [Stmt.RangeValue], [Stmt.RangeOver], and [Stmt.Block].
	StmtForRange
	// StmtSwitch is a switch on [Stmt.Cond] with [Stmt.Cases]
	// holding [StmtCase] (and at most one [StmtDefault]).
	StmtSwitch
	// StmtCase is a case clause inside a [StmtSwitch] — values
	// in [Stmt.Returns] (reused as the case-value list), body in
	// [Stmt.Block].
	StmtCase
	// StmtDefault is the default clause inside a [StmtSwitch]. Body
	// in [Stmt.Block].
	StmtDefault
	// StmtDefer queues [Stmt.Call] to run at function exit.
	StmtDefer
	// StmtGo launches [Stmt.Call] as a new goroutine.
	StmtGo
	// StmtBreak transfers control out of the enclosing loop /
	// switch. [Stmt.Label] is the optional target label.
	StmtBreak
	// StmtContinue advances to the next iteration of the enclosing
	// loop. [Stmt.Label] is the optional target label.
	StmtContinue
	// StmtLabel is a labelled statement — [Stmt.Label] : [Stmt.Inner].
	StmtLabel
	// StmtVar is a local var declaration with [Stmt.DeclName],
	// optional [Stmt.DeclType], and optional [Stmt.Call] as the
	// initialiser expression.
	StmtVar
	// StmtConst is a local const declaration. Same shape as StmtVar.
	StmtConst
	// StmtRaw is verbatim text in [Stmt.RawText] — the escape hatch
	// for language-specific constructs not modelled above. Use
	// sparingly; the structured kinds give backends more leverage.
	StmtRaw
	// StmtRender defers this statement's spelling to the node in
	// [Stmt.Node], which the backend renders through the template
	// registered for that node's [Node.Kind].
	//
	// It exists for the plugin that wants to contribute into a
	// statement slot — a [Method] or [Function] "prebody" /
	// "postbody" — while rendering through its own template rather
	// than by assembling this union by hand. Those slots are
	// constrained to [KindStmt], so a plugin's own emit kind cannot
	// be appended to them directly; wrapping satisfies the
	// constraint while leaving the spelling to the wrapped node.
	//
	// The wrapper is deliberately thin. Backends type a statement
	// block as []*Stmt end to end, so a slot item has to BE a Stmt;
	// widening that to [Node] would push a runtime type check into
	// the block renderer, which exists precisely to assume its
	// contents are statements.
	StmtRender
)

// String returns the lower-case textual form of k for diagnostics.
func (k StmtKind) String() string {
	switch k {
	case StmtBlock:
		return "block"
	case StmtExpr:
		return "expr"
	case StmtAssign:
		return "assign"
	case StmtReturn:
		return "return"
	case StmtIf:
		return "if"
	case StmtFor:
		return "for"
	case StmtForRange:
		return "for_range"
	case StmtSwitch:
		return "switch"
	case StmtCase:
		return "case"
	case StmtDefault:
		return "default"
	case StmtDefer:
		return "defer"
	case StmtGo:
		return "go"
	case StmtBreak:
		return "break"
	case StmtContinue:
		return "continue"
	case StmtLabel:
		return "label"
	case StmtVar:
		return "var"
	case StmtConst:
		return "const"
	case StmtRaw:
		return labelRaw
	case StmtRender:
		return "render"
	default:
		return "stmt_kind(?)"
	}
}

// Stmt is a discriminated-union statement node. [Stmt.StmtKind]
// determines which fields are meaningful; constructors below
// populate only the relevant fields for each variant.
//
// Templates dispatch on [Stmt.StmtKind] (typically via a per-kind
// sub-template) to render the statement in the target language.
// The model is language-agnostic — the same Stmt value renders
// differently per backend.
type Stmt struct {
	BaseEmit

	// StmtKind discriminates the variant.
	StmtKind StmtKind

	// Block holds the body for compound statements (StmtBlock,
	// StmtIf-then, StmtFor body, StmtForRange body, StmtCase /
	// StmtDefault body, StmtSwitch wrapper body when empty).
	Block []*Stmt

	// Else holds the else-branch for StmtIf. nil when no else
	// branch is present.
	Else []*Stmt

	// Cases holds the case / default clauses inside StmtSwitch.
	Cases []*Stmt

	// Cond is the controlling expression for StmtIf, StmtFor,
	// StmtSwitch.
	Cond *Expr

	// Init is the optional init statement for StmtIf and StmtFor.
	Init *Stmt

	// Post is the optional post statement for StmtFor.
	Post *Stmt

	// RangeKey is the range key variable name for StmtForRange.
	// Empty when the key is unused (Go's `for _, v := range …`).
	RangeKey string

	// RangeValue is the range value variable name for StmtForRange.
	// Empty when the value is unused.
	RangeValue string

	// RangeOver is the range source expression for StmtForRange.
	RangeOver *Expr

	// Op is the operator for StmtAssign ("=", ":=", "+=", …).
	Op string

	// Targets is the LHS for StmtAssign.
	Targets []*Expr

	// Values is the RHS for StmtAssign and the case-value list for
	// StmtCase.
	Values []*Expr

	// Returns is the return-value list for StmtReturn.
	Returns []*Expr

	// Call is the bare expression for StmtExpr, the deferred /
	// go-routine call for StmtDefer / StmtGo, and the initialiser
	// expression for StmtVar / StmtConst.
	Call *Expr

	// Label is the target label for StmtBreak / StmtContinue, and
	// the label name for StmtLabel.
	Label string

	// Inner is the labelled statement under StmtLabel.
	Inner *Stmt

	// DeclName is the declaration identifier for StmtVar / StmtConst.
	DeclName string

	// DeclType is the declared type for StmtVar / StmtConst. May be
	// nil when the type is inferred.
	DeclType Ref

	// RawText is the verbatim text for StmtRaw.
	RawText string

	// Node is the node whose own template renders this statement,
	// for StmtRender. Nil is a plugin error rather than an empty
	// render: a wrapper with nothing to delegate to means the
	// contributing plugin built the value wrong, and reporting that
	// at render time names it, where silently emitting nothing would
	// leave a contribution missing from the output with no signal.
	Node Node
}

// Kind returns [KindStmt] regardless of StmtKind — [StmtKind]
// discriminates *within* the statement family, not across the
// node hierarchy.
func (*Stmt) Kind() kind.Kind { return KindStmt }

// NewBlock returns a [StmtBlock] containing the given statements.
func NewBlock(stmts ...*Stmt) *Stmt {
	return &Stmt{StmtKind: StmtBlock, Block: stmts}
}

// NewExprStmt returns a bare-expression statement wrapping e.
func NewExprStmt(e *Expr) *Stmt {
	return &Stmt{StmtKind: StmtExpr, Call: e}
}

// NewAssign returns an assignment statement with the given LHS,
// operator, and RHS. Use ":=" for short variable declarations and
// "=" / "+=" / etc. for ordinary assignment.
func NewAssign(targets []*Expr, op string, values []*Expr) *Stmt {
	return &Stmt{StmtKind: StmtAssign, Op: op, Targets: targets, Values: values}
}

// NewReturn returns a return statement carrying the given values.
// Zero values renders as a bare `return`.
func NewReturn(values ...*Expr) *Stmt {
	return &Stmt{StmtKind: StmtReturn, Returns: values}
}

// NewIf returns an if statement with the given condition and
// then-block. No else branch, no init clause.
func NewIf(cond *Expr, then []*Stmt) *Stmt {
	return &Stmt{StmtKind: StmtIf, Cond: cond, Block: then}
}

// NewIfElse returns an if/else statement.
func NewIfElse(cond *Expr, then, els []*Stmt) *Stmt {
	return &Stmt{StmtKind: StmtIf, Cond: cond, Block: then, Else: els}
}

// NewIfInit returns an if with an init clause (Go's
// `if x := f(); cond { … }`).
func NewIfInit(init *Stmt, cond *Expr, then, els []*Stmt) *Stmt {
	return &Stmt{StmtKind: StmtIf, Init: init, Cond: cond, Block: then, Else: els}
}

// NewFor returns a `for cond { … }` loop. Pass a nil cond for
// `for { … }`.
func NewFor(cond *Expr, body []*Stmt) *Stmt {
	return &Stmt{StmtKind: StmtFor, Cond: cond, Block: body}
}

// NewForFull returns a `for init; cond; post { … }` loop. Any of
// init / cond / post may be nil.
func NewForFull(init *Stmt, cond *Expr, post *Stmt, body []*Stmt) *Stmt {
	return &Stmt{StmtKind: StmtFor, Init: init, Cond: cond, Post: post, Block: body}
}

// NewForRange returns a range-loop. Pass empty key / value strings
// to omit them (Go's `for _, v := range x` / `for k := range x`).
func NewForRange(key, value string, over *Expr, body []*Stmt) *Stmt {
	return &Stmt{
		StmtKind:   StmtForRange,
		RangeKey:   key,
		RangeValue: value,
		RangeOver:  over,
		Block:      body,
	}
}

// NewSwitch returns a switch on cond with the given case clauses.
// Pass a nil cond for a `switch { case x: … }` form.
func NewSwitch(cond *Expr, cases []*Stmt) *Stmt {
	return &Stmt{StmtKind: StmtSwitch, Cond: cond, Cases: cases}
}

// NewSwitchInit returns a switch with an init clause.
func NewSwitchInit(init *Stmt, cond *Expr, cases []*Stmt) *Stmt {
	return &Stmt{StmtKind: StmtSwitch, Init: init, Cond: cond, Cases: cases}
}

// NewCase returns one case clause inside a switch. values are the
// case-match expressions; body is the case body.
func NewCase(values []*Expr, body []*Stmt) *Stmt {
	return &Stmt{StmtKind: StmtCase, Values: values, Block: body}
}

// NewDefault returns the default clause of a switch.
func NewDefault(body []*Stmt) *Stmt {
	return &Stmt{StmtKind: StmtDefault, Block: body}
}

// NewDefer returns a defer of the call expression.
func NewDefer(call *Expr) *Stmt {
	return &Stmt{StmtKind: StmtDefer, Call: call}
}

// NewGo returns a go-statement launching call as a goroutine.
func NewGo(call *Expr) *Stmt {
	return &Stmt{StmtKind: StmtGo, Call: call}
}

// NewBreak returns a break, optionally targeting label. Empty label
// renders as a bare `break`.
func NewBreak(label string) *Stmt {
	return &Stmt{StmtKind: StmtBreak, Label: label}
}

// NewContinue returns a continue, optionally targeting label.
func NewContinue(label string) *Stmt {
	return &Stmt{StmtKind: StmtContinue, Label: label}
}

// NewLabel wraps inner with the given label name.
func NewLabel(name string, inner *Stmt) *Stmt {
	return &Stmt{StmtKind: StmtLabel, Label: name, Inner: inner}
}

// NewVarStmt returns a local var declaration. Either typ or init
// may be nil — but both nil is invalid (the caller passes a typed
// or initialised var).
func NewVarStmt(name string, typ Ref, init *Expr) *Stmt {
	return &Stmt{StmtKind: StmtVar, DeclName: name, DeclType: typ, Call: init}
}

// NewConstStmt returns a local const declaration.
func NewConstStmt(name string, typ Ref, value *Expr) *Stmt {
	return &Stmt{StmtKind: StmtConst, DeclName: name, DeclType: typ, Call: value}
}

// NewRawStmt returns a verbatim raw-text statement. The text is
// rendered as-is by backends; use sparingly when no structured
// variant fits.
func NewRawStmt(text string) *Stmt {
	return &Stmt{StmtKind: StmtRaw, RawText: text}
}

// NewRenderStmt wraps n as a statement whose spelling comes from n's
// own template, letting a plugin contribute into a [KindStmt]-
// constrained slot without assembling the [Stmt] union by hand.
//
// The usual caller is a cross-cutting plugin appending into a [Method]
// or [Function] "prebody" / "postbody":
//
//	entry := &Entry{Subject: owner + "." + m.Name}
//	_ = c.AppendPrebody(m, NewRenderStmt(entry), EntryID)
//
// n must declare a [Node.Kind] the backend has a template for, and the
// plugin ships that template like any other. Nothing else about the
// contribution changes — provenance, ordering and import collection
// work exactly as they do for a hand-built statement, because the
// wrapped node is walked as part of the graph.
func NewRenderStmt(n Node) *Stmt {
	return &Stmt{StmtKind: StmtRender, Node: n}
}
