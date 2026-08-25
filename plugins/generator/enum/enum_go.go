// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package enum

import (
	"embed"

	sdkgo "go.thesmos.sh/eidos/lang/golang/sdk"
	"go.thesmos.sh/eidos/sdk"
)

// GoSuffix is the per-source-basename trailer for the primary output.
// Enumerations declared in `status.go` produce `status_enum.gen.go`.
const GoSuffix = "_enum.gen.go"

// GoTestSuffix is the trailer for the tagged check output.
//
// The `_test.go` ending triggers the framework's automatic
// `<pkg>_test` package shift, so the checks drive the surface the way
// a consumer does rather than reaching inside the package.
const GoTestSuffix = "_enum.gen_test.go"

// goTemplatesFS is the Go template tree. The backend reads it once at
// Build time and registers every `*.tmpl` under `templates/golang/` by
// base filename.
//
//go:embed templates/golang/*.tmpl
var goTemplatesFS embed.FS

// GoOutputs returns the Go output set: the surface, and the checks
// over it.
//
// The untagged entry comes first because the framework reserves the
// empty tag for a plugin's primary output and requires it at index 0.
//
// Both land beside the source and neither can be routed to another
// package: the surface declares methods on the enumeration's type,
// which Go permits only in that type's own package, and the checks
// drive it by name.
func GoOutputs() []sdk.Output {
	return []sdk.Output{
		{Suffix: GoSuffix},
		{Tag: TestOutputTag, Suffix: GoTestSuffix},
	}
}

// goSupport is everything this plugin declares for Go: the template
// tree, the files it emits, how a Go declaration is read, and the
// words its identifiers carry.
//
// The read side arrives with the rest through [sdkgo.Support], which
// is what lets the plugin's core name no language: it asks the
// declared rules what each variant renders as and how an identifier is
// spelled, and a second target language is a sibling file and one more
// For call rather than an edit to what the plugin is.
func goSupport() (string, sdk.LanguageSupport) {
	lang, s := sdkgo.Support(goTemplatesFS, GoOutputs()...)
	s.Words = goWords()
	return lang, s
}

// goWords is this plugin's vocabulary in Go.
//
// The names Go's own standard library gives these six jobs, which is
// what makes the generated surface reachable by everything that
// already looks for them: `fmt` reaches for `String`, `encoding/json`
// and every YAML library reach for `MarshalText`, and it is
// `MarshalText` rather than `MarshalJSON` that makes the type legal as
// a map key.
//
// `IsValid` and `Values` have no counterpart upstream to defer to.
// Neither is stated by any convention, so they are declared here
// rather than reached for through a shared constant that would imply
// one.
//
// How a word joins the type name is not decided here: the language
// answers that through [sdk.SourceRules.TypeName], so this map carries
// words and nothing else — which is why `Parse` and `Values` read the
// same though one leads its identifier and the other trails it.
func goWords() map[string]string {
	return map[string]string{
		WordRender:  "String",
		WordParse:   "Parse",
		WordValues:  "Values",
		WordValid:   "IsValid",
		WordEncode:  "MarshalText",
		WordDecode:  "UnmarshalText",
		WordUnknown: "Unknown",
	}
}
