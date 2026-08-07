// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"encoding/json"
	"fmt"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/node"
)

// The `go.*` metadata vocabulary every Go-speaking part of a
// pipeline shares: the frontend stamps it, the backend reads it,
// bridges from other source languages write it, and plugins query
// it.
//
// Declared here rather than in the frontend that produces most of
// it, because a meta key is interned by name and a consumer that
// cannot import the declaring package re-declares it by string
// instead. Three did — the Go backend for five keys, a shape
// detector for one, and a downstream generator for two — each
// silently forfeiting the compile-time link to the declaration and
// the rename-safety that comes with it.
//
// lang/golang is the one package every Go-speaking consumer can
// import: depguard forbids a backend importing a frontend and
// forbids plugins importing either, and this is a leaf over node
// and emit that all three may depend on.
var (
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
	// MetaGoType stamps the Go-side rendered type expression on a
	// [node.TypeRef] — `int32`, `[]byte`, `*timestamppb.Timestamp`.
	// Written by a bridge translating another source language into
	// Go; the backend's render site reads it first and falls back to
	// the underlying name when no bridge ran.
	MetaGoType = meta.EnsureKey(
		"go.type",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaGoName stamps the Go-idiomatic identifier for a
	// [node.Field] or [node.Package] — the PascalCase form of a
	// field name, or a package clause.
	MetaGoName = meta.EnsureKey(
		"go.name",
		meta.StringParser,
	) //nolint:gochecknoglobals // typed registry-singleton key

	// MetaGoImport stamps the Go import path on a [node.Package].
	// Cross-package references in rendered output resolve through
	// this value rather than the source language's own qualifier.
	MetaGoImport = meta.EnsureKey(
		"go.import",
		meta.StringParser,
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

// jsonUnmarshalString decodes raw into v via [encoding/json].
//
// Used by the [meta.Parser] callbacks that accept a structured JSON
// value off the directive-override path; programmatic stamping
// bypasses the parser entirely. The error is wrapped so a malformed
// override names the package that rejected it rather than surfacing
// a bare encoding/json message.
func jsonUnmarshalString(raw string, v any) error {
	if err := json.Unmarshal([]byte(raw), v); err != nil {
		return fmt.Errorf("lang/golang: parse meta value: %w", err)
	}
	return nil
}
