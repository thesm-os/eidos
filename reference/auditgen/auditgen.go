// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package auditgen

import (
	"embed"
	"fmt"

	"go.thesmos.sh/eidos/reference/handlergen"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier.
const Name = "auditgen"

// Version is the plugin's declared version. It composes into the
// pipeline fingerprint frontends fold into their cache keys, so a bump
// invalidates a cache populated when this plugin behaved differently.
const Version = "1.0.0"

// Kind is the emit kind this plugin declares. It must equal the
// `define` name in templates/golang/auditgen.entry.tmpl.
const Kind sdk.Kind = "auditgen.entry"

// EntryID is the [sdk.Provenance] ID stamped on this plugin's entry,
// exported so a later contributor can position against it.
const EntryID = "auditgen.entry"

//go:embed templates/golang/*.tmpl
var goTemplates embed.FS

// Entry is this plugin's contribution to a handler body.
//
// It is a plugin-defined emit kind rather than a bare statement so the
// plugin owns how it renders: handlergen's template calls `render` on
// each slot item, dispatching to the template registered under that
// item's Kind. The host never learns what a audit log entry looks like.
type Entry struct {
	sdk.BaseEmit

	// FuncRef is the call this entry installs.
	FuncRef *sdk.Expr

	// Handler names the host type, for the rendered comment.
	Handler string
}

// Kind binds this value to its template.
func (*Entry) Kind() sdk.Kind { return Kind }

// Plugin contributes one audit log entry into every handler.
type Plugin struct{ *sdk.Base }

// New returns a plugin instance.
//
// It declares no [sdk.Output]: a contributor renders inside a file it
// does not own. The template file is named for the plugin because the
// backend registers templates under their base filename as well as
// their `define` name — two contributors each shipping `entry.tmpl`
// collide at merge and the whole run writes nothing.
//
// It publishes no capability, deliberately: nothing orders against
// this plugin, and it needs no label to be ordered, because the
// finalize bucket already places it after everything else. A
// capability nobody names is noise in the topo graph.
func New() *Plugin {
	return &Plugin{Base: sdk.NewPlugin(Name).
		For(goSupport()).
		Version(Version).
		Priority(sdk.GeneratorFinalize).
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
			FuncRef:  sdk.NewExternal("example.com/httpmw/audit", "Record"),
			Handler:  host.Source,
		}
		if err := host.Slot(handlergen.PostbodySlot).Append(entry, c.Provenance(EntryID)); err != nil {
			return fmt.Errorf("%s: append entry for %q: %w", Name, host.Source, err)
		}
	}
	return nil
}
