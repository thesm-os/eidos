// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package repogen

import (
	sdkgo "go.thesmos.sh/eidos/lang/golang/sdk"
	"go.thesmos.sh/eidos/sdk"
)

// goSupport is what this plugin declares for Go: the file it owns,
// rendered through the backend's own kind templates rather than a
// tree of its own.
//
// The plugin's core names no language and reads this as a pair, so a
// second target language is a sibling file and one more For call
// rather than an edit to what the plugin is.
func goSupport() (string, sdk.LanguageSupport) {
	return sdkgo.Builtin(sdk.Output{Suffix: FilenameSuffix})
}
