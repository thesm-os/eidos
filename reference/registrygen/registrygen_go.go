// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package registrygen

import (
	sdkgo "go.thesmos.sh/eidos/lang/golang/sdk"
	"go.thesmos.sh/eidos/sdk"
)

// goSupport is everything this plugin declares for Go — its template
// tree and the files it emits.
//
// The plugin's core names no language and reads this as a pair, so a
// second target language is a sibling file and one more For call
// rather than an edit to what the plugin is.
func goSupport() (string, sdk.LanguageSupport) {
	return sdkgo.Support(goTemplates, sdk.Output{Suffix: FilenameSuffix})
}
