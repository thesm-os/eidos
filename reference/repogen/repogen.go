// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package repogen synthesises a Repository-pattern interface and a
// thin implementing struct for every source struct annotated with
// `+gen:repo`. The emitted interface carries the canonical CRUD
// method set (`Get` / `List` / `Save` / `Delete`); the implementing
// struct carries empty bodies — downstream cross-cutting plugins
// fill the bodies via the method `prebody` / `postbody` slots.
//
// Detection is directive-driven; the heuristic-driven shape-writer
// pattern doesn't apply. `+gen:repo` opts a struct into emission;
// `-gen:repo` suppresses it.
//
// # Output routing
//
// repogen owns no routing configuration of its own — the
// framework's routing layer composes every emit decl's
// [sdk.EmitTarget] from the source struct's origin plus the project
// / per-plugin output config and CLI overrides. The plugin
// declares its output set in [New] and sets
// [sdk.BaseEmit.OriginNode] on every emit decl; the Layout phase
// does the rest.
package repogen

import (
	"errors"

	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier surfaced through
// [sdk.Plugin.Name].
const Name = "repogen"

// Version is the plugin's declared version. It composes into the
// pipeline's plugin fingerprint, which frontends fold into their cache
// keys — so bumping it invalidates a warm cache populated when this
// plugin behaved differently. A plugin that declares no version
// contributes an empty string and can never invalidate anything, which
// is a silent staleness bug waiting for its first behavioural change.
const Version = "1.0.0"

// Capability is the capability label other plugins declare in
// [plugin.CapabilityProvider.Requires] when they need repogen's
// output as input — most notably the mock generator.
const Capability = "repository"

// DirectiveName is the bare directive name (without the `+gen:` or
// `-gen:` prefix) the plugin reads from each source struct.
const DirectiveName sdk.DirectiveName = "repo"

// FilenameSuffix is appended to the source-file basename (without
// the `.go` extension) to form the alongside-source output
// filename: `<src-file>_repo.go`. The stringer-style convention
// means every `+gen:repo` struct declared in `article.go` composes
// into a single `article_repo.go`. The suffix is distinct from
// the source's own basename so re-runs don't conflate generated
// output with hand-authored code.
const FilenameSuffix = "_repo.go"

// NamingPascal selects the canonical PascalCase identifier shape
// (e.g. `ArticleRepository`, `Get`). Default for the Naming option.
const NamingPascal = "Pascal"

// NamingCamel selects camelCase identifier shape (e.g.
// `articleRepository`, `get`) — produces unexported emitted
// identifiers, useful for internal repository surfaces.
const NamingCamel = "Camel"

// Options carries the plugin's user-tunable settings. Routing
// is owned by the framework's routing layer; repogen exposes no
// output-package option — project / per-plugin output config
// and the CLI `-layout` / `-p` / `-output-dir` flags drive
// where rendered output lands.
type Options struct {
	// InterfaceSuffix is appended to the source struct's name to
	// form the emitted interface's identifier
	// (`<Type><InterfaceSuffix>`). Defaults to `Repository`.
	InterfaceSuffix string `eidos:"interface_suffix,default=Repository"`

	// StructSuffix is appended to the source struct's name to form
	// the emitted implementing-struct's identifier
	// (`<Type><StructSuffix>`). Defaults to `Repo`.
	StructSuffix string `eidos:"struct_suffix,default=Repo"`

	// Naming selects the casing style for emitted identifiers.
	// `Pascal` produces exported identifiers; `Camel` produces
	// unexported (lowercase-first) identifiers. Defaults to
	// `Pascal`.
	Naming string `eidos:"naming,one_of=Pascal|Camel,default=Pascal"`
}

// Plugin is the repository-pattern generator. The zero value is
// unusable — go through [New] so the embedded [sdk.Base] carries
// the plugin's declarations and the embedded [sdk.Holder] binds to
// the plugin's options field.
type Plugin struct {
	*sdk.Base
	*sdk.Holder[Options]
	opts Options
}

// New returns a fresh plugin instance with the options holder bound.
// The pipeline overlays caller-supplied option values via
// [Plugin.SetOptions] (promoted from [sdk.Holder]) at Build time.
//
// The single declared [sdk.Output] carries [FilenameSuffix] and
// nothing else, because repogen retains no filename control: the
// routing layer composes the rest. Only Go is declared, so
// [sdk.Base.Outputs] answers nil for any other language and Layout
// reports a missing provider rather than composing a Go suffix that
// would not match what was rendered. Another language joins the set
// when matching templates and its own output declarations land.
//
// [sdk.LanguageSupport.Builtin] rather than a template tree: every
// decl this plugin emits is an interface, a struct or a method, all
// of which the backend already renders from its own kind templates.
// The plugin defines no [sdk.Kind] of its own, so a tree would hold
// nothing — and saying so explicitly is what separates this shape
// from the generator that defines a kind and forgot to ship its
// templates. It also keeps the shared helper bundle out of the
// backend's funcmap registry, which is the right outcome for a
// plugin owning no template able to call it.
//
// The foundation bucket is where the plugin belongs: composition-
// bucket generators — mockgen, most obviously — consume the
// interfaces it emits, so those have to exist first. It advertises
// [Capability] for such a plugin to name as a Requires entry, and
// declares no Requires of its own, which is why the bucket rather
// than a capability is what orders it.
func New() *Plugin {
	p := &Plugin{Base: sdk.NewPlugin(Name).
		Version(Version).
		Priority(sdk.GeneratorFoundation).
		Provides(Capability).
		Directives(directives()...).
		For(goSupport()).
		Build()}
	p.Holder = sdk.BindOptions(&p.opts)
	return p
}

// directives declares the `+gen:repo` / `-gen:repo` schema with
// the pipeline so directive validation rejects malformed uses at
// frontend-parse time.
func directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			On(sdk.NodeKindStruct).
			Describe("Forces (+) or suppresses (-) repository emission for the host struct.").
			Build(),
	}
}

// Generate walks the scoped source-struct bucket, emitting a
// Repository interface + implementing struct for each `+gen:repo`
// target. Suppression via `-gen:repo` skips the source struct
// even when other generators (builder, registry) act on it.
//
// Iteration goes through `ctx.Reader.Structs()` so the pipeline's
// scope predicate is honoured at the iterator level — a
// `-target Article` run sees only Article-shaped source structs,
// matching what `+gen:repo` was actually annotated with for the
// in-scope set.
//
// Output routing is owned entirely by the framework's routing
// layer: the plugin sets each emit decl's Origin to the source
// struct and leaves [sdk.EmitTarget] zero. The Layout phase
// composes Dir / Filename / Package / ImportPath from the
// origin and the resolved [pipeline.LayoutPolicy] for this
// plugin.
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
	groups, order := groupByPackage(ctx, p.shouldEmit)
	for _, path := range order {
		srcPkg, ok := ctx.Reader.Store().Nodes().Packages().ByQName(path)
		if !ok {
			continue
		}
		c := sdk.NewProvenance(Name)
		pkg := c.Package(srcPkg.Name, srcPkg.Path)
		for _, s := range groups[path] {
			p.emitOne(pkg, s, sdk.External(s.Package, s.Name))
		}
		out, err := pkg.Build()
		if err != nil {
			return err
		}
		if err := ctx.Store.Emit().AddPackage(out); err != nil {
			return errors.Join(errAddPackage, err)
		}
	}
	return nil
}

// errAddPackage is the sentinel wrapped around store-side AddPackage
// failures. Tests and callers detect the class with errors.Is.
//
//nolint:gochecknoglobals // sentinel.
var errAddPackage = errors.New("repogen: add package to store")

// groupByPackage walks the scoped source-struct bucket and groups
// every match of pred by the source struct's import path. The
// returned order slice preserves first-encountered path order so
// iteration of the grouping stays deterministic across runs.
func groupByPackage(
	ctx *sdk.GeneratorContext,
	pred func(*sdk.Struct) bool,
) (map[string][]*sdk.Struct, []string) {
	groups := map[string][]*sdk.Struct{}
	order := []string{}
	for s := range ctx.Reader.Structs().All() {
		if !pred(s) {
			continue
		}
		if _, seen := groups[s.Package]; !seen {
			order = append(order, s.Package)
		}
		groups[s.Package] = append(groups[s.Package], s)
	}
	return groups, order
}

// shouldEmit reports whether the source struct opts into
// repository emission. Only `+gen:repo` enables; absence or
// `-gen:repo` suppresses.
func (*Plugin) shouldEmit(s *sdk.Struct) bool {
	return s.HasPositiveDirective(DirectiveName)
}

// emitOne appends the interface + implementing-struct + method
// set for one source struct to the in-progress package. The
// emitted method signatures cover the canonical CRUD set with
// `context.Context` first parameters and `error` returns; bodies
// are empty so cross-cutting weavers have a clean slot surface.
//
// srcRef is the reference the emitted methods use for the source
// type. The plugin always passes [sdk.External]; the renderer
// elides self-imports for same-package references via the
// Target.ImportPath the Layout phase composes from the source
// package, so the qualified form stays correct under both
// alongside-source and centralised layouts.
//
// The Origin of each emitted decl is set to src so the Layout
// phase can resolve every Target field downstream — the plugin
// itself never constructs an [sdk.EmitTarget] literal.
//
// # Known gap: interface-method doc lines are not rendered
//
// The doc lines set on the interface's methods below reach the emit
// graph and are dropped at render time — `backend/golang`'s
// `emit.interface` template renders each method's name, params and
// returns and never consults its DocLines, where the struct-field
// renderer does consult theirs. The calls stay because the intent is
// right and the fix belongs one layer down; the tests assert docs on
// the implementing struct's methods, which do render.
func (p *Plugin) emitOne(pkg *sdk.PackageBuilder, src *sdk.Struct, srcRef sdk.Ref) {
	ifaceName := p.identifier(src.Name + p.opts.InterfaceSuffix)
	structName := p.identifier(src.Name + p.opts.StructSuffix)
	get := p.identifier("Get")
	list := p.identifier("List")
	save := p.identifier("Save")
	del := p.identifier("Delete")

	pkg.Interface(ifaceName, func(i *sdk.InterfaceBuilder) {
		i.Origin(src)
		i.Docs(ifaceName + " stores and retrieves " + src.Name + " values.")
		i.Method(get, func(m *sdk.MethodBuilder) {
			m.Docs(get + " returns the " + src.Name + " stored under id.")
			m.Param("ctx", sdk.External("context", "Context"))
			m.Param("id", sdk.Builtin("string"))
			m.Return(sdk.Ptr(srcRef))
			m.Return(sdk.Builtin("error"))
		})
		i.Method(list, func(m *sdk.MethodBuilder) {
			m.Docs(list + " returns every stored " + src.Name + ".")
			m.Param("ctx", sdk.External("context", "Context"))
			m.Return(sdk.SliceOf(sdk.Ptr(srcRef)))
			m.Return(sdk.Builtin("error"))
		})
		i.Method(save, func(m *sdk.MethodBuilder) {
			m.Docs(save + " stores value, replacing any " + src.Name +
				" already held under the same identity.")
			m.Param("ctx", sdk.External("context", "Context"))
			m.Param("value", sdk.Ptr(srcRef))
			m.Return(sdk.Builtin("error"))
		})
		i.Method(del, func(m *sdk.MethodBuilder) {
			m.Docs(del + " removes the " + src.Name + " stored under id.")
			m.Param("ctx", sdk.External("context", "Context"))
			m.Param("id", sdk.Builtin("string"))
			m.Return(sdk.Builtin("error"))
		})
	})

	pkg.Struct(structName, func(st *sdk.StructBuilder) {
		st.Origin(src)
		st.Docs(structName + " is the default in-memory implementation of " + ifaceName + ".")
		st.Method(get, func(m *sdk.MethodBuilder) {
			m.Docs(implDoc(get, ifaceName)...)
			m.Receiver("r", sdk.Ptr(sdk.Internal(st.Node())))
			m.Param("ctx", sdk.External("context", "Context"))
			m.Param("id", sdk.Builtin("string"))
			m.Return(sdk.Ptr(srcRef))
			m.Return(sdk.Builtin("error"))
			m.Body(sdk.NewReturn(sdk.NewLiteralNil(), sdk.NewLiteralNil()))
		})
		st.Method(list, func(m *sdk.MethodBuilder) {
			m.Docs(implDoc(list, ifaceName)...)
			m.Receiver("r", sdk.Ptr(sdk.Internal(st.Node())))
			m.Param("ctx", sdk.External("context", "Context"))
			m.Return(sdk.SliceOf(sdk.Ptr(srcRef)))
			m.Return(sdk.Builtin("error"))
			m.Body(sdk.NewReturn(sdk.NewLiteralNil(), sdk.NewLiteralNil()))
		})
		st.Method(save, func(m *sdk.MethodBuilder) {
			m.Docs(implDoc(save, ifaceName)...)
			m.Receiver("r", sdk.Ptr(sdk.Internal(st.Node())))
			m.Param("ctx", sdk.External("context", "Context"))
			m.Param("value", sdk.Ptr(srcRef))
			m.Return(sdk.Builtin("error"))
			m.Body(sdk.NewReturn(sdk.NewLiteralNil()))
		})
		st.Method(del, func(m *sdk.MethodBuilder) {
			m.Docs(implDoc(del, ifaceName)...)
			m.Receiver("r", sdk.Ptr(sdk.Internal(st.Node())))
			m.Param("ctx", sdk.External("context", "Context"))
			m.Param("id", sdk.Builtin("string"))
			m.Return(sdk.Builtin("error"))
			m.Body(sdk.NewReturn(sdk.NewLiteralNil()))
		})
	})
}

// implDoc returns the doc comment for one method on the emitted
// implementing struct.
//
// The second paragraph is the load-bearing one. The struct is emitted
// with empty bodies by design — cross-cutting weavers fill them
// through the method `prebody` / `postbody` slots — so a build in
// which no weaver ran leaves every method returning zero values and
// reporting no error. A caller reading the hover text has to be told
// that, because the signature says "may fail" and the unwoven body
// never does.
func implDoc(method, iface string) []string {
	return []string{
		method + " implements " + iface + ".",
		"",
		"The body is empty until a cross-cutting plugin fills the",
		"method's slots; unwoven, it returns zero values and never",
		"reports an error.",
	}
}

// identifier applies the configured naming style. Pascal returns
// the name unchanged; Camel lower-cases the first rune so the
// emitted identifier is unexported.
func (p *Plugin) identifier(name string) string {
	if p.opts.Naming != NamingCamel || name == "" {
		return name
	}
	return string(name[0]|0x20) + name[1:]
}
