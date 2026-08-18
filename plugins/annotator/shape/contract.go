// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape

import (
	"slices"

	"go.thesmos.sh/eidos/sdk"
)

// Contract is one named multi-callable protocol with a fixed role
// vocabulary. Per-protocol sub-packages (`persister`, `tx`,
// `saga`, `pool`, …) export one of these from their public API;
// the consumer composes them into the umbrella plugin via
// [Plugin.Contracts].
//
// Contract is orthogonal to [Detector]: a callable can carry both
// a structural shape (from a detector) and one or more contract
// memberships (from `+gen:contract` directives) without either
// overwriting the other.
type Contract struct {
	// Name is the contract's stable identifier (e.g. `"tx"`,
	// `"persister"`, `"saga"`). Used as the contract-name argument
	// to the `+gen:contract` directive and as a path component in
	// every per-contract meta key.
	Name string

	// Roles enumerates the role vocabulary the contract recognises
	// (e.g. `["begin", "commit", "rollback"]` for tx;
	// `["writer", "reader"]` for persister). The refinement
	// resolver rejects directives that name an undeclared role.
	Roles []string

	// Params enumerates the KV keys the directive accepts, each with
	// how its value resolves — see [ParamKind]. A key with no
	// declaration to resolve against is [KindOpaque], which is the zero
	// value, so `{Key: "version"}` reads as a literal reference.
	//
	// Params land under `shape.contract.<name>.param.<key>`.
	Params []Param

	// Required maps a role to the partner roles that must be
	// specified when this role's directive appears. The refinement
	// resolver emits a positioned diagnostic when any required
	// partner is missing from the directive.
	//
	// Leave empty when no per-role partner-presence requirements
	// apply (e.g. for the simple two-member persister contract
	// where the structural-shape detector already enforces
	// signature shape on each side).
	Required map[string][]string

	// Validate, when non-nil, runs in the validation-priority
	// annotator pass after sibling resolution completes. Receives
	// the per-contract-instance member set keyed by role and
	// returns any invariants the implementation violated. Use for
	// structural checks like "Pool has exactly one Get and one
	// Put" or "every Saga step has a compensation".
	Validate ContractValidator
}

// ContractValidator is the signature of the optional per-contract
// invariant check the [Validator] annotator runs after sibling
// resolution completes. Receives the per-contract-instance member
// set grouped by role; each [ContractMember] carries the host
// callable plus the qualified-name partner stamps the resolver
// already rewrote, so the validator can correlate pairings (e.g.
// which saga step paired with which compensate) without re-walking
// the meta bag itself. Returns the list of violations found
// (empty / nil on success); the validator annotator attaches each
// violation to its host node's diagnostic sink.
type ContractValidator func(members map[string][]ContractMember) []ContractViolation

// ContractMember is one callable's participation in a resolved
// contract instance, as the validator sees it after sibling
// resolution. The validator picks up the host node plus a partner
// pointer map keyed by partner role.
type ContractMember struct {
	// Host is the callable participating in the contract.
	Host sdk.Node

	// Partners maps partner role names to the resolved qualified
	// name of the callable filling that role for this specific
	// host. Empty values mean the partner stamp was not set on
	// this host (the Required check, when configured, surfaces
	// the omission separately).
	Partners map[string]string

	// Params maps the contract's declared KV keys to their stamped
	// values for this host — qualified names for the resolvable
	// kinds, raw strings for [KindOpaque]. Absent keys are omitted
	// rather than present-and-empty, so a validator tests presence
	// with the comma-ok form.
	//
	// [Contract.Required] covers partner roles only. A contract
	// demanding a param — the keys a [Param.Role] scopes to one arm
	// are the case — enforces it from [Contract.Validate], and
	// snapshotting here is what lets that hook read the directive
	// without reaching back through [Node.Meta] to re-walk the bag
	// this member already summarises. Mirrors [MixinAttachment.Params].
	Params map[string]string
}

// ContractViolation is one invariant breach reported by a
// [ContractValidator]. The [Validator] annotator surfaces it as
// a positioned diagnostic against the host node.
type ContractViolation struct {
	// Host is the node the diagnostic attaches to. Pick the
	// member that most directly demonstrates the failure (e.g.
	// the orphan Get when "no matching Put" is violated).
	Host sdk.Node

	// Message is the human-readable violation summary.
	Message string
}

// MetaContracts is the per-callable list of contracts the callable
// participates in. Populated by [Plugin.applyContracts] each time a
// non-negated `+gen:contract` directive is recognised; consumers
// iterate this list to discover every contract the callable is
// part of, then read the per-contract role + partner keys.
//
//nolint:gochecknoglobals // registry-singleton key
var MetaContracts = sdk.EnsureKey("shape.contracts", sdk.StringListParser)

// ContractRoleKey returns the typed meta key carrying the
// callable's role within the named contract — stamped at
// `shape.contract.<name>.role`. Constructed on demand via
// [sdk.EnsureKey] so multiple per-contract sub-packages
// referencing the same name resolve to one canonical key.
func ContractRoleKey(name string) sdk.Key[string] {
	return sdk.EnsureKey("shape.contract."+name+".role", sdk.StringParser)
}

// ContractPartnerKey returns the typed meta key carrying the
// partner callable filling the named role within the contract —
// stamped at `shape.contract.<contract>.partner.<role>`. The
// stamped value is a raw sibling name as the umbrella plugin
// records it and a qualified name after the refinement resolver
// rewrites it.
func ContractPartnerKey(contract, role string) sdk.Key[string] {
	return sdk.EnsureKey(
		"shape.contract."+contract+".partner."+role,
		sdk.StringParser,
	)
}

// ContractParamKey returns the typed meta key carrying a directive
// parameter value — stamped at
// `shape.contract.<contract>.param.<key>`. Used for KV pairs
// declared in [Contract.Params].
//
// The stamped value is the raw string for a [KindOpaque] key and a
// qualified name once the refinement resolver has rewritten a
// resolvable one, so a consumer running after the refinement bucket
// reads qualified names for every kind but [KindOpaque].
func ContractParamKey(contract, key string) sdk.Key[string] {
	return sdk.EnsureKey(
		"shape.contract."+contract+".param."+key,
		sdk.StringParser,
	)
}

// contractStampedBy is the setBy attribution used for every
// contract-related stamp this plugin writes. Distinct from
// [PluginName] so meta provenance distinguishes structural-shape
// stamps from contract-membership stamps.
const contractStampedBy = PluginName + ".contract"

// applyContracts stamps every non-negated `+gen:contract` directive
// on bag. Unknown contract names are silently skipped — the
// refinement resolver surfaces them as positioned
// diagnostics. Unknown KV keys besides reserved ones are stamped
// verbatim as partner refs so the resolver has the raw data
// needed to diagnose them.
//
// The function is permissive because the framework's directive
// validator handles schema-level enforcement (missing `role=`,
// malformed positional, etc.) at Build time; this pass concerns
// itself only with meta stamping for callables whose directives
// already passed parse-time validation.
func (p *Plugin) applyContracts(
	host sdk.Node, bag *sdk.Bag, dirs []*sdk.Directive, sink *sdk.PluginSink,
) {
	for _, d := range dirs {
		// The negated guard is defence-in-depth: the schema denies
		// that form, so it cannot arrive from parsed source.
		if d == nil || d.Name != ContractDirectiveName || d.Negated {
			continue
		}
		name := contractNameFromDirective(d)
		if name == "" {
			continue
		}
		spec, registered := p.contracts[name]
		if !registered {
			reportUnregisteredContract(host, name, sink)
			continue
		}
		role := d.KV["role"]
		if role == "" {
			continue
		}
		ContractRoleKey(name).Set(bag, role, contractStampedBy)
		// Role-scoped: a key declared for another role is not a param
		// here, so it falls through to the partner branch and keeps
		// whatever meaning it has under this role.
		params := paramSet(ParamKeys(ParamsForRole(spec.Params, role)))
		for k, v := range d.KV {
			if k == "role" || v == "" {
				continue
			}
			if _, isParam := params[k]; isParam {
				ContractParamKey(name, k).Set(bag, v, contractStampedBy)
				continue
			}
			ContractPartnerKey(name, k).Set(bag, v, contractStampedBy)
		}
		appendContract(bag, name)
	}
}

// paramSet builds a lookup set from a [Contract.Params] slice for
// O(1) "is this KV key a param?" checks inside the stamping loop.
// Returns an empty (but non-nil) map when params is empty so the
// caller can skip a nil check.
func paramSet(params []string) map[string]struct{} {
	out := make(map[string]struct{}, len(params))
	for _, p := range params {
		out[p] = struct{}{}
	}
	return out
}

// contractNameFromDirective returns the contract name declared by
// d — the first positional argument. Returns empty when no
// positional was supplied; the schema marks the slot Required, so
// that cannot arrive from parsed source, and the guard covers
// callers that build [sdk.Directive] values directly.
func contractNameFromDirective(d *sdk.Directive) string {
	if len(d.Args) > 0 {
		return d.Args[0]
	}
	return ""
}

// appendContract adds name to the [MetaContracts] list on bag,
// preserving insertion order and skipping duplicates. Idempotent:
// repeated calls with the same name leave the list unchanged.
func appendContract(bag *sdk.Bag, name string) {
	current, _ := MetaContracts.Get(bag)
	if slices.Contains(current, name) {
		return
	}
	MetaContracts.Set(bag, append(current, name), contractStampedBy)
}

// Contracts returns the contracts the callable participates in,
// in insertion order. Returns empty when the callable carries no
// contract memberships.
//
// Consumers wanting per-contract role + partner data combine the
// list with [ContractRoleKey] and [ContractPartnerKey]:
//
//	for _, name := range shape.Contracts(m.Meta()) {
//	    role, _ := shape.ContractRoleKey(name).Get(m.Meta())
//	    // …
//	}
func Contracts(bag *sdk.Bag) []string {
	if bag == nil {
		return nil
	}
	out, _ := MetaContracts.Get(bag)
	return out
}

// reportUnregisteredContract emits the diagnostic for a contract name
// this pipeline has no [Contract] registered for.
//
// A contract name selects an entire law family downstream, so a
// mistyped one drops every member of it while a consumer's output
// still lists the callable as classified. Skipping it silently made
// the two vocabularies disagree one line apart: the same typo in a
// mixin name has always been reported.
//
// Reported here rather than by the resolver, which cannot see it: the
// name is not stamped, so a pass iterating stamped memberships has
// nothing to find. The only pass that can report an unregistered name
// is the one that declined to stamp it.
func reportUnregisteredContract(host sdk.Node, name string, sink *sdk.PluginSink) {
	sink.Errorf(host.Pos(),
		"shape.contract: %q is not registered with this pipeline. Check the "+
			"spelling or register the contract; an unregistered name classifies "+
			"nothing, so every law selected by it is silently dropped",
		name)
}
