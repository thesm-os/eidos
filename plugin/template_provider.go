// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugin

import (
	"io/fs"
	"text/template"
)

// TemplateProvider is the optional capability for plugins that ship
// templates and / or extend a backend's template func-map. The
// backend collects every TemplateProvider it sees, parses templates
// from each provider's filesystem into the rendering tree, and
// merges func-maps before rendering begins.
//
// All three methods are language-scoped via the lang argument.
// Plugins return ("", false) for [TemplateProvider.Templates] when
// they have no templates for the requested language; func-map
// methods return nil to indicate "nothing to add".
type TemplateProvider interface {
	TemplateSource
	TemplateFuncSource
	TemplateOverrideSource
}

// TemplateSource is the template-shipping half of
// [TemplateProvider], declared separately so the pipeline can tell
// "did not implement the capability" from "implemented part of it".
//
// A Go interface assertion is all-or-nothing: a plugin declaring
// two of the three methods satisfies neither this composite nor any
// consumer's check for it, and every consumer skips it in silence.
// Probing the halves individually is what turns that into a
// [ErrPartialCapability] naming the missing method.
type TemplateSource interface {
	// Templates returns a filesystem of template files for the
	// requested language and a boolean indicating whether the
	// plugin contributes templates to that language. The backend
	// parses every "*.tmpl" file in the returned filesystem.
	Templates(lang string) (fs.FS, bool)
}

// TemplateFuncSource is the funcmap-extension half of
// [TemplateProvider]. See [TemplateSource] for why the halves are
// declared separately.
type TemplateFuncSource interface {
	// TemplateFuncs returns func-map extensions to register for the
	// requested language. The backend rejects extensions whose
	// names collide with previously-registered names from another
	// plugin (use [TemplateProvider.TemplateOverrides] for
	// intentional override).
	TemplateFuncs(lang string) template.FuncMap
}

// TemplateOverrideSource is the deliberate-override half of
// [TemplateProvider]. See [TemplateSource] for why the halves are
// declared separately.
//
// This is the method a plugin most often omits: a plugin shipping
// templates and funcmap entries has no override to declare and the
// stub returning nil reads as pointless — right up until its
// absence costs the plugin its whole template contribution.
type TemplateOverrideSource interface {
	// TemplateOverrides returns func-map entries that intentionally
	// replace previously-registered names. The backend records each
	// override as a diagnostic so users can see which plugin's
	// definition won.
	TemplateOverrides(lang string) template.FuncMap
}
