// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// The signature and type queries a Go generator composes before it
// decides what to emit.
//
// # How these answer
//
// Every predicate here answers as the union: the `go.*` stamp where
// a frontend supplied one, the Go spelling otherwise. Neither half
// suffices alone. A stamp-only answer reads false on a graph no Go
// frontend produced — a test fixture, a bridge from another source
// language, a generator's own synthesised node — and a
// spelling-only answer cannot see a fact the frontend derived from
// the type checker.
//
// The spelling half is gated on [node.TypeRef.IsBuiltin], so a
// qualified type cannot match: `mypkg.error` is not the predeclared
// error. The residual false positive is a type declared in the
// package under generation and literally named `error`, which
// shadows the predeclared identifier and is pathological in source
// that compiles.

// Callable returns the parameter and return slices of any callable
// declaration.
//
// Returns `(nil, nil)` for a node that is not callable, so a caller
// can early-return on a length check rather than writing a
// type-assertion ladder before every query.
func Callable(n node.Node) (params []*node.Param, returns []*node.Return) {
	switch v := n.(type) {
	case *node.Function:
		return v.Params, v.Returns
	case *node.Method:
		return v.Params, v.Returns
	default:
		return nil, nil
	}
}

// ReceiverOf returns a method's receiver type, or nil for a
// function or an interface method.
//
// An interface method's receiver is nil by construction — the
// interface names no variable — so a caller distinguishing a
// concrete method from a contract reads this rather than the node
// kind.
func ReceiverOf(n node.Node) *node.TypeRef {
	m, ok := n.(*node.Method)
	if !ok {
		return nil
	}
	return m.Receiver
}

// IsInterfaceMethod reports whether m is declared on an interface
// rather than on a concrete type.
//
// Decided by the absent receiver, which is what the model records.
// The distinction matters wherever a generator emits a body: an
// interface method has none to emit.
func IsInterfaceMethod(m *node.Method) bool {
	return m != nil && m.Receiver == nil
}

// HasContext reports whether the first parameter is
// `context.Context`.
//
// First rather than any: Go's convention places it first and a
// context elsewhere is not the cancellation scope a generator
// threads through.
func HasContext(params []*node.Param) bool {
	return len(params) > 0 && IsContext(params[0].Type)
}

// ContextParam returns the leading context parameter, or nil.
func ContextParam(params []*node.Param) *node.Param {
	if !HasContext(params) {
		return nil
	}
	return params[0]
}

// StripContext returns params without a leading context.
//
// Returns the input unchanged when there is none, so a caller
// classifying on arity applies it unconditionally.
func StripContext(params []*node.Param) []*node.Param {
	if !HasContext(params) {
		return params
	}
	return params[1:]
}

// TrailingVariadic returns the last parameter when it is variadic,
// or nil.
//
// Only the last can be: Go's grammar admits one variadic parameter
// and requires it last, so a caller peeling `...T` off the end
// needs no scan.
func TrailingVariadic(params []*node.Param) *node.Param {
	n := len(params)
	if n == 0 || !params[n-1].Variadic {
		return nil
	}
	return params[n-1]
}

// StripVariadic returns params without a trailing variadic
// parameter, unchanged when there is none.
func StripVariadic(params []*node.Param) []*node.Param {
	if TrailingVariadic(params) == nil {
		return params
	}
	return params[:len(params)-1]
}

// ErrorReturn returns the slot carrying the builtin error, or nil.
//
// The slot rather than the index, for a caller that wants the
// declared name — which is what a generated body binds the value
// to.
func ErrorReturn(returns []*node.Return) *node.Return {
	i := ErrorSlot(returns)
	if i < 0 {
		return nil
	}
	return returns[i]
}

// HasError reports whether the signature returns the builtin error.
//
// The question a classifier asks, beside [ErrorSlot] for the one that
// needs the position and [ErrorReturn] for the one that needs the
// slot. Spelled out because it is asked far more often than either,
// and `ErrorSlot(returns) >= 0` at every such site is a comparison a
// reader has to decode back into the question it answers.
func HasError(returns []*node.Return) bool {
	return ErrorSlot(returns) >= 0
}

// StripError returns every return slot except the first error, in
// declaration order.
//
// Slots rather than types, unlike [StripErrorTypes]: a generator
// deriving identifiers needs the declared names, and a classifier
// that does not can project them away itself.
func StripError(returns []*node.Return) []*node.Return {
	i := ErrorSlot(returns)
	if i < 0 {
		return returns
	}
	out := make([]*node.Return, 0, len(returns)-1)
	out = append(out, returns[:i]...)
	return append(out, returns[i+1:]...)
}

// StripErrorTypes returns the declared types of every return except
// the first error.
//
// The projection a classifier wants: arity and element kinds decide
// which shape a signature is, and a return's binding name has no
// bearing on it.
func StripErrorTypes(returns []*node.Return) []*node.TypeRef {
	return node.ReturnTypes(StripError(returns))
}

// PointerElem returns a pointer's element type, or nil when t is
// not a pointer.
func PointerElem(t *node.TypeRef) *node.TypeRef {
	if t == nil || !t.IsPointer() {
		return nil
	}
	return t.Elem
}

// SliceElem returns a slice's element type, or nil when t is not a
// slice.
//
// Answers for `[]byte` too, unlike [IsSlice], which routes the byte
// slice elsewhere so a template can pick a bytes-flavoured branch.
// A caller asking for the element wants it either way.
func SliceElem(t *node.TypeRef) *node.TypeRef {
	if t == nil || t.TypeKind != node.TypeRefSlice {
		return nil
	}
	return t.Elem
}

// ArrayElem returns an array's element type and declared length, or
// `(nil, 0)` when t is not an array.
//
// The length travels with the element because an array's zero value
// cannot be spelled without it, and a generator emitting one needs
// both or neither.
func ArrayElem(t *node.TypeRef) (elem *node.TypeRef, length int) {
	if t == nil || !t.IsArray() {
		return nil, 0
	}
	return t.Elem, t.ArrayLen
}

// MapKey returns a map's key type, or nil when t is not a map.
func MapKey(t *node.TypeRef) *node.TypeRef {
	if t == nil || !t.IsMap() {
		return nil
	}
	return t.MapKey
}

// MapValue returns a map's value type, or nil when t is not a map.
func MapValue(t *node.TypeRef) *node.TypeRef {
	if t == nil || !t.IsMap() {
		return nil
	}
	return t.MapValue
}

// FuncSignature returns a function type's parameter and return
// types, or `(nil, nil)` when t is not one.
//
// A function type carries no parameter names — the model records
// only types — so a generator emitting a field of this type has
// nothing to bind and needs nothing to.
func FuncSignature(t *node.TypeRef) (params, returns []*node.TypeRef) {
	if t == nil || !t.IsFunc() {
		return nil, nil
	}
	return t.FuncParams, t.FuncReturns
}

// Deref strips every pointer layer from t and returns the type
// underneath.
//
// Iterative rather than single-step: `**T` is legal Go, and a
// caller resolving the named type at the bottom wants the bottom.
// Returns t unchanged when it is not a pointer, and nil for nil.
func Deref(t *node.TypeRef) *node.TypeRef {
	for t != nil && t.IsPointer() {
		t = t.Elem
	}
	return t
}

// IteratorOfType classifies a type reference as a range-over-func
// sequence.
//
// Keyed on the reference rather than on the declaration that
// returns it, which is what lets a caller ask about any slot —
// a field's type, a parameter, one return among several. The
// declaration-keyed [IteratorOf] remains for the common case and
// reads the frontend's stamp.
//
// Matched on the stdlib package path and name: a consumer's own
// two-parameter generic type is not a sequence, and treating one as
// a sequence emits helpers that do not compile.
func IteratorOfType(t *node.TypeRef) Iterator {
	if t == nil || t.Package != "iter" {
		return NotIterator
	}
	switch {
	case t.Name == "Seq" && len(t.TypeArgs) == 1:
		return SeqIterator
	case t.Name == "Seq2" && len(t.TypeArgs) == 2:
		return Seq2Iterator
	default:
		return NotIterator
	}
}

// IteratorElem returns a sequence's element type, or nil when t is
// not a sequence.
//
// For a Seq2 this is the first argument, which is the value a
// caller collects — the second is the key, or the error in the
// failable spelling.
func IteratorElem(t *node.TypeRef) *node.TypeRef {
	if IteratorOfType(t) == NotIterator {
		return nil
	}
	return t.TypeArgs[0]
}

// IteratorSecond returns a Seq2's second type argument, or nil for
// a Seq and for anything that is not a sequence.
func IteratorSecond(t *node.TypeRef) *node.TypeRef {
	if IteratorOfType(t) != Seq2Iterator {
		return nil
	}
	return t.TypeArgs[1]
}

// IteratorYieldsError reports the `iter.Seq2[V, error]` shape.
//
// The one sequence spelling where a generated helper can usefully
// append a terminal failure: the second slot is the failure channel
// rather than a key.
func IteratorYieldsError(t *node.TypeRef) bool {
	return IteratorOfType(t) == Seq2Iterator && IsError(t.TypeArgs[1])
}

// Sequence is a method's range-over-func return, in the form a
// render site uses.
//
// The zero value reads as "not a sequence" — [Sequence.Kind] is
// [NotIterator] and every ref is nil — so a template branching on it
// needs no separate presence flag.
type Sequence struct {
	// Kind classifies the shape, or is [NotIterator] when the method
	// returns none.
	Kind Iterator

	// Elem is the value a caller collects: a Seq's element, or a
	// Seq2's first argument. Nil unless Kind is set.
	Elem emit.Ref

	// Second is a Seq2's second argument — the key, or the error in
	// the failable spelling. Nil for a Seq and for no sequence.
	Second emit.Ref

	// YieldsError reports the `iter.Seq2[V, error]` spelling, the one
	// shape where a generated helper can usefully append a terminal
	// failure rather than a key.
	YieldsError bool

	// Source is the return's own type reference, kept so a caller
	// needing the source-side meta — a stamp, a position — does not
	// have to re-find the return it just asked about.
	Source *node.TypeRef
}

// SequenceOf reports m's sole return as a sequence, or the zero
// [Sequence] when it returns none.
//
// Sole return only, and that is a judgement rather than a
// simplification: a method returning `(iter.Seq[V], error)` is not a
// sequence a helper can generate against, because the helper would
// have to invent a value for the error before it could iterate. Every
// generator that reached for this arrived at the same rule
// independently, which is the argument for stating it once here.
//
// One call replaces the four accessors plus the nil guard around
// them. The guard is load-bearing and easy to omit: [FromNode]
// documents that it propagates nil rather than refusing it, so a
// template that skipped the branch renders a nil ref and fails at the
// backend, naming the file rather than the method.
func SequenceOf(m *node.Method) Sequence {
	if m == nil || len(m.Returns) != 1 || m.Returns[0] == nil {
		return Sequence{}
	}
	t := m.Returns[0].Type
	kind := IteratorOfType(t)
	if kind == NotIterator {
		return Sequence{}
	}
	return Sequence{
		Kind:        kind,
		Elem:        FromNode(IteratorElem(t)),
		Second:      FromNode(IteratorSecond(t)),
		YieldsError: IteratorYieldsError(t),
		Source:      t,
	}
}

// IsBuiltinNamed reports whether t is the unqualified builtin
// spelled want.
//
// The primitive the named predicates below compose. Gated on
// [node.TypeRef.IsBuiltin] so a qualified type of the same name
// cannot match.
func IsBuiltinNamed(t *node.TypeRef, want string) bool {
	return t != nil && t.IsBuiltin() && t.Name == want
}

// IsBool reports whether t is the builtin bool.
func IsBool(t *node.TypeRef) bool { return IsBuiltinNamed(t, typeBool) }

// IsString reports whether t is the builtin string.
func IsString(t *node.TypeRef) bool { return IsBuiltinNamed(t, typeString) }

// IsInteger reports whether t is a builtin integer type, byte and
// rune included.
func IsInteger(t *node.TypeRef) bool {
	if t == nil || !t.IsBuiltin() {
		return false
	}
	_, ok := integerBuiltins[t.Name]
	return ok
}

// IsFloat reports whether t is a builtin floating-point type.
func IsFloat(t *node.TypeRef) bool {
	return IsBuiltinNamed(t, typeFloat32) || IsBuiltinNamed(t, typeFloat64)
}

// IsComplex reports whether t is a builtin complex type.
func IsComplex(t *node.TypeRef) bool {
	return IsBuiltinNamed(t, typeComplex64) || IsBuiltinNamed(t, typeComplex128)
}

// IsNumeric reports whether t is any builtin number.
//
// The union of the three above, which is the set whose zero value
// is the literal 0 — the question a generator writing a composite
// literal actually asks.
func IsNumeric(t *node.TypeRef) bool {
	return IsInteger(t) || IsFloat(t) || IsComplex(t)
}

// IsAny reports whether t is the empty interface, in either
// spelling.
//
// `any` arrives as a builtin named ref and `interface{}` as an
// anonymous interface declaring nothing, so a caller checking one
// spelling misses code written the other way. This is the edge a
// private copy of the predicate gets wrong.
func IsAny(t *node.TypeRef) bool {
	if t == nil {
		return false
	}
	if IsBuiltinNamed(t, typeAny) {
		return true
	}
	return t.IsAnonInterface() && len(t.Methods) == 0 && len(t.Embeds) == 0
}

// IsBlank reports whether name is the blank identifier.
//
// A blank cannot be referenced, so a generator deriving a body from
// a declaration that used one has to supply its own name — which is
// why this is asked rather than assumed.
func IsBlank(name string) bool { return name == "_" }

// Nilable reports whether t's zero value is the nil keyword.
//
// The question behind "may I write nil here": pointers, slices,
// maps, functions, channels and interfaces zero to nil and nothing
// else does. Distinct from comparability — a slice is nilable and
// not comparable, a struct is comparable and not nilable.
func Nilable(t *node.TypeRef) bool {
	if t == nil {
		return false
	}
	switch {
	case t.IsPointer(), t.IsSlice(), t.IsMap(), t.IsFunc(), t.IsAnonInterface():
		return true
	case IsChannel(t):
		return true
	case IsBuiltinNamed(t, typeError), IsAny(t):
		return true
	default:
		return IsInterface(t)
	}
}

// Keyable reports whether t can carry a map key or a
// same-key-same-value claim.
//
// Slices, maps and functions have no equality in Go at all, so "the
// same key" is not expressible for them. An inline interface is
// refused for the adjacent reason: a value behind one may hold a
// dynamic type that is itself uncomparable, and the comparison
// panics at run time rather than failing to compile.
//
// Deliberately broader than Go's own rule, which also rejects a
// struct or array containing an uncomparable field. Resolving that
// needs the declaration, which a caller holding only a reference
// does not have — see the resolver-taking variants for the answer
// that can.
func Keyable(t *node.TypeRef) bool {
	if t == nil {
		return false
	}
	switch {
	case t.IsSlice(), t.IsMap(), t.IsFunc():
		return false
	case t.IsAnonInterface():
		return false
	default:
		return true
	}
}

// integerBuiltins is every builtin whose values are integers.
//
// Listed rather than prefix-matched: `int`/`uint` prefixes admit
// names that are not builtins, and the set is closed by the
// language spec.
//
//nolint:gochecknoglobals // closed language-defined set
var integerBuiltins = map[string]struct{}{
	typeInt: {}, typeInt8: {}, typeInt16: {}, typeInt32: {}, typeInt64: {},
	typeUint: {}, typeUint8: {}, typeUint16: {}, typeUint32: {}, typeUint64: {},
	typeUintptr: {}, typeByte: {}, typeRune: {},
}
