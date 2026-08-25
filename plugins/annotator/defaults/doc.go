// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package defaults owns the declared default: the value a generated
// constructor seeds a field with when its caller supplies none.
//
// # Why it is a plugin of its own
//
// The builder generator is the first reader, not the only one. A
// fixture generator seeds the same field the same way, and a
// validation generator needs the declared default to say what "unset"
// means. A directive may be registered once per run, so declaring it
// inside any one generator would make every other reader depend on
// that generator being registered.
//
// Readers take the stamp, never this plugin's behaviour:
//
//	value, pkg := defaults.DefaultOf(f.Meta()), defaults.DefaultPackage(f.Meta())
//
// # Two ways to declare one
//
// A directive, which every source language spells the same way:
//
//	//+gen:default 8080
//	Port int
//
// Or a struct tag, for languages that have them:
//
//	Port int `default:"8080"`
//
// The tag exists because a struct carrying a default on most of its
// fields reads better with them on the field lines than with a
// directive comment above each. Both produce the same stamp, so a
// reader never learns which was written.
//
// The directive wins where a field carries both. It is the more
// specific statement — a tag is often copied with the field it sits
// on, while a directive is written deliberately at the line it
// governs — and a rule that let the tag win would make the directive
// unable to correct one.
//
// # The value is carried verbatim
//
// The declared value is source text and is stamped unparsed.
// `"localhost"`, `8080`, `true`, `0.75` and `nil` all reach a
// template as themselves, which costs nothing and avoids a
// type-directed parser that would have to know every literal form the
// language admits — and would have to be told the field's type to
// tell `0` from `0.0`.
//
// What is checked is that the value is something the language can
// render. A typo fails here, positioned at the declaration, rather
// than in the consumer's compiler against generated code they did not
// write.
//
// # An explicit zero is not an absent default
//
// `//+gen:default 0` stamps. A reader seeing a value knows the author
// asked for it; one reading a bare zero cannot tell "seed this to
// zero" from "nothing was declared", and would emit the same
// constructor either way. That distinction is why the stamp is a
// string rather than a typed value: the empty string is the only
// absence.
//
// # A qualified value carries its package
//
// `//+gen:default time.Second` stamps `Second` under
// [MetaDefault] and `time` under [MetaDefaultPackage]. Two keys
// rather than one qualified string, because a rendered file has to
// register the import and only a reference can carry one — text
// cannot ask for it.
//
// # Language boundary
//
// What a declared value looks like belongs to the language it was
// written in: Go resolves a qualifier against the declaring file's
// import block, and struct tags are a Go spelling. That knowledge is
// [sdk.SourceRules], declared through the same [sdk.Builder.For] a
// generator declares what it renders with — one namespace of language
// names, two halves of one declaration. The stamp, the directive, the
// precedence rule and the diagnostics are neutral.
//
// Which half applies is decided by which language a caller asks
// about. This plugin asks about the package in front of it, through
// [sdk.LanguageOf], so a run mixing frontends reads each package with
// the rules that parsed it — and a run parsing Go while rendering
// something else still reads Go, which asking about the render
// language would get wrong in silence.
//
// A second language is a sibling `defaults_<lang>.go` holding its
// rules and one more For call.
//
// A package written in a language the plugin cannot read is reported
// once, rather than skipped in silence: every default in it would
// otherwise go unstamped and every constructor would seed nothing,
// which renders as a plausible file that ignored the source.
package defaults
