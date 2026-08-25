// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugin

import "text/template"

// ImportRegistrar names a package in the file currently rendering.
//
// The capability a template already has and a plugin's own helpers
// did not. A backend's template funcmap binds its import-collecting
// entries to the file being written, so a template composing a
// qualified reference gets the import registered as a side effect of
// asking for the qualifier. A helper supplied by a plugin is an
// ordinary function with no such binding, so text it returns naming
// another package produces a file that does not import it — output
// that renders cleanly and fails the consumer's build.
type ImportRegistrar interface {
	// Import registers path with the rendering file's import set and
	// returns the local name to spell references with.
	//
	// The local name rather than the path: two packages whose paths
	// end in the same element cannot both be spelled by that element,
	// and which one keeps it is a decision the import set makes for
	// the file as a whole.
	Import(path string) (string, error)
}

// ImportAwareFuncs is the optional interface a plugin implements when
// its template helpers emit references to other packages.
//
// Where [TemplateProvider.TemplateFuncs] and
// [TemplateProvider.TemplateOverrides] return helpers built once for
// the whole run, these are built per rendered file, against that
// file's import set.
//
// The case it exists for is replacing a generator's own vocabulary
// with one from a library. A backend ships assertion helpers that
// spell a check in terms of the standard library and nothing else, so
// the generated file depends on nothing a consumer did not choose; a
// consumer who does want a helper library replaces those entries, and
// the replacements name a package the generated file has to import.
// Without this the replacement is unwritable, and the extension point
// serves only helpers that never leave the file.
//
// The same reserved-name rules apply as to the static forms: an entry
// naming a canonical backend helper is rejected at Build, checked by
// invoking the factory once against a registrar that records nothing.
type ImportAwareFuncs interface {
	// TemplateFuncsFor returns the plugin's helpers for lang, bound to
	// the file being rendered.
	//
	// Called once per rendered file. An implementation captures reg
	// and returns closures over it; it must not retain reg beyond the
	// returned map, because the next file has a different import set
	// and a stale one writes into a file already finished.
	TemplateFuncsFor(lang string, reg ImportRegistrar) template.FuncMap
}
