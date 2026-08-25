// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package gofixture

import (
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// Named returns a Named [node.TypeRef] with no package — the
// frontend-conventional shape for primitive and in-scope types
// ("int", "string", "any", in-package "User").
func Named(name string) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefNamed, Name: name}
}

// TypeParamRef returns a use-site reference to a generic type
// parameter ([node.TypeRefTypeParam]). The Name carries the
// declaring TypeParam's identifier — e.g. `TypeParamRef("T")` for
// the receiver-side `T` in `func (l *List[T]) Get() T`.
func TypeParamRef(name string) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefTypeParam, Name: name}
}

// AnonStruct returns an inline anonymous-struct type reference
// ([node.TypeRefAnonStruct]) carrying the supplied fields and embeds.
// Each Field's Owner back-pointer is wired to the returned ref so
// consumers walking up from a field locate the anonymous host.
func AnonStruct(fields []*node.Field, embeds []*node.Embed) *node.TypeRef {
	r := &node.TypeRef{TypeKind: node.TypeRefAnonStruct, Fields: fields, Embeds: embeds}
	for _, f := range fields {
		f.Owner = r
	}
	for _, e := range embeds {
		e.Owner = r
	}
	return r
}

// AnonInterface returns an inline anonymous-interface type reference
// ([node.TypeRefAnonInterface]) carrying the supplied methods and
// embeds. Each Method's and Embed's Owner back-pointer is wired to
// the returned ref.
func AnonInterface(methods []*node.Method, embeds []*node.Embed) *node.TypeRef {
	r := &node.TypeRef{TypeKind: node.TypeRefAnonInterface, Methods: methods, Embeds: embeds}
	for _, m := range methods {
		m.Owner = r
	}
	for _, e := range embeds {
		e.Owner = r
	}
	return r
}

// Constraint returns a [node.Constraint] with the supplied refs as
// embedded named bounds — the universal shape across languages
// (interfaces, traits, protocols, the `comparable` predeclared
// identifier). A nil return value reads as "any" via
// [node.Constraint.IsAny].
//
// Leaves [node.Constraint.Raw] empty, which a real frontend never
// does. Reach for [Bound] when the code under test reads Raw.
func Constraint(embeds ...*node.TypeRef) *node.Constraint {
	return &node.Constraint{Embedded: embeds}
}

// Bound returns a constraint carrying its printed source form as well
// as its named bounds — the shape a frontend actually produces.
//
// [node.Constraint] has two fields and [Constraint] populates one. A
// frontend populates both, and consumers split on which they trust:
// the structured walk reads Embedded, while anything reasoning about
// what the author *wrote* — a type-set form like `~int | ~string`,
// which Embedded cannot express at all — reads Raw and treats an
// empty one as "no bound stated". A fixture built with [Constraint]
// therefore models a constraint no source can produce, and a
// derivation keyed on Raw declines it while every structural
// assertion about the fixture passes.
//
// raw is printed verbatim by [Builder.GoSource] when no embeds are
// supplied, so a form naming another package needs that package
// imported — see [Builder.ImportAs].
func Bound(raw string, embeds ...*node.TypeRef) *node.Constraint {
	return &node.Constraint{Raw: raw, Embedded: embeds}
}

// Chan returns the type reference a Go frontend records for `chan T`.
// [RecvChan] and [SendChan] are the directional forms.
//
// [node.TypeRef] has no channel variant, deliberately: a channel is a
// Go concurrency primitive with no counterpart in most targets, so the
// Go frontend models one as a *named* ref in a synthetic `go` package
// — `go`.`chan`, element on TypeArgs[0] — and stamps the real facts
// as `go.isChannel`, `go.chanDir` and `go.chanElem`. The Go backend
// renders channels by reading exactly those keys, and reaches the
// ExternalRef path without them, which emits `import "go"`.
//
// Spelled here rather than left to the caller because a fixture that
// gets it half right is worse than one that cannot express a channel
// at all: a ref claiming `go.isChannel` with no type argument renders
// as an error naming the plugin, and one carrying the structure
// without the stamp renders as `go.chan[T]`, which does not parse.
func Chan(elem *node.TypeRef) *node.TypeRef {
	return chanRef(elem, golang.ChanBoth)
}

// RecvChan returns the receive-only channel reference — `<-chan T`.
func RecvChan(elem *node.TypeRef) *node.TypeRef {
	return chanRef(elem, golang.ChanRecv)
}

// SendChan returns the send-only channel reference — `chan<- T`.
func SendChan(elem *node.TypeRef) *node.TypeRef {
	return chanRef(elem, golang.ChanSend)
}

// chanRef assembles the named ref plus the three stamps, keeping the
// shape in one place so the three directional entry points cannot
// drift from each other or from the frontend.
//
// go.chanElem carries the printed element form, which is the
// templates-friendly view of what TypeArgs[0] holds structurally.
// [golang.TypeStringQualified] is the projection that matches what
// go/types prints for the same element, so a consumer comparing the
// two sees the same string a real run would produce.
func chanRef(elem *node.TypeRef, dir golang.ChanDirection) *node.TypeRef {
	if elem == nil {
		// Test-only fixture; callers expect a panic on misuse. A
		// channel with no element renders as a named error at the
		// backend, attributed to whichever plugin built the ref —
		// which would be a lie about where this one came from.
		panic("gofixture: a channel needs an element type") //nolint:forbidigo
	}
	ref := &node.TypeRef{
		TypeKind: node.TypeRefNamed,
		Package:  chanPkg,
		Name:     chanName,
		TypeArgs: []*node.TypeRef{elem},
	}
	bag := ref.EnsureMeta()
	golang.MetaIsChannel.Set(bag, true, fixtureAuthority)
	golang.MetaChanDir.Set(bag, string(dir), fixtureAuthority)
	golang.MetaChanElem.Set(bag, golang.TypeStringQualified(elem), fixtureAuthority)
	return ref
}

// The synthetic qualified name the Go frontend gives a channel ref.
// Written out here rather than composed, because the two halves land
// in different fields and `go.chan` is the [golang.QName] of the pair
// rather than either one.
const (
	chanPkg  = "go"
	chanName = "chan"
)

// fixtureAuthority names this package as the source of a stamp, so a
// provenance trail through a fixture-built graph says where the value
// came from rather than attributing it to a frontend that never ran.
const fixtureAuthority = "gofixture"

// PkgNamed returns a Named [node.TypeRef] qualified by a package
// path. Use this for cross-package references such as
// `context.Context` or `time.Time`.
func PkgNamed(pkg, name string) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefNamed, Package: pkg, Name: name}
}

// WithArgs returns a copy of named with the supplied generic type
// arguments. named must be a Named ref; calling WithArgs on a
// non-Named ref panics — the shape is invalid for any other variant.
func WithArgs(named *node.TypeRef, args ...*node.TypeRef) *node.TypeRef {
	if named == nil || named.TypeKind != node.TypeRefNamed {
		// Test-only fixture; callers expect a panic on misuse.
		panic("gofixture: WithArgs requires a Named TypeRef") //nolint:forbidigo
	}
	clone := *named
	clone.TypeArgs = append([]*node.TypeRef(nil), args...)
	return &clone
}

// Pointer returns a pointer [node.TypeRef] over elem.
func Pointer(elem *node.TypeRef) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefPointer, Elem: elem}
}

// Slice returns a slice [node.TypeRef] over elem.
func Slice(elem *node.TypeRef) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: elem}
}

// Array returns a fixed-length array [node.TypeRef] of length n over
// elem.
func Array(elem *node.TypeRef, n int) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefArray, Elem: elem, ArrayLen: n}
}

// Map returns a map [node.TypeRef] keyed by key with value type
// value.
func Map(key, value *node.TypeRef) *node.TypeRef {
	return &node.TypeRef{TypeKind: node.TypeRefMap, MapKey: key, MapValue: value}
}

// Func returns a function-type [node.TypeRef] with the supplied
// parameter and return types.
func Func(params, returns []*node.TypeRef) *node.TypeRef {
	return &node.TypeRef{
		TypeKind:    node.TypeRefFunc,
		FuncParams:  params,
		FuncReturns: returns,
	}
}
