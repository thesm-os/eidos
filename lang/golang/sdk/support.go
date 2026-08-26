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

// The method-set reporting a generator over interfaces needs,
// re-exported for the reason [Language] is: a plugin's Go binding
// calls it, and without these it reaches past this façade into the
// language package to do so.
//
// Not on [sdk.SourceRules] because the wording is language-side by
// design — what a short method set costs is a sentence about Go
// embedding, and a neutral interface would either carry Go's phrasing
// for every language or make each caller write its own.
type (
	// Consequence is the clause a generator's method-set diagnostics
	// end on: what its output loses when an embed contributes nothing.
	Consequence = golang.Consequence

	// Reporter is where those diagnostics go. `ctx.Diag` satisfies it.
	Reporter = golang.Reporter
)

// ReportMethodSet reports every embed that contributed nothing to a
// resolved method set and returns whether the result is usable.
//
// A caller emitting from a false result writes a double short a
// method, which does not satisfy the interface it doubles — and the
// pipeline carries every phase to completion, so the file lands on
// disk with a non-zero exit as the only sign.
func ReportMethodSet(
	r Reporter, set sdk.MethodSetResult, iface *sdk.Interface, plugin string, why Consequence,
) bool {
	return golang.ReportMethodSet(r, set, iface, plugin, why)
}

// StructOf resolves a type reference to the struct declaring it,
// false for anything else — including a type this run never loaded.
//
// The reader a plugin is handed is the [sdk.Resolver] to pass; see
// resolver.go for why no adapter is needed.
func StructOf(t *sdk.TypeRef, r sdk.Resolver) (*sdk.Struct, bool) {
	return golang.StructOf(t, r)
}

// MemberField finds an exported member by name and answers its
// declared type, promoted members included.
//
// The pair a generator aiming emitted code at a struct's members
// needs, re-exported for the reason [ReportMethodSet] is: both encode
// Go's own rules — that a selector reaches a promoted member, and
// that a generated file in another package cannot name an unexported
// one — so neither belongs on [sdk.SourceRules], and without them
// here a plugin reaches past this façade to spell them.
func MemberField(s *sdk.Struct, name string, r sdk.Resolver) (*sdk.TypeRef, bool) {
	return golang.MemberField(s, name, r)
}

// source is the Go read-side rules every declaration above carries.
//
// Held once as a package value rather than constructed per call: it
// is stateless, and the identity makes a plugin's declaration
// comparable in a test.
//
//nolint:gochecknoglobals // stateless value, immutable after init.
var source sdk.SourceRules = golang.Source{}

// The optional rule sets Go answers, asserted here so dropping one is
// a compile error in this package rather than a generator finding the
// assertion fail at run time and generating its degraded form.
//
//nolint:gochecknoglobals // compile-time assertions, never read.
var (
	_ sdk.SigRules   = golang.Source{}
	_ sdk.EnumRules  = golang.Source{}
	_ sdk.ErrorRules = golang.Source{}
)
