// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape

import (
	"maps"
	"slices"

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

	// Params enumerates the KV parameter names the mixin accepts.
	// Exported for documentation and future validation; the
	// umbrella plugin's stamping is permissive and accepts any
	// KV (unknown keys still stamp).
	Params []string

	// SiblingParams enumerates param keys whose VALUES are sibling
	// callable names the refinement resolver should rewrite into
	// qualified names — e.g. `readafterwrite write=Save` has
	// `SiblingParams: []string{"write"}` so the resolver looks
	// `Save` up in scope and rewrites the stamp to its qname.
	//
	// Leave empty when every param is an opaque literal.
	SiblingParams []string

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
			if _, registered := p.mixins[name]; !registered {
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
			for k, v := range params {
				if v == "" {
					continue
				}
				MixinParamKey(name, k).Set(bag, v, mixinStampedBy)
			}
			appendMixin(bag, name)
		}
	}
}

// reportUnregisteredMixin emits the diagnostic for a mixin name
// this pipeline has no [Mixin] registered for. Mirrors the
// resolver's treatment of an unregistered contract: a name nobody
// can interpret is an authoring mistake, not a silent no-op.
//
// The name is not stamped, so downstream consumers never observe a
// mixin the pipeline cannot describe.
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
