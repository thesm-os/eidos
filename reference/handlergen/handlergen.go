// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package handlergen

import (
	"embed"
	"fmt"
	"io/fs"
	"text/template"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
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

const langGo = "golang"

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
	WriterRef  emit.Ref
	RequestRef emit.Ref

	pre, post *emit.Slot
}

// Kind binds this value to its template.
func (*Handler) Kind() sdk.Kind { return Kind }

// Prebody returns the slot rendered before this plugin's statements.
func (h *Handler) Prebody() *emit.Slot {
	if h.pre == nil {
		h.pre = emit.NewSlot(PrebodySlot, "")
		h.pre.Owner = h
	}
	return h.pre
}

// Postbody returns the slot rendered after this plugin's statements.
func (h *Handler) Postbody() *emit.Slot {
	if h.post == nil {
		h.post = emit.NewSlot(PostbodySlot, "")
		h.post.Owner = h
	}
	return h.post
}

// Slot satisfies [emit.SlotHost] so the backend's `slot` helper can
// reach either slot by name. An unknown name yields an empty slot
// rather than nil, so a template asking for one this kind does not
// have renders nothing instead of failing.
func (h *Handler) Slot(name string) *emit.Slot {
	switch name {
	case PrebodySlot:
		return h.Prebody()
	case PostbodySlot:
		return h.Postbody()
	default:
		return emit.NewSlot(name, "")
	}
}

var _ emit.SlotHost = (*Handler)(nil)

// Plugin emits the handler every other plugin in the ensemble extends.
type Plugin struct{}

// New returns a plugin instance.
func New() *Plugin { return &Plugin{} }

// Name satisfies [sdk.Plugin].
func (*Plugin) Name() string { return Name }

// Version satisfies [sdk.Versioned].
func (*Plugin) Version() string { return Version }

// Priority places the plugin in the foundation bucket: it emits the
// value every later plugin contributes to, so it must run first.
func (*Plugin) Priority() sdk.Priority { return sdk.GeneratorFoundation }

// Provides publishes the capability contributors order against.
func (*Plugin) Provides() []string { return []string{Capability} }

// Requires reports no dependencies.
func (*Plugin) Requires() []string { return nil }

// Directives declares `+gen:handler`, which this plugin owns.
func (*Plugin) Directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			On(node.KindStruct).
			DenyNegation().
			Describe("Emit an HTTP handler for this struct. Owned by handlergen; other " +
				"plugins in the ensemble read the stamp rather than redeclaring it.").
			Build(),
	}
}

// Outputs declares the single file this plugin owns.
func (*Plugin) Outputs(lang string) []sdk.Output {
	if lang == langGo {
		return []sdk.Output{{Suffix: GoSuffix}}
	}
	return nil
}

// Templates ships the handler template.
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
// The shared Go helpers are already merged into the backend's
// overrideable funcmap, so returning them re-registers existing names —
// a Build-time ErrTemplateFuncCollision, which would mean this plugin
// could not appear beside any other that did the same.
func (*Plugin) TemplateFuncs(string) template.FuncMap { return nil }

// TemplateOverrides replaces nothing.
func (*Plugin) TemplateOverrides(string) template.FuncMap { return nil }

// Generate emits one handler per annotated struct.
func (*Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name, sdk.EmitTarget{})
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
			WriterRef:  emit.External("net/http", "ResponseWriter"),
			RequestRef: emit.Ptr(emit.External("net/http", "Request")),
		}
		if err := ctx.Store.Emit().AppendOriginSlot(
			src, "top", h, c.Provenance(Name+".handler."+src.Name),
		); err != nil {
			return fmt.Errorf("%s: append handler: %w", Name, err)
		}
	}
	return nil
}
