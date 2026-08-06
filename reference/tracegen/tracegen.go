// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tracegen

import (
	"embed"
	"fmt"
	"io/fs"
	"text/template"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/reference/middlewaregen"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier.
const Name = "tracegen"

// Version is the plugin's declared version. It composes into the
// pipeline's plugin fingerprint, which frontends fold into their cache
// keys — so bumping it invalidates a warm cache populated when this
// plugin behaved differently. A plugin that declares no version
// contributes an empty string and can never invalidate anything, which
// is a silent staleness bug waiting for its first behavioural change.
const Version = "1.0.0"

// Capability is this contributor's published label.
const Capability = "http.trace"

// Kind is the emit kind this plugin declares. It must equal the
// `define` name in templates/golang/entry.tmpl.
const Kind sdk.Kind = "tracegen.entry"

// EntryID is the [sdk.Provenance] ID stamped on this plugin's entry,
// exported so a later contributor can position against it.
const EntryID = "tracegen.entry"

const langGo = "golang"

//go:embed templates/golang/*.tmpl
var goTemplates embed.FS

// Entry is this plugin's contribution to a middleware chain.
//
// It is a plugin-defined emit kind rather than a bare expression so
// the plugin owns how it renders: the host's template calls `render`
// on each slot item, which dispatches to the template registered under
// that item's Kind. The host never learns what a tracing entry looks
// like.
type Entry struct {
	sdk.BaseEmit

	// FuncRef is the middleware constructor this entry installs.
	FuncRef *emit.Expr

	// Handler names the source type, for the rendered comment.
	Handler string
}

// Kind binds this value to its template.
func (*Entry) Kind() sdk.Kind { return Kind }

// Plugin contributes one tracing entry into every middleware chain.
//
// Third in the chain, demonstrating that a slot's order is a chain of declared Requires rather than a registration sequence.
type Plugin struct{}

// New returns a plugin instance.
func New() *Plugin { return &Plugin{} }

// Name satisfies [sdk.Plugin].
func (*Plugin) Name() string { return Name }

// Version satisfies [sdk.Versioned].
func (*Plugin) Version() string { return Version }

// Priority places the plugin in the composition bucket, one after the
// host's foundation bucket. Requires resolves only within a bucket, so
// the bucket is what orders this plugin against its host.
func (*Plugin) Priority() sdk.Priority { return sdk.GeneratorComposition }

// Provides publishes this contributor's label.
func (*Plugin) Provides() []string { return []string{Capability} }

// Requires names "http.metrics" so the topo-sort places this plugin\n// after that contributor. Order inside a slot is declared, never\n// arranged by appending last.
func (*Plugin) Requires() []string { return []string{"http.metrics"} }

// Templates ships the entry template.
//
// A contributor shipping a template needs no [sdk.FilenameProvider]:
// templates say how a value renders, outputs say where a file lands,
// and this plugin renders inside a file it does not own.
func (*Plugin) Templates(lang string) (fs.FS, bool) {
	if lang != langGo {
		return nil, false
	}
	sub, err := fs.Sub(goTemplates, "templates/golang")
	if err != nil {
		return nil, false
	}
	return sub, true
}

// TemplateFuncs contributes nothing.
//
// The shared Go helpers (fieldType, elemType, typeArgs, …) are already
// merged into the backend's overrideable funcmap, so a plugin that
// returns them here re-registers names that exist. TemplateFuncs is
// for *new* registrations and a duplicate is a Build-time
// ErrTemplateFuncCollision — meaning two plugins that both contribute
// the shared map cannot appear in the same pipeline. Return nil unless
// the plugin has a helper of its own.
func (*Plugin) TemplateFuncs(string) template.FuncMap { return nil }

// TemplateOverrides replaces nothing.
func (*Plugin) TemplateOverrides(string) template.FuncMap { return nil }

// Generate appends this plugin's entry to every chain in the run.
//
// The stacks are still queued as pending origin-slot contributions —
// Layout has not run, so they are attached to no file yet. Reading the
// pending list is how a contributor reaches a host value before
// routing, and it is why this plugin must not declare [sdk.NodesOnly].
func (*Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name, sdk.EmitTarget{})
	for _, pending := range ctx.Store.Emit().PendingOriginSlots() {
		stack, ok := pending.Item.(*middlewaregen.MiddlewareStack)
		if !ok {
			continue
		}
		entry := &Entry{
			BaseEmit: sdk.BaseEmit{SetByName: c.SetBy(), SourcePos: stack.Pos()},
			FuncRef:  sdk.NewExternal("example.com/httpmw/trace", "StartSpan"),
			Handler:  stack.TypeName,
		}
		if err := stack.Chain().Append(entry, c.Provenance(EntryID)); err != nil {
			return fmt.Errorf("%s: append chain entry for %q: %w", Name, stack.TypeName, err)
		}
	}
	return nil
}
