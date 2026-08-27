// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape

import (
	"maps"
	"slices"
	"strings"

	"go.thesmos.sh/eidos/sdk"
)

// Mixin is one orthogonal invariant assertion that decorates a
// callable on top of its structural shape. The mental model is
// layered: the callable's structural [shape] picks the base laws
// the downstream test suite asserts (e.g. Writer →
// "write-then-observe"); each mixin adds one more invariant
// (atomic, idempotent, monotonic, …).
//
// Unlike [Contract], a mixin is a per-callable stamp — it does
// not bind multiple callables together. Mixin directives may
// still carry KV parameters; the values are opaque to the
// umbrella plugin (use a custom resolver if a param value names
// a sibling callable that needs qname rewriting).
//
// Mixin sub-packages export one of these from their public API;
// the consumer composes them into the umbrella plugin via
// [Plugin.Mixins].
type Mixin struct {
	// Name is the mixin's stable identifier (e.g. `"atomic"`,
	// `"idempotent"`, `"monotonic"`). Used as the directive-name
	// argument to `+gen:mixin` and as a path component in every
	// per-mixin param meta key.
	Name string

	// Params enumerates the KV keys the directive accepts, each with
	// how its value resolves — see [ParamKind]. A key with no
	// declaration to resolve against is [KindOpaque], which is the zero
	// value.
	Params []Param

	// Documentary marks a classification that carries information
	// rather than an invariant: it decorates a declaration for a
	// reader or a downstream generator and licenses no assertion.
	//
	// The field exists because the judgement was already being made
	// and could only travel as prose. `errors` says outright that a
	// consumer "should not report this mixin as an underivable gap;
	// it derives nothing by design", and `scope` that it "owes
	// documentation, not a check" — so a consumer listing the
	// classifications no rule reached had to transcribe those
	// sentences into a local table, and a classification marked here
	// later became a reported gap downstream that no rule could ever
	// close.
	//
	// It is not "no rule exists yet". That is the state this
	// separates itself from: an unmarked classification nothing
	// checks is a gap a rule could close, and a marked one is a
	// silence that is owed. A consumer reporting coverage needs both
	// answers and can derive neither.
	//
	// The zero value keeps every classification claimable, so this
	// is additive: a catalog entry is marked when its own package
	// decides it states no invariant.
	Documentary bool

	// Validate, when non-nil, runs in the validation-bucket
	// annotator pass after sibling resolution completes. Receives
	// every callable the mixin is attached to (across the store)
	// as [MixinAttachment] values. Use for invariants like
	// "every readafterwrite's write partner resolves to a known
	// callable".
	Validate MixinValidator
}

// MixinValidator is the signature of the optional per-mixin
// invariant check the [Validator] annotator runs after sibling
// resolution. Returns the list of violations found (empty / nil
// on success); the validator attaches each violation to its host
// node's diagnostic sink.
type MixinValidator func(attachments []MixinAttachment) []MixinViolation

// MixinAttachment is one callable's attachment to a mixin, as
// observed by the validator after sibling resolution. The
// validator iterates this list to correlate the mixin's params
// across the store.
type MixinAttachment struct {
	// Host is the callable the mixin is attached to.
	Host sdk.Node

	// Params maps the mixin's KV parameter keys to their stamped
	// values. Sibling-param values are qualified names after the
	// resolver runs; literal-param values are the raw strings.
	Params map[string]string
}

// MixinViolation is one invariant breach reported by a
// [MixinValidator]. The validator surfaces it as a positioned
// diagnostic against the host node.
type MixinViolation struct {
	// Host is the node the diagnostic attaches to.
	Host sdk.Node

	// Message is the human-readable violation summary.
	Message string
}

// MixinDirectiveName is the `+gen:` directive consumers write to
// attach a mixin to a callable.
//
// Syntax:
//
//	//+gen:mixin <name> [<param>=<value>]…
//
// Examples:
//
//	//+gen:mixin atomic
//	//+gen:mixin idempotent
//	//+gen:mixin readafterwrite write=Save
//	//+gen:mixin rate-limited limit=100 burst=10
//
// Multiple `+gen:mixin` directives on one callable stack: each
// one appends to [MetaMixins] and stamps its parameters under
// its own per-mixin namespace.
const MixinDirectiveName = sdk.DirectiveName("mixin")

// MetaMixins is the per-callable list of mixins decorating the
// callable. Populated by [Plugin.applyMixins] each time a
// non-negated `+gen:mixin` directive is recognised; consumers
// iterate this list to discover every mixin attached to the
// callable, then read the per-mixin param keys.
//
//nolint:gochecknoglobals // registry-singleton key
var MetaMixins = sdk.EnsureKey("shape.mixins", sdk.StringListParser)

// MixinParamKey returns the typed meta key carrying a mixin's
// KV parameter value — stamped at `shape.mixin.<name>.<param>`.
// Constructed on demand via [sdk.EnsureKey] so multiple
// per-mixin sub-packages referencing the same name resolve to
// one canonical key.
func MixinParamKey(name, param string) sdk.Key[string] {
	return sdk.EnsureKey(
		"shape.mixin."+name+"."+param,
		sdk.StringParser,
	)
}

// mixinStampedBy is the setBy attribution used for every
// mixin-related stamp this plugin writes. Distinct from
// [PluginName] so meta provenance distinguishes structural-shape
// stamps from mixin-attachment stamps.
const mixinStampedBy = PluginName + ".mixin"

// applyMixins stamps every `+gen:mixin` directive on bag. A
// directive may name several mixins; each is stamped in the order
// written. A name with no registered [Mixin] is reported and
// skipped without affecting the other names on the same line.
// Unknown parameter keys are still stamped verbatim so a resolver
// has the raw data needed to diagnose them.
//
// The function is otherwise permissive — the framework's directive
// validator enforces the schema (mandatory name, negation, KV
// shape) at Build time — with one exception it owns outright:
// parameters paired with several names. That constraint is
// conditional on the arg count, which [sdk.DirectiveSchema] cannot
// express, and it has to be checked while the directive is intact,
// so it lives here. See [Plugin.reportAmbiguousMixinParams].

func (p *Plugin) applyMixins(
	host sdk.Node, bag *sdk.Bag, dirs []*sdk.Directive, sink *sdk.PluginSink,
) {
	for _, d := range dirs {
		// Negation is denied by the schema and a missing name is
		// rejected as a mandatory positional, so both guards are
		// defence-in-depth for callers that bypass validation.
		if d == nil || d.Name != MixinDirectiveName || d.Negated {
			continue
		}
		if len(d.Args) == 0 {
			continue
		}
		// KV ownership is only well-defined for a single name. The
		// schema cannot express "keys allowed only when exactly one
		// positional", so the rule is enforced here, where the
		// directive is still intact — by the time the validator pass
		// runs, names have been flattened into [MetaMixins] and the
		// key-to-name association is gone.
		params := d.KV
		if len(params) > 0 && len(d.Args) > 1 {
			p.reportAmbiguousMixinParams(host, d, sink)
			params = nil
		}
		for _, name := range d.Args {
			spec, registered := p.mixins[name]
			if !registered {
				// Report and skip this name only — an unregistered
				// name must not discard the rest of the line.
				//
				// This is the only signal a mistyped name or a stray
				// token gets. Mixin parameters are KV-only, so a
				// positional written as a parameter
				// (`+gen:mixin bounded 100`) parses as a second name
				// and surfaces here rather than as an arity error.
				reportUnregisteredMixin(host, name, sink)
				continue
			}
			accepted := paramSet(ParamKeys(spec.Params))
			for k, v := range params {
				if v == "" {
					continue
				}
				if _, declared := accepted[k]; !declared {
					reportUnknownMixinParam(host, spec, k, sink)
					continue
				}
				MixinParamKey(name, k).Set(bag, v, mixinStampedBy)
			}
			appendMixin(bag, name)
		}
	}
}

// reportUnregisteredMixin emits the diagnostic for a mixin name
// this pipeline has no [Mixin] registered for: a name nobody can
// interpret is an authoring mistake, not a silent no-op.
//
// Reported here and not by the resolver, and the reason is structural
// rather than a division of labour. The name is not stamped, so a
// resolver iterating stamped attachments has nothing to iterate — the
// only pass that can see an unregistered name is the one that
// declined to stamp it. [reportUnregisteredContract] carries the same
// argument for the same reason.
func reportUnregisteredMixin(host sdk.Node, name string, sink *sdk.PluginSink) {
	sink.Errorf(host.Pos(),
		"shape.mixin: %q is not registered with this pipeline. Check the spelling, "+
			"register the mixin, or — if it was meant as a parameter — note that mixin "+
			"parameters are `key=value` only; a bare token is read as another mixin name",
		name)
}

// reportAmbiguousMixinParams emits the diagnostic for a directive
// that pairs KV parameters with several mixin names. The names are
// still attached — dropping them would lose information the author
// clearly intended — but the parameters are discarded, because
// guessing an owner would fabricate meta under a namespace the
// other mixins never declared.
func (*Plugin) reportAmbiguousMixinParams(
	host sdk.Node, d *sdk.Directive, sink *sdk.PluginSink,
) {
	keys := slices.Sorted(maps.Keys(d.KV))
	sink.Errorf(host.Pos(),
		"shape.mixin: %d parameter(s) %v supplied with %d mixin names %v; "+
			"parameters belong to exactly one mixin — put %q on its own line. "+
			"The names were attached; the parameters were dropped",
		len(keys), keys, len(d.Args), d.Args, d.Args[0],
	)
}

// appendMixin adds name to the [MetaMixins] list on bag,
// preserving insertion order and skipping duplicates. Idempotent:
// repeated calls with the same name leave the list unchanged.
func appendMixin(bag *sdk.Bag, name string) {
	current, _ := MetaMixins.Get(bag)
	if slices.Contains(current, name) {
		return
	}
	MetaMixins.Set(bag, append(current, name), mixinStampedBy)
}

// Mixins returns the mixins attached to the callable, in
// insertion order. Returns empty when the callable carries no
// mixin attachments.
//
// Consumers wanting per-mixin parameter data combine the list
// with [MixinParamKey]:
//
//	for _, name := range shape.Mixins(m.Meta()) {
//	    limit, _ := shape.MixinParamKey(name, "limit").Get(m.Meta())
//	    // …
//	}
func Mixins(bag *sdk.Bag) []string {
	if bag == nil {
		return nil
	}
	out, _ := MetaMixins.Get(bag)
	return out
}

// reportUnknownMixinParam emits the diagnostic for a KV key the named
// mixin does not declare, and declines to stamp it.
//
// A mixin can refuse an unknown key where a [Contract] cannot, and the
// asymmetry is a property of the two vocabularies rather than an
// oversight in one. A contract's undeclared keys are partner
// references — a second, open vocabulary keyed by role — so it routes
// what it does not recognise instead of rejecting it. A mixin has no
// such second reading: every key is either declared or a typo.
//
// Silence here was the worse half of that asymmetry. Every parameter
// is a claim something downstream binds against, so a misspelled key
// stamps into a namespace nobody reads and un-arms exactly one check
// while the run stays green — which is the failure a classifier must
// not have, because its output is a promise about what was verified.
func reportUnknownMixinParam(host sdk.Node, spec Mixin, key string, sink *sdk.PluginSink) {
	accepted := ParamKeys(spec.Params)
	if len(accepted) == 0 {
		sink.Errorf(host.Pos(),
			"shape.mixin %q: %q is not a parameter it accepts; this mixin takes none",
			spec.Name, key)
		return
	}
	sink.Errorf(host.Pos(),
		"shape.mixin %q: %q is not a parameter it accepts. Declared: %s",
		spec.Name, key, strings.Join(accepted, ", "))
}
