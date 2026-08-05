// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape

import (
	"sort"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/sdk"
)

// PluginName is the stable identifier the framework uses for the
// umbrella shape plugin in its plugin registry.
const PluginName = "shape"

// DirectiveName is the `+gen:` directive consumers write to pin a
// shape explicitly on a callable. The umbrella plugin checks the
// directive list before dispatching to detectors so user-supplied
// stamps win over inference.
//
// Syntax forms accepted (per the framework's directive parser):
//
//	//+gen:shape reader
//	//+gen:shape writer key=string value=Article
//	//+gen:shape kind=lifecycle
//
// The first positional argument or the `kind=` key carries the
// shape name; optional `key=` and `value=` modifiers populate the
// matching `shape.key_type` / `shape.value_type` meta keys
// verbatim.
//
// `-gen:shape` is rejected. Suppressing an inferred shape is a
// coherent thing to want and is not implemented; the form is denied
// so it fails at the source line rather than parsing and leaving
// detection to stamp a shape regardless.
const DirectiveName sdk.DirectiveName = "shape"

// ContractDirectiveName is the `+gen:` directive consumers write
// to declare a callable's membership in a named contract — a
// multi-callable protocol with a fixed role vocabulary. Contracts
// are orthogonal to structural shapes: a callable can carry both
// a structural [DirectiveName] stamp and one or more
// [ContractDirectiveName] memberships without overwriting either.
//
// Syntax:
//
//	//+gen:contract <contract-name> role=<role> [<partner-role>=<sibling-name>]…
//
// Examples:
//
//	//+gen:contract tx role=begin commit=Commit rollback=Rollback
//	//+gen:contract persister role=writer reader=GetByID
//	//+gen:contract saga role=step compensate=RefundCharge
//	//+gen:contract pool role=get put=Put
//
// `role=` is mandatory; every other KV pair (besides reserved
// keys) is interpreted as a partner reference keyed by the
// partner's role within the same contract.
//
// `-gen:contract` is rejected. A membership exists only where one
// is declared, so removing the directive is the suppression.
const ContractDirectiveName sdk.DirectiveName = "contract"

// DetectFunc is the per-language detection signature. The umbrella
// plugin hands every callable (a [*node.Function] or
// [*node.Method]) to the function; detectors type-assert as
// needed (typically through [GoCallable]) and use the lazy
// helpers in this package to compose queries.
//
// A `(Match{}, false)` return is the permissive skip — the next
// registered detector gets a turn. Detectors return it freely.
type DetectFunc func(n node.Node) (Match, bool)

// Detector is one shape's contribution to the umbrella plugin: a
// canonical shape name plus a per-frontend detection function
// map. Per-shape sub-packages (Reader, Writer, …) export one of
// these from their public API; the consumer composes them into
// the umbrella plugin via [Plugin.Detectors].
//
// A shape that supports only Go registers `{"golang": detectGo}`;
// adding language support is purely additive — register the new
// frontend key alongside.
type Detector struct {
	// Name is the canonical shape name stamped on a positive hit
	// (e.g. `"reader"`). Owned by the registering shape package
	// and used as the default for [Match.Shape] when the detector
	// leaves it empty.
	Name string

	// Priority controls dispatch order. Higher Priority detectors
	// run first within [Plugin.Detectors]; equal Priorities tie-
	// break on registration order. Per-shape sub-packages encode
	// their catalog priority here so the consumer cannot
	// accidentally register a permissive shape before a more
	// specific one (e.g. Writer before Deleter). A zero value
	// places the detector at the end of the dispatch order.
	Priority int

	// Detect maps a frontend marker (`"golang"`, `"protobuf"`, …)
	// to the per-language detection function. Sources from any
	// frontend not in this map are skipped without stamping.
	Detect map[string]DetectFunc
}

// Match is the stamp a detector returns on a positive hit. KeyType
// and ValueType carry qualified-type strings for shapes that have
// a key/value distinction; per-shape extras flow through
// [Match.StringStamps] and [Match.ListStamps] for shapes whose
// meta surface is richer than the universal triple.
type Match struct {
	// Shape is the canonical shape name (e.g. `"reader"`). When
	// empty, the umbrella plugin uses the parent [Detector.Name]
	// as the default. Populate this explicitly only when the
	// detector wants to stamp a different name than its
	// registration (e.g. a writer variant).
	Shape string

	// KeyType is the qualified type of the key/input. Empty for
	// shapes without a key.
	KeyType string

	// ValueType is the qualified type of the value/output. Empty
	// for shapes without a value.
	ValueType string

	// StringStamps carries per-shape extras the detector wants
	// the umbrella plugin to stamp under its own per-shape
	// namespace — for shapes whose meta surface extends beyond
	// the universal triple. Each entry is `{Key, Value}`; the
	// umbrella stamps with the detector's setBy attribution.
	StringStamps []StringStamp

	// ListStamps is the list-typed analogue of [Match.StringStamps]
	// — used by shapes that record collections (e.g. the full
	// non-error return-type list for a multi-value reader).
	ListStamps []ListStamp
}

// StringStamp is one (typed key, string value) pair a detector
// returns through [Match.StringStamps] for the umbrella plugin to
// stamp on the host callable's meta bag.
type StringStamp struct {
	Key   meta.Key[string]
	Value string
}

// ListStamp is the list-typed counterpart to [StringStamp].
type ListStamp struct {
	Key   meta.Key[[]string]
	Value []string
}

// Plugin is the umbrella shape plugin. One instance per pipeline
// owns both the `+gen:shape` and `+gen:contract` directive schemas
// and dispatches to every registered per-shape [Detector] +
// per-contract [Contract] during the framework's annotator walk.
//
// The merged design means consumers register one plugin
// regardless of how many shapes or contracts their pipeline
// recognises, and the framework's "one owner per directive" rule
// is satisfied by construction.
type Plugin struct {
	detectors []Detector
	contracts map[string]Contract
	mixins    map[string]Mixin

	// frontByMethod / frontByFunc map callable pointers to the
	// frontend marker stamped on their containing package.
	// Populated in [Plugin.BeforeNodes]; consumed by the per-
	// callable hooks so dispatch resolves the source language in
	// O(1) without depending on the optional
	// [node.Method.Owner] back-pointer. Reset every Annotate
	// call — per-run state, not cross-run cache.
	frontByMethod map[*node.Method]string
	frontByFunc   map[*node.Function]string
}

// New returns an empty umbrella [Plugin]. Configure it through
// [Plugin.Detectors] and [Plugin.Contracts] before passing to the
// pipeline:
//
//	pipe.Use(shape.New().
//	    Detectors(reader.Detector(), writer.Detector()).
//	    Contracts(persister.Contract(), saga.Contract()),
//	)
//
// Detector iteration order is the registration order; the first
// positive match wins. Honour ordering for shapes that overlap
// (e.g. Deleter must claim its signature before Writer falls
// back). Contract registration order is irrelevant — contracts
// are indexed by name.
func New() *Plugin { return &Plugin{} }

// Detectors registers one or more per-shape signature detectors
// with the plugin and sorts the cumulative list by [Detector.Priority]
// (descending) so the first-positive-match cascade honours the
// catalog ordering regardless of registration order. Returns the
// plugin so calls chain.
func (p *Plugin) Detectors(ds ...Detector) *Plugin {
	p.detectors = append(p.detectors, ds...)
	sort.SliceStable(p.detectors, func(i, j int) bool {
		return p.detectors[i].Priority > p.detectors[j].Priority
	})
	return p
}

// Contracts registers one or more named contracts with the
// plugin. Returns the plugin so calls chain.
//
// Contracts are looked up by [Contract.Name] when the
// `+gen:contract` directive is parsed; registering two contracts
// under the same name overwrites the earlier registration in
// favour of the latest call.
func (p *Plugin) Contracts(cs ...Contract) *Plugin {
	if p.contracts == nil {
		p.contracts = make(map[string]Contract, len(cs))
	}
	for _, c := range cs {
		p.contracts[c.Name] = c
	}
	return p
}

// Mixins registers one or more named mixins with the plugin.
// Returns the plugin so calls chain.
//
// Mixins are looked up by [Mixin.Name] when the `+gen:mixin`
// directive is parsed; registering two mixins under the same
// name overwrites the earlier registration in favour of the
// latest call.
func (p *Plugin) Mixins(ms ...Mixin) *Plugin {
	if p.mixins == nil {
		p.mixins = make(map[string]Mixin, len(ms))
	}
	for _, m := range ms {
		p.mixins[m.Name] = m
	}
	return p
}

// Name returns [PluginName].
func (*Plugin) Name() string { return PluginName }

// Priority places the plugin in the annotator-shape-detection
// bucket. The merged plugin runs every directive override and
// every detector in one pass, so override and detection share a
// single priority band.
func (*Plugin) Priority() sdk.Priority { return sdk.AnnotatorShape }

// Directives declares the `+gen:shape` and `+gen:contract`
// schemas. The framework's directive registry holds exactly one
// owner per directive name; registering this plugin twice in one
// pipeline surfaces as a duplicate-directive error at Build time.
func (*Plugin) Directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			Describe(
				"Pins the shape classification for the annotated callable. "+
					"Positional `name` (or `kind=<name>`) carries the canonical shape; "+
					"optional `key=<type>` and `value=<type>` populate the matching "+
					"shape.key_type / shape.value_type meta keys. User-supplied stamps "+
					"win over per-shape detector inference. The negated form is "+
					"rejected: shapes are inferred whether or not you ask, so "+
					"suppressing one is meaningful and is not implemented yet — "+
					"until it is, `-gen:shape` would parse and do nothing.",
			).
			Positional("name").
			AllowedKeys("kind", "key", "value").
			AllowExtraPositional().
			// Placeholder, not policy. Unlike contract and mixin, a
			// shape exists without anyone asking for it, so "do not
			// classify this" is a real thing to want and there is no
			// way to say it today — `+gen:shape` stamps whatever name
			// it is given. Denying is the reversible half: the form
			// errors loudly now, and lifting the denial later is
			// additive to every consumer.
			DenyNegation().
			Build(),
		sdk.NewDirective(ContractDirectiveName).
			Describe(
				"Declares the annotated callable's membership in a named "+
					"contract. Positional `name` carries the contract name; "+
					"mandatory `role=<role>` names this callable's role within "+
					"the contract; every other KV pair names a partner role and "+
					"the sibling callable filling it (resolved by the refinement "+
					"resolver into a qualified name). The negated form is "+
					"rejected: a membership exists only where one is declared, "+
					"so deleting the line is the suppression.",
			).
			Positional("name", sdk.Required()).
			RequiredKeys("role").
			// Permanent, unlike the shape schema's denial. Contract
			// membership comes only from reading these directives —
			// nothing is inferred — so there is never anything for a
			// negated form to suppress. Same reasoning as mixin.
			DenyNegation().
			Build(),
		sdk.NewDirective(MixinDirectiveName).
			Describe(
				"Attaches one or more mixins to the annotated callable. "+
					"Mixins are orthogonal invariant assertions that decorate "+
					"a callable on top of its structural shape (atomic, "+
					"idempotent, monotonic, ...), so a callable commonly "+
					"carries several. Every positional arg is a mixin name, "+
					"stamped in the order written — mixin parameters are "+
					"`key=value` only, so a bare token is read as another "+
					"name, never as a parameter. KV pairs are stamped under "+
					"the mixin's parameter namespace and are permitted only "+
					"when exactly one name is given — with several names the "+
					"owning mixin would be ambiguous, so split the "+
					"parameterised mixin onto its own line.",
			).
			Positional("name", sdk.Required()).
			AllowExtraPositional().
			DenyNegation().
			Build(),
	}
}

// Annotate delegates to the framework's annotator walk via
// [sdk.Walk]. The plugin only implements the hook methods the
// walker invokes — there is no plugin-local node-tree traversal,
// so the pipeline's single pass over the store covers every
// callable.
func (p *Plugin) Annotate(ctx *sdk.AnnotatorContext) error {
	return sdk.Walk(ctx, p)
}

// BeforeNodes builds the callable-to-frontend lookup maps once
// per Annotate call. Walking each [node.Package]'s own struct /
// interface / function slices avoids relying on the optional
// [node.Method.Owner] back-pointer the per-method buckets
// otherwise need to resolve a method's containing package.
func (p *Plugin) BeforeNodes(ctx *sdk.AnnotatorContext) {
	p.frontByMethod = make(map[*node.Method]string)
	p.frontByFunc = make(map[*node.Function]string)
	ctx.Reader.Packages().Each(func(pkg *node.Package) {
		front, _ := frontendMarker.Get(pkg.Meta())
		for _, s := range pkg.Structs {
			for _, m := range s.Methods {
				p.frontByMethod[m] = front
			}
		}
		for _, i := range pkg.Interfaces {
			for _, m := range i.Methods {
				p.frontByMethod[m] = front
			}
		}
		for _, fn := range pkg.Functions {
			p.frontByFunc[fn] = front
		}
	})
}

// OnMethod dispatches detection over every method in the store
// (struct- and interface-declared alike). Interface methods carry
// a nil [node.Method.Receiver]; detectors that care about the
// receiver shape must handle the absence explicitly.
func (p *Plugin) OnMethod(ctx *sdk.AnnotatorContext, m *node.Method) {
	p.handle(ctx, m, m.Meta(), m.Directives(), p.frontByMethod[m])
}

// OnFunction dispatches detection over every free function in the
// store.
func (p *Plugin) OnFunction(ctx *sdk.AnnotatorContext, fn *node.Function) {
	p.handle(ctx, fn, fn.Meta(), fn.Directives(), p.frontByFunc[fn])
}

// handle is the per-callable pipeline. Contract and mixin
// attachment stamps run unconditionally — they are orthogonal
// to structural shape and never collide with it. Structural-shape
// detection then follows the override-then-detect cascade with
// the already-stamped guard.
func (p *Plugin) handle(
	ctx *sdk.AnnotatorContext,
	n node.Node,
	bag *meta.Bag,
	dirs []*directive.Directive,
	front string,
) {
	sink := ctx.Diag.For(PluginName)
	p.applyContracts(bag, dirs)
	p.applyMixins(n, bag, dirs, sink)

	if IsStamped(bag) {
		return
	}
	if match, ok := matchFromDirective(dirs); ok {
		stamp(bag, match, PluginName+".directive")
		return
	}
	for _, d := range p.detectors {
		fn, ok := d.Detect[front]
		if !ok {
			continue
		}
		match, ok := fn(n)
		if !ok {
			continue
		}
		if match.Shape == "" {
			match.Shape = d.Name
		}
		stamp(bag, match, PluginName+"."+d.Name)
		return
	}
}

// matchFromDirective extracts a structural-shape stamp from the
// first `+gen:shape` directive on the callable. Returns
// `(Match{}, false)` when no usable directive is present.
// Validation of malformed directives surfaces as a positioned
// diagnostic from the framework's directive validator at Build
// time, not from this plugin at runtime.
//
// The negated guard is defence-in-depth for callers that build
// [directive.Directive] values directly; the schema denies the form,
// so it cannot arrive from parsed source. Skipping is deliberately
// not suppression — a negated directive that reached here would
// fall through to detection and be stamped anyway, which is why the
// schema rejects it rather than letting it read as a suppression
// that silently does nothing.
func matchFromDirective(dirs []*directive.Directive) (Match, bool) {
	for _, d := range dirs {
		if d == nil || d.Name != DirectiveName || d.Negated {
			continue
		}
		name := shapeNameFromDirective(d)
		if name == "" {
			continue
		}
		return Match{
			Shape:     name,
			KeyType:   d.KV["key"],
			ValueType: d.KV["value"],
		}, true
	}
	return Match{}, false
}

// shapeNameFromDirective returns the shape name declared by d,
// drawing from (in order): the first positional argument, the
// `kind=` KV value. Returns empty when neither form supplied a
// name.
func shapeNameFromDirective(d *directive.Directive) string {
	if len(d.Args) > 0 && d.Args[0] != "" {
		return d.Args[0]
	}
	return d.KV["kind"]
}

// stamp writes the Match across the structural-shape contract
// meta keys and any per-shape extras the detector returned via
// [Match.StringStamps] / [Match.ListStamps]. KeyType / ValueType
// stamps only land when the detector populated them — shapes
// without a key/value leave those keys absent so consumers can
// distinguish "no key by design" from "key happened to be empty".
func stamp(bag *meta.Bag, m Match, setBy string) {
	MetaShape.Set(bag, m.Shape, setBy)
	if m.KeyType != "" {
		MetaKeyType.Set(bag, m.KeyType, setBy)
	}
	if m.ValueType != "" {
		MetaValueType.Set(bag, m.ValueType, setBy)
	}
	for _, s := range m.StringStamps {
		s.Key.Set(bag, s.Value, setBy)
	}
	for _, s := range m.ListStamps {
		s.Key.Set(bag, s.Value, setBy)
	}
}

// frontendMarker is the cross-frontend provenance key every
// frontend stamps on its produced packages — the value carries
// the producing frontend's plugin name (`"golang"`,
// `"protobuf"`). [Plugin.BeforeNodes] reads it to pick the
// per-language detector.
//
// Declared here rather than imported from `frontend/protobuf`
// (which is in a different module) or `frontend/golang` (same
// reason) — [meta.EnsureKey] returns the canonical singleton
// regardless of which package registered it first.
//
//nolint:gochecknoglobals // cross-package registry-singleton key
var frontendMarker = meta.EnsureKey("frontend", meta.StringParser)
