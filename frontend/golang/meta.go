// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"go/types"

	"go.thesmos.sh/eidos/core/meta"
	langgo "go.thesmos.sh/eidos/lang/golang"
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

	// The `go.*` keys moved to [lang/golang] so every Go-speaking
	// consumer can import the declaration instead of re-declaring
	// it by string. These aliases are the same registry singletons
	// and keep existing readers compiling.
	//
	// Deprecated: use the identically-named key in
	// go.thesmos.sh/eidos/lang/golang. Removed no earlier than the
	// next minor release.
	MetaIsChannel             = langgo.MetaIsChannel             //nolint:gochecknoglobals // re-exported registry singleton
	MetaChanDir               = langgo.MetaChanDir               //nolint:gochecknoglobals // re-exported registry singleton
	MetaChanElem              = langgo.MetaChanElem              //nolint:gochecknoglobals // re-exported registry singleton
	MetaIsContext             = langgo.MetaIsContext             //nolint:gochecknoglobals // re-exported registry singleton
	MetaIsError               = langgo.MetaIsError               //nolint:gochecknoglobals // re-exported registry singleton
	MetaIsStringer            = langgo.MetaIsStringer            //nolint:gochecknoglobals // re-exported registry singleton
	MetaIsComparable          = langgo.MetaIsComparable          //nolint:gochecknoglobals // re-exported registry singleton
	MetaIsInterface           = langgo.MetaIsInterface           //nolint:gochecknoglobals // re-exported registry singleton
	MetaEmbedsInterface       = langgo.MetaEmbedsInterface       //nolint:gochecknoglobals // re-exported registry singleton
	MetaIsEmptyInterface      = langgo.MetaIsEmptyInterface      //nolint:gochecknoglobals // re-exported registry singleton
	MetaIsConstraintInterface = langgo.MetaIsConstraintInterface //nolint:gochecknoglobals // re-exported registry singleton
	MetaUnderlyingKind        = langgo.MetaUnderlyingKind        //nolint:gochecknoglobals // re-exported registry singleton
	MetaIsIterSeq             = langgo.MetaIsIterSeq             //nolint:gochecknoglobals // re-exported registry singleton
	MetaIsIterSeq2            = langgo.MetaIsIterSeq2            //nolint:gochecknoglobals // re-exported registry singleton
	MetaIterKeyType           = langgo.MetaIterKeyType           //nolint:gochecknoglobals // re-exported registry singleton
	MetaIterValueType         = langgo.MetaIterValueType         //nolint:gochecknoglobals // re-exported registry singleton
	MetaIotaValue             = langgo.MetaIotaValue             //nolint:gochecknoglobals // re-exported registry singleton
	MetaReceiverIsPointer     = langgo.MetaReceiverIsPointer     //nolint:gochecknoglobals // re-exported registry singleton
	MetaConstraintTerms       = langgo.MetaConstraintTerms       //nolint:gochecknoglobals // re-exported registry singleton
)

// MetaTagPrefix is the per-key namespace struct-tag entries are
// stamped under.
//
// Deprecated: use [lang/golang.MetaTagPrefix]. Removed no earlier
// than the next minor release.
const MetaTagPrefix = langgo.MetaTagPrefix

// ConstraintTerm carries one disjunctive type-set term from a Go
// generic constraint.
//
// Deprecated: use [lang/golang.ConstraintTerm]. Removed no earlier
// than the next minor release.
type ConstraintTerm = langgo.ConstraintTerm

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
