// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package protogo

import langgo "go.thesmos.sh/eidos/lang/golang"

// The bridge stamps Go-side facts under the `go.*` namespace —
// the same namespace the Go frontend uses for its own
// language-specific stamps. Consumers (the Go backend's
// render-site rules, downstream generators that want Go-flavoured
// type information) read these keys without dispatching on
// frontend origin.
//
// Each key uses [meta.EnsureKey] so multiple bridges (a future
// `protorust`, `prototypescript`) can declare overlapping keys
// without an init-order coupling. Cross-language consumers that
// need the Go-side facts always read these stable names.
var (
	// The three `go.*` render keys moved to lang/golang so every
	// Go-speaking consumer imports one declaration instead of
	// re-declaring it by string — which the Go backend was doing for
	// all three.
	//
	// Deprecated: use the identically-named key in
	// go.thesmos.sh/eidos/lang/golang. Removed no earlier than the
	// next minor release.
	MetaGoType = langgo.MetaGoType //nolint:gochecknoglobals // re-exported registry singleton

	// Deprecated: use [lang/golang.MetaGoName].
	MetaGoName = langgo.MetaGoName //nolint:gochecknoglobals // re-exported registry singleton

	// Deprecated: use [lang/golang.MetaGoImport].
	MetaGoImport = langgo.MetaGoImport //nolint:gochecknoglobals // re-exported registry singleton
)
