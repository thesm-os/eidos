// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package pluginfixture supplies a minimal test-only plugin that
// exercises the [plugin.TemplateProvider] surface end-to-end: it
// defines a custom emit kind ([Banner]) outside the `emit.*`
// namespace, ships a template that renders it, and round-trips
// through any backend that respects the plugin extension story.
//
// The package is consumed by end-to-end backend tests in the
// backend/golang/ tree. It carries no production responsibility —
// production plugins live elsewhere.
package pluginfixture

import (
	"embed"
	"io/fs"
	"text/template"

	"go.thesmos.sh/eidos/core/kind"
	"go.thesmos.sh/eidos/emit"
)

// BannerKind is the [kind.Kind] discriminator [Banner] returns
// from [Banner.Kind]. The dotted form keeps the kind name outside
// the `emit.*` namespace reserved for core emit types, matching
// the plugin-defined emit-kind convention.
const BannerKind kind.Kind = "bannergen.banner"

// Banner is the plugin-defined emit kind this fixture ships. It
// embeds [emit.BaseEmit] for the standard [emit.Node] methods and
// adds [Banner.Title] / [Banner.Message] / [Banner.Target] as the
// content fields the matching template renders.
type Banner struct {
	emit.BaseEmit

	// Title is the banner header text — rendered as the first line
	// of the comment block.
	Title string

	// Message is the banner body text — rendered on the second
	// line of the comment block.
	Message string

	// Target identifies the output file the banner contributes to.
	// Generators populate this so the backend can route the banner
	// alongside the other declarations destined for that file.
	Target emit.Target
}

// Kind returns [BannerKind].
func (*Banner) Kind() kind.Kind { return BannerKind }

// Name is the plugin identifier this fixture registers under. It
// appears in diagnostic attribution and override Info messages so
// tests can match against a stable identifier.
const Name = "bannergen"

// Plugin satisfies [plugin.Plugin] and [plugin.TemplateProvider].
// The zero value is usable — there is no configuration surface.
type Plugin struct{}

// Name returns the plugin's stable identifier — [Name].
func (Plugin) Name() string { return Name }

//go:embed templates/golang/*.tmpl
var templatesFS embed.FS

// Templates returns the embedded template filesystem for the
// requested language. The fixture ships templates only for the
// `golang` language; other languages get the empty signal.
func (Plugin) Templates(lang string) (fs.FS, bool) {
	if lang != "golang" {
		return nil, false
	}
	sub, err := fs.Sub(templatesFS, "templates/"+lang)
	if err != nil {
		// Sub fails only when the named subdirectory is missing
		// from the embed — a build-time impossibility given the
		// embed directive above. Returning ok=false on that
		// theoretical path keeps the consumer safe rather than
		// surfacing a panic.
		return nil, false
	}
	return sub, true
}

// TemplateFuncs returns nil — the fixture ships no funcmap
// extensions.
func (Plugin) TemplateFuncs(string) template.FuncMap { return nil }

// TemplateOverrides returns nil — the fixture ships no funcmap
// overrides.
func (Plugin) TemplateOverrides(string) template.FuncMap { return nil }
