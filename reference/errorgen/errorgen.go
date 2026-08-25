// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package errorgen

import (
	"embed"
	"fmt"

	"go.thesmos.sh/eidos/reference/handlergen"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier.
const Name = "errorgen"

// Version is the plugin's declared version. It composes into the
// pipeline fingerprint frontends fold into their cache keys, so a bump
// invalidates a cache populated when this plugin behaved differently.
const Version = "1.0.0"

// Capability is this contributor's published label, exported so a
// plugin ordering against it names the const rather than a literal
// the two could drift apart on.
const Capability = "http.error"

// Kind is the emit kind this plugin declares. It must equal the
// `define` name in templates/golang/errorgen.entry.tmpl.
const Kind sdk.Kind = "errorgen.entry"

// EntryID is the [sdk.Provenance] ID stamped on this plugin's entry,
// exported so a later contributor can position against it.
const EntryID = "errorgen.entry"

//go:embed templates/golang/*.tmpl
var goTemplates embed.FS

// Entry is this plugin's contribution to a handler body.
//
// It is a plugin-defined emit kind rather than a bare statement so the
// plugin owns how it renders: handlergen's template calls `render` on
// each slot item, dispatching to the template registered under that
// item's Kind. The host never learns what a recover-and-respond entry looks like.
type Entry struct {
	sdk.BaseEmit

	// FuncRef is the call this entry installs.
	FuncRef *sdk.Expr

	// Handler names the host type, for the rendered comment.
	Handler string
}

// Kind binds this value to its template.
func (*Entry) Kind() sdk.Kind { return Kind }

// Plugin contributes one recover-and-respond entry into every handler.
type Plugin struct{ *sdk.Base }

// New returns a plugin instance.
//
// It declares no [sdk.Output]: a contributor renders inside a file it
// does not own. The template file is named for the plugin because the
// backend registers templates under their base filename as well as
// their `define` name — two contributors each shipping `entry.tmpl`
// collide at merge and the whole run writes nothing.
//
// The cross-cutting bucket places it after handlergen's foundation
// bucket. Requires resolves only within a bucket, so the bucket — not
// a capability — is what orders this plugin against its host.
func New() *Plugin {
	return &Plugin{Base: sdk.NewPlugin(Name).
		For(goSupport()).
		Version(Version).
		Priority(sdk.GeneratorCrossCutting).
		Provides(Capability).
		Build()}
}

// Generate appends this plugin's entry to every handler in the run.
//
// The handlers are still queued as pending origin-slot contributions —
// Layout has not run, so they are attached to no file yet. Reading the
// pending list is how a contributor reaches a host value before
// routing, and it is why this plugin must not declare [sdk.NodesOnly].
//
// A run without handlergen finds no handlers and emits nothing: no
// orphan file, no partial output, no configuration needed to say so.
func (*Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name)
	for _, pending := range ctx.Store.Emit().PendingOriginSlots() {
		host, ok := pending.Item.(*handlergen.Handler)
		if !ok {
			continue
		}
		entry := &Entry{
			BaseEmit: sdk.BaseEmit{SetByName: c.SetBy(), SourcePos: host.Pos()},
			FuncRef:  sdk.NewExternal("example.com/httpmw/errors", "Recover"),
			Handler:  host.Source,
		}
		if err := host.Slot(handlergen.PostbodySlot).Append(entry, c.Provenance(EntryID)); err != nil {
			return fmt.Errorf("%s: append entry for %q: %w", Name, host.Source, err)
		}
	}
	return nil
}
