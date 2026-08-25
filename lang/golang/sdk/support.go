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

// The assertion dialect's entry names, re-exported for the same
// reason [Language] is.
//
// Replacing the dialect is a plugin-author activity — it is the case
// [sdk.ImportAwareFuncs] documents itself as existing for — and a
// plugin doing it has to name the entries it replaces. Without these
// it reaches past this façade into the language package to do so,
// which is the import this package exists to remove.
//
// [FuncNeedsDiffHelper] belongs with them rather than apart: a
// replacement that answers the equality entries and forgets this one
// leaves the generated file declaring a comparison helper nothing
// calls, and importing whatever that helper names.
const (
	FuncAssertEqual     = golang.FuncAssertEqual
	FuncAssertDeepEqual = golang.FuncAssertDeepEqual
	FuncAssertNotEqual  = golang.FuncAssertNotEqual
	FuncAssertTrue      = golang.FuncAssertTrue
	FuncAssertFalse     = golang.FuncAssertFalse
	FuncAssertNil       = golang.FuncAssertNil
	FuncAssertNotNil    = golang.FuncAssertNotNil
	FuncAssertLen       = golang.FuncAssertLen
	FuncAssertNoError   = golang.FuncAssertNoError
	FuncAssertError     = golang.FuncAssertError
	FuncNeedsDiffHelper = golang.FuncNeedsDiffHelper
)

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
		Source:    source,
	}
}

// Builtin returns the support a plugin declares when it owns a Go
// file but renders it through the backend's own kind templates
// rather than a tree of its own.
func Builtin(outputs ...sdk.Output) (string, sdk.LanguageSupport) {
	return Language, sdk.LanguageSupport{
		Outputs: outputs,
		Builtin: true,
		Source:  source,
	}
}

// Reads returns the support a plugin declares when it reads Go
// declarations but emits nothing — an annotator, or a generator that
// contributes only into another plugin's slots.
//
// The read side alone. Every constructor here carries it, because a
// plugin that speaks Go can read Go whether or not it also renders
// it, and a plugin left to declare the two separately is one that can
// declare half.
func Reads() (string, sdk.LanguageSupport) {
	return Language, sdk.LanguageSupport{Source: source}
}

// source is the Go read-side rules every declaration above carries.
//
// Held once as a package value rather than constructed per call: it
// is stateless, and the identity makes a plugin's declaration
// comparable in a test.
//
//nolint:gochecknoglobals // stateless value, immutable after init.
var source sdk.SourceRules = golang.Source{}
