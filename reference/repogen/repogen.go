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
// [emit.Target] from the source struct's origin plus the project
// / per-plugin output config and CLI overrides. The plugin
// declares its output set via [Plugin.Outputs] and
// sets [emit.BaseEmit.OriginNode] on every emit decl; the
// Layout phase does the rest.
package repogen

import (
	"errors"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
	"go.thesmos.sh/eidos/node"
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

// Language is the backend language whose output set
// [Plugin.Outputs] returns. Other languages get an empty slice
// until matching templates and per-language output declarations
// land.
const Language = "golang"

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
// unusable — go through [New] so the embedded [opt.Holder] binds
// to the plugin's options field.
type Plugin struct {
	*sdk.Holder[Options]
	opts Options
}

// New returns a fresh plugin instance with the options holder bound.
// The pipeline overlays caller-supplied option values via
// [Plugin.SetOptions] (promoted from [sdk.Holder]) at Build time.
func New() *Plugin {
	p := &Plugin{}
	p.Holder = sdk.BindOptions(&p.opts)
	return p
}

// Name returns [Name].
func (*Plugin) Name() string { return Name }

// Version satisfies [sdk.Versioned].
func (*Plugin) Version() string { return Version }

// Priority places the plugin in the foundation generator bucket so
// it runs before composition-bucket generators (e.g. mockgen) that
// consume its emitted interfaces.
func (*Plugin) Priority() sdk.Priority { return sdk.GeneratorFoundation }

// Provides returns [Capability] — composition-bucket plugins
// declare it as a Requires entry to document the dependency.
func (*Plugin) Provides() []string { return []string{Capability} }

// Requires returns nil — repogen has no upstream dependency.
func (*Plugin) Requires() []string { return nil }

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

// Directives declares the `+gen:repo` / `-gen:repo` schema with
// the pipeline so directive validation rejects malformed uses at
// frontend-parse time.
func (*Plugin) Directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			On(node.KindStruct).
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
// struct and leaves [emit.Target] zero. The Layout phase
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
			p.emitOne(pkg, s, emit.External(s.Package, s.Name))
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
	pred func(*node.Struct) bool,
) (map[string][]*node.Struct, []string) {
	groups := map[string][]*node.Struct{}
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
func (*Plugin) shouldEmit(s *node.Struct) bool {
	return s.HasPositiveDirective(DirectiveName)
}

// emitOne appends the interface + implementing-struct + method
// set for one source struct to the in-progress package. The
// emitted method signatures cover the canonical CRUD set with
// `context.Context` first parameters and `error` returns; bodies
// are empty so cross-cutting weavers have a clean slot surface.
//
// srcRef is the reference the emitted methods use for the source
// type. The plugin always passes [emit.External]; the renderer
// elides self-imports for same-package references via the
// Target.ImportPath the Layout phase composes from the source
// package, so the qualified form stays correct under both
// alongside-source and centralised layouts.
//
// The Origin of each emitted decl is set to src so the Layout
// phase can resolve every Target field downstream — the plugin
// itself never constructs an [emit.Target] literal.
func (p *Plugin) emitOne(pkg *builder.PackageBuilder, src *node.Struct, srcRef emit.Ref) {
	ifaceName := p.identifier(src.Name + p.opts.InterfaceSuffix)
	structName := p.identifier(src.Name + p.opts.StructSuffix)

	pkg.Interface(ifaceName, func(i *builder.InterfaceBuilder) {
		i.Origin(src)
		i.Docs(ifaceName + " stores and retrieves " + src.Name + " values.")
		i.Method(p.identifier("Get"), func(m *builder.MethodBuilder) {
			m.Param("ctx", emit.External("context", "Context"))
			m.Param("id", emit.Builtin("string"))
			m.Return(emit.Ptr(srcRef))
			m.Return(emit.Builtin("error"))
		})
		i.Method(p.identifier("List"), func(m *builder.MethodBuilder) {
			m.Param("ctx", emit.External("context", "Context"))
			m.Return(emit.SliceOf(emit.Ptr(srcRef)))
			m.Return(emit.Builtin("error"))
		})
		i.Method(p.identifier("Save"), func(m *builder.MethodBuilder) {
			m.Param("ctx", emit.External("context", "Context"))
			m.Param("value", emit.Ptr(srcRef))
			m.Return(emit.Builtin("error"))
		})
		i.Method(p.identifier("Delete"), func(m *builder.MethodBuilder) {
			m.Param("ctx", emit.External("context", "Context"))
			m.Param("id", emit.Builtin("string"))
			m.Return(emit.Builtin("error"))
		})
	})

	pkg.Struct(structName, func(st *builder.StructBuilder) {
		st.Origin(src)
		st.Docs(structName + " is the default in-memory implementation of " + ifaceName + ".")
		st.Method(p.identifier("Get"), func(m *builder.MethodBuilder) {
			m.Receiver("r", emit.Ptr(emit.Internal(st.Node())))
			m.Param("ctx", emit.External("context", "Context"))
			m.Param("id", emit.Builtin("string"))
			m.Return(emit.Ptr(srcRef))
			m.Return(emit.Builtin("error"))
			m.Body(emit.NewReturn(emit.NewLiteralNil(), emit.NewLiteralNil()))
		})
		st.Method(p.identifier("List"), func(m *builder.MethodBuilder) {
			m.Receiver("r", emit.Ptr(emit.Internal(st.Node())))
			m.Param("ctx", emit.External("context", "Context"))
			m.Return(emit.SliceOf(emit.Ptr(srcRef)))
			m.Return(emit.Builtin("error"))
			m.Body(emit.NewReturn(emit.NewLiteralNil(), emit.NewLiteralNil()))
		})
		st.Method(p.identifier("Save"), func(m *builder.MethodBuilder) {
			m.Receiver("r", emit.Ptr(emit.Internal(st.Node())))
			m.Param("ctx", emit.External("context", "Context"))
			m.Param("value", emit.Ptr(srcRef))
			m.Return(emit.Builtin("error"))
			m.Body(emit.NewReturn(emit.NewLiteralNil()))
		})
		st.Method(p.identifier("Delete"), func(m *builder.MethodBuilder) {
			m.Receiver("r", emit.Ptr(emit.Internal(st.Node())))
			m.Param("ctx", emit.External("context", "Context"))
			m.Param("id", emit.Builtin("string"))
			m.Return(emit.Builtin("error"))
			m.Body(emit.NewReturn(emit.NewLiteralNil()))
		})
	})
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
