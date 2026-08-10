// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package authgen

import (
	"embed"
	"fmt"

	"go.thesmos.sh/eidos/reference/middlewaregen"
	"go.thesmos.sh/eidos/sdk"
	sdkgo "go.thesmos.sh/eidos/sdk/golang"
)

// Name is the plugin's stable identifier.
const Name = "authgen"

// Version is the plugin's declared version. It composes into the
// pipeline's plugin fingerprint, which frontends fold into their cache
// keys — so bumping it invalidates a warm cache populated when this
// plugin behaved differently. A plugin that declares no version
// contributes an empty string and can never invalidate anything, which
// is a silent staleness bug waiting for its first behavioural change.
const Version = "1.0.0"

// Capability is this contributor's published label.
const Capability = "http.auth"

// Kind is the emit kind this plugin declares. It must equal the
// `define` name in templates/golang/entry.tmpl.
const Kind sdk.Kind = "authgen.entry"

// EntryID is the [sdk.Provenance] ID stamped on this plugin's entry,
// exported so a later contributor can position against it.
const EntryID = "authgen.entry"

//go:embed templates/golang/*.tmpl
var goTemplates embed.FS

// Entry is this plugin's contribution to a middleware chain.
//
// It is a plugin-defined emit kind rather than a bare expression so
// the plugin owns how it renders: the host's template calls `render`
// on each slot item, which dispatches to the template registered under
// that item's Kind. The host never learns what a auth entry looks
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

// Plugin contributes one auth entry into every middleware chain.
//
// It renders through its own template, so the host's chain template
// never encodes what authentication looks like.
type Plugin struct{ *sdkgo.Base }

// New returns a plugin instance.
//
// It declares no [sdk.Output]. A contributor renders inside a file it
// does not own: templates say how a value renders, outputs say where a
// file lands, and this plugin only ever does the former.
//
// The composition bucket places it one after the host's foundation
// bucket. Requires resolves only within a bucket, so the bucket — not
// the capability — is what orders this plugin against its host.
func New() *Plugin {
	return &Plugin{Base: sdkgo.NewPlugin(Name).
		Templates(goTemplates).
		Version(Version).
		Priority(sdk.GeneratorComposition).
		Provides(Capability).
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
			FuncRef:  sdk.NewExternal("example.com/httpmw/auth", "RequireAuth"),
			Handler:  stack.TypeName,
		}
		if err := stack.Chain().Append(entry, c.Provenance(EntryID)); err != nil {
			return fmt.Errorf("%s: append chain entry for %q: %w", Name, stack.TypeName, err)
		}
	}
	return nil
}
