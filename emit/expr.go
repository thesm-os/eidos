// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

import (
	"strconv"

	"go.thesmos.sh/eidos/core/kind"
)

// ExprKind discriminates the variant forms an [Expr] can take.
// Add new variants at the end so existing ordinals stay stable.
type ExprKind int

// Expr variants in declaration order.
const (
	// ExprLiteral is a primitive literal value (int, string, bool,
	// nil, …) carried as a rendered string in [Expr.RawText] with
	// the literal sub-kind in [Expr.LitKind].
	ExprLiteral ExprKind = iota
	// ExprIdent is a bare identifier — [Expr.Name] holds the name.
	ExprIdent
	// ExprField is a field/method selector on a receiver
	// ([Expr.Receiver] . [Expr.Name]).
	ExprField
	// ExprIndex is an indexing operation ([Expr.Receiver] [
	// [Expr.IndexExpr] ]).
	ExprIndex
	// ExprIndexList is an indexing operation carrying several indexes
	// ([Expr.Receiver] [ [Expr.Args]... ]) — Go's instantiation of a
	// declaration taking more than one type parameter, `Pair[K, V]`.
	//
	// Distinct from [ExprIndex] rather than a list on it, which is the
	// same split go/ast makes between IndexExpr and IndexListExpr and
	// for the same reason: `T[A][B]` and `T[A, B]` both parse, mean
	// different things, and are one field apart if a single form
	// carries both.
	ExprIndexList
	// ExprSlice is a slice operation ([Expr.Receiver] [
	// [Expr.Low] : [Expr.High] : [Expr.Max] ]). Any of Low / High /
	// Max may be nil to denote default bounds.
	ExprSlice
	// ExprBinary is a binary operator: [Expr.Left] [Expr.Op]
	// [Expr.Right].
	ExprBinary
	// ExprUnary is a unary operator: [Expr.Op] [Expr.Receiver].
	ExprUnary
	// ExprCall is a function call: [Expr.Callee] ( [Expr.Args]... )
	// with optional generic instantiation via [Expr.TypeArgs].
	ExprCall
	// ExprMethodCall is a method call ([Expr.Receiver] . [Expr.Name]
	// ( [Expr.Args]... )) — sugar over (ExprField + ExprCall) that
	// backends can render more idiomatically.
	ExprMethodCall
	// ExprTypeAssert is a type assertion ([Expr.Receiver] . (
	// [Expr.AsType] )).
	ExprTypeAssert
	// ExprNew is the new(T) builtin — [Expr.AsType] holds T.
	ExprNew
	// ExprMake is the make(T, ...) builtin — [Expr.AsType] holds T,
	// [Expr.Args] the size/capacity / channel-buffer arguments.
	ExprMake
	// ExprComposite is a composite literal T{ … }. [Expr.AsType]
	// holds T, [Expr.Args] holds the positional element list (for
	// keyed composite literals see ExprCompositeKeyed).
	ExprComposite
	// ExprCompositeKeyed is a keyed composite literal T{ K: V, … }.
	// [Expr.AsType] holds T, [Expr.Keys] and [Expr.Args] are
	// parallel slices.
	ExprCompositeKeyed
	// ExprFuncLit is an anonymous function literal — [Expr.FuncParams],
	// [Expr.FuncReturns], [Expr.FuncBody].
	ExprFuncLit
	// ExprParen is a parenthesised expression — [Expr.Receiver]
	// holds the inner expression. Backends may render or elide
	// parens based on operator precedence.
	ExprParen
	// ExprDeref is the dereference operator (*x) — [Expr.Receiver]
	// holds x. Distinct from a pointer type [CompositeRef] (which
	// is a Ref, not an Expr).
	ExprDeref
	// ExprAddr is the address-of operator (&x).
	ExprAddr
	// ExprRaw is verbatim text in [Expr.RawText] — the escape hatch
	// for expressions not modelled above.
	ExprRaw
	// ExprExternal references a symbol exported from another
	// package. [Expr.Pkg] holds the package's import path,
	// [Expr.Name] the symbol's identifier. Backends register the
	// import with the file's [emit.File.ImportsSlot] (transparently,
	// via the same import set the typed [emit.ExternalRef] flows
	// through) and render the expression as
	// `<resolved-alias>.<Name>` — the canonical way to refer to a
	// cross-package symbol from a slot-contributed statement or
	// expression.
	ExprExternal
)

// String returns the lower-case textual form of k for diagnostics.
func (k ExprKind) String() string {
	switch k {
	case ExprLiteral:
		return "literal"
	case ExprIdent:
		return "ident"
	case ExprField:
		return "field"
	case ExprIndex:
		return "index"
	case ExprSlice:
		return "slice"
	case ExprBinary:
		return "binary"
	case ExprUnary:
		return "unary"
	case ExprCall:
		return "call"
	case ExprMethodCall:
		return "method_call"
	case ExprTypeAssert:
		return "type_assert"
	case ExprNew:
		return "new"
	case ExprMake:
		return "make"
	case ExprComposite:
		return "composite"
	case ExprCompositeKeyed:
		return "composite_keyed"
	case ExprFuncLit:
		return "func_lit"
	case ExprParen:
		return "paren"
	case ExprDeref:
		return "deref"
	case ExprAddr:
		return "addr"
	case ExprRaw:
		return labelRaw
	case ExprExternal:
		return "external"
	default:
		return "expr_kind(?)"
	}
}

// LiteralKind discriminates the primitive-literal sub-variants of an
// [ExprLiteral] expression. Backends use the kind to render the
// value correctly (quoting strings, suffixing untyped numeric
// literals, etc.).
type LiteralKind int

// LiteralKind values in declaration order.
const (
	// LitString is a string literal — [Expr.RawText] holds the
	// unquoted text; the backend re-quotes for the target language.
	LitString LiteralKind = iota
	// LitInt is an integer literal — [Expr.RawText] holds the
	// decimal text.
	LitInt
	// LitUint is an unsigned integer literal.
	LitUint
	// LitFloat is a floating-point literal.
	LitFloat
	// LitBool is a boolean literal — [Expr.RawText] is "true" or
	// "false".
	LitBool
	// LitNil is the nil literal.
	LitNil
	// LitRune is a rune / character literal — [Expr.RawText] holds
	// the unquoted single character or escape sequence.
	LitRune
	// LitRaw is a literal whose rendered form is verbatim
	// [Expr.RawText]. Use for literals not covered above (complex
	// numbers, language-specific syntactic forms).
	LitRaw
)

// String returns the lower-case textual form of k for diagnostics.
func (k LiteralKind) String() string {
	switch k {
	case LitString:
		return "string"
	case LitInt:
		return "int"
	case LitUint:
		return "uint"
	case LitFloat:
		return "float"
	case LitBool:
		return "bool"
	case LitNil:
		return "nil"
	case LitRune:
		return "rune"
	case LitRaw:
		return labelRaw
	default:
		return "literal_kind(?)"
	}
}

// Expr is a discriminated-union expression node. [Expr.ExprKind]
// determines which fields are meaningful; constructors below
// populate only the relevant fields for each variant.
//
// Templates dispatch on [Expr.ExprKind] (typically via a per-kind
// sub-template) to render the expression in the target language.
type Expr struct {
	BaseEmit

	// ExprKind discriminates the variant.
	ExprKind ExprKind

	// LitKind sub-discriminates [ExprLiteral] expressions.
	LitKind LiteralKind

	// Name is the identifier name for [ExprIdent], the field /
	// method name for [ExprField] / [ExprMethodCall], and the
	// operator for [ExprBinary] / [ExprUnary] (when the backend
	// prefers a name-keyed render path).
	Name string

	// Op is the operator string for [ExprBinary] / [ExprUnary]
	// (e.g., "+", "<", "&&", "!", "-"). Distinct from [Expr.Name]
	// so backends can branch on operator without name collisions.
	Op string

	// Receiver is the left-hand side for [ExprField], [ExprIndex],
	// [ExprSlice], [ExprMethodCall], [ExprTypeAssert], the operand
	// for [ExprUnary] / [ExprDeref] / [ExprAddr], and the inner
	// expression for [ExprParen].
	Receiver *Expr

	// Callee is the called function expression for [ExprCall].
	Callee *Expr

	// Left and Right are the operands for [ExprBinary].
	Left, Right *Expr

	// IndexExpr is the index for [ExprIndex].
	IndexExpr *Expr

	// Low / High / Max are the slice bounds for [ExprSlice]. Any
	// may be nil to denote default bounds.
	Low, High, Max *Expr

	// AsType is the type used by [ExprNew], [ExprMake],
	// [ExprComposite], [ExprCompositeKeyed], [ExprTypeAssert].
	AsType Ref

	// Args holds the positional argument list for [ExprCall] /
	// [ExprMethodCall], the size/capacity arguments for [ExprMake],
	// the index list for [ExprIndexList], and the element list for
	// [ExprComposite] / [ExprCompositeKeyed].
	Args []*Expr

	// Keys holds the parallel-to-Args field-name list for
	// [ExprCompositeKeyed].
	Keys []string

	// TypeArgs holds generic instantiation type arguments for
	// [ExprCall] / [ExprMethodCall] (e.g. Go's `f[int](…)`).
	TypeArgs []Ref

	// FuncParams / FuncReturns / FuncBody are the signature and
	// body for [ExprFuncLit].
	FuncParams  []*Param
	FuncReturns []Ref
	FuncBody    []*Stmt

	// RawText is the rendered text for [ExprLiteral] (interpreted
	// per [Expr.LitKind]) and [ExprRaw].
	RawText string

	// Pkg is the import path for [ExprExternal] — the package the
	// referenced symbol lives in. Backends register the path with
	// the rendered file's import set and resolve it to an alias.
	Pkg string
}

// Kind returns [KindExpr] regardless of ExprKind — [ExprKind]
// discriminates *within* the expression family, not across the
// node hierarchy.
func (*Expr) Kind() kind.Kind { return KindExpr }

// NewLiteralString returns a string-literal expression. The supplied
// text is the unquoted string content; the backend re-quotes
// according to the target language's escape rules.
func NewLiteralString(s string) *Expr {
	return &Expr{ExprKind: ExprLiteral, LitKind: LitString, RawText: s}
}

// NewLiteralInt returns an integer-literal expression rendered as
// the decimal form of i.
func NewLiteralInt(i int64) *Expr {
	return &Expr{ExprKind: ExprLiteral, LitKind: LitInt, RawText: strconv.FormatInt(i, 10)}
}

// NewLiteralUint returns an unsigned-integer-literal expression.
func NewLiteralUint(u uint64) *Expr {
	return &Expr{ExprKind: ExprLiteral, LitKind: LitUint, RawText: strconv.FormatUint(u, 10)}
}

// NewLiteralFloat returns a floating-point-literal expression.
func NewLiteralFloat(f float64) *Expr {
	return &Expr{ExprKind: ExprLiteral, LitKind: LitFloat, RawText: strconv.FormatFloat(f, 'g', -1, 64)}
}

// NewLiteralBool returns a boolean-literal expression.
func NewLiteralBool(b bool) *Expr {
	return &Expr{ExprKind: ExprLiteral, LitKind: LitBool, RawText: strconv.FormatBool(b)}
}

// NewLiteralNil returns the nil literal.
func NewLiteralNil() *Expr {
	return &Expr{ExprKind: ExprLiteral, LitKind: LitNil, RawText: "nil"}
}

// NewLiteralRune returns a rune/character literal. The supplied
// text is the unquoted character or escape sequence.
func NewLiteralRune(r string) *Expr {
	return &Expr{ExprKind: ExprLiteral, LitKind: LitRune, RawText: r}
}

// NewLiteralRaw returns a literal whose rendered form is the
// supplied text verbatim. Use for primitive forms not covered by
// the typed helpers.
func NewLiteralRaw(text string) *Expr {
	return &Expr{ExprKind: ExprLiteral, LitKind: LitRaw, RawText: text}
}

// NewIdent returns an identifier-reference expression.
func NewIdent(name string) *Expr {
	return &Expr{ExprKind: ExprIdent, Name: name}
}

// NewExternal returns a package-qualified symbol-reference
// expression. pkg is the import path of the package the symbol
// lives in; name is the symbol's identifier. Backends register the
// import with the rendered file's import set and emit the
// expression as `<resolved-alias>.<name>` — the canonical way to
// reference a cross-package symbol from a slot-contributed
// statement or expression, equivalent in role to [emit.External]
// (which models the same concept in type position via
// [emit.ExternalRef]).
func NewExternal(pkg, name string) *Expr {
	return &Expr{ExprKind: ExprExternal, Pkg: pkg, Name: name}
}

// NewField returns a field/method selector — receiver.name.
func NewField(receiver *Expr, name string) *Expr {
	return &Expr{ExprKind: ExprField, Receiver: receiver, Name: name}
}

// NewIndex returns an indexing expression — receiver[index].
func NewIndex(receiver, index *Expr) *Expr {
	return &Expr{ExprKind: ExprIndex, Receiver: receiver, IndexExpr: index}
}

// NewSlice returns a slice expression — receiver[low:high:cap].
// Any of low / high / cap may be nil to denote default bounds. The
// `cap` argument populates [Expr.Max] (the optional third index in a
// full-slice expression).
func NewSlice(receiver, low, high, capacity *Expr) *Expr {
	return &Expr{ExprKind: ExprSlice, Receiver: receiver, Low: low, High: high, Max: capacity}
}

// NewBinary returns a binary-operator expression — left op right.
func NewBinary(left *Expr, op string, right *Expr) *Expr {
	return &Expr{ExprKind: ExprBinary, Left: left, Op: op, Right: right}
}

// NewUnary returns a unary-operator expression — op operand.
func NewUnary(op string, operand *Expr) *Expr {
	return &Expr{ExprKind: ExprUnary, Op: op, Receiver: operand}
}

// NewCall returns a function call — callee(args...).
func NewCall(callee *Expr, args ...*Expr) *Expr {
	return &Expr{ExprKind: ExprCall, Callee: callee, Args: args}
}

// NewCallGeneric returns a function call with generic
// instantiation — callee[typeArgs...](args...).
func NewCallGeneric(callee *Expr, typeArgs []Ref, args ...*Expr) *Expr {
	return &Expr{ExprKind: ExprCall, Callee: callee, TypeArgs: typeArgs, Args: args}
}

// NewMethodCall returns a method call — receiver.method(args...).
func NewMethodCall(receiver *Expr, method string, args ...*Expr) *Expr {
	return &Expr{ExprKind: ExprMethodCall, Receiver: receiver, Name: method, Args: args}
}

// NewTypeAssert returns a type-assertion expression —
// receiver.(asType).
func NewTypeAssert(receiver *Expr, asType Ref) *Expr {
	return &Expr{ExprKind: ExprTypeAssert, Receiver: receiver, AsType: asType}
}

// NewNew returns a new(T) expression.
func NewNew(typ Ref) *Expr {
	return &Expr{ExprKind: ExprNew, AsType: typ}
}

// NewMake returns a make(T, args...) expression.
func NewMake(typ Ref, args ...*Expr) *Expr {
	return &Expr{ExprKind: ExprMake, AsType: typ, Args: args}
}

// NewComposite returns a positional composite-literal expression —
// T{elements...}.
func NewComposite(typ Ref, elements []*Expr) *Expr {
	return &Expr{ExprKind: ExprComposite, AsType: typ, Args: elements}
}

// NewCompositeKeyed returns a keyed composite-literal expression —
// T{keys[i]: elements[i], …}. The keys and elements slices must
// have the same length (the constructor does not validate; backends
// rendering keyed composites assume parallelism).
func NewCompositeKeyed(typ Ref, keys []string, elements []*Expr) *Expr {
	return &Expr{ExprKind: ExprCompositeKeyed, AsType: typ, Keys: keys, Args: elements}
}

// NewFuncLit returns an anonymous-function literal expression.
func NewFuncLit(params []*Param, returns []Ref, body []*Stmt) *Expr {
	return &Expr{ExprKind: ExprFuncLit, FuncParams: params, FuncReturns: returns, FuncBody: body}
}

// NewParen returns a parenthesised expression wrapping inner.
func NewParen(inner *Expr) *Expr {
	return &Expr{ExprKind: ExprParen, Receiver: inner}
}

// NewDeref returns a dereference expression — *target.
func NewDeref(target *Expr) *Expr {
	return &Expr{ExprKind: ExprDeref, Receiver: target}
}

// NewAddr returns an address-of expression — &target.
func NewAddr(target *Expr) *Expr {
	return &Expr{ExprKind: ExprAddr, Receiver: target}
}

// NewRawExpr returns a verbatim raw-text expression.
func NewRawExpr(text string) *Expr {
	return &Expr{ExprKind: ExprRaw, RawText: text}
}
