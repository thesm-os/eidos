// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape

import (
	"slices"
	"strings"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/sdk"
)

// ResolverName is the stable identifier the framework uses for
// the contract-resolution refinement plugin.
const ResolverName = "shape.contract.resolver"

// Resolver is the refinement-bucket companion to the umbrella
// shape plugin. It runs in the [sdk.AnnotatorRefinement] bucket
// (one priority band above [sdk.AnnotatorShape]) and consumes
// the raw partner-name stamps the umbrella plugin left behind:
//
//   - Validates each callable's contract membership (role,
//     partner roles) against the registered [Contract.Roles] and
//     emits positioned diagnostics for unknown roles or
//     unregistered contracts.
//   - Looks up every partner reference in the host callable's
//     scope (struct- or interface-bound for methods, package-
//     bound for free functions) and rewrites the partner meta
//     value from the raw source name into a qualified name.
//   - Back-stamps the resolved partner with the contract
//     membership it was referenced by, including the reverse
//     partner pointer to the originating callable.
//
// The Resolver shares its [Contract] specs with the umbrella
// plugin — construct one via [Plugin.Resolver].
type Resolver struct {
	contracts map[string]Contract
	mixins    map[string]Mixin

	// Per-Annotate scope indexes built by [Resolver.BeforeNodes]
	// and consumed by the per-callable hooks. Reset every
	// Annotate call.
	methodOwner map[*node.Method]methodOwner
	funcPkg     map[*node.Function]*node.Package
}

// methodOwner is the per-method index entry: the owning struct or
// interface plus its qualified name (cached so qname composition
// during back-stamping is O(1)).
type methodOwner struct {
	qname   string
	methods []*node.Method
}

// Resolver returns a fresh [Resolver] sharing p's contract
// registrations. Register the resolver alongside the umbrella so
// it runs in the refinement bucket after the umbrella plugin:
//
//	s := shape.New().Detectors(...).Contracts(...)
//	pipe.Use(s)
//	pipe.Use(s.Resolver())
func (p *Plugin) Resolver() *Resolver {
	return &Resolver{
		contracts: p.contracts,
		mixins:    p.mixins,
	}
}

// Name returns [ResolverName].
func (*Resolver) Name() string { return ResolverName }

// Priority places the resolver in the annotator-refinement bucket
// so it runs strictly after the umbrella plugin's stamping pass.
func (*Resolver) Priority() sdk.Priority { return sdk.AnnotatorRefinement }

// Provides returns nil: the partner resolver publishes its results as metadata
// keys rather than as a named capability, so nothing can usefully
// declare a dependency on the label.
//
// The method exists because [plugin.CapabilityProvider] is an
// all-or-nothing interface — Priority, Provides and Requires
// together. Declaring Priority alone does not satisfy it, so the
// pipeline's type assertion fails and the plugin silently collapses
// into the default bucket, discarding the ordering Priority was
// declared to express.
func (*Resolver) Provides() []string { return nil }

// Requires returns nil. Ordering within the annotator phase comes
// from [Resolver.Priority]; expressing it as a capability
// dependency instead would make registering the plugins
// individually a hard error rather than a caller's choice.
func (*Resolver) Requires() []string { return nil }

// Annotate delegates to the framework's annotator walk via
// [sdk.Walk]; per-callable resolution lives in [Resolver.OnMethod]
// and [Resolver.OnFunction].
func (r *Resolver) Annotate(ctx *sdk.AnnotatorContext) error {
	return sdk.Walk(ctx, r)
}

// BeforeNodes builds the scope-lookup indexes the per-callable
// hooks consult when resolving partner names. Indexed each
// Annotate call from the live store so the resolver remains
// stateless across runs.
func (r *Resolver) BeforeNodes(ctx *sdk.AnnotatorContext) {
	r.methodOwner = make(map[*node.Method]methodOwner)
	r.funcPkg = make(map[*node.Function]*node.Package)
	ctx.Reader.Packages().Each(func(pkg *node.Package) {
		for _, s := range pkg.Structs {
			owner := methodOwner{qname: s.QName(), methods: s.Methods}
			for _, m := range s.Methods {
				r.methodOwner[m] = owner
			}
		}
		for _, i := range pkg.Interfaces {
			owner := methodOwner{qname: i.QName(), methods: i.Methods}
			for _, m := range i.Methods {
				r.methodOwner[m] = owner
			}
		}
		for _, fn := range pkg.Functions {
			r.funcPkg[fn] = pkg
		}
	})
}

// OnMethod resolves contract memberships and mixin sibling
// params on m using the owning struct or interface as the
// partner-lookup scope.
func (r *Resolver) OnMethod(ctx *sdk.AnnotatorContext, m *node.Method) {
	owner := r.methodOwner[m]
	scope := methodScope(owner)
	hostQName := methodQName(owner.qname, m.Name)
	r.resolve(ctx, m, m.Meta(), hostQName, scope)
	r.resolveMixins(ctx, m, m.Meta(), scope)
}

// OnFunction resolves contract memberships and mixin sibling
// params on fn using fn's containing package's functions as the
// partner-lookup scope.
func (r *Resolver) OnFunction(ctx *sdk.AnnotatorContext, fn *node.Function) {
	scope := packageScope(r.funcPkg[fn])
	r.resolve(ctx, fn, fn.Meta(), fn.QName(), scope)
	r.resolveMixins(ctx, fn, fn.Meta(), scope)
}

// resolveScope is the abstract sibling-lookup the per-callable
// hooks pass to [Resolver.resolve]. Implementations close over
// the appropriate slice (owner methods or package functions);
// each implementation returns the qualified name of the sibling
// matching name, or the empty string when no sibling matches.
type resolveScope func(name string) string

// methodScope returns a [resolveScope] that searches owner's
// method list for a name match and returns the qualified name on
// hit. Returns the empty-scope sentinel (always misses) when
// owner has no recorded qname — defensive guard for fixtures
// where Method.Owner wasn't wired.
func methodScope(owner methodOwner) resolveScope {
	if owner.qname == "" {
		return emptyScope
	}
	return func(name string) string {
		for _, m := range owner.methods {
			if m.Name == name {
				return methodQName(owner.qname, m.Name)
			}
		}
		return ""
	}
}

// packageScope returns a [resolveScope] that searches pkg's
// free-function list for a name match and returns the qualified
// name on hit. Returns the empty-scope sentinel when pkg is nil.
func packageScope(pkg *node.Package) resolveScope {
	if pkg == nil {
		return emptyScope
	}
	return func(name string) string {
		for _, fn := range pkg.Functions {
			if fn.Name == name {
				return fn.QName()
			}
		}
		return ""
	}
}

// emptyScope is the resolve-scope sentinel used when the host
// callable has no resolvable scope (typically a test-fixture
// edge case). Always returns the empty string so the resolver
// emits the canonical "partner not found" diagnostic.
//
//nolint:gochecknoglobals // shared sentinel value
var emptyScope resolveScope = func(string) string { return "" }

// methodQName composes the canonical method-bucket key
// (`<ownerQName>.<methodName>`) the store uses. Mirrors the
// helper [shapewriter.methodQName] in the reference plugin.
func methodQName(owner, method string) string {
	if owner == "" {
		return method
	}
	return owner + "." + method
}

// resolve runs the per-callable refinement cascade. For every
// contract membership stamped on bag by the umbrella plugin:
// validate roles, rewrite partner names to qnames, back-stamp
// partners. Diagnostics attach to ctx.Diag under the resolver's
// plugin attribution.
func (r *Resolver) resolve(
	ctx *sdk.AnnotatorContext,
	host node.Node,
	bag *meta.Bag,
	hostQName string,
	scope resolveScope,
) {
	memberships := Contracts(bag)
	if len(memberships) == 0 {
		return
	}
	sink := ctx.Diag.For(ResolverName)
	for _, contractName := range memberships {
		spec, ok := r.contracts[contractName]
		if !ok {
			sink.Errorf(host.Pos(),
				"shape: contract %q is stamped on this callable but not registered with the resolver",
				contractName)
			continue
		}
		r.resolveOne(host, bag, hostQName, scope, spec, sink)
	}
}

// resolveOne handles one contract membership on host: validate
// its role, then resolve and back-stamp each partner reference.
func (r *Resolver) resolveOne(
	host node.Node,
	bag *meta.Bag,
	hostQName string,
	scope resolveScope,
	spec Contract,
	sink *diag.PluginSink,
) {
	role, _ := ContractRoleKey(spec.Name).Get(bag)
	if role != "" && !slices.Contains(spec.Roles, role) {
		sink.Errorf(host.Pos(),
			"shape.contract %q: role %q is not in the declared role vocabulary %v",
			spec.Name, role, spec.Roles)
	}
	for _, partnerRole := range spec.Roles {
		if partnerRole == role {
			continue
		}
		r.resolvePartner(host, bag, hostQName, role, partnerRole, scope, spec, sink)
	}
	r.flagUnknownPartnerRoles(host, bag, spec, sink)
}

// resolvePartner resolves a single partner reference for the
// (host, contract, partnerRole) triple. Skips when no partner ref
// was stamped (partner is optional) or when the stamp already
// holds a qname from a prior pass (idempotency).
func (r *Resolver) resolvePartner(
	host node.Node,
	bag *meta.Bag,
	hostQName, hostRole, partnerRole string,
	scope resolveScope,
	spec Contract,
	sink *diag.PluginSink,
) {
	partnerKey := ContractPartnerKey(spec.Name, partnerRole)
	raw, present := partnerKey.Get(bag)
	if !present || raw == "" {
		return
	}
	if isQualified(raw) {
		// User supplied an explicit cross-scope qname — no
		// rewriting needed, but the partner still needs the
		// back-stamp.
		r.backstamp(spec, partnerRole, hostQName, hostRole, raw)
		return
	}
	qname := scope(raw)
	if qname == "" {
		sink.Errorf(host.Pos(),
			"shape.contract %q: partner %q=%q not found in scope",
			spec.Name, partnerRole, raw)
		return
	}
	partnerKey.Set(bag, qname, ResolverName)
	r.backstamp(spec, partnerRole, hostQName, hostRole, qname)
}

// flagUnknownPartnerRoles emits a diagnostic for any partner KV
// stamped by the umbrella plugin whose role name is not in the
// contract's declared vocabulary. Partner stamps reach the
// resolver via the underlying meta bag, so we iterate every
// recorded name to discover them.
func (*Resolver) flagUnknownPartnerRoles(
	host node.Node,
	bag *meta.Bag,
	spec Contract,
	sink *diag.PluginSink,
) {
	prefix := "shape.contract." + spec.Name + ".partner."
	for _, name := range bag.Names() {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		role := strings.TrimPrefix(name, prefix)
		if slices.Contains(spec.Roles, role) {
			continue
		}
		sink.Errorf(host.Pos(),
			"shape.contract %q: partner role %q is not in the declared role vocabulary %v",
			spec.Name, role, spec.Roles)
	}
}

// backstamp writes the reverse-direction stamps onto a resolved
// partner: appends the contract to the partner's contracts list,
// sets the partner's role (if not already self-stamped), and sets
// the reverse partner-pointer keyed by the host's role.
//
// Looking up the partner's bag goes through the resolver's
// already-built scope index — the same index used for the
// forward lookup so consistent fixtures resolve to the same
// callable.
func (r *Resolver) backstamp(
	spec Contract,
	partnerRole, hostQName, hostRole, partnerQName string,
) {
	partnerBag := r.bagByQName(partnerQName)
	if partnerBag == nil {
		// Index-lookup miss is a defence-in-depth path; the
		// forward resolver already produced the qname from the
		// same scope, so this case shouldn't fire in practice.
		return
	}
	if !slices.Contains(Contracts(partnerBag), spec.Name) {
		appendContract(partnerBag, spec.Name)
	}
	roleKey := ContractRoleKey(spec.Name)
	if existing, _ := roleKey.Get(partnerBag); existing == "" {
		roleKey.Set(partnerBag, partnerRole, ResolverName)
	}
	if hostRole != "" {
		reverse := ContractPartnerKey(spec.Name, hostRole)
		if existing, _ := reverse.Get(partnerBag); existing == "" {
			reverse.Set(partnerBag, hostQName, ResolverName)
		}
	}
}

// bagByQName returns the meta bag of the callable identified by
// qname, or nil when no such callable exists in either the
// method-owner or function-package index. Used by [Resolver.backstamp]
// to reach the resolved partner without re-walking the store.
func (r *Resolver) bagByQName(qname string) *meta.Bag {
	for m, owner := range r.methodOwner {
		if methodQName(owner.qname, m.Name) == qname {
			return m.Meta()
		}
	}
	for fn := range r.funcPkg {
		if fn.QName() == qname {
			return fn.Meta()
		}
	}
	return nil
}

// isQualified reports whether name has already been rewritten to
// a qualified form (`pkg.Name` or `pkg.Type.Method`). The check
// is the conservative "contains a `.`" — raw source identifiers
// in Go never contain `.`, so the test is unambiguous in
// practice.
func isQualified(name string) bool {
	return strings.Contains(name, ".")
}

// resolveMixins iterates every mixin attached to bag and rewrites
// declared [Mixin.SiblingParams] values from raw names to
// qualified names sourced from the host's scope. Mixins without
// SiblingParams are no-ops here.
func (r *Resolver) resolveMixins(ctx *sdk.AnnotatorContext, host node.Node, bag *meta.Bag, scope resolveScope) {
	attached := Mixins(bag)
	if len(attached) == 0 {
		return
	}
	sink := ctx.Diag.For(ResolverName)
	for _, name := range attached {
		spec, ok := r.mixins[name]
		if !ok {
			continue
		}
		for _, param := range spec.SiblingParams {
			r.resolveMixinSibling(host, bag, spec.Name, param, scope, sink)
		}
	}
}

// resolveMixinSibling rewrites a single mixin sibling-param value
// from raw name to qname. Idempotent: skips already-qualified
// stamps and skips when the param is absent.
func (*Resolver) resolveMixinSibling(
	host node.Node,
	bag *meta.Bag,
	mixinName, param string,
	scope resolveScope,
	sink *diag.PluginSink,
) {
	key := MixinParamKey(mixinName, param)
	raw, present := key.Get(bag)
	if !present || raw == "" || isQualified(raw) {
		return
	}
	qname := scope(raw)
	if qname == "" {
		sink.Errorf(host.Pos(),
			"shape.mixin %q: sibling param %q=%q not found in scope",
			mixinName, param, raw)
		return
	}
	key.Set(bag, qname, ResolverName)
}
