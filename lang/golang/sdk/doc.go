// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package sdk is the plugin base every Go-generating plugin
// embeds.
//
// A plugin tells the pipeline six things before it generates
// anything: what it is called, what version it is, which bucket it
// runs in, what it provides and requires, which directives it
// declares, and — per backend language — what it emits and how to
// render it. Ten methods, of which nine are the same for every Go
// generator apart from the values they return.
//
// Written out per plugin, those nine drift. The set this package
// replaces had sixteen copies of the language dispatch, and two of
// them tested the language marker against a local constant rather
// than [golang.Language]: a plugin that silently emitted nothing,
// with no diagnostic, because the string did not match.
//
// # Use
//
//	type Plugin struct {
//		*golang.Base
//		*sdk.Holder[Options]
//		opts Options
//	}
//
//	func New() *Plugin {
//		p := &Plugin{Base: golang.NewGenerator(Name, goTemplates, GoOutputs()...).
//			Version(Version).
//			Priority(sdk.GeneratorFoundation).
//			Provides(Capability).
//			Directives(directives()...).
//			Build()}
//		p.Holder = sdk.BindOptions(&p.opts)
//		return p
//	}
//
// [Builder] is construction state; [Base] is the frozen value the
// backend's render pool reads concurrently. They are two types
// rather than one on purpose: a single mutable type would leave a
// plugin's declared outputs and template tree writable for the
// life of the process, from any goroutine holding the plugin.
//
// # What the base does not answer
//
// [Base] answers the six declaration methods and nothing else. It
// has no Generate, no Annotate, and no opinion about emit values —
// a plugin embedding it still writes the whole of what makes it
// that plugin.
//
// # The cost of a shared base
//
// A plugin that wrote its own [sdk.CapabilityProvider] methods
// stops compiling when the pipeline adds one, and its author
// decides what the new method should answer. Embedding [Base]
// converts that compile error into a silent default chosen here,
// in a file the plugin author does not read.
//
// Two things bound the damage. [Base] answers the value that means
// *not provided* for everything not explicitly set — nil, and
// [sdk.DefaultPriority], which is the documented bucket a plugin
// implementing no capability already occupies — so a default is
// never a plausible-looking guess. And TestBaseSatisfiesExactly
// pins the provider set: adding a method to any of those
// interfaces fails that test loudly even though every plugin in
// the workspace still compiles.
//
// # Failure mode
//
// [Builder.Build] panics on a malformed declaration rather than
// returning an error. It runs inside a plugin's New, so it fires
// deterministically at process start on the first run of any test
// that constructs the plugin — before a pipeline exists to report
// a diagnostic through, and long before a generated file could be
// wrong because of it. The alternative, an error every caller
// discards, moves the failure to the first run that renders
// nothing and explains why in no output at all.
package sdk
