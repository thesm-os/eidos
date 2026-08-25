// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sentinel

import (
	"embed"

	sdkgo "go.thesmos.sh/eidos/lang/golang/sdk"
	"go.thesmos.sh/eidos/sdk"
)

// GoSuffix is appended to the anchor declaration's source basename to
// form the output filename.
//
// The `_test.go` ending triggers the framework's automatic
// `<pkg>_test` package shift, so the checks drive the package the way
// a consumer does rather than reaching inside it — which matters here
// more than for most generators: what is being asserted is what a
// caller can observe, and a check reaching inside the package could
// assert things no caller can.
const GoSuffix = "_sentinel.gen_test.go"

// goTemplatesFS is the Go template tree. The backend reads it once at
// Build time and registers every `*.tmpl` under `templates/golang/` by
// base filename.
//
//go:embed templates/golang/*.tmpl
var goTemplatesFS embed.FS

// GoOutputs returns the Go output set: one file per annotated package.
//
// A single output, unlike the builder and the enum generator. Those
// generate something and then check it; this one only checks, so there
// is no production half to route separately.
func GoOutputs() []sdk.Output {
	return []sdk.Output{{Suffix: GoSuffix}}
}

// goSupport is everything this plugin declares for Go: the template
// tree, the file it emits, and how a Go declaration is read.
//
// The read side arrives with the rest through [sdkgo.Support], which
// is what lets the plugin's core name no language: it asks the
// declared rules which identifiers are declared errors and what
// contract a declaration carries, and a second target language is a
// sibling file and one more For call rather than an edit to what the
// plugin is.
//
// No vocabulary, unlike the enum generator. This plugin composes no
// identifier at all — it names what the author already declared — so
// there is no word for a language to supply.
func goSupport() (string, sdk.LanguageSupport) {
	return sdkgo.Support(goTemplatesFS, GoOutputs()...)
}
