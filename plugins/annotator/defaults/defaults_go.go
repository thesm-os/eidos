// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package defaults

import (
	sdkgo "go.thesmos.sh/eidos/lang/golang/sdk"
	"go.thesmos.sh/eidos/sdk"
)

// goSupport is everything this plugin declares for Go: how a
// declaration written in Go is read.
//
// The read side alone — no template tree and no output, because this
// plugin emits no file. What it needs from Go is the qualifier and
// literal rules a declared value is written under and the struct-tag
// vocabulary the second declaration form uses, and both are answered
// by `lang/golang` rather than restated here.
//
// The plugin's core names no language and reads this as a pair, so a
// second source language is a sibling file and one more
// [sdk.Builder.For] rather than an edit to what the plugin is.
func goSupport() (string, sdk.LanguageSupport) {
	return sdkgo.Reads()
}
