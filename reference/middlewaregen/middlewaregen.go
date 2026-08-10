// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package middlewaregen

import (
	"fmt"

	"go.thesmos.sh/eidos/reference/handlergen"
	"go.thesmos.sh/eidos/sdk"
	sdkgo "go.thesmos.sh/eidos/sdk/golang"
)

// Name is the plugin's stable identifier.
const Name = "middlewaregen"

// Version is the plugin's declared version. It composes into the
// pipeline's plugin fingerprint, which frontends fold into their cache
// keys — so bumping it invalidates a warm cache populated when this
// plugin behaved differently. A plugin that declares no version
// contributes an empty string and can never invalidate anything, which
// is a silent staleness bug waiting for its first behavioural change.
const Version = "1.0.0"

// Capability is the label contributors name in Requires to order
// themselves after this plugin within a bucket. Cross-bucket ordering
// comes from the bucket, not from this label — see [Plugin.Priority].
const Capability = "http.middleware"

// DirectiveName is the directive this plugin reacts to. It is
// declared and owned by [go.thesmos.sh/eidos/reference/handlergen];
// this plugin only reads the stamp.
//
// A directive name may be registered once per run, so redeclaring it
// here would be ErrDuplicateDirective at Build. One plugin owns a
// directive and the rest read it — otherwise the parsing rule exists
// in two places and gets to disagree with itself.
const DirectiveName = handlergen.DirectiveName

// SlotName is the file-level slot the stack decl is queued into.
const SlotName = "top"

// ChainSlot is the name of the slot this plugin's emit kind exposes
// for other plugins to contribute into.
//
// Exported because it is the coupling: a contributor calls
// [MiddlewareStack.Chain], and anything positioning itself relative to
// another entry passes a [sdk.Provenance] ID. The name is part of the
// published contract, not an implementation detail.
const ChainSlot = "chain"

// Kind is the emit kind this plugin declares. It must equal the
// `define` name in templates/golang/stack.tmpl; a mismatch renders
// nothing and fails nowhere.
const Kind sdk.Kind = "middlewaregen.stack"

// DefaultSuffix is the chain variable's name suffix when unset.
const DefaultSuffix = "Middleware"

// Options configures the generated chain variable's name.
type Options struct {
	// Suffix is appended to the source type name to form the chain
	// variable's identifier.
	Suffix string `eidos:"suffix,default=Middleware"`
}

// Plugin emits one middleware chain per `+gen:handler` struct and
// exposes a slot other plugins fill.
//
// It is the slot-owning half of the composition pattern: the plugin
// that declares an emit kind carrying a named slot, so that
// cross-cutting plugins contribute into a structure this plugin owns
// rather than inlining themselves into a core slot. The consequence
// worth understanding is directional — a contributor that runs when
// this plugin did not produces nothing at all, because there is no
// stack to append to. That is deliberate: a chain assembled from
// middleware with no handler to wrap is not a partial result, it is a
// wrong one.
type Plugin struct {
	*sdkgo.Base
	*sdk.Holder[Options]
	opts Options
}

// New returns a plugin with the options holder bound.
//
// The foundation bucket is not a preference: this plugin must run
// before any contributor, and cross-bucket ordering is the only
// ordering the framework honours between buckets. Provides publishes
// the label contributors name in Requires to order themselves after
// it within a bucket; it Requires nothing itself.
//
// It registers no template helper of its own. The shared Go helpers
// (fieldType, elemType, typeArgs, …) are already in the bundle [Base]
// merges under this plugin's prefix, so re-declaring them through
// [sdkgo.Builder.Funcs] would only shadow them with themselves.
func New() *Plugin {
	p := &Plugin{Base: sdkgo.NewGenerator(Name, goTemplates, GoOutputs()...).
		Version(Version).
		Priority(sdk.GeneratorFoundation).
		Provides(Capability).
		Build()}
	p.Holder = sdk.BindOptions(&p.opts)
	return p
}

// MiddlewareStack is the emit value rendered as one chain variable.
//
// It owns a named slot rather than a plain slice because the entries
// come from plugins this one does not know about: the slot carries
// per-entry provenance, orders contributions by the pipeline's
// capability topology rather than by append order, and lets a later
// contributor position itself relative to an earlier one by ID.
type MiddlewareStack struct {
	sdk.BaseEmit

	// VarName is the identifier of the generated chain variable.
	VarName string

	// TypeName is the source struct the chain belongs to, used in the
	// generated doc comment.
	TypeName string

	// HandlerRef is the type the chain's functions wrap. Held as a
	// ref rather than a rendered string so the backend registers the
	// import and the template carries no import block.
	HandlerRef sdk.Ref

	chain *sdk.Slot
}

// Kind binds this value to its template.
func (*MiddlewareStack) Kind() sdk.Kind { return Kind }

// Chain returns the slot contributors append middleware entries into,
// creating it on first use.
//
// The slot is deliberately unconstrained. Each contributor declares
// its own emit kind and ships the template that renders it, so the
// slot's content is heterogeneous by design and no single ElemKind
// could describe it — which is precisely the case an empty ElemKind
// exists for.
//
// The trade is real and worth stating: a kind-constrained slot rejects
// a wrong contribution at the append call with the offending plugin
// named, while this one accepts anything and fails later, at render,
// with [backend/golang.ErrTemplateMissing] naming a kind rather than a
// plugin. Constrain the slot when every contribution is the same shape;
// leave it open when contributors bring their own templates.
func (s *MiddlewareStack) Chain() *sdk.Slot {
	if s.chain == nil {
		s.chain = sdk.NewSlot(ChainSlot, "")
		s.chain.Owner = s
	}
	return s.chain
}

// Slot satisfies [sdk.SlotHost] so the backend's `slot` template
// helper reaches the chain by name. Any other name returns an empty
// unconstrained slot rather than nil, so a template asking for a slot
// this kind does not have renders nothing instead of failing.
func (s *MiddlewareStack) Slot(name string) *sdk.Slot {
	if name == ChainSlot {
		return s.Chain()
	}
	return sdk.NewSlot(name, "")
}

var _ sdk.SlotHost = (*MiddlewareStack)(nil)

// Generate emits one stack per annotated struct.
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name)
	for src := range ctx.Reader.Structs().All() {
		if !src.HasPositiveDirective(DirectiveName) {
			continue
		}
		stack := &MiddlewareStack{
			BaseEmit: sdk.BaseEmit{
				OriginNode: src,
				SetByName:  c.SetBy(),
				SourcePos:  src.Pos(),
			},
			VarName:    src.Name + p.opts.Suffix,
			TypeName:   src.Name,
			HandlerRef: sdk.External("net/http", "Handler"),
		}
		if err := ctx.Store.Emit().AppendOriginSlot(
			src, SlotName, stack, c.Provenance(Name+".stack."+src.Name),
		); err != nil {
			return fmt.Errorf("%s: append stack slot: %w", Name, err)
		}
	}
	return nil
}
