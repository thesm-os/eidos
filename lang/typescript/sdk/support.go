// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import (
	"embed"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/sdk"
)

// Language is the identifier the TypeScript adapter answers to,
// re-exported so a plugin's TypeScript binding need not import the
// language package beside this one.
const Language = typescript.Language

// Support returns the [sdk.LanguageSupport] a TypeScript-generating
// plugin declares, pre-filled with the language's read-side rules.
func Support(templates embed.FS, outputs ...sdk.Output) (string, sdk.LanguageSupport) {
	return Language, sdk.LanguageSupport{
		Templates: templates,
		Outputs:   outputs,
		Source:    source,
	}
}

// Builtin returns the support a plugin declares when it owns a
// TypeScript file but renders it through the backend's own kind
// templates rather than a tree of its own.
func Builtin(outputs ...sdk.Output) (string, sdk.LanguageSupport) {
	return Language, sdk.LanguageSupport{
		Outputs: outputs,
		Builtin: true,
		Source:  source,
	}
}

// Reads returns the support a plugin declares when it reads
// TypeScript declarations but emits nothing — an annotator, or a
// generator that contributes only into another plugin's slots.
//
// The read side alone. Every constructor here carries it, because a
// plugin that speaks TypeScript can read TypeScript whether or not it
// also renders it, and one left to declare the two separately is one
// that can declare half.
func Reads() (string, sdk.LanguageSupport) {
	return Language, sdk.LanguageSupport{Source: source}
}

// source is the TypeScript read-side rules every declaration above
// carries.
//
// Held once as a package value rather than constructed per call: it
// is stateless, and the identity makes a plugin's declaration
// comparable in a test.
//
//nolint:gochecknoglobals // stateless value, immutable after init.
var source sdk.SourceRules = typescript.Source{}

// The optional rule sets TypeScript answers, asserted here so
// dropping one is a compile error in this package rather than a
// generator finding the assertion fail at run time and generating its
// degraded form.
//
//nolint:gochecknoglobals // compile-time assertions, never read.
var (
	_ sdk.SigRules   = typescript.Source{}
	_ sdk.EnumRules  = typescript.Source{}
	_ sdk.ErrorRules = typescript.Source{}
)
