// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package witness

import (
	sdkgo "go.thesmos.sh/eidos/lang/golang/sdk"
	"go.thesmos.sh/eidos/sdk"
)

// goSupport is everything this plugin declares for Go: how a Go
// declaration is read, and nothing else.
//
// [sdkgo.Reads] rather than one of the emitting constructors, for the
// reason the sample annotator gives: the plugin ships no template tree
// and writes no file, and declaring an output it never fills would
// give Layout a filename to compose for a contribution that never
// arrives.
//
// The read side is the whole of what it needs. Resolving the type an
// author named against the imports of the file that declared the
// parameter is a Go rule, and so are the two notations that name
// accepts.
func goSupport() (string, sdk.LanguageSupport) {
	return sdkgo.Reads()
}
