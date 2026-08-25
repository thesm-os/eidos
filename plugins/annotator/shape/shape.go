// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape

import (
	"sort"

	"go.thesmos.sh/eidos/sdk"
)

// PluginName is the stable identifier the framework uses for the
// umbrella shape plugin in its plugin registry.
const PluginName = "shape"

// Version is the plugin's declared version. It composes into the
// pipeline's plugin fingerprint, which frontends fold into their cache
// keys — so bumping it invalidates a warm cache populated when this
// plugin behaved differently. A plugin that declares no version
// contributes an empty string and can never invalidate anything, which
// is a silent staleness bug waiting for its first behavioural change.
const Version = "1.0.0"

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
// plugin hands every callable (a [*sdk.Function] or [*sdk.Method])
// to the function, which composes its query from the language's own
// vocabulary — a Go detector destructures through
// [go.thesmos.sh/eidos/lang/golang.Callable] and asks with the
// predicates beside it.
//
// The vocabulary is the language package's, not this one's. Shape
// once carried its own Go helpers, and they answered by Go spelling
// alone where `lang/golang` answers as the union of the frontend's
// `go.*` stamp and the spelling — so a named error type the frontend
// had already identified read as not-an-error, and every detector
// keyed on it declined without saying why.
//
// A `(Match{}, false)` return is the permissive skip — the next
// registered detector gets a turn. Detectors return it freely.
type DetectFunc func(n sdk.Node) (Match, bool)

// Detector is one shape's contribution to the umbrella plugin: a
// canonical shape name plus a per-frontend detection function
// map. Per-shape sub-packages (Reader, Writer, …) export one of
// these from their public API; the consumer composes them into
// the umbrella plugin via [Plugin.Detectors].
//
// A shape that supports only Go registers
// `{golang.Language: detectGolang}`; adding a language is purely
// additive — register the new key alongside. The key is spelled
// through the language package's own constant rather than as a
// literal, so a detector cannot register under a name no frontend
// stamps and then silently never run.
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

	// Detect maps the language a package was written in — the name
	// its frontend stamped, read through [sdk.LanguageOf] — to the
	// detection function for that language. Declarations in any
	// language not in this map are skipped without stamping.
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
	Key   sdk.Key[string]
	Value string
}

// ListStamp is the list-typed counterpart to [StringStamp].
type ListStamp struct {
	Key   sdk.Key[[]string]
	Value []string
}

// Options carries the plugin's user-tunable settings.
//
// The plugin has no behaviour toggles today: what it recognises comes
// from the registered vocabulary, and what it stamps comes from the
// source. The struct exists so a future setting lands without
// changing the plugin's options surface.
type Options struct{}

// Plugin is the umbrella shape plugin. One instance per pipeline
// owns the `+gen:shape`, `+gen:contract` and `+gen:mixin` directive
// schemas and dispatches to every registered [Detector], [Contract]
// and [Mixin].
//
// The merged design means consumers register one plugin regardless of
// how many shapes, contracts or mixins their pipeline recognises, and
// the framework's "one owner per directive" rule is satisfied by
// construction.
//
// Zero value is unusable; go through [New] so the embedded
// [sdk.Holder] binds to the options field.
//
// # Concurrency
//
// The vocabulary is written by the registration methods during
// wiring and read by every Annotate afterwards. Nothing locks and
// nothing mutates once the pipeline is built — in particular no
// per-run state lives here, so two runs sharing one plugin cannot
// see each other's work.
type Plugin struct {
	*sdk.Base
	*sdk.Holder[Options]
	opts Options

	detectors []Detector
	contracts map[string]Contract
	mixins    map[string]Mixin
}

// New returns an umbrella [Plugin] with no vocabulary registered.
// Configure it through [Plugin.Detectors], [Plugin.Contracts] and
// [Plugin.Mixins] before passing it to the pipeline:
//
//	pipe.WithAnnotator(shape.New().
//	    Detectors(reader.Detector(), writer.Detector()).
//	    Contracts(persister.Contract(), saga.Contract()),
//	)
//
// A plugin registering nothing stamps nothing, which is a legitimate
// configuration — a pipeline wanting contracts and no structural
// shapes. Take [go.thesmos.sh/eidos/plugins/annotator/shape/full.New]
// for the whole catalog rather than assembling one by hand.
//
// The shape-detection bucket is where the merged plugin belongs: it
// runs every directive override and every detector in one pass, so
// override and detection share a single priority band.
//
// Nothing is provided or required. The annotator publishes its
// results as metadata keys rather than as a named capability, so
// nothing could usefully declare a dependency on the label; and
// ordering within the annotator phase comes from the bucket, where
// expressing it as a capability dependency instead would make
// registering the three shape plugins individually a hard error
// rather than a caller's choice. Both still have to be *answered*,
// because [sdk.CapabilityProvider] is all-or-nothing — declaring a
// bucket alone fails the pipeline's type assertion and collapses the
// plugin into the default bucket, discarding the ordering the bucket
// was declared to express. [sdk.Base] answers all three together,
// which is what makes that failure unreachable.
//
// No language is declared through [sdk.Builder.For]. This plugin
// ships no templates and emits no file, and the language a
// declaration is read with is the [Detector.Detect] key — which the
// registered detectors own, not the umbrella. Declaring Go here would
// claim a language on behalf of a vocabulary that might carry none.
func New() *Plugin {
	p := &Plugin{
		Base: sdk.NewPlugin(PluginName).
			Version(Version).
			Priority(sdk.AnnotatorShape).
			Directives(directives()...).
			Build(),
		contracts: map[string]Contract{},
		mixins:    map[string]Mixin{},
	}
	p.Holder = sdk.BindOptions(&p.opts)
	return p
}

// Detectors registers one or more per-shape signature detectors and
// sorts the cumulative list by [Detector.Priority] (descending) so
// the first-positive-match cascade honours the catalog ordering
// regardless of registration order. Returns the plugin so calls
// chain.
//
// The sort is what stops a permissive shape claiming a signature a
// more specific one owns — Writer before Deleter being the case the
// catalog encodes.
func (p *Plugin) Detectors(ds ...Detector) *Plugin {
	p.detectors = append(p.detectors, ds...)
	sort.SliceStable(p.detectors, func(i, j int) bool {
		return p.detectors[i].Priority > p.detectors[j].Priority
	})
	return p
}

// Contracts registers one or more named contracts. Returns the
// plugin so calls chain.
//
// Contracts are looked up by [Contract.Name] when the
// `+gen:contract` directive is read; registering two under one name
// takes the later, which makes a call chain a correction rather than
// a conflict.
func (p *Plugin) Contracts(cs ...Contract) *Plugin {
	for _, c := range cs {
		p.contracts[c.Name] = c
	}
	return p
}

// Mixins registers one or more named mixins. Returns the plugin so
// calls chain. Looked up by [Mixin.Name], with the same later-wins
// rule contracts follow.
func (p *Plugin) Mixins(ms ...Mixin) *Plugin {
	for _, m := range ms {
		p.mixins[m.Name] = m
	}
	return p
}

// directives declares the `+gen:shape`, `+gen:contract` and
// `+gen:mixin` schemas. The framework's directive registry holds
// exactly one owner per directive name; registering this plugin
// twice in one pipeline surfaces as a duplicate-directive error at
// Build time.
func directives() []sdk.DirectiveSchema {
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
					"the contract. Every other KV pair is read as a parameter "+
					"when the contract declares that key for this role, and as "+
					"a partner role plus the sibling callable filling it "+
					"otherwise; both are resolved to qualified names by the "+
					"refinement resolver. The negated form is "+
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

// Annotate dispatches detection over every callable, package by
// package.
//
// Read through [sdk.StoreReader] rather than the framework's
// [sdk.Walk] helper, which iterates the store directly. Reads the
// Reader captures compose the plugin's cache key; a read that goes
// around it is one the cache cannot invalidate on, so a source change
// leaves the fingerprint identical and the next run serves stamps
// derived from declarations that have since moved. Nothing reports
// it — the output is stale and looks current.
//
// Per package rather than over the flat buckets, because the language
// a declaration is read with is a fact about the package that
// produced it. Walking the packages hands each callable its language
// on the way past; the flat buckets do not, which is why this
// previously needed two callable-to-language maps built on the plugin
// once per run — per-run state on a value every phase shares.
//
// Interface methods carry a nil [sdk.Method.Receiver]; detectors that
// care about the receiver shape handle the absence explicitly.
//
// All four method-carrying declarations are walked. A struct and an
// interface are the obvious two; the other two are the ones a walk
// written from memory forgets. Go attaches methods to any defined
// type, so `type Weekday int` carries them on an [sdk.Alias] — and
// when a const block turns that same declaration into an [sdk.Enum],
// they move to the enum instead. Missing either leaves a callable
// unclassified with nothing to say it was skipped.
func (p *Plugin) Annotate(ctx *sdk.AnnotatorContext) error {
	for _, pkg := range ctx.Reader.Packages().Slice() {
		lang := sdk.LanguageOf(pkg)
		for _, s := range pkg.Structs {
			p.handleMethods(ctx, s.Methods, lang)
		}
		for _, i := range pkg.Interfaces {
			p.handleMethods(ctx, i.Methods, lang)
		}
		for _, e := range pkg.Enums {
			p.handleMethods(ctx, e.Methods, lang)
		}
		for _, a := range pkg.Aliases {
			p.handleMethods(ctx, a.Methods, lang)
		}
		for _, fn := range pkg.Functions {
			p.handle(ctx, fn, fn.EnsureMeta(), fn.Directives(), lang)
		}
	}
	return nil
}

// handleMethods dispatches over one declaration's method set.
func (p *Plugin) handleMethods(ctx *sdk.AnnotatorContext, ms []*sdk.Method, lang string) {
	for _, m := range ms {
		p.handle(ctx, m, m.EnsureMeta(), m.Directives(), lang)
	}
}

// handle is the per-callable pipeline. Contract and mixin
// attachment stamps run unconditionally — they are orthogonal
// to structural shape and never collide with it. Structural-shape
// detection then follows the override-then-detect cascade with
// the already-stamped guard.
func (p *Plugin) handle(
	ctx *sdk.AnnotatorContext,
	n sdk.Node,
	bag *sdk.Bag,
	dirs []*sdk.Directive,
	front string,
) {
	sink := ctx.Diag.For(PluginName)
	p.applyContracts(n, bag, dirs, sink)
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
// [sdk.Directive] values directly; the schema denies the form,
// so it cannot arrive from parsed source. Skipping is deliberately
// not suppression — a negated directive that reached here would
// fall through to detection and be stamped anyway, which is why the
// schema rejects it rather than letting it read as a suppression
// that silently does nothing.
func matchFromDirective(dirs []*sdk.Directive) (Match, bool) {
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
func shapeNameFromDirective(d *sdk.Directive) string {
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
func stamp(bag *sdk.Bag, m Match, setBy string) {
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
