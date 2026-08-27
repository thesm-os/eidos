// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape

import (
	"slices"
	"strings"

	"go.thesmos.sh/eidos/lang/golang"
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
	methodOwner map[*sdk.Method]methodOwner
	funcPkg     map[*sdk.Function]*sdk.Package
	hostPkg     map[sdk.Node]*sdk.Package
	typeMethods map[string][]*sdk.Method
	typeStructs map[string]*sdk.Struct

	// reader is the run's declaration index, captured per Annotate
	// for the promoted-field lookup a [KindValueField] resolution
	// makes: a version stamp on an embedded base is exactly the
	// shape the kind names, and a flat match would report a correct
	// directive as wrong.
	reader golang.Resolver
}

// methodOwner is the per-method index entry: the owning struct or
// interface plus its qualified name (cached so qname composition
// during back-stamping is O(1)).
type methodOwner struct {
	qname   string
	methods []*sdk.Method
}

// Resolver returns a fresh [Resolver] sharing p's contract
// registrations. Register the resolver alongside the umbrella so
// it runs in the refinement bucket after the umbrella plugin:
//
//	s := shape.New().Detectors(...).Contracts(...)
//	pipe.WithAnnotator(s)
//	pipe.WithAnnotator(s.Resolver())
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
	r.methodOwner = make(map[*sdk.Method]methodOwner)
	r.funcPkg = make(map[*sdk.Function]*sdk.Package)
	r.hostPkg = make(map[sdk.Node]*sdk.Package)
	r.typeMethods = make(map[string][]*sdk.Method)
	r.typeStructs = make(map[string]*sdk.Struct)
	r.reader = ctx.Reader
	ctx.Reader.Packages().Each(func(pkg *sdk.Package) {
		for _, s := range pkg.Structs {
			owner := methodOwner{qname: s.QName(), methods: s.Methods}
			r.typeMethods[owner.qname] = s.Methods
			r.typeStructs[owner.qname] = s
			for _, m := range s.Methods {
				r.methodOwner[m] = owner
				r.hostPkg[m] = pkg
			}
		}
		for _, i := range pkg.Interfaces {
			owner := methodOwner{qname: i.QName(), methods: i.Methods}
			r.typeMethods[owner.qname] = i.Methods
			for _, m := range i.Methods {
				r.methodOwner[m] = owner
				r.hostPkg[m] = pkg
			}
		}
		for _, fn := range pkg.Functions {
			r.funcPkg[fn] = pkg
			r.hostPkg[fn] = pkg
		}
	})
}

// OnMethod resolves contract memberships and mixin sibling
// params on m using the owning struct or interface as the
// partner-lookup scope.
func (r *Resolver) OnMethod(ctx *sdk.AnnotatorContext, m *sdk.Method) {
	owner := r.methodOwner[m]
	scope := methodScope(owner)
	hostQName := methodQName(owner.qname, m.Name)
	r.resolve(ctx, m, m.EnsureMeta(), hostQName, scope)
	r.resolveMixins(ctx, m, m.EnsureMeta(), scope)
}

// OnFunction resolves contract memberships and mixin sibling
// params on fn using fn's containing package's functions as the
// partner-lookup scope.
func (r *Resolver) OnFunction(ctx *sdk.AnnotatorContext, fn *sdk.Function) {
	scope := packageScope(r.funcPkg[fn])
	r.resolve(ctx, fn, fn.EnsureMeta(), fn.QName(), scope)
	r.resolveMixins(ctx, fn, fn.EnsureMeta(), scope)
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
func packageScope(pkg *sdk.Package) resolveScope {
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

// scopeFor returns the lookup a param of the given kind resolves
// through, or nil when the kind needs no resolution.
//
// One place decides what each kind means, so a new kind is a case here
// rather than a branch at every call site. [KindOpaque] returns nil
// because the value names nothing to look up — a literal handed to any
// scope would report every correct one as missing.
//
// callable is the host's own scope, threaded in by the caller that
// already computed it; the others derive from the host.
func (r *Resolver) scopeFor(kind ParamKind, host sdk.Node, callable resolveScope) resolveScope {
	switch kind {
	case KindCallable:
		if callable == nil {
			return r.callableScope(host)
		}
		return callable
	case KindVar:
		return varScope(r.hostPkg[host])
	case KindMember:
		return r.memberScope(host)
	case KindValueField:
		return r.valueFieldScope(host)
	case KindParam:
		return paramScope(host)
	case KindOpaque:
		return nil
	default:
		return nil
	}
}

// callableScope recovers the host's own sibling scope.
//
// The contract path resolves params after the partner loop, which does
// not thread its scope down, so this rebuilds it from the same indexes
// [Resolver.BeforeNodes] fills.
func (r *Resolver) callableScope(host sdk.Node) resolveScope {
	switch h := host.(type) {
	case *sdk.Method:
		return methodScope(r.methodOwner[h])
	case *sdk.Function:
		return packageScope(r.funcPkg[h])
	default:
		return emptyScope
	}
}

// memberScope returns a [resolveScope] searching the methods of the
// type host answers — the handle a role's callable returns.
//
// The third scope, and the one whose reach depends on the run rather
// than on the declaration. A watcher's Next and Stop live on the
// subscription Watch returns, not on the interface Watch is declared
// on, so neither the callable scope nor the var scope sees them.
//
// The answered type is the first non-error result, pointer stripped: a
// handle is returned by pointer as often as by value and the members
// are the same either way.
//
// Returns the empty scope when the run did not load the answered
// type's declaration, which is the one place in this vocabulary where
// a diagnostic's presence depends on the run's patterns. A handle from
// an unloaded package stamps unvalidated and the generated file's
// compile is the loud failure; silence here is not a pass.
func (r *Resolver) memberScope(host sdk.Node) resolveScope {
	_, returns := golang.Callable(host)
	results := golang.StripErrorTypes(returns)
	if len(results) == 0 {
		return nil
	}
	answered := results[0]
	if elem := golang.PointerElem(answered); elem != nil {
		answered = elem
	}
	owner := golang.QName(answered)
	methods := r.typeMethods[owner]
	if owner == "" || len(methods) == 0 {
		// Nil rather than the empty scope, which always misses and
		// would be read as "the member is not there". The run did not
		// load the declaration, so there is nothing to check against —
		// the param stamps unvalidated and the generated file's
		// compile is the loud failure.
		return nil
	}
	return func(name string) string {
		for _, m := range methods {
			if m != nil && m.Name == name {
				return methodQName(owner, m.Name)
			}
		}
		return ""
	}
}

// valueFieldScope returns a [resolveScope] searching the fields of
// the host's value type, per the rule [KindValueField] states: the
// first non-error result, or — for a host answering nothing — each
// non-context parameter's type in declaration order, taking the
// first that declares the field. Pointer-stripped in both positions.
//
// The lookup is [golang.MemberField], so the field must be exported
// and promotion is honoured. A hit rewrites the stamp into
// `<type-qname>.<Field>`, the composed form every other resolved
// kind uses; a consumer takes the trailing identifier back off with
// [golang.LocalName], as it already does for a sibling callable.
//
// Nil when any candidate names a declaration the run did not load —
// the param stamps unvalidated rather than reported, since the field
// may live on exactly the type the resolver cannot see, and a false
// error on a correct directive is worse than the silence
// [KindMember] already accepts for an unloaded handle. A builtin is
// not that case: it has no declaration to load and can declare no
// field, so it stays in the candidate list and misses honestly.
func (r *Resolver) valueFieldScope(host sdk.Node) resolveScope {
	params, returns := golang.Callable(host)
	candidates := golang.StripErrorTypes(returns)
	if len(candidates) > 0 {
		candidates = candidates[:1]
	} else {
		for _, p := range golang.StripContext(params) {
			if p != nil && p.Type != nil {
				candidates = append(candidates, p.Type)
			}
		}
	}
	type valueType struct {
		qname string
		decl  *sdk.Struct
	}
	var loaded []valueType
	for _, t := range candidates {
		if elem := golang.PointerElem(t); elem != nil {
			t = elem
		}
		qname := golang.QName(t)
		decl, ok := r.typeStructs[qname]
		if !ok && !t.IsBuiltin() {
			return nil
		}
		loaded = append(loaded, valueType{qname: qname, decl: decl})
	}
	if len(loaded) == 0 {
		return nil
	}
	reader := r.reader
	return func(name string) string {
		for _, vt := range loaded {
			if vt.decl == nil {
				continue
			}
			if _, found := golang.MemberField(vt.decl, name, reader); found {
				return vt.qname + "." + name
			}
		}
		return ""
	}
}

// paramScope returns a [resolveScope] matching the host's own
// parameter names.
//
// A hit answers the raw name back rather than a qualified form,
// because a parameter has no package-level spelling — the stamp
// stays as the author wrote it, and what the resolution buys is the
// check that the name is genuinely in the signature. The miss is the
// diagnostic [partition]'s and [scope]'s Validate hooks used to
// raise by hand.
func paramScope(host sdk.Node) resolveScope {
	params, _ := golang.Callable(host)
	return func(name string) string {
		for _, p := range params {
			if p != nil && p.Name == name {
				return name
			}
		}
		return ""
	}
}

// varScope returns a [resolveScope] searching pkg's package-level
// vars for a name match, returning the qualified name on hit.
//
// The scope a sentinel resolves in, and the reason [KindVar]
// is a separate declaration from [KindCallable]: a var is not
// in the callable scope, so resolving one there reports every correct
// sentinel as missing.
func varScope(pkg *sdk.Package) resolveScope {
	if pkg == nil {
		return emptyScope
	}
	return func(name string) string {
		for _, v := range pkg.Variables {
			if v != nil && v.Name == name {
				return v.QName()
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
	host sdk.Node,
	bag *sdk.Bag,
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
	host sdk.Node,
	bag *sdk.Bag,
	hostQName string,
	scope resolveScope,
	spec Contract,
	sink *sdk.PluginSink,
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
	r.resolveContractVars(host, bag, role, spec, sink)
	r.flagUnknownPartnerRoles(host, bag, spec, sink)
}

// resolveContractVars rewrites every declared param value on host
// from a raw name to the qualified name of what it names, through
// whichever scope the param's [ParamKind] selects.
//
// Separate from the partner loop above because the scope differs: a
// partner is a callable reached through the host's own scope, and a
// param may want the package (a sentinel is declared beside the type
// rather than on it) or the type the host answers.
//
// Scoped by role through [ParamsForRole], the same predicate the
// stamping pass routes with. Filtering here is not merely defensive:
// a key declared for two roles with two kinds would otherwise resolve
// through whichever scope came first in declaration order, and the
// contract that needs role-scoped params at all is exactly the one
// where that ordering is invisible to its author.
func (r *Resolver) resolveContractVars(
	host sdk.Node,
	bag *sdk.Bag,
	role string,
	spec Contract,
	sink *sdk.PluginSink,
) {
	for _, p := range ParamsForRole(spec.Params, role) {
		resolveIn := r.scopeFor(p.Kind, host, nil)
		if resolveIn == nil {
			continue
		}
		param := p.Key
		key := ContractParamKey(spec.Name, param)
		raw, present := key.Get(bag)
		if !present || raw == "" || isQualified(raw) {
			continue
		}
		qname := resolveIn(raw)
		if qname == "" {
			sink.Errorf(host.Pos(),
				"shape.contract %q: %s=%q names no %s",
				spec.Name, param, raw, p.Kind.scopeNoun())
			continue
		}
		key.Set(bag, qname, ResolverName)
	}
}

// resolvePartner resolves a single partner reference for the
// (host, contract, partnerRole) triple. Skips when no partner ref
// was stamped (partner is optional) or when the stamp already
// holds a qname from a prior pass (idempotency).
func (r *Resolver) resolvePartner(
	host sdk.Node,
	bag *sdk.Bag,
	hostQName, hostRole, partnerRole string,
	scope resolveScope,
	spec Contract,
	sink *sdk.PluginSink,
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
	host sdk.Node,
	bag *sdk.Bag,
	spec Contract,
	sink *sdk.PluginSink,
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
func (r *Resolver) bagByQName(qname string) *sdk.Bag {
	for m, owner := range r.methodOwner {
		if methodQName(owner.qname, m.Name) == qname {
			return m.EnsureMeta()
		}
	}
	for fn := range r.funcPkg {
		if fn.QName() == qname {
			return fn.EnsureMeta()
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
// declared [KindCallable] values from raw names to
// qualified names sourced from the host's scope. Mixins without
// Params of [KindOpaque] are no-ops here.
func (r *Resolver) resolveMixins(ctx *sdk.AnnotatorContext, host sdk.Node, bag *sdk.Bag, scope resolveScope) {
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
		for _, p := range spec.Params {
			resolveIn := r.scopeFor(p.Kind, host, scope)
			if resolveIn == nil {
				continue
			}
			r.resolveMixinSibling(host, bag, spec.Name, p, resolveIn, sink)
		}
	}
}

// resolveMixinSibling rewrites a single mixin param value from raw
// name to qname. Idempotent: skips already-qualified stamps and
// skips when the param is absent.
//
// The miss names what the kind's value must be — the contract path's
// wording, adopted here so a [KindParam] miss says "names no
// parameter of the annotated callable" rather than calling a
// parameter a sibling.
func (*Resolver) resolveMixinSibling(
	host sdk.Node,
	bag *sdk.Bag,
	mixinName string,
	p Param,
	scope resolveScope,
	sink *sdk.PluginSink,
) {
	key := MixinParamKey(mixinName, p.Key)
	raw, present := key.Get(bag)
	if !present || raw == "" || isQualified(raw) {
		return
	}
	qname := scope(raw)
	if qname == "" {
		sink.Errorf(host.Pos(),
			"shape.mixin %q: %s=%q names no %s",
			mixinName, p.Key, raw, p.Kind.scopeNoun())
		return
	}
	key.Set(bag, qname, ResolverName)
}
