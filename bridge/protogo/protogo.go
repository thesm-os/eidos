// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package protogo

import (
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/frontend/protobuf"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier.
const Name = "protogo"

// Version is the plugin's declared version. It composes into the
// pipeline's plugin fingerprint, which frontends fold into their cache
// keys — so bumping it invalidates a warm cache populated when this
// plugin behaved differently. A plugin that declares no version
// contributes an empty string and can never invalidate anything, which
// is a silent staleness bug waiting for its first behavioural change.
const Version = "1.0.0"

// Capability is the capability label the bridge advertises.
// Downstream plugins that need Go-flavoured proto-derived
// translation declare `Requires: ["go-types"]` so the
// capability-topo plan resolver orders them after this bridge.
const Capability = "go-types"

// Plugin is the proto→Go bridge annotator.
type Plugin struct {
	*opt.Holder[Options]
	opts Options
}

// Options is the plugin's user-tunable settings. Empty for now;
// the surface exists so the plugin satisfies the standard
// [plugin.OptionsProvider] contract and future per-pair tuning
// (initialism overrides, custom well-known mappings) lands here.
type Options struct{}

// New returns a fresh plugin instance with the options holder
// bound.
func New() *Plugin {
	p := &Plugin{}
	p.Holder = opt.Bind(&p.opts)
	return p
}

// Name returns [Name].
func (*Plugin) Name() string { return Name }

// Version satisfies [plugin.Versioned].
func (*Plugin) Version() string { return Version }

// Priority places the bridge in the annotator-refinement bucket
// — it runs after shape-detection annotators but before
// validation annotators, which lets shape stamps inform the
// translation tables (when a future pair needs them) while
// validation-time checks see the fully-translated meta.
func (*Plugin) Priority() sdk.Priority { return sdk.AnnotatorRefinement }

// Provides returns the bridge's capability label.
func (*Plugin) Provides() []string { return []string{Capability} }

// Requires returns nil: the bridge reads only frontend-produced
// meta and depends on no other plugin's contribution.
//
// The method exists because [plugin.CapabilityProvider] is
// satisfied as a whole or not at all. Without it the type missed
// the interface, and the pipeline silently fell back to
// [sdk.DefaultPriority] and registered no capability label — so
// neither the bucket documented on [Plugin.Priority] nor the
// [Capability] label other plugins declare a dependency on took
// effect.
func (*Plugin) Requires() []string { return nil }

// Directives declares an empty schema. The bridge has no
// user-facing directive; it operates by reading and stamping
// meta on proto-derived nodes filtered by the cross-frontend
// marker. Returning a non-nil empty slice satisfies the
// [directive.Owner] contract without registering a schema.
func (*Plugin) Directives() []sdk.DirectiveSchema { return nil }

// Annotate walks every proto-derived [node.Package] in the
// store and stamps the Go-namespaced translation meta on every
// reachable type reference, field, and package. Packages whose
// frontend-marker meta isn't `"protobuf"` are skipped — the
// bridge is scoped to its source language.
func (*Plugin) Annotate(ctx *sdk.AnnotatorContext) error {
	ps := ctx.Diag.For(Name)
	for pkg := range ctx.Reader.Packages().All() {
		marker, ok := protobuf.MetaFrontend.Get(pkg.Meta())
		if !ok || marker != protobuf.FrontendName {
			continue
		}
		annotatePackage(pkg, ps)
	}
	return nil
}

// annotatePackage stamps the Go-namespaced meta on pkg and
// every reachable type reference / field. The host-loop
// iteration order is documented: package-level first
// (`go.import`, `go.name`), then per-struct fields (`go.name`
// + `go.type` per field), then per-interface methods (param +
// return type refs). ps carries the per-plugin diagnostic sink
// so package-level warnings (missing `go_package`) anchor to
// the right plugin.
func annotatePackage(pkg *node.Package, ps *diag.PluginSink) {
	stampPackageMeta(pkg, ps)
	for _, s := range pkg.Structs {
		annotateStruct(s)
	}
	for _, iface := range pkg.Interfaces {
		annotateInterface(iface)
	}
}
