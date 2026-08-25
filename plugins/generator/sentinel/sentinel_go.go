// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sentinel

import (
	"embed"

	sdkgo "go.thesmos.sh/eidos/lang/golang/sdk"
	"go.thesmos.sh/eidos/sdk"
)

// GoSuffix is the per-source-basename suffix the Go adapter
// reports through [GoOutputs]. The `_test.go` ending
// triggers the Go backend's automatic `<pkg>_test` package
// shift so the rendered tests live in an external test
// package and can't accidentally read private state.
const GoSuffix = "_sentinel_test.go"

// goTemplatesFS is the Go adapter's template tree. The
// backend reads it once at Build time and registers every
// `*.tmpl` under `templates/golang/` — the root the SDK's
// DefaultTemplateDir already names, so [New] hands the tree
// over without overriding it.
//
//go:embed templates/golang/*.tmpl
var goTemplatesFS embed.FS

// GoOutputs returns the Go adapter's output set — a single
// `<basename>_sentinel_test.go` file per annotated source
// package the Layout phase routes contributions to.
func GoOutputs() []sdk.Output {
	return []sdk.Output{{Suffix: GoSuffix}}
}

// goSupport is everything this plugin declares for Go — its template
// tree and the files it emits.
//
// The plugin's core names no language and reads this as a pair, so a
// second target language is a sibling file and one more For call
// rather than an edit to what the plugin is.
func goSupport() (string, sdk.LanguageSupport) {
	return sdkgo.Support(goTemplatesFS, GoOutputs()...)
}
