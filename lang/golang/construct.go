// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// Building the emit values a Go generator assembles from a
// signature.
//
// These are the shapes every double, stub and wrapper produces: the
// func type behind a configurable field, the body that delegates to
// it, the compile-time assertion that the result satisfies what it
// stands in for. Each is four or five calls into [emit]'s
// constructors and each is written out again per generator, where
// the copies are free to disagree about the one detail that matters
// — whether a variadic argument carries its ellipsis.
//
// # What is not here
//
// Anything that renders a type to text. `renderType` is bound to
// the backend's render state because it registers the file's
// imports and elides same-package qualifiers, and a free function
// can reproduce neither. Everything here returns an [emit] value
// carrying its references, so the backend resolves them at render
// time and the rendered file's import block is correct by
// construction.

// FuncTypeOf returns the `func(P…) (R…)` type a signature
// describes.
//
// What a configurable double stores per method: a field of this
// type, assigned by a test, invoked by the generated body. Names
// are dropped because a function type carries none — the
// identifiers a body binds are not part of the type.
func FuncTypeOf(s *Sig) emit.Ref {
	if s == nil {
		return emit.FuncOf(nil, nil)
	}
	params := make([]emit.Ref, 0, len(s.Params))
	for i := range s.Params {
		params = append(params, s.Params[i].Type)
	}
	returns := make([]emit.Ref, 0, len(s.Returns))
	for i := range s.Returns {
		returns = append(returns, s.Returns[i].Type)
	}
	return emit.FuncOf(params, returns)
}

// EmitParams lifts a signature's parameters into the [emit.Param]
// list a method declaration takes.
//
// The variadic marker travels with each parameter. Dropping it is
// the failure this exists to prevent: a double of
// `Print(args ...string)` that declares `Print(args string)` takes
// one value where the interface takes many, does not satisfy it,
// and fails to compile at the consumer's assignment rather than
// here.
func EmitParams(s *Sig) []*emit.Param {
	if s == nil {
		return nil
	}
	out := make([]*emit.Param, 0, len(s.Params))
	for i := range s.Params {
		p := &s.Params[i]
		out = append(out, &emit.Param{Name: p.Name, Type: p.Type, Variadic: p.Variadic})
	}
	return out
}

// EmitReturns lifts a signature's return slots into the
// [emit.Return] list a method declaration takes.
//
// Names are carried only when [Sig.NamedReturns] holds, because Go
// requires results to be all named or all anonymous and the emit
// layer rejects the mixed slice. A source signature reaches the
// mixed state legitimately — `(_ User, err error)` is valid Go and
// the blank normalises to unnamed — so the decision is made once,
// for the whole list.
func EmitReturns(s *Sig) []*emit.Return {
	if s == nil {
		return nil
	}
	out := make([]*emit.Return, 0, len(s.Returns))
	for i := range s.Returns {
		r := &s.Returns[i]
		entry := &emit.Return{Type: r.Type}
		if s.NamedReturns {
			entry.Name = r.Name
		}
		out = append(out, entry)
	}
	return out
}

// CallArgs returns the argument expressions that forward a
// signature's parameters to another call.
//
// A variadic tail is spread. Forwarding one without the ellipsis
// passes the slice as a single element, which type-checks against
// `...any` and silently records one argument where the caller
// passed several.
func CallArgs(s *Sig) []*emit.Expr {
	if s == nil {
		return nil
	}
	out := make([]*emit.Expr, 0, len(s.Params))
	for i := range s.Params {
		p := &s.Params[i]
		if p.Variadic {
			// The model has no spread node, and the ellipsis belongs to
			// the call site rather than to the identifier — a raw
			// expression is the only spelling, and it names an
			// identifier the projection already made safe, so nothing
			// unresolved reaches the renderer.
			out = append(out, emit.NewRawExpr(p.Name+"..."))
			continue
		}
		out = append(out, emit.NewIdent(p.Name))
	}
	return out
}

// FieldCall returns `recv.field(args…)` for a signature.
//
// The call behind a func-valued double: the field holds the
// behaviour, the generated method forwards to it.
func FieldCall(recv, field string, s *Sig) *emit.Expr {
	return emit.NewCall(emit.NewField(emit.NewIdent(recv), field), CallArgs(s)...)
}

// MethodCall returns `recv.method(args…)` for a signature.
//
// The delegation form: a wrapper forwarding to the value it wraps,
// where the target is a method rather than a stored function.
func MethodCall(recv, method string, s *Sig) *emit.Expr {
	return emit.NewMethodCall(emit.NewIdent(recv), method, CallArgs(s)...)
}

// DelegateBody returns the statement list of a method that forwards
// to a func-valued field: `[return ]recv.field(args…)`.
//
// A method returning nothing drops the return — `return f()` on a
// void call does not compile — which is the branch every hand-rolled
// copy of this has to remember.
func DelegateBody(recv, field string, s *Sig) []*emit.Stmt {
	call := FieldCall(recv, field, s)
	if !s.HasResults() {
		return []*emit.Stmt{emit.NewExprStmt(call)}
	}
	return []*emit.Stmt{emit.NewReturn(call)}
}

// CaptureAssign returns the statement binding a call's results to
// the signature's capture locals.
//
// `=` when the signature declares named results, since those are
// already declared and `:=` would redeclare them; `:=` otherwise.
// Returns a bare expression statement for a call with no results,
// where there is nothing to bind.
func CaptureAssign(s *Sig, call *emit.Expr) *emit.Stmt {
	if !s.HasResults() {
		return emit.NewExprStmt(call)
	}
	targets := make([]*emit.Expr, 0, len(s.Returns))
	for _, local := range s.Locals() {
		targets = append(targets, emit.NewIdent(local))
	}
	op := ":="
	if s.NamedReturns {
		op = "="
	}
	return emit.NewAssign(targets, op, []*emit.Expr{call})
}

// ReturnLocals returns the statement handing the capture locals
// back to the caller.
//
// Explicit rather than naked, even where the signature names its
// results: a bare `return` in generated code reads as an omission,
// and a reader cannot tell it from a body that forgot to assign.
func ReturnLocals(s *Sig) *emit.Stmt {
	if !s.HasResults() {
		return emit.NewReturn()
	}
	values := make([]*emit.Expr, 0, len(s.Returns))
	for _, local := range s.Locals() {
		values = append(values, emit.NewIdent(local))
	}
	return emit.NewReturn(values...)
}

// RecordCall returns the statement appending a recorded call to its
// slice: `recv.field = append(recv.field, CallType{…})`.
//
// The parameters go in under their recorded-call field names, and
// the returns under theirs when captured is true — which a
// generator sets once it has run [CaptureAssign], and clears when
// it records the arguments before invoking anything.
func RecordCall(recv, field string, callType emit.Ref, s *Sig, captured bool) *emit.Stmt {
	keys := make([]string, 0, len(s.Params)+len(s.Returns))
	vals := make([]*emit.Expr, 0, len(s.Params)+len(s.Returns))
	for i := range s.Params {
		p := &s.Params[i]
		keys = append(keys, p.Field)
		vals = append(vals, emit.NewIdent(p.Name))
	}
	if captured {
		for i := range s.Returns {
			r := &s.Returns[i]
			keys = append(keys, r.Field)
			vals = append(vals, emit.NewIdent(r.Local))
		}
	}
	target := emit.NewField(emit.NewIdent(recv), field)
	appended := emit.NewCall(
		emit.NewIdent("append"),
		target,
		emit.NewCompositeKeyed(callType, keys, vals),
	)
	return emit.NewAssign([]*emit.Expr{target}, "=", []*emit.Expr{appended})
}

// RecordFields returns the field list of the struct one recorded
// call is stored in.
//
// One field per parameter and one per return, under the names the
// projection chose — so the struct a test asserts on and the
// statement that populates it cannot drift.
//
// A variadic parameter's field holds a slice: the method takes many
// values and the record has to keep all of them, which is the one
// place the field's type differs from the parameter's.
func RecordFields(s *Sig) []*emit.Field {
	if s == nil {
		return nil
	}
	out := make([]*emit.Field, 0, len(s.Params)+len(s.Returns))
	for i := range s.Params {
		p := &s.Params[i]
		typ := p.Type
		if p.Variadic {
			typ = emit.SliceOf(typ)
		}
		out = append(out, &emit.Field{Name: p.Field, Type: typ})
	}
	for i := range s.Returns {
		r := &s.Returns[i]
		out = append(out, &emit.Field{Name: r.Field, Type: r.Type})
	}
	return out
}

// SatisfiesAssertion returns the compile-time assertion that a
// generated type implements an interface —
// `var _ Store = (*StoreStub)(nil)`.
//
// Every double wants this and each writes it out again. It earns
// its place beyond the boilerplate: without it, a double missing a
// method fails where a consumer assigns it to the interface, which
// is a file the generator did not write and a line the author did
// not choose. With it, the failure lands in the generated file and
// names the type that is short.
//
// byPointer selects the form. A method set declared on the pointer
// receiver is satisfied by `*T` and not by `T`, so a value-form
// assertion against pointer-receiver methods fails while a
// pointer-form assertion against value-receiver methods does not —
// which makes the pointer form the safe default when a caller is
// unsure.
func SatisfiesAssertion(iface, impl emit.Ref, byPointer bool) *emit.Variable {
	return &emit.Variable{Name: "_", Type: iface, Init: NilOf(impl, byPointer)}
}

// NilOf returns the typed-nil conversion `(*T)(nil)` or `(T)(nil)`.
//
// The idiom a compile-time assertion uses because it allocates
// nothing: the value exists only for the type checker.
//
// Composed rather than written as text so the type travels as a
// reference — the backend registers its import and elides the
// qualifier where the rendered file shares its package, neither of
// which a string can ask for.
func NilOf(impl emit.Ref, byPointer bool) *emit.Expr {
	target := refExpr(impl)
	if byPointer {
		target = emit.NewDeref(target)
	}
	return emit.NewCall(emit.NewParen(target), emit.NewLiteralNil())
}

// refExpr spells a type reference in value position.
//
// [emit.ExprExternal] is the one expression form that registers an
// import, which is what a conversion's target needs. A builtin
// names itself; anything the model cannot reduce to a package and a
// name falls back to a bare identifier, which is correct for a
// same-package reference and visible in the output when it is not.
func refExpr(r emit.Ref) *emit.Expr {
	switch v := r.(type) {
	case *emit.ExternalRef:
		return &emit.Expr{ExprKind: emit.ExprExternal, Pkg: v.Package, Name: v.Name}
	case *emit.BuiltinRef:
		return emit.NewIdent(v.Name)
	default:
		return emit.NewIdent("")
	}
}

// ZeroValueExpr returns the expression spelling a type's zero
// value, and whether one is derivable.
//
// The expression form of [ZeroLiteral], for a generator building an
// emit tree rather than rendering text. The second result carries
// the same meaning: a caller that cannot derive a zero must omit
// the field rather than write `nil` into one that does not accept
// it.
func ZeroValueExpr(t *node.TypeRef) (*emit.Expr, bool) {
	text, ok := ZeroLiteral(t)
	if !ok {
		return nil, false
	}
	switch text {
	case litNil:
		return emit.NewLiteralNil(), true
	case litFalse:
		return emit.NewLiteralBool(false), true
	case litEmpty:
		return emit.NewLiteralString(""), true
	default:
		return emit.NewLiteralInt(0), true
	}
}

// FuncTypeFrom builds a `func(P…) (R…)` type from bare reference
// lists.
//
// The form without a projection behind it, for a caller assembling
// a signature that has no source declaration — a callback a
// generator invents, or one lowered from another generator's
// output.
func FuncTypeFrom(params, returns []emit.Ref) emit.Ref {
	return emit.FuncOf(params, returns)
}
