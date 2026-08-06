// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"go/types"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/node"
)

// The Go frontend stamps Go-idiomatic facts under the `go.*`
// namespace. The keys below are the complete set the converter
// produces; each is a registry-singleton declared at package init.
// Consumers typed-read via [meta.Key.Get]; templates read via the
// string-keyed `metaBool` / `metaStr` funcmap helpers.
//
// MetaFrontend is the one exception to the `go.*` namespace: it
// carries the bare name `frontend` because it is a cross-frontend
// convention. Multiple frontends declare the same key
// independently (each through [meta.EnsureKey] to avoid
// init-order coupling), and every produced [node.Package] carries
// the marker with the producing frontend's plugin name as value.
var (
	// MetaFrontend stamps the producing frontend's plugin name on
	// every [node.Package] entry the Go frontend emits. The value
	// is the string `"golang"` for Go-derived packages; the
	// protobuf frontend stamps the matching `"protobuf"` on its
	// packages. Bridge annotators (protogo and future language
	// pairs) and the cross-namespace audit step filter their walks
	// by reading this stamp.
	MetaFrontend = meta.EnsureKey(
		"frontend",
		meta.StringParser,
	) //nolint:gochecknoglobals // cross-frontend registry-singleton key

	// MetaIsChannel reports whether a [node.TypeRef] models a Go
	// channel.
	//
	// Registered with [meta.EnsureKey] rather than [meta.NewKey]
	// because the Go backend reads it too — depguard forbids a
	// backend importing a frontend, so both sides resolve the same
	// registry singleton by name. NewKey panics on a duplicate
	// registration, so whichever package's init ran second would
	// take the process down once both were linked into one binary.
	MetaIsChannel = meta.EnsureKey(
		"go.isChannel",
		meta.BoolParser,
	) //nolint:gochecknoglobals // shared registry-singleton key

	// MetaChanDir carries the channel's directionality ("both",
	// "send", or "recv"). Shared with the Go backend on the same
	// terms as [MetaIsChannel].
	MetaChanDir = meta.EnsureKey(
		"go.chanDir",
		meta.StringParser,
	) //nolint:gochecknoglobals // shared registry-singleton key

	// MetaChanElem stamps the printed source form of a channel's
	// element type ("int", "context.Context", "*pkg.Type"). The
	// element type also rides on the channel ref's first type
	// argument; the meta stamp is the templates-friendly view, the
	// type arg is the structured one.
	MetaChanElem = meta.NewKey(
		"go.chanElem",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaIsContext reports that the carrying node references the
	// `context.Context` interface.
	MetaIsContext = meta.NewKey(
		"go.isContext",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaIsError reports that the carrying node references the
	// predeclared `error` interface.
	MetaIsError = meta.NewKey(
		"go.isError",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaIsStringer reports that the carrying node's type
	// implements `fmt.Stringer`.
	MetaIsStringer = meta.NewKey(
		"go.isStringer",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaIsComparable reports that the carrying node's type
	// satisfies Go's `comparable` constraint.
	MetaIsComparable = meta.NewKey(
		"go.isComparable",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaIsInterface reports that the carrying ref's underlying
	// type is an interface, type parameters excluded.
	//
	// The node IR deliberately keeps no Go-specific type-kind
	// variants, so a named ref carries a package and an identifier
	// and nothing about what they resolve to: `io.Reader` and
	// `time.Duration` are indistinguishable to a plugin. Plugins
	// that must tell a collaborator or a stream from a plain value
	// read this key rather than resolving names themselves, which
	// they cannot do for types outside the loaded packages.
	//
	// A type parameter is not reported, even though its constraint
	// is an interface — `K comparable` is a key, not a
	// collaborator, and treating it as one misroutes every generic
	// signature.
	MetaIsInterface = meta.NewKey(
		"go.isInterface",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaEmbedsInterface reports that a [node.Struct] embeds at
	// least one interface (Go's promotion-by-embedding case).
	MetaEmbedsInterface = meta.NewKey(
		"go.embedsInterface",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaIsEmptyInterface reports that a [node.Interface] declares
	// no methods and no embeds — Go's empty-interface form.
	MetaIsEmptyInterface = meta.NewKey(
		"go.isEmptyInterface",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaIsConstraintInterface reports that a [node.Interface]
	// declares at least one type-set entry or `~T` approximate
	// term — i.e. the interface is intended as a generic-
	// constraint declaration rather than a method-set contract.
	MetaIsConstraintInterface = meta.NewKey(
		"go.isConstraintInterface",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaUnderlyingKind records the kind of the underlying type
	// for a [node.Alias] — one of "basic", "func", "map", "slice",
	// "array", "pointer", "chan". The value names the type's kind,
	// not the type itself: an alias to `int` carries "basic", not
	// "int". Struct and interface underlying types never reach this
	// key because [convertTypeSpec] routes them to [node.Struct] /
	// [node.Interface] instead of an alias; see [underlyingKindOf]
	// for the empty-string fallback on shapes outside the set.
	MetaUnderlyingKind = meta.NewKey(
		"go.underlyingKind",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaIsIterSeq reports that a [node.Function]'s return type
	// is `iter.Seq[T]`.
	MetaIsIterSeq = meta.NewKey(
		"go.isIterSeq",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaIsIterSeq2 reports that a [node.Function]'s return type
	// is `iter.Seq2[K, V]`.
	MetaIsIterSeq2 = meta.NewKey(
		"go.isIterSeq2",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaIterKeyType stamps the printed source form of an
	// `iter.Seq2`'s key-type parameter.
	MetaIterKeyType = meta.NewKey(
		"go.iterKeyType",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaIterValueType stamps the printed source form of an
	// `iter.Seq` / `iter.Seq2`'s value-type parameter.
	MetaIterValueType = meta.NewKey(
		"go.iterValueType",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaIotaValue stamps the numeric value a typed constant
	// resolves to, and is forwarded onto the [node.EnumVariant] that
	// [promoteAliasToEnum] builds from that constant. Named for the
	// iota-driven enum case it exists to serve; [stampConstantMeta]
	// stamps every constant whose value is an integer exactly
	// representable as an int64, iota-derived or not.
	MetaIotaValue = meta.NewKey(
		"go.iotaValue",
		meta.IntParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaReceiverIsPointer reports that a [node.Method] is
	// declared on a pointer receiver (`func (*T) Foo()`).
	MetaReceiverIsPointer = meta.NewKey(
		"go.receiverIsPointer",
		meta.BoolParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaConstraintTerms records the disjunctive type-set terms a
	// Go generic constraint declares (the `~int | ~string` form).
	MetaConstraintTerms = meta.NewKey(
		"go.constraintTerms",
		constraintTermsParser,
	) //nolint:gochecknoglobals // typed registry-singleton key
)

// MetaTagPrefix is the per-key namespace under which struct-tag
// entries are stamped on [node.Field] meta. For a field tag
// `json:"id" db:"id_col"`, the converter stamps
// `go.tag.json="id"` and `go.tag.db="id_col"`.
const MetaTagPrefix = "go.tag."

// constraintTermsParser is the [meta.Parser] for
// [MetaConstraintTerms]. The body shape mirrors the JSON wire form
// documented on [ConstraintTerm].
func constraintTermsParser(raw string) ([]ConstraintTerm, error) {
	return unmarshalConstraintTerms(raw)
}

// ConstraintTerm carries one disjunctive type-set term from a Go
// generic constraint, mirroring the type-checker's [types.Term]
// view in a JSON-friendly shape.
type ConstraintTerm struct {
	// Type is the term's [node.TypeRef].
	Type *node.TypeRef `json:"type,omitempty"`

	// Approximate reports whether the term carries Go's `~`
	// operator (any type whose underlying type is Type) or names
	// the type exactly.
	Approximate bool `json:"approximate,omitempty"`
}

// stampFrontendMarker records the cross-frontend provenance marker
// on pkg's meta bag. Every package the Go frontend emits carries
// this stamp; bridge annotators and the cross-namespace audit step
// filter their walks by reading it.
func stampFrontendMarker(pkg *node.Package) {
	MetaFrontend.Set(pkg.EnsureMeta(), FrontendName, FrontendName)
}

// stampChanMeta records [MetaIsChannel], [MetaChanDir], and
// [MetaChanElem] on ref using the channel's direction and element
// type. The originating position is taken from the ref so the
// provenance trail surfaces the source-level type expression in
// --explain output.
func stampChanMeta(ref *node.TypeRef, ch *types.Chan) {
	pos := ref.Pos()
	MetaIsChannel.SetAt(ref.EnsureMeta(), true, meta.AuthorityPlugin, FrontendName, pos)
	MetaChanDir.SetAt(ref.EnsureMeta(), chanDirString(ch.Dir()), meta.AuthorityPlugin, FrontendName, pos)
	MetaChanElem.SetAt(ref.EnsureMeta(), ch.Elem().String(), meta.AuthorityPlugin, FrontendName, pos)
}

// chanDirString translates a [types.ChanDir] into the convention
// string [MetaChanDir] carries on a channel ref.
func chanDirString(d types.ChanDir) string {
	switch d {
	case types.SendOnly:
		return "send"
	case types.RecvOnly:
		return "recv"
	default:
		return "both"
	}
}

// unmarshalConstraintTerms decodes a JSON-encoded slice of
// [ConstraintTerm] from raw.
func unmarshalConstraintTerms(raw string) ([]ConstraintTerm, error) {
	if raw == "" {
		return nil, nil
	}
	var out []ConstraintTerm
	if err := jsonUnmarshalString(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
