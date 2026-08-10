// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape

import "go.thesmos.sh/eidos/sdk"

// ValidatorName is the stable identifier the framework uses for
// the contract-validation annotator that runs after the
// refinement resolver.
const ValidatorName = "shape.contract.validator"

// Validator is the validation-bucket companion to the umbrella
// plugin and the [Resolver]. It runs at
// [sdk.AnnotatorValidation] priority — strictly after the
// resolver — and enforces two kinds of contract invariants:
//
//   - **Required partners.** [Contract.Required] declares the
//     partner roles each self-role must supply; the validator
//     emits a positioned diagnostic for any missing partner key.
//   - **Custom invariants.** [Contract.Validate], when non-nil,
//     receives the resolved member set keyed by role and returns
//     [ContractViolation] entries the validator surfaces as
//     positioned diagnostics against the offending member.
//
// Construct via [Plugin.Validator] so the validator shares the
// umbrella plugin's contract registrations.
type Validator struct {
	contracts map[string]Contract
	mixins    map[string]Mixin

	// members accumulates the (callable, role, contract) triples
	// observed during the walk. Used by [Validator.AfterNodes] to
	// group callables into per-contract member sets before
	// invoking each [Contract.Validate] hook. Reset every
	// Annotate call.
	members map[string]map[string][]ContractMember

	// attachments accumulates one [MixinAttachment] per (mixin,
	// host) pair observed during the walk. Used by
	// [Validator.AfterNodes] to invoke each [Mixin.Validate]
	// hook. Reset every Annotate call.
	attachments map[string][]MixinAttachment
}

// Validator returns a fresh [Validator] sharing p's contract
// registrations. Register alongside the umbrella plugin and the
// resolver so the three run in priority order:
//
//	s := shape.New().Detectors(...).Contracts(...)
//	pipe.WithAnnotator(s)
//	pipe.WithAnnotator(s.Resolver())
//	pipe.WithAnnotator(s.Validator())
func (p *Plugin) Validator() *Validator {
	return &Validator{
		contracts: p.contracts,
		mixins:    p.mixins,
	}
}

// Annotators returns the three plugins that make up a complete shape
// registration, in the order the pipeline runs them.
//
// The registration is three instances rather than one, and getting it
// wrong is silent. Registering the umbrella alone still stamps shapes,
// so every structural assertion about the output passes — but partner
// names stay raw, and every [Contract.Required] declaration and
// [Mixin.Validate] hook goes unenforced. What a consumer loses is
// diagnostics, which is precisely the thing whose absence looks like
// success.
//
//	for _, a := range shape.New().Contracts(contracts.All()...).Annotators() {
//	    pipe.WithAnnotator(a)
//	}
//
// Order is the contract: the umbrella stamps, the resolver qualifies
// what it stamped, the validator checks the result. Each depends on
// its predecessor having run, and the priorities enforce that
// independently — this returns them in the same order so a caller
// reading the slice sees the sequence rather than having to know it.
func (p *Plugin) Annotators() []sdk.Annotator {
	return []sdk.Annotator{p, p.Resolver(), p.Validator()}
}

// Name returns [ValidatorName].
func (*Validator) Name() string { return ValidatorName }

// Priority places the validator in the annotator-validation
// bucket so it runs strictly after the resolver.
func (*Validator) Priority() sdk.Priority { return sdk.AnnotatorValidation }

// Provides returns nil: the contract validator publishes its results as metadata
// keys rather than as a named capability, so nothing can usefully
// declare a dependency on the label.
//
// The method exists because [plugin.CapabilityProvider] is an
// all-or-nothing interface — Priority, Provides and Requires
// together. Declaring Priority alone does not satisfy it, so the
// pipeline's type assertion fails and the plugin silently collapses
// into the default bucket, discarding the ordering Priority was
// declared to express.
func (*Validator) Provides() []string { return nil }

// Requires returns nil. Ordering within the annotator phase comes
// from [Validator.Priority]; expressing it as a capability
// dependency instead would make registering the plugins
// individually a hard error rather than a caller's choice.
func (*Validator) Requires() []string { return nil }

// Annotate delegates to the framework's annotator walk via
// [sdk.Walk]; per-callable required-partner checks live in
// [Validator.OnMethod] and [Validator.OnFunction]; per-contract
// member-set validation runs once in [Validator.AfterNodes].
func (v *Validator) Annotate(ctx *sdk.AnnotatorContext) error {
	return sdk.Walk(ctx, v)
}

// BeforeNodes resets the per-Annotate accumulators so the
// validator stays stateless across runs.
func (v *Validator) BeforeNodes(*sdk.AnnotatorContext) {
	v.members = make(map[string]map[string][]ContractMember)
	v.attachments = make(map[string][]MixinAttachment)
}

// OnMethod runs the required-partner check on m for every
// contract it participates in, and accumulates m into the
// member set keyed by (contract, role).
func (v *Validator) OnMethod(ctx *sdk.AnnotatorContext, m *sdk.Method) {
	v.visit(ctx, m, m.Meta())
}

// OnFunction runs the required-partner check on fn for every
// contract it participates in, and accumulates fn into the
// member set keyed by (contract, role).
func (v *Validator) OnFunction(ctx *sdk.AnnotatorContext, fn *sdk.Function) {
	v.visit(ctx, fn, fn.Meta())
}

// AfterNodes invokes each registered [Contract.Validate] and
// [Mixin.Validate] hook against its accumulated member /
// attachment set and surfaces any returned violations as
// positioned diagnostics on ctx.Diag.
func (v *Validator) AfterNodes(ctx *sdk.AnnotatorContext) {
	sink := ctx.Diag.For(ValidatorName)
	for contractName, members := range v.members {
		spec, ok := v.contracts[contractName]
		if !ok || spec.Validate == nil {
			continue
		}
		for _, violation := range spec.Validate(members) {
			sink.Errorf(posOf(violation.Host),
				"shape.contract %q: %s", contractName, violation.Message)
		}
	}
	for mixinName, attachments := range v.attachments {
		spec, ok := v.mixins[mixinName]
		if !ok || spec.Validate == nil {
			continue
		}
		for _, violation := range spec.Validate(attachments) {
			sink.Errorf(posOf(violation.Host),
				"shape.mixin %q: %s", mixinName, violation.Message)
		}
	}
}

// visit runs the per-callable required-partner check on contract
// memberships, accumulates host into the contract member set,
// and accumulates host's mixin attachments for the AfterNodes
// validator pass.
func (v *Validator) visit(ctx *sdk.AnnotatorContext, host sdk.Node, bag *sdk.Bag) {
	sink := ctx.Diag.For(ValidatorName)
	for _, contractName := range Contracts(bag) {
		spec, ok := v.contracts[contractName]
		if !ok {
			continue
		}
		role, _ := ContractRoleKey(spec.Name).Get(bag)
		v.checkRequired(host, bag, role, spec, sink)
		v.accumulate(spec, role, host, bag)
	}
	for _, mixinName := range Mixins(bag) {
		spec, ok := v.mixins[mixinName]
		if !ok {
			continue
		}
		v.accumulateMixin(spec, host, bag)
	}
}

// checkRequired emits a diagnostic for each partner role
// declared in spec.Required[role] that is missing a stamped
// partner key on bag.
func (*Validator) checkRequired(
	host sdk.Node,
	bag *sdk.Bag,
	role string,
	spec Contract,
	sink *sdk.PluginSink,
) {
	required, ok := spec.Required[role]
	if !ok {
		return
	}
	for _, partnerRole := range required {
		got, _ := ContractPartnerKey(spec.Name, partnerRole).Get(bag)
		if got != "" {
			continue
		}
		sink.Errorf(host.Pos(),
			"shape.contract %q: role %q requires partner %q, none stamped",
			spec.Name, role, partnerRole)
	}
}

// accumulate records host as a member of (spec.Name, role) in
// the per-contract member set, snapshotting the host's partner
// stamps into a [ContractMember] so [Contract.Validate] can read
// the pairings directly. Roles are deduplicated by host pointer
// so the same callable joining a contract twice (via self-stamp
// + back-stamp) appears once per role.
func (v *Validator) accumulate(spec Contract, role string, host sdk.Node, bag *sdk.Bag) {
	byRole, ok := v.members[spec.Name]
	if !ok {
		byRole = make(map[string][]ContractMember)
		v.members[spec.Name] = byRole
	}
	for _, existing := range byRole[role] {
		if existing.Host == host {
			return
		}
	}
	partners := make(map[string]string)
	for _, partnerRole := range spec.Roles {
		if partnerRole == role {
			continue
		}
		if v, ok := ContractPartnerKey(spec.Name, partnerRole).Get(bag); ok && v != "" {
			partners[partnerRole] = v
		}
	}
	byRole[role] = append(byRole[role], ContractMember{Host: host, Partners: partners})
}

// accumulateMixin records host's attachment to spec, snapshotting
// the mixin's declared params from bag so [Mixin.Validate] can
// read them without re-walking the meta. Deduplicated by host
// pointer.
func (v *Validator) accumulateMixin(spec Mixin, host sdk.Node, bag *sdk.Bag) {
	for _, existing := range v.attachments[spec.Name] {
		if existing.Host == host {
			return
		}
	}
	params := make(map[string]string)
	for _, p := range spec.Params {
		if val, ok := MixinParamKey(spec.Name, p).Get(bag); ok && val != "" {
			params[p] = val
		}
	}
	v.attachments[spec.Name] = append(v.attachments[spec.Name],
		MixinAttachment{Host: host, Params: params})
}

// posOf returns n's source position via the [sdk.Node.Pos]
// method. Defensive guard: nil hosts produce a zero position.
func posOf(n sdk.Node) sdk.Pos {
	if n == nil {
		return sdk.Pos{}
	}
	return n.Pos()
}
