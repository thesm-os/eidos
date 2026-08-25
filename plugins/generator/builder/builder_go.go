// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder

import (
	"embed"

	sdkgo "go.thesmos.sh/eidos/lang/golang/sdk"
	"go.thesmos.sh/eidos/sdk"
)

// GoSuffix is the per-source-basename trailer for the primary output.
// Types declared in `domain.go` produce `domain_builder.go`.
const GoSuffix = "_builder.gen.go"

// GoTestSuffix is the trailer for the tagged check output.
//
// The `_test.go` ending triggers the framework's automatic
// `<pkg>_test` package shift, so the checks land outside the package
// and drive the builder the way a consumer does rather than reaching
// inside it.
const GoTestSuffix = "_builder.gen_test.go"

// goTemplatesFS is the Go template tree. The backend reads it once at
// Build time and registers every `*.tmpl` under `templates/golang/` by
// base filename.
//
//go:embed templates/golang/*.tmpl
var goTemplatesFS embed.FS

// GoOutputs returns the Go output set: the builder, and the checks
// over it.
//
// The untagged entry comes first because the framework reserves the
// empty tag for a plugin's primary output and requires it at index 0.
func GoOutputs() []sdk.Output {
	return []sdk.Output{
		{Suffix: GoSuffix},
		{Tag: TestOutputTag, Suffix: GoTestSuffix},
	}
}

// goSupport is everything this plugin declares for Go: the template
// tree, the files it emits, and how a Go declaration is read.
//
// The read side arrives with the rest through [sdkgo.Support], which
// is what lets the plugin's core name no language: it asks the
// declared rules what shape a member's type has and how an identifier
// is spelled, and a second target language is a sibling file and one
// more For call rather than an edit to what the plugin is.
//
// No template helper is registered. Everything the templates call is
// either a backend builtin — `renderType`, `renderTypeParams`,
// `renderExpr`, `external`, `slot` — or an entry in the Go bundle the
// backend merges once, which is where the naming conventions that
// spell each setter live.
func goSupport() (string, sdk.LanguageSupport) {
	lang, s := sdkgo.Support(goTemplatesFS, GoOutputs()...)
	s.Words = goWords()
	return lang, s
}

// goWords is this plugin's vocabulary in Go.
//
// The words a Go reader expects: a builder for `User` is
// `UserBuilder`, its seed is `UserDefaults`, and the constructor
// taking an existing value is `NewUserFrom`. Each is English in
// PascalCase because that is what Go's own standard library reads
// like — another language declares its own, and neither the plugin's
// core nor its templates hold a word on the other's behalf.
//
// How a word joins the type name is not decided here either: the
// language answers that through [sdk.SourceRules.TypeName], so this
// map carries words and nothing else.
func goWords() map[string]string {
	return map[string]string{
		WordBuilder:   "Builder",
		WordCompanion: "Defaults",
		WordFrom:      "From",
		WordSet:       "With",
		WordAppend:    "Append",
		WordText:      "String",
		WordEntry:     "Entry",
		WordEntries:   "Entries",
	}
}
