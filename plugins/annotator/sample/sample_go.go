// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sample

import (
	sdkgo "go.thesmos.sh/eidos/lang/golang/sdk"
	"go.thesmos.sh/eidos/sdk"
)

// goSupport is everything this plugin declares for Go: how a Go
// declaration is read, and nothing else.
//
// [sdkgo.Reads] rather than one of the emitting constructors. The
// plugin ships no template tree and writes no file — it stamps
// metadata the language reads back — and declaring an output it never
// fills would give Layout a filename to compose for a contribution
// that never arrives.
//
// The read side is the whole of what it needs: resolving the name an
// author wrote against the imports of the file that declared the type
// is a Go rule, and the two notations that name accepts are Go's.
func goSupport() (string, sdk.LanguageSupport) {
	return sdkgo.Reads()
}
