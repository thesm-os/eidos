// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package mockgen synthesises a mock implementation for every
// targeted interface. A targeted interface is either an emit-store
// interface produced by an upstream generator (typically repogen's
// `<Type>Repository`) or a source-side interface carrying
// `+gen:mock`.
//
// For each target, the plugin emits a `<Type><Suffix>` struct with
// one func-valued field per method (`<Method>Func`) plus an
// implementing method that dispatches to that field. The emitted
// mock is suitable for table-driven tests without an extra mocking
// dependency.
//
// One shape to know before writing a stub: a variadic method keeps
// its `...` on the mock — it has to, or the mock does not implement
// the interface — but its field takes the slice form, so
// `Log(format string, args ...any)` is configured through
// `LogFunc func(string, []any)`. See [funcRefFor] for why the emit
// layer leaves no other option.
//
// mockgen sits in the [sdk.GeneratorComposition] bucket; requiring
// [repogen.Capability] documents the dependency on repogen's output
// even though the strict-by-priority bucket ordering already runs
// foundation generators first.
//
// # Output routing
//
// mockgen targets external test packages by default: every mock
// lands in a `<srcPkg>_test` sdk.EmitPackage and the rendered file
// carries the `_mock_test.go` suffix, so the Go test toolchain
// confines it to test builds and the import identity diverges from
// the regular source package (no whitebox same-package elision).
// The plugin owns no routing configuration of its own — package
// selection flows through the framework's routing layer:
// `+gen:out:<plugin>` directives, the project / per-plugin
// `output.*` config, and the CLI `-o` / `-p` overrides reshape
// the destination per-source or per-run when the user has a
// non-default requirement (whitebox testing, production mocks,
// sibling-directory routing).
package mockgen

import (
	"errors"

	refconv "go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/reference/repogen"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier surfaced through
// [sdk.Plugin.Name].
const Name = "mockgen"

// Version is the plugin's declared version. It composes into the
// pipeline's plugin fingerprint, which frontends fold into their cache
// keys — so bumping it invalidates a warm cache populated when this
// plugin behaved differently. A plugin that declares no version
// contributes an empty string and can never invalidate anything, which
// is a silent staleness bug waiting for its first behavioural change.
const Version = "1.0.0"

// Capability is the capability label mockgen advertises through
// [plugin.CapabilityProvider.Provides].
const Capability = "mock"

// DirectiveName is the bare directive name (without the `+gen:` or
// `-gen:` prefix) the plugin reads from interfaces on both the
// source and the emit side.
const DirectiveName sdk.DirectiveName = "mock"

// FilenameSuffix is appended to the source-file basename (without
// the `.go` extension) to form the alongside-source output
// filename: `<src-file>_mock_test.go`. The `_test.go` ending pins
// the rendered mock to Go's test-build convention so it never
// reaches a production binary, and the `_mock` infix keeps the
// generated file distinguishable from a user-authored
// `<src-file>_test.go` test file.
const FilenameSuffix = "_mock_test.go"

// TestPackageSuffix is appended to the source package's short name
// (and import path) to form the emit-side package mockgen lands
// every mock in. The Go convention `<pkg>_test` produces an
// external test package whose import identity differs from the
// regular package — same-package elision stays inert so the
// rendered mock's references back into the regular package
// qualify naturally.
const TestPackageSuffix = "_test"

// GoOutputs returns the Go adapter's output set: the single
// alongside-source file carrying [FilenameSuffix].
//
// Go-only by construction. The plugin emits standard Go decls, so a
// consumer targeting another backend language gets no output set at
// all and the Layout phase surfaces a missing-FilenameProvider error
// rather than a Go suffix that would not match the rendered output.
// [sdk.Base] applies that language gate; the set is exported so a
// test or a downstream plugin can name the file this one owns.
func GoOutputs() []sdk.Output {
	return []sdk.Output{{Suffix: FilenameSuffix}}
}

// Options carries the plugin's user-tunable settings. Routing is
// owned by the framework's routing layer; mockgen exposes no
// test/production toggle. Users wanting a non-default destination
// (whitebox mock in the source package, production mock in a
// custom location, a sibling-directory route, etc.) drive it
// through the routing surface — `+gen:out:mockgen <path> pkg=…`
// on the source, or `-o` / `-p` on the CLI.
type Options struct {
	// Suffix is appended to the targeted interface's name to form
	// the emitted mock struct's identifier (`<Type><Suffix>`).
	// Defaults to `Mock`.
	Suffix string `eidos:"suffix,default=Mock"`
}

// Plugin is the mock-implementation generator. The zero value is
// unusable — go through [New] so the embedded holder binds to the
// plugin's options field.
type Plugin struct {
	*sdk.Base
	*sdk.Holder[Options]
	opts Options
}

// New returns a fresh plugin instance with the options holder bound.
// The pipeline overlays caller-supplied option values via
// [Plugin.SetOptions] (promoted from [sdk.Holder]) at Build time.
//
// [sdk.LanguageSupport.Builtin] rather than a template tree:
// every decl this plugin emits is a struct, a field or a method, all
// of which the backend already renders from its own kind templates.
// The plugin defines no [sdk.Kind] of its own, so a tree would hold
// nothing — and declaring that is what separates this shape from the
// generator that defines a kind and forgot to ship its templates,
// which renders a short file and fails nowhere.
//
// The composition bucket is where it has to run: it doubles
// interfaces the foundation generators synthesise, so those have to
// exist first. Requires names [repogen.Capability] — the published
// const rather than a literal, so the two cannot drift into a
// dependency that silently stops being declared. The strict
// cross-bucket ordering already runs foundation before composition,
// so that declaration carries documentary intent rather than
// ordering force.
func New() *Plugin {
	p := &Plugin{Base: sdk.NewPlugin(Name).
		Version(Version).
		Priority(sdk.GeneratorComposition).
		Provides(Capability).
		Requires(repogen.Capability).
		Directives(directives()...).
		For(goSupport()).
		Build()}
	p.Holder = sdk.BindOptions(&p.opts)
	return p
}

// directives declares the `+gen:mock` / `-gen:mock` schema.
// Positive directives opt source-side interfaces in; negated
// directives skip emit-side interfaces that would otherwise be
// mocked. A negated directive on a source-side method skips that
// individual method when its parent interface still opts in.
func directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			On(sdk.NodeKindInterface).
			On(sdk.NodeKindMethod).
			On(sdk.EmitKindInterface).
			Describe("Opts a source interface into mock generation (+) or skips an emit interface or single method (-).").
			Build(),
	}
}

// Generate emits a mock struct for every targeted interface. The
// plugin walks two sources: source-side interfaces carrying
// `+gen:mock` (one mock per match, anchored to the source
// interface) and emit-store interfaces (one mock per upstream
// interface that hasn't opted out via `-gen:mock`, anchored to
// the upstream interface's Origin so the mock composes into the
// same file as the interface it shadows).
//
// Iteration uses the scoped reader's `Interfaces()` and
// `EmitInterfaces()` queries so the pipeline's scope predicate is
// honoured at the iterator level — a `-target X` run sees only
// X-shaped interfaces, source-side or emit-side.
//
// Routing is owned entirely by the framework's routing layer:
// every emit decl carries Origin set and leaves [sdk.EmitTarget]
// zero. mockgen drops each mock into a `<srcPkg>_test`
// sdk.EmitPackage so the Layout phase's alongside-source rule —
// which reads Target.Package from the emit-side package —
// composes the rendered file's `package <pkg>_test` clause
// without the framework knowing anything about Go's test-package
// convention.
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
	srcGroups, srcOrder := groupSourceInterfaces(ctx)
	for _, path := range srcOrder {
		srcPkg, ok := ctx.Reader.Store().Nodes().Packages().ByQName(path)
		if !ok {
			continue
		}
		c := sdk.NewProvenance(Name)
		pkg := c.Package(srcPkg.Name+TestPackageSuffix, srcPkg.Path+TestPackageSuffix)
		emitted := 0
		for _, si := range srcGroups[path] {
			// Declined before the walk. A constraint's embeds are
			// terms constraining its type set, so walking one asks the
			// resolver for `int`, misses, and reports an embed the run
			// did not load — for a declaration with no method set to
			// mock. Through the frontend's stamp because the model
			// cannot tell `interface{ int | int64 }` from
			// `interface{ error }`.
			if refconv.IsConstraintInterface(si) {
				ctx.Diag.Errorf(si.Pos(),
					"%s: %q carries +gen:%s but is a generic constraint, not a "+
						"method-set contract; there is nothing to mock",
					Name, si.QName(), DirectiveName)
				continue
			}
			// Resolved rather than declared: a mock built from the
			// declared methods alone is missing whatever the
			// interface embeds, and does not satisfy the interface
			// it mocks.
			set := ctx.Reader.MethodSet(si)
			for _, issue := range set.Issues {
				name, _ := sdk.EmbedName(issue.Embed)
				ctx.Diag.Errorf(si.Pos(),
					"%s: interface %q embeds %q, which %s; the generated mock would be missing its methods",
					Name, si.QName(), name, issue.Reason)
			}
			p.emitForSourceInterface(pkg, si, sdk.External(si.Package, si.Name), set.Methods)
			emitted++
		}
		// Counted rather than assumed. Every interface in a package can
		// be declined — a package of constraints is the ordinary case —
		// and adding the package anyway renders a file carrying nothing
		// but the generated-by header, which reads as a generator that
		// ran and failed rather than one that had nothing to do.
		if emitted == 0 {
			continue
		}
		if err := buildAndAdd(ctx, pkg); err != nil {
			return err
		}
	}
	emitGroups, emitOrder := groupEmitInterfaces(ctx)
	for _, key := range emitOrder {
		group := emitGroups[key]
		c := sdk.NewProvenance(Name)
		pkg := c.Package(group.pkgName+TestPackageSuffix, group.pkgPath+TestPackageSuffix)
		for _, ei := range group.items {
			p.emitForEmitInterface(pkg, ei)
		}
		if err := buildAndAdd(ctx, pkg); err != nil {
			return err
		}
	}
	return nil
}

// buildAndAdd finalises the in-progress package and folds it into
// the emit store. Wrap-and-Join keeps the call sites in [Generate]
// uniform across the source-side and emit-side passes.
func buildAndAdd(ctx *sdk.GeneratorContext, pkg *sdk.PackageBuilder) error {
	out, err := pkg.Build()
	if err != nil {
		return err
	}
	if err := ctx.Store.Emit().AddPackage(out); err != nil {
		return errors.Join(errAddPackage, err)
	}
	return nil
}

// errAddPackage is the sentinel wrapped around store-side AddPackage
// failures. Tests and callers detect the class with errors.Is.
//
//nolint:gochecknoglobals // sentinel.
var errAddPackage = errors.New("mockgen: add package to store")

// groupSourceInterfaces walks the scoped source-interface bucket
// and groups every `+gen:mock` match by source-package path. The
// returned order slice preserves first-encountered path order so
// iteration of the grouping stays deterministic across runs.
func groupSourceInterfaces(
	ctx *sdk.GeneratorContext,
) (map[string][]*sdk.Interface, []string) {
	groups := map[string][]*sdk.Interface{}
	order := []string{}
	for i := range ctx.Reader.Interfaces().All() {
		if !i.HasPositiveDirective(DirectiveName) {
			continue
		}
		if _, seen := groups[i.Package]; !seen {
			order = append(order, i.Package)
		}
		groups[i.Package] = append(groups[i.Package], i)
	}
	return groups, order
}

// emitInterfaceGroup buckets emit-side interfaces by their
// containing sdk.EmitPackage identity (Name + Path) so each rendered
// mock package mirrors the upstream interface's package — with
// the [TestPackageSuffix] applied at AddPackage time.
type emitInterfaceGroup struct {
	pkgName, pkgPath string
	items            []*sdk.EmitInterface
}

// groupEmitInterfaces walks the scoped emit-interface bucket and
// groups every non-suppressed interface by its containing
// sdk.EmitPackage's (Name, Path). Order is first-encountered.
func groupEmitInterfaces(
	ctx *sdk.GeneratorContext,
) (map[string]*emitInterfaceGroup, []string) {
	pkgByQName := map[string]*sdk.EmitPackage{}
	for _, epkg := range ctx.Reader.Store().Emit().Packages().Items() {
		pkgByQName[epkg.Path] = epkg
	}
	groups := map[string]*emitInterfaceGroup{}
	order := []string{}
	for ei := range ctx.Reader.EmitInterfaces().All() {
		if ei.HasNegatedDirective(DirectiveName) {
			continue
		}
		host := pkgByQName[ei.Package]
		if host == nil {
			continue
		}
		key := host.Name + "\x00" + host.Path
		if _, seen := groups[key]; !seen {
			order = append(order, key)
			groups[key] = &emitInterfaceGroup{pkgName: host.Name, pkgPath: host.Path}
		}
		groups[key].items = append(groups[key].items, ei)
	}
	return groups, order
}

// methodSig is the minimal per-method information mockgen needs to
// emit one Mock-struct method. Both the emit-interface and the
// source-interface entry points lower their respective method shapes
// onto this common form.
type methodSig struct {
	name    string
	params  []paramSig
	returns []sdk.Ref
}

// idents returns the identifier each parameter is bound to, in the
// emitted signature and in the dispatch body alike.
//
// Derived for the whole list at once rather than per parameter,
// because the positional fallback and a declared name collide:
// `Put(arg0 string, string)` names the first parameter exactly what
// the second falls back to, and two parameters of one name do not
// compile. [refconv.ParamIdentsFor] owns that uniqueness pass;
// asking [refconv.ParamIdent] per item reproduces the defect.
func (s methodSig) idents() []string {
	names := make([]string, len(s.params))
	for i, p := range s.params {
		names[i] = p.name
	}
	return refconv.ParamIdentsFor(names)
}

// receiverIdent returns the receiver identifier for a method emitted
// on mockName.
//
// Disambiguated against the parameter identifiers, because the
// receiver shares their scope. A source interface declaring
// `Do(m string)` would otherwise emit
//
//	func (m *FooMock) Do(m string) { return m.DoFunc(m) }
//
// where the parameter shadows the receiver and `m.DoFunc` resolves
// against a string. It compiles nowhere, and no fixture whose
// parameters avoid the receiver letter ever reaches it.
func (s methodSig) receiverIdent(mockName string) string {
	return refconv.ReceiverIdent(mockName, s.idents()...)
}

// paramSig describes one positional parameter — name, type, and
// whether it is the signature's trailing variadic. Anonymous
// parameters carry an empty name; the emit code rewrites them to
// `arg<N>` so the mock body can reference them.
//
// The variadic flag is not decoration. A mock exists to be assignable
// to the interface it doubles, and Go's assignability rules make
// `Log(format string, args []any)` and `Log(format string, args
// ...any)` different method sets: dropping the marker produces a mock
// that compiles, that every substring assertion accepts, and that
// satisfies nothing.
type paramSig struct {
	name     string
	typ      sdk.Ref
	variadic bool
}

// emitForEmitInterface emits a Mock struct for an emit-store
// interface anchored to the upstream interface's Origin. The Mock
// references the source interface by [sdk.Internal] so the
// rendered struct correctly resolves the in-target name regardless
// of the emit interface's own package. Generic emit interfaces
// propagate their type parameters to the mock so the rendered
// struct, methods, and ifaceRef-instantiation all thread
// `[T1, T2, …]` consistently.
func (p *Plugin) emitForEmitInterface(pkg *sdk.PackageBuilder, i *sdk.EmitInterface) {
	sigs := make([]methodSig, 0, len(i.Methods))
	for _, m := range i.Methods {
		params := make([]paramSig, 0, len(m.Params))
		for _, mp := range m.Params {
			params = append(params, paramSig{name: mp.Name, typ: mp.Type, variadic: mp.Variadic})
		}
		returns := make([]sdk.Ref, 0, len(m.Returns))
		for _, r := range m.Returns {
			returns = append(returns, r.Type)
		}
		sigs = append(sigs, methodSig{name: m.Name, params: params, returns: returns})
	}
	tps := emitTypeParamsFromEmit(i.TypeParams)
	typeArgs := sdk.TypeArgsFromEmitParams(i.TypeParams)
	p.emitMock(pkg, i.Name, sdk.Internal(i, typeArgs...), i.Origin(), tps, sigs)
}

// emitForSourceInterface emits a Mock struct for a source-side
// interface, lifting its node-layer types into emit refs through
// [sdk.FromNode] so the generated mock parses against the same
// signatures the source declares. ifaceRef is the reference the
// emitted mock uses for the source interface — the plugin always
// passes [sdk.External]; the renderer qualifies references back
// into the regular package because the test-package import
// identity differs from the regular package's.
//
// Generic source interfaces propagate their type parameters to the
// mock and thread the type-arg list through ifaceRef so the
// rendered struct, methods, and embedded reference all carry
// `[T1, T2, …]` consistently.
func (p *Plugin) emitForSourceInterface(
	pkg *sdk.PackageBuilder,
	i *sdk.Interface,
	ifaceRef sdk.Ref,
	methods []*sdk.Method,
) {
	sigs := make([]methodSig, 0, len(methods))
	for _, m := range methods {
		if m.HasNegatedDirective(DirectiveName) {
			// Per-method opt-out: the directive on this source
			// method skips it without affecting other methods on
			// the same interface.
			continue
		}
		params := make([]paramSig, 0, len(m.Params))
		for _, mp := range m.Params {
			params = append(params, paramSig{
				name:     mp.Name,
				typ:      refconv.FromNode(mp.Type),
				variadic: mp.Variadic,
			})
		}
		// Return names are available on m.Returns but not carried
		// here: a mock method delegates to its Func field, so a
		// named result would be declared and never assigned. A
		// generator deriving identifiers from returns — a recorded-
		// call struct, say — reads r.Name instead.
		returns := make([]sdk.Ref, 0, len(m.Returns))
		for _, r := range m.Returns {
			returns = append(returns, refconv.FromNode(r.Type))
		}
		sigs = append(sigs, methodSig{name: m.Name, params: params, returns: returns})
	}
	tps := emitTypeParamsFromNode(i.TypeParams)
	typeArgs := sdk.TypeArgsFromNodeParams(i.TypeParams)
	p.emitMock(pkg, i.Name, sdk.ApplyTypeArgs(ifaceRef, typeArgs), i, tps, sigs)
}

// emitMock appends one Mock struct decl carrying the func-valued
// fields and dispatching methods for every signature in sigs. The
// mocked interface is rendered through ifaceRef in field-receiver
// position so callers can pass the Mock anywhere the source
// interface is required. typeParams (when non-empty) propagate the
// host interface's generic parameters to the mock so the rendered
// struct, methods, and receivers all carry the same `[T1, T2, …]`
// bracket list.
//
// Origin is set on the emitted struct so the Layout phase can
// resolve every Target field downstream — the plugin itself
// never constructs an [sdk.EmitTarget] literal.
func (p *Plugin) emitMock(
	pkg *sdk.PackageBuilder,
	ifaceName string,
	ifaceRef sdk.Ref,
	origin sdk.Node,
	typeParams []emitTypeParamSpec,
	sigs []methodSig,
) {
	// ifaceRef is carried but not emitted. The obvious use is a
	// `var _ Iface = (*Mock)(nil)` satisfaction assertion beside the
	// struct, and the emit layer renders one correctly — but the
	// store indexes every variable under `<pkg>.<Name>`, so a second
	// blank-named one in the same emit package is
	// [sdk.ErrDuplicateQName], and this plugin groups every
	// interface of a source package into one emit package. Two
	// further carve-outs would be needed even then: a generic mock
	// has no concrete instantiation to spell, and a mock trimmed by
	// `-gen:mock` on a method is deliberately short of the interface
	// it names. The rendered fixture asserts satisfaction from a
	// hand-written support file instead — see the mockgen test's
	// sourcePackage.
	_ = ifaceRef
	mockName := ifaceName + p.opts.Suffix

	pkg.Struct(mockName, func(b *sdk.StructBuilder) {
		b.Origin(origin)
		b.Docs(mockName + " is a func-valued mock implementation of " + ifaceName + ".")
		for _, tp := range typeParams {
			b.TypeParam(tp.Name, tp.Constraint)
		}
		for _, s := range sigs {
			b.Field(s.name+"Func", funcRefFor(s), nil)
		}
		typeArgs := typeArgsFromSpecs(typeParams)
		recv := sdk.Ptr(sdk.Internal(b.Node(), typeArgs...))
		for _, s := range sigs {
			b.Method(s.name, func(m *sdk.MethodBuilder) {
				// Every exported declaration a generator emits is read
				// by the consumer's editor and linted by their
				// configuration; an undocumented exported method
				// reports as the consumer's own lint failure in a file
				// they did not write. Its sibling repogen documents the
				// methods it emits for the same reason.
				m.Docs(s.name + " implements " + ifaceName +
					" by dispatching to " + s.name + "Func.")
				idents := s.idents()
				m.Receiver(s.receiverIdent(mockName), recv)
				for i, param := range s.params {
					m.Param(idents[i], param.typ, variadicOpts(param)...)
				}
				for _, ret := range s.returns {
					m.Return(ret)
				}
				m.Body(dispatchBody(s, s.receiverIdent(mockName))...)
			})
		}
	})
}

// emitTypeParamSpec captures the per-parameter shape needed to
// stamp generic parameters on the emitted mock struct: the
// parameter name and its resolved constraint.
type emitTypeParamSpec struct {
	Name       string
	Constraint *sdk.EmitConstraint
}

// emitTypeParamsFromNode lifts a [sdk.TypeParam] slice into the
// emitTypeParamSpec slice the mock builder consumes. Constraint
// conversion runs through [sdk.ConstraintFromNode] so the
// any-constraint shape collapses to nil for round-tripping through
// the renderer's IsAny path.
func emitTypeParamsFromNode(params []*sdk.TypeParam) []emitTypeParamSpec {
	if len(params) == 0 {
		return nil
	}
	out := make([]emitTypeParamSpec, 0, len(params))
	for _, tp := range params {
		out = append(out, emitTypeParamSpec{
			Name:       tp.Name,
			Constraint: refconv.ConstraintFromNode(tp.Constraint),
		})
	}
	return out
}

// emitTypeParamsFromEmit projects an [sdk.EmitTypeParam] slice (the
// upstream-generator-produced interfaces this plugin consumes) into
// the spec form. The constraint passes through verbatim since it
// already lives on the emit layer.
func emitTypeParamsFromEmit(params []*sdk.EmitTypeParam) []emitTypeParamSpec {
	if len(params) == 0 {
		return nil
	}
	out := make([]emitTypeParamSpec, 0, len(params))
	for _, tp := range params {
		out = append(out, emitTypeParamSpec{Name: tp.Name, Constraint: tp.Constraint})
	}
	return out
}

// typeArgsFromSpecs lifts the local [emitTypeParamSpec] slice (the
// normalised intermediate the source-side and emit-side paths
// converge on) into the parallel bare-name [sdk.Ref] list a
// generic host's receiver references take as their type arguments.
// The two layer-specific lifters live in [sdk.TypeArgsFromNodeParams]
// / [sdk.TypeArgsFromEmitParams]; this helper stays mockgen-local
// because [emitTypeParamSpec] is private to the package.
func typeArgsFromSpecs(specs []emitTypeParamSpec) []sdk.Ref {
	if len(specs) == 0 {
		return nil
	}
	out := make([]sdk.Ref, 0, len(specs))
	for _, s := range specs {
		out = append(out, sdk.Builtin(s.Name))
	}
	return out
}

// variadicOpts returns the [sdk.ParamBuilder] configuration a
// parameter needs, which is a marker for the trailing variadic and
// nothing at all for every other parameter.
//
// Spelled as a slice of options rather than an `if` at the call site
// so the parameter loop stays one statement per parameter.
func variadicOpts(p paramSig) []func(*sdk.ParamBuilder) {
	if !p.variadic {
		return nil
	}
	return []func(*sdk.ParamBuilder){func(b *sdk.ParamBuilder) { b.Variadic() }}
}

// funcRefFor builds the `func(<params>) <returns>` type for the
// func-valued field that backs a Mock method. Anonymous parameters
// keep their empty names — fields render in func-type position so
// the names don't appear in source.
//
// A trailing variadic parameter is lowered to its slice form:
// `Log(format string, args ...any)` backs onto `LogFunc func(string,
// []any)`. Two reasons, one of them a hard constraint.
//
// The constraint: [sdk.CompositeRef]'s func shape carries parameter
// refs and no per-parameter variadic marker, so `func(string, ...any)`
// is not expressible on the emit layer at all. Emitting the element
// type bare — `func(string, any)` — is what this replaced, and it made
// the dispatch call a compile error the moment the method forwarded
// its slice.
//
// The design agreement: inside the dispatching method the variadic
// parameter *is* a slice, so forwarding it unspread is the direct
// call. A test configuring the mock writes `m.LogFunc = func(f string,
// args []any) {…}` and reads the arguments as the slice it already
// had.
func funcRefFor(s methodSig) sdk.Ref {
	params := make([]sdk.Ref, 0, len(s.params))
	for _, p := range s.params {
		if p.variadic {
			params = append(params, sdk.SliceOf(p.typ))
			continue
		}
		params = append(params, p.typ)
	}
	return sdk.FuncOf(params, s.returns)
}

// dispatchBody returns the statement list for one Mock method's
// body: `[return ]m.<Method>Func(<args...>)`. Zero-return methods
// drop the leading return.
//
// A variadic parameter is forwarded unspread, which pairs with the
// slice-typed field [funcRefFor] declares for it: inside the method
// the parameter is already a `[]T`, so the plain identifier is the
// call that type-checks.
func dispatchBody(s methodSig, recvIdent string) []*sdk.Stmt {
	idents := s.idents()
	args := make([]*sdk.Expr, 0, len(idents))
	for _, id := range idents {
		args = append(args, sdk.NewIdent(id))
	}
	call := sdk.NewCall(
		sdk.NewField(sdk.NewIdent(recvIdent), s.name+"Func"),
		args...,
	)
	if len(s.returns) == 0 {
		return []*sdk.Stmt{sdk.NewExprStmt(call)}
	}
	return []*sdk.Stmt{sdk.NewReturn(call)}
}
