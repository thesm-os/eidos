// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import (
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/sdk"
)

// The Go questions a plugin asks that no neutral interface answers.
//
// [sdk.SourceRules] answers what every language answers and stays
// narrow on purpose — a method added there is one every language has
// to answer, so a question only Go generators ask does not belong on
// it. These are those questions, forwarded for the reason
// [ReportMethodSet] is: without them a plugin reaches past this façade
// into the language package, which is the import this package exists
// to remove.
//
// A plugin's Go *binding* is a different case and needs none of this.
// A file named for the language it speaks imports `lang/golang`
// directly, which is what the split is for. What these serve is the
// half that names no language in its own file name and still has one
// Go fact to read.
//
// Each forwards and does not reimplement. A second walk here would
// drift from the language package's the moment its rules changed, and
// the drift is invisible: both spellings compile and both answer.

// QName renders a type reference's qualified name — `time.Time` for a
// package-qualified type, the bare name for a builtin.
func QName(t *sdk.TypeRef) string { return golang.QName(t) }

// LocalName drops a qualified name's package qualifier, leaving the
// identifier a generated file in that package would write.
func LocalName(qualified string) string { return golang.LocalName(qualified) }

// Deref strips one pointer, answering t unchanged when it is not one.
func Deref(t *sdk.TypeRef) *sdk.TypeRef { return golang.Deref(t) }

// ElemType projects the inner type of a pointer, slice, array or map
// into the reference a generated declaration carries.
func ElemType(t *sdk.TypeRef) sdk.Ref { return golang.ElemType(t) }

// FromNode lifts a source type reference into its emit counterpart.
func FromNode(t *sdk.TypeRef) sdk.Ref { return golang.FromNode(t) }

// FuncSignature splits a func type into its parameter and result
// references.
func FuncSignature(t *sdk.TypeRef) (params, returns []*sdk.TypeRef) {
	return golang.FuncSignature(t)
}

// IsContext reports whether t names `context.Context`.
func IsContext(t *sdk.TypeRef) bool { return golang.IsContext(t) }

// IsString reports whether t is a string-kinded builtin.
func IsString(t *sdk.TypeRef) bool { return golang.IsString(t) }

// IsInteger reports whether t is an integer-kinded builtin.
func IsInteger(t *sdk.TypeRef) bool { return golang.IsInteger(t) }

// IsFloat reports whether t is a float-kinded builtin.
func IsFloat(t *sdk.TypeRef) bool { return golang.IsFloat(t) }

// Nilable reports whether a value of t may be nil.
func Nilable(t *sdk.TypeRef) bool { return golang.Nilable(t) }

// The comparability walk and the vocabulary it reports through.
//
// The problems are re-exported beside it because a caller that takes
// the walk and then has to name [NotLoaded] to read its answer has
// gained nothing: the import it was spared comes straight back.
type (
	// UnresolvedType names one type a walk could not reach.
	UnresolvedType = golang.UnresolvedType

	// ResolveProblem classifies why.
	ResolveProblem = golang.ResolveProblem
)

// The reasons a walk stops.
const (
	NoResolver   = golang.NoResolver
	NotLoaded    = golang.NotLoaded
	GenericEmbed = golang.GenericEmbed
	TooDeep      = golang.TooDeep
)

// ComparableDeep reports whether values of t may be compared with
// `==`, resolving named types through r.
//
// A non-empty problem list means the answer is smaller than the truth,
// which is the caller's to weigh: a generator emitting an equality
// check on an undetermined type writes one that may not compile.
func ComparableDeep(t *sdk.TypeRef, r sdk.Resolver) (equalable bool, problems []UnresolvedType) {
	return golang.ComparableDeep(t, r)
}

// The range-over-func projection and its classifier, re-exported
// together for the reason the walk above is: reading a [Sequence]'s
// Kind means naming one of these.
type (
	// Sequence is what a method's return says about iteration.
	Sequence = golang.Sequence

	// Iterator classifies the sequence shape.
	Iterator = golang.Iterator
)

// The sequence shapes, with the zero meaning "not one".
const (
	NotIterator  = golang.NotIterator
	SeqIterator  = golang.SeqIterator
	Seq2Iterator = golang.Seq2Iterator
)

// SequenceOf projects what a method's return says about iteration.
func SequenceOf(m *sdk.Method) Sequence { return golang.SequenceOf(m) }

// SentinelSubject strips a sentinel's `Err` prefix, reporting whether
// there was one.
func SentinelSubject(ident string) (subject string, prefixed bool) {
	return golang.SentinelSubject(ident)
}

// NamedReturnsUsable reports whether a generated signature may carry
// the source's return names.
//
// All-or-nothing, and false where any name would collide with an
// identifier the body already binds — which is what `taken` names.
func NamedReturnsUsable(returns []*sdk.Return, taken ...string) bool {
	return golang.NamedReturnsUsable(returns, taken...)
}
