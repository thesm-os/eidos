// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import (
	"embed"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/sdk"
)

// Language is the identifier the Go adapter answers to, re-exported
// so a plugin's Go binding need not import the language package
// beside this one.
const Language = golang.Language

// Support returns the [sdk.LanguageSupport] a Go-generating plugin
// declares, pre-filled with Go's own funcmap bundle.
//
// Returned as a (language, support) pair so a plugin's
// language-neutral core can spread it straight into
// [sdk.Builder.For] and never name a language itself:
//
//	sdk.NewPlugin(Name).Version(Version).For(goSupport()).Build()
//
// where goSupport lives in the plugin's `_go.go` file beside its
// embedded template tree. That is the whole convention: the core
// declares what the plugin is, each binding file declares what it
// emits for one language.
func Support(templates embed.FS, outputs ...sdk.Output) (string, sdk.LanguageSupport) {
	return Language, sdk.LanguageSupport{
		Templates: templates,
		Outputs:   outputs,
	}
}

// Builtin returns the support a plugin declares when it owns a Go
// file but renders it through the backend's own kind templates
// rather than a tree of its own.
func Builtin(outputs ...sdk.Output) (string, sdk.LanguageSupport) {
	return Language, sdk.LanguageSupport{
		Outputs: outputs,
		Builtin: true,
	}
}
