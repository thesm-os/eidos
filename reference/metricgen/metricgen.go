// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package metricgen

import (
	"embed"
	"fmt"

	"go.thesmos.sh/eidos/reference/authgen"
	"go.thesmos.sh/eidos/reference/middlewaregen"
	"go.thesmos.sh/eidos/sdk"
	sdkgo "go.thesmos.sh/eidos/sdk/golang"
)

// Name is the plugin's stable identifier.
const Name = "metricgen"

// Version is the plugin's declared version. It composes into the
// pipeline's plugin fingerprint, which frontends fold into their cache
// keys — so bumping it invalidates a warm cache populated when this
// plugin behaved differently. A plugin that declares no version
// contributes an empty string and can never invalidate anything, which
// is a silent staleness bug waiting for its first behavioural change.
const Version = "1.0.0"

// Capability is this contributor's published label.
const Capability = "http.metrics"

// Kind is the emit kind this plugin declares. It must equal the
// `define` name in templates/golang/entry.tmpl.
const Kind sdk.Kind = "metricgen.entry"

// EntryID is the [sdk.Provenance] ID stamped on this plugin's entry,
// exported so a later contributor can position against it.
const EntryID = "metricgen.entry"

//go:embed templates/golang/*.tmpl
var goTemplates embed.FS

// Entry is this plugin's contribution to a middleware chain.
//
// It is a plugin-defined emit kind rather than a bare expression so
// the plugin owns how it renders: the host's template calls `render`
// on each slot item, which dispatches to the template registered under
// that item's Kind. The host never learns what a metrics entry looks
// like.
type Entry struct {
	sdk.BaseEmit

	// FuncRef is the middleware constructor this entry installs.
	FuncRef *sdk.Expr

	// Handler names the source type, for the rendered comment.
	Handler string
}

// Kind binds this value to its template.
func (*Entry) Kind() sdk.Kind { return Kind }

// Plugin contributes one metrics entry into every middleware chain.
//
// Ordered after authgen, and renders through its own template.
type Plugin struct{ *sdkgo.Base }

// New returns a plugin instance.
//
// It declares no [sdk.Output]: a contributor renders inside a file it
// does not own.
//
// Requires names [authgen.Capability] — the published const rather
// than a literal, so the two cannot drift into an ordering that
// silently stops holding — which puts this plugin after that
// contributor in the topo-sort. Order inside a slot is declared, never
// arranged by appending last.
func New() *Plugin {
	return &Plugin{Base: sdkgo.NewPlugin(Name).
		Templates(goTemplates).
		Version(Version).
		Priority(sdk.GeneratorComposition).
		Provides(Capability).
		Requires(authgen.Capability).
		Build()}
}

// Generate appends this plugin's entry to every chain in the run.
//
// The stacks are still queued as pending origin-slot contributions —
// Layout has not run, so they are attached to no file yet. Reading the
// pending list is how a contributor reaches a host value before
// routing, and it is why this plugin must not declare [sdk.NodesOnly].
func (*Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name)
	for _, pending := range ctx.Store.Emit().PendingOriginSlots() {
		stack, ok := pending.Item.(*middlewaregen.MiddlewareStack)
		if !ok {
			continue
		}
		entry := &Entry{
			BaseEmit: sdk.BaseEmit{SetByName: c.SetBy(), SourcePos: stack.Pos()},
			FuncRef:  sdk.NewExternal("example.com/httpmw/metrics", "RecordLatency"),
			Handler:  stack.TypeName,
		}
		if err := stack.Chain().Append(entry, c.Provenance(EntryID)); err != nil {
			return fmt.Errorf("%s: append chain entry for %q: %w", Name, stack.TypeName, err)
		}
	}
	return nil
}
