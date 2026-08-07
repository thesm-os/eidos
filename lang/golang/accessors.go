// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"reflect"
	"strings"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/node"
)

// The typed readers for the `go.*` vocabulary.
//
// Declaring a key and leaving consumers to call Get on it is half a
// contract. A caller then writes the value-plus-ok dance at every
// site, decides for itself what an absent stamp means, and — where
// the dance is awkward — re-derives the fact structurally instead,
// which is how a consumer ends up disagreeing with the frontend
// that stamped it. Each reader below fixes one answer for "the
// stamp is absent" so every consumer gets the same one.
//
// Absent reads as false for every boolean: a fact nobody stamped is
// a fact not in evidence, and a run whose frontend never stamped it
// must not have every type report as an interface.

// metaCarrier is any node exposing a metadata bag.
type metaCarrier interface{ Meta() *meta.Bag }

// bagOf returns n's metadata bag, or nil when n is a typed nil.
//
// Guarded by reflection rather than by `n == nil`: a nil *node.Alias
// stored in this interface is a non-nil interface value, so the
// plain comparison never fires and the method call dereferences the
// nil pointer. Readers are called from templates and per-node loops
// where a nil is a data gap rather than a programming error, and a
// panic there surfaces as a framework fault against the caller's
// plugin.
//
// The nil bag is the empty bag as far as [meta.Key] is concerned,
// so an absent carrier and an absent stamp converge on one answer.
func bagOf(n metaCarrier) *meta.Bag {
	if n == nil {
		return nil
	}
	if v := reflect.ValueOf(n); v.Kind() == reflect.Pointer && v.IsNil() {
		return nil
	}
	return n.Meta()
}

// readBool returns the stamped boolean, false when unstamped.
func readBool(n metaCarrier, k meta.Key[bool]) bool {
	v, _ := k.Get(bagOf(n))
	return v
}

// readString returns the stamped string, empty when unstamped.
func readString(n metaCarrier, k meta.Key[string]) string {
	v, _ := k.Get(bagOf(n))
	return v
}

// IsError reports whether t references the predeclared error
// interface.
//
// Answers as the union documented in query.go: the stamp where a
// frontend supplied one, the unqualified spelling otherwise. The
// stamp alone reads false on every graph no Go frontend produced —
// a fixture, a bridge, a synthesised node — and a generator asking
// which return slot carries the error would then find none.
//
// The spelling half is gated on [node.TypeRef.IsBuiltin], so a
// qualified `mypkg.error` cannot match. What remains is a type
// declared in the package under generation and named `error`, which
// shadows the predeclared identifier; source doing that is
// pathological and every Go classifier in this repository already
// accepts the same risk.
func IsError(t *node.TypeRef) bool {
	return readBool(t, MetaIsError) || IsBuiltinNamed(t, "error")
}

// IsContext reports whether t references context.Context, which is
// what a generator branches on to decide whether a signature
// threads a cancellation.
//
// Union of stamp and spelling, for the reason [IsError] gives. The
// spelling half requires the `context` qualifier, so there is no
// same-package shadowing case here at all.
func IsContext(t *node.TypeRef) bool {
	if readBool(t, MetaIsContext) {
		return true
	}
	return t != nil && t.Name == "Context" && t.Package == "context"
}

// IsStringer reports whether t's type implements fmt.Stringer in
// either its value or its pointer form.
func IsStringer(t *node.TypeRef) bool { return readBool(t, MetaIsStringer) }

// IsComparable reports whether t's type satisfies Go's comparable
// constraint — usable as a map key, usable as a type argument to a
// comparable-bounded parameter.
func IsComparable(t *node.TypeRef) bool { return readBool(t, MetaIsComparable) }

// IsInterface reports whether t's underlying type is an interface,
// type parameters excluded.
//
// A named ref carries a package and an identifier and nothing about
// what they resolve to, so `io.Reader` and `time.Duration` are
// otherwise indistinguishable. A consumer telling a collaborator
// from a plain value reads this rather than resolving names, which
// it cannot do for types outside the loaded packages.
func IsInterface(t *node.TypeRef) bool { return readBool(t, MetaIsInterface) }

// EmbedsInterface reports whether s embeds at least one interface —
// Go's promotion-by-embedding case, where a struct acquires methods
// no field of it declares.
func EmbedsInterface(s *node.Struct) bool { return readBool(s, MetaEmbedsInterface) }

// IsEmptyInterface reports whether i declares no methods and no
// embeds — Go's `any`.
func IsEmptyInterface(i *node.Interface) bool { return readBool(i, MetaIsEmptyInterface) }

// IsConstraintInterface reports whether i declares a type-set entry
// or `~T` term, making it a generic constraint rather than a
// method-set contract.
//
// Prefer this over [node.IsConstraint] where the Go frontend ran:
// the frontend resolved the declaration through the type checker,
// while the model-level answer infers from structure.
func IsConstraintInterface(i *node.Interface) bool {
	return readBool(i, MetaIsConstraintInterface)
}

// ReceiverIsPointer reports whether m is declared on a pointer
// receiver, which decides whether a caller composing a value writes
// `&T{}` or `T{}`.
func ReceiverIsPointer(m *node.Method) bool { return readBool(m, MetaReceiverIsPointer) }

// UnderlyingKind returns the kind of a's underlying type — one of
// "basic", "func", "map", "slice", "array", "pointer", "chan" — or
// empty for a shape the frontend did not classify.
//
// The value names the kind, not the type: an alias to `int` carries
// "basic", not "int".
func UnderlyingKind(a *node.Alias) string { return readString(a, MetaUnderlyingKind) }

// IotaValue returns the numeric value a constant resolves to, and
// whether one was recorded.
//
// Recorded only for values exactly representable as an int64, so
// the second result distinguishes "not an integer constant" from a
// legitimate zero — which is the value an iota block starts at.
func IotaValue(n metaCarrier) (int, bool) {
	return MetaIotaValue.Get(bagOf(n))
}

// ChanDirection is a Go channel's directionality.
type ChanDirection string

// The directions the Go frontend stamps.
const (
	// ChanBoth is a channel that can be sent to and received from.
	// Only this form is legal in `make`.
	ChanBoth ChanDirection = "both"

	// ChanSend is a send-only channel (`chan<- T`).
	ChanSend ChanDirection = "send"

	// ChanRecv is a receive-only channel (`<-chan T`).
	ChanRecv ChanDirection = "recv"
)

// IsChannel reports whether t models a Go channel.
//
// The node model has no channel variant, so a channel arrives as a
// named reference with this stamped beside it — reading the name
// alone cannot tell it from a user-declared type.
func IsChannel(t *node.TypeRef) bool { return readBool(t, MetaIsChannel) }

// ChanDir returns t's channel direction, or empty when t is not a
// channel.
func ChanDir(t *node.TypeRef) ChanDirection {
	return ChanDirection(readString(t, MetaChanDir))
}

// IsBidirectionalChan reports whether t is a channel that can be
// both sent to and received from.
//
// Matched positively rather than by excluding the directional
// spellings: a caller generally wants this in order to write
// `make(...)`, which is not legal on a directional channel, and a
// direction this does not recognise is likelier to be one `make`
// rejects than one it accepts.
func IsBidirectionalChan(t *node.TypeRef) bool {
	return IsChannel(t) && ChanDir(t) == ChanBoth
}

// ChanElem returns a channel's element type, or nil when t is not a
// channel or carries no element.
//
// Read from the ref's first type argument — the structured view —
// rather than from the printed form the templates-friendly stamp
// carries, so a caller receives something it can lift into an
// [emit.Ref] rather than a string it would have to parse.
func ChanElem(t *node.TypeRef) *node.TypeRef {
	if !IsChannel(t) || len(t.TypeArgs) == 0 {
		return nil
	}
	return t.TypeArgs[0]
}

// Iterator classifies a function's return as a range-over-func
// sequence.
type Iterator string

// The sequence shapes the Go frontend recognises.
const (
	// NotIterator is the zero value, so a projection that never
	// looked reads as "not one" rather than as an unhandled case.
	NotIterator Iterator = ""

	// SeqIterator is `iter.Seq[V]` — a sequence that cannot fail
	// mid-iteration.
	SeqIterator Iterator = "seq"

	// Seq2Iterator is `iter.Seq2[K, V]`, including the
	// `iter.Seq2[V, error]` spelling for a sequence that can.
	Seq2Iterator Iterator = "seq2"
)

// IteratorOf classifies f's return as a range-over-func sequence.
//
// Read from the frontend's stamp rather than matched on package and
// name: a consumer's own two-parameter generic type is not a
// sequence, and a structural match would emit range-over-func
// helpers that do not compile against it.
func IteratorOf(f *node.Function) Iterator {
	switch {
	case readBool(f, MetaIsIterSeq2):
		return Seq2Iterator
	case readBool(f, MetaIsIterSeq):
		return SeqIterator
	default:
		return NotIterator
	}
}

// IterKeyType returns the printed source form of a Seq2's key type,
// or empty for anything else.
func IterKeyType(f *node.Function) string { return readString(f, MetaIterKeyType) }

// IterValueType returns the printed source form of a sequence's
// value type, or empty when f returns none.
func IterValueType(f *node.Function) string { return readString(f, MetaIterValueType) }

// ConstraintTerms returns the disjunctive type-set terms a type
// parameter's constraint declares, or nil when it declares none.
func ConstraintTerms(p *node.TypeParam) []ConstraintTerm {
	terms, _ := MetaConstraintTerms.Get(bagOf(p))
	return terms
}

// GoType returns the rendered Go type expression a bridge stamped
// on t, and whether one was stamped.
//
// The second result matters: an unstamped ref is one the render
// site falls back to the node's own name for, which is not the same
// as a ref a bridge deliberately rendered as the empty string.
func GoType(t *node.TypeRef) (string, bool) {
	return MetaGoType.Get(bagOf(t))
}

// GoName returns the Go-idiomatic identifier a bridge stamped on a
// field or package, and whether one was stamped.
func GoName(n metaCarrier) (string, bool) {
	return MetaGoName.Get(bagOf(n))
}

// GoImport returns the Go import path a bridge stamped on a
// package, and whether one was stamped.
func GoImport(p *node.Package) (string, bool) {
	return MetaGoImport.Get(bagOf(p))
}

// Tag returns the value of one struct-tag entry on f, and whether
// the field carried it.
//
// Tag names are not known until a field is read, so the keys are
// registered dynamically under [MetaTagPrefix]; this resolves the
// same singleton a stamping frontend used.
func Tag(f *node.Field, name string) (string, bool) {
	if name == "" {
		return "", false
	}
	return meta.EnsureKey(MetaTagPrefix+name, meta.StringParser).Get(bagOf(f))
}

// Tags returns every struct-tag entry stamped on f, keyed by tag
// name.
//
// Built by walking the field's own bag rather than by probing a
// known set, because the set is whatever the source declared.
func Tags(f *node.Field) map[string]string {
	bag := bagOf(f)
	if bag == nil {
		return nil
	}
	var out map[string]string
	for _, name := range bag.Names() {
		if !strings.HasPrefix(name, MetaTagPrefix) {
			continue
		}
		raw, ok := bag.RawValue(name)
		if !ok {
			continue
		}
		value, ok := raw.(string)
		if !ok {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[strings.TrimPrefix(name, MetaTagPrefix)] = value
	}
	return out
}
