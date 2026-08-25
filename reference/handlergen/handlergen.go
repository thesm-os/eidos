// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package handlergen

import (
	"embed"
	"fmt"

	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier.
const Name = "handlergen"

// Capability is the label other plugins name in Requires.
const Capability = "http.handler"

// Version is the plugin's declared version.
//
// It composes into the pipeline's plugin fingerprint, which frontends
// fold into their cache keys — so bumping it invalidates a warm cache
// that was populated when this plugin behaved differently. A plugin
// that declares no version contributes an empty string and can never
// invalidate anything, which is a silent staleness bug waiting for its
// first behavioural change.
const Version = "1.0.0"

// DirectiveName marks a struct as an HTTP handler.
//
// This plugin owns the directive; every other plugin in the ensemble
// reads the stamp with HasPositiveDirective instead of declaring it
// again. A directive name may be registered once per run, so a second
// declaration is ErrDuplicateDirective at Build.
const DirectiveName sdk.DirectiveName = "handler"

// Kind is the emit kind this plugin declares. It must equal the
// `define` name in templates/golang/handlergen.handler.tmpl.
const Kind sdk.Kind = "handlergen.handler"

// PrebodySlot and PostbodySlot are the slots contributors append into,
// rendered before and after this plugin's own statements.
//
// Exported because they are the coupling. A contributor names one of
// these; nothing else about this plugin's shape is its business.
const (
	PrebodySlot  = "prebody"
	PostbodySlot = "postbody"
)

// GoSuffix is the per-source trailer Layout appends for this plugin.
const GoSuffix = "_handler.go"

//go:embed templates/golang/*.tmpl
var goTemplates embed.FS

// Handler is the emit value rendered as one HTTP handler.
//
// It declares its own slots rather than reusing the framework's
// method-level `prebody` / `postbody` because it also declares its own
// emit kind: a plugin-defined kind gets no slots for free, and owning
// them is what lets this plugin decide where in the rendered body a
// contribution lands.
//
// Both slots are unconstrained. Contributors declare their own emit
// kinds and ship the templates that render them, so the content is
// heterogeneous by design and no single element kind describes it.
type Handler struct {
	sdk.BaseEmit

	// TypeName is the generated handler's identifier.
	TypeName string

	// Source is the struct the handler serves, for the doc comment.
	Source string

	// WriterRef and RequestRef are held as refs rather than rendered
	// strings so the backend registers their imports and the template
	// carries no import block.
	WriterRef  sdk.Ref
	RequestRef sdk.Ref

	pre, post *sdk.Slot
}

// Kind binds this value to its template.
func (*Handler) Kind() sdk.Kind { return Kind }

// Prebody returns the slot rendered before this plugin's statements.
func (h *Handler) Prebody() *sdk.Slot {
	if h.pre == nil {
		h.pre = sdk.NewSlot(PrebodySlot, "")
		h.pre.Owner = h
	}
	return h.pre
}

// Postbody returns the slot rendered after this plugin's statements.
func (h *Handler) Postbody() *sdk.Slot {
	if h.post == nil {
		h.post = sdk.NewSlot(PostbodySlot, "")
		h.post.Owner = h
	}
	return h.post
}

// Slot satisfies [sdk.SlotHost] so the backend's `slot` helper can
// reach either slot by name. An unknown name yields an empty slot
// rather than nil, so a template asking for one this kind does not
// have renders nothing instead of failing.
func (h *Handler) Slot(name string) *sdk.Slot {
	switch name {
	case PrebodySlot:
		return h.Prebody()
	case PostbodySlot:
		return h.Postbody()
	default:
		return sdk.NewSlot(name, "")
	}
}

var _ sdk.SlotHost = (*Handler)(nil)

// Plugin emits the handler every other plugin in the ensemble extends.
type Plugin struct{ *sdk.Base }

// New returns a plugin instance.
//
// It declares one [sdk.Output], carrying [GoSuffix]: this plugin owns
// the handler file, and the suffix is the whole of what Layout needs to
// compose a filename from a source file. Contributors into that file
// declare no output of their own — they append to the value this plugin
// emitted, so a run without handlergen writes nothing rather than an
// orphan half-file.
//
// The foundation bucket is where it has to run: it emits the value
// every later plugin contributes to, so it must run before any of them.
// Provides publishes the label those contributors order against, and
// nothing is required back — first in the graph depends on no one.
func New() *Plugin {
	return &Plugin{Base: sdk.NewPlugin(Name).
		For(goSupport()).
		Version(Version).
		Priority(sdk.GeneratorFoundation).
		Provides(Capability).
		Directives(directives()...).
		Build()}
}

// directives declares `+gen:handler`, which this plugin owns — every
// other plugin in the ensemble reads the stamp rather than redeclaring
// it, because a directive name may be registered only once per run.
func directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			On(sdk.NodeKindStruct).
			DenyNegation().
			Describe("Emit an HTTP handler for this struct. Owned by handlergen; other " +
				"plugins in the ensemble read the stamp rather than redeclaring it.").
			Build(),
	}
}

// Generate emits one handler per annotated struct.
func (*Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name)
	for src := range ctx.Reader.Structs().All() {
		if !src.HasPositiveDirective(DirectiveName) {
			continue
		}
		h := &Handler{
			BaseEmit: sdk.BaseEmit{
				OriginNode: src,
				SetByName:  c.SetBy(),
				SourcePos:  src.Pos(),
			},
			TypeName:   src.Name + "Handler",
			Source:     src.Name,
			WriterRef:  sdk.External("net/http", "ResponseWriter"),
			RequestRef: sdk.Ptr(sdk.External("net/http", "Request")),
		}
		// Through AppendOrigin rather than AppendOriginSlot: the
		// framework composes the `<kind>.<origin-name>` provenance id
		// every generator otherwise spells by hand, and the copies are
		// what drift — a sibling of this one had already lost the plugin
		// prefix from its id.
		if err := ctx.Store.Emit().AppendOrigin(c.SetBy(), "top", src, h); err != nil {
			return fmt.Errorf("%s: append handler: %w", Name, err)
		}
	}
	return nil
}
