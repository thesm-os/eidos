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
// mockgen sits in the [sdk.GeneratorComposition] bucket; the
// `Requires: ["repository"]` declaration documents the dependency
// on repogen's output even though the strict-by-priority bucket
// ordering already runs foundation generators first.
//
// # Output routing
//
// mockgen targets external test packages by default: every mock
// lands in a `<srcPkg>_test` emit.Package and the rendered file
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
	"strconv"

	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
	refconv "go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
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

// RequiresRepository names the upstream capability mockgen documents
// a dependency on so [pipeline.Pipeline.Plan] records the
// cross-bucket relationship.
const RequiresRepository = "repository"

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

// Language is the backend language whose output set
// [Plugin.Outputs] returns. Other languages get an empty slice
// until matching templates and per-language output declarations
// land.
const Language = "golang"

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
// unusable — go through [New] so the embedded [opt.Holder] binds
// to the plugin's options field.
type Plugin struct {
	*opt.Holder[Options]
	opts Options
}

// New returns a fresh plugin instance with the options holder bound.
// The pipeline overlays caller-supplied option values via
// [Plugin.SetOptions] (promoted from [opt.Holder]) at Build time.
func New() *Plugin {
	p := &Plugin{}
	p.Holder = opt.Bind(&p.opts)
	return p
}

// Name returns [Name].
func (*Plugin) Name() string { return Name }

// Version satisfies [sdk.Versioned].
func (*Plugin) Version() string { return Version }

// Priority places the plugin in the composition generator bucket so
// it runs after the foundation generators that synthesise the
// interfaces it mocks.
func (*Plugin) Priority() sdk.Priority { return sdk.GeneratorComposition }

// Provides returns [Capability].
func (*Plugin) Provides() []string { return []string{Capability} }

// Requires returns the repository capability label. The strict
// cross-bucket ordering already runs foundation before composition,
// but the declared dependency carries documentary intent.
func (*Plugin) Requires() []string { return []string{RequiresRepository} }

// Outputs returns the ordered set of rendered files this plugin
// produces in the named language. Implements
// [plugin.FilenameProvider]. The plugin emits standard Go decls
// today; consumers targeting another backend language receive
// nil so the Layout phase surfaces a missing-FilenameProvider
// error rather than producing a Go suffix that wouldn't match
// the rendered output.
func (*Plugin) Outputs(lang string) []sdk.Output {
	if lang == Language {
		return []sdk.Output{{Suffix: FilenameSuffix}}
	}
	return nil
}

// Directives declares the `+gen:mock` / `-gen:mock` schema.
// Positive directives opt source-side interfaces in; negated
// directives skip emit-side interfaces that would otherwise be
// mocked. A negated directive on a source-side method skips that
// individual method when its parent interface still opts in.
func (*Plugin) Directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			On(node.KindInterface).
			On(node.KindMethod).
			On(emit.KindInterface).
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
// every emit decl carries Origin set and leaves [emit.Target]
// zero. mockgen drops each mock into a `<srcPkg>_test`
// emit.Package so the Layout phase's alongside-source rule —
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
		c := builder.For(Name)
		pkg := c.Package(srcPkg.Name+TestPackageSuffix, srcPkg.Path+TestPackageSuffix)
		for _, si := range srcGroups[path] {
			// Resolved rather than declared: a mock built from the
			// declared methods alone is missing whatever the
			// interface embeds, and does not satisfy the interface
			// it mocks.
			set := ctx.Reader.MethodSet(si)
			for _, issue := range set.Issues {
				name, _ := node.EmbedName(issue.Embed)
				ctx.Diag.Errorf(si.Pos(),
					"%s: interface %q embeds %q, which %s; the generated mock would be missing its methods",
					Name, si.QName(), name, issue.Reason)
			}
			p.emitForSourceInterface(pkg, si, emit.External(si.Package, si.Name), set.Methods)
		}
		if err := buildAndAdd(ctx, pkg); err != nil {
			return err
		}
	}
	emitGroups, emitOrder := groupEmitInterfaces(ctx)
	for _, key := range emitOrder {
		group := emitGroups[key]
		c := builder.For(Name)
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
func buildAndAdd(ctx *sdk.GeneratorContext, pkg *builder.PackageBuilder) error {
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
) (map[string][]*node.Interface, []string) {
	groups := map[string][]*node.Interface{}
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
// containing emit.Package identity (Name + Path) so each rendered
// mock package mirrors the upstream interface's package — with
// the [TestPackageSuffix] applied at AddPackage time.
type emitInterfaceGroup struct {
	pkgName, pkgPath string
	items            []*emit.Interface
}

// groupEmitInterfaces walks the scoped emit-interface bucket and
// groups every non-suppressed interface by its containing
// emit.Package's (Name, Path). Order is first-encountered.
func groupEmitInterfaces(
	ctx *sdk.GeneratorContext,
) (map[string]*emitInterfaceGroup, []string) {
	pkgByQName := map[string]*emit.Package{}
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
	returns []emit.Ref
}

// paramSig describes one positional parameter — name and type.
// Anonymous parameters carry an empty name; the emit code rewrites
// them to `arg<N>` so the mock body can reference them.
type paramSig struct {
	name string
	typ  emit.Ref
}

// emitForEmitInterface emits a Mock struct for an emit-store
// interface anchored to the upstream interface's Origin. The Mock
// references the source interface by [emit.Internal] so the
// rendered struct correctly resolves the in-target name regardless
// of the emit interface's own package. Generic emit interfaces
// propagate their type parameters to the mock so the rendered
// struct, methods, and ifaceRef-instantiation all thread
// `[T1, T2, …]` consistently.
func (p *Plugin) emitForEmitInterface(pkg *builder.PackageBuilder, i *emit.Interface) {
	sigs := make([]methodSig, 0, len(i.Methods))
	for _, m := range i.Methods {
		params := make([]paramSig, 0, len(m.Params))
		for _, mp := range m.Params {
			params = append(params, paramSig{name: mp.Name, typ: mp.Type})
		}
		returns := make([]emit.Ref, 0, len(m.Returns))
		for _, r := range m.Returns {
			returns = append(returns, r.Type)
		}
		sigs = append(sigs, methodSig{name: m.Name, params: params, returns: returns})
	}
	tps := emitTypeParamsFromEmit(i.TypeParams)
	typeArgs := builder.TypeArgsFromEmitParams(i.TypeParams)
	p.emitMock(pkg, i.Name, emit.Internal(i, typeArgs...), i.Origin(), tps, sigs)
}

// emitForSourceInterface emits a Mock struct for a source-side
// interface, lifting its node-layer types into emit refs through
// [golang.FromNode] so the generated mock parses against the same
// signatures the source declares. ifaceRef is the reference the
// emitted mock uses for the source interface — the plugin always
// passes [emit.External]; the renderer qualifies references back
// into the regular package because the test-package import
// identity differs from the regular package's.
//
// Generic source interfaces propagate their type parameters to the
// mock and thread the type-arg list through ifaceRef so the
// rendered struct, methods, and embedded reference all carry
// `[T1, T2, …]` consistently.
func (p *Plugin) emitForSourceInterface(
	pkg *builder.PackageBuilder,
	i *node.Interface,
	ifaceRef emit.Ref,
	methods []*node.Method,
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
			params = append(params, paramSig{name: mp.Name, typ: refconv.FromNode(mp.Type)})
		}
		// Return names are available on m.Returns but not carried
		// here: a mock method delegates to its Func field, so a
		// named result would be declared and never assigned. A
		// generator deriving identifiers from returns — a recorded-
		// call struct, say — reads r.Name instead.
		returns := make([]emit.Ref, 0, len(m.Returns))
		for _, r := range m.Returns {
			returns = append(returns, refconv.FromNode(r.Type))
		}
		sigs = append(sigs, methodSig{name: m.Name, params: params, returns: returns})
	}
	tps := emitTypeParamsFromNode(i.TypeParams)
	typeArgs := builder.TypeArgsFromNodeParams(i.TypeParams)
	p.emitMock(pkg, i.Name, builder.ApplyTypeArgs(ifaceRef, typeArgs), i, tps, sigs)
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
// never constructs an [emit.Target] literal.
func (p *Plugin) emitMock(
	pkg *builder.PackageBuilder,
	ifaceName string,
	ifaceRef emit.Ref,
	origin node.Node,
	typeParams []emitTypeParamSpec,
	sigs []methodSig,
) {
	_ = ifaceRef // reserved for future "var _ Iface = (*Mock)(nil)" emission.
	mockName := ifaceName + p.opts.Suffix

	pkg.Struct(mockName, func(b *builder.StructBuilder) {
		b.Origin(origin)
		b.Docs(mockName + " is a func-valued mock implementation of " + ifaceName + ".")
		for _, tp := range typeParams {
			b.TypeParam(tp.Name, tp.Constraint)
		}
		for _, s := range sigs {
			b.Field(s.name+"Func", funcRefFor(s), nil)
		}
		typeArgs := typeArgsFromSpecs(typeParams)
		recv := emit.Ptr(emit.Internal(b.Node(), typeArgs...))
		for _, s := range sigs {
			b.Method(s.name, func(m *builder.MethodBuilder) {
				m.Receiver("m", recv)
				for i, param := range s.params {
					m.Param(paramName(param.name, i), param.typ)
				}
				for _, ret := range s.returns {
					m.Return(ret)
				}
				m.Body(dispatchBody(s)...)
			})
		}
	})
}

// emitTypeParamSpec captures the per-parameter shape needed to
// stamp generic parameters on the emitted mock struct: the
// parameter name and its resolved constraint.
type emitTypeParamSpec struct {
	Name       string
	Constraint *emit.Constraint
}

// emitTypeParamsFromNode lifts a [node.TypeParam] slice into the
// emitTypeParamSpec slice the mock builder consumes. Constraint
// conversion runs through [golang.ConstraintFromNode] so the
// any-constraint shape collapses to nil for round-tripping through
// the renderer's IsAny path.
func emitTypeParamsFromNode(params []*node.TypeParam) []emitTypeParamSpec {
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

// emitTypeParamsFromEmit projects an [emit.TypeParam] slice (the
// upstream-generator-produced interfaces this plugin consumes) into
// the spec form. The constraint passes through verbatim since it
// already lives on the emit layer.
func emitTypeParamsFromEmit(params []*emit.TypeParam) []emitTypeParamSpec {
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
// converge on) into the parallel bare-name [emit.Ref] list a
// generic host's receiver references take as their type arguments.
// The two layer-specific lifters live in [builder.TypeArgsFromNodeParams]
// / [builder.TypeArgsFromEmitParams]; this helper stays mockgen-local
// because [emitTypeParamSpec] is private to the package.
func typeArgsFromSpecs(specs []emitTypeParamSpec) []emit.Ref {
	if len(specs) == 0 {
		return nil
	}
	out := make([]emit.Ref, 0, len(specs))
	for _, s := range specs {
		out = append(out, emit.Builtin(s.Name))
	}
	return out
}

// funcRefFor builds the `func(<params>) <returns>` type for the
// func-valued field that backs a Mock method. Anonymous parameters
// keep their empty names — fields render in func-type position so
// the names don't appear in source.
func funcRefFor(s methodSig) emit.Ref {
	params := make([]emit.Ref, 0, len(s.params))
	for _, p := range s.params {
		params = append(params, p.typ)
	}
	return emit.FuncOf(params, s.returns)
}

// paramName returns the in-method identifier for parameter index i:
// the source name when non-empty, or `arg<N>` when the source name
// is empty so the dispatch body can reference the parameter.
func paramName(name string, i int) string {
	if name != "" {
		return name
	}
	return "arg" + strconv.Itoa(i)
}

// dispatchBody returns the statement list for one Mock method's
// body: `[return ]m.<Method>Func(<args...>)`. Zero-return methods
// drop the leading return.
func dispatchBody(s methodSig) []*emit.Stmt {
	args := make([]*emit.Expr, 0, len(s.params))
	for i, p := range s.params {
		args = append(args, emit.NewIdent(paramName(p.name, i)))
	}
	call := emit.NewCall(
		emit.NewField(emit.NewIdent("m"), s.name+"Func"),
		args...,
	)
	if len(s.returns) == 0 {
		return []*emit.Stmt{emit.NewExprStmt(call)}
	}
	return []*emit.Stmt{emit.NewReturn(call)}
}
