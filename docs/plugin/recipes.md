# Pattern catalog

For each common plugin pattern, this catalog walks through the
matching reference plugin under `reference/`. Each section
includes a working code snippet so you can see the shape without
leaving the doc; for the full plugin (with helper functions,
edge-case handling, options-struct binding, etc.) follow the
link to the source file.

Every reference plugin satisfies the framework conformance suite
and is production-grade.

## Annotator — shape inference

**Pattern:** read source nodes, stamp typed metadata, run before
the generator phase. Other plugins read the stamped meta to
decide whether their codegen path applies.

**Reference:** [`reference/shapewriter`](../../reference/shapewriter)

Detect every struct that satisfies the `io.Writer` shape (a
`Write([]byte) (int, error)` method) and stamp a typed meta key:

```go
package shapewriter

import (
    "go.thesmos.sh/eidos/core/meta"
    "go.thesmos.sh/eidos/node"
    "go.thesmos.sh/eidos/sdk"
)

const Name = "shape-writer"

// Detected is the meta key the plugin stamps. Consumers read via
// shapewriter.Detected.Get(node.Meta()).
var Detected = meta.NewKey("shape.writer.detected", meta.BoolParser)

type Plugin struct{}

func New() *Plugin { return &Plugin{} }

func (*Plugin) Name() string         { return Name }
func (*Plugin) Priority() sdk.Priority { return sdk.AnnotatorShape }

// Annotate dispatches to the per-kind hooks via sdk.Walk.
func (p *Plugin) Annotate(ctx *sdk.AnnotatorContext) error {
    return sdk.Walk(ctx, p)
}

// OnStruct is the sdk.StructHook entry point — invoked once per
// struct in stable insertion order.
func (*Plugin) OnStruct(_ *sdk.AnnotatorContext, s *node.Struct) {
    detected := hasWriteMethod(s)
    Detected.Set(s.Meta(), detected, Name)
}
```

**Key idioms:**

- `sdk.Walk(ctx, p)` dispatches to whichever hook interfaces (`OnStruct`, `OnInterface`, `OnMethod`, `OnFunction`, `BeforeNodes`, `AfterNodes`) the plugin implements
- `sdk.AnnotatorShape` is the earliest annotator priority; refinement / validation annotators see the inferred shapes
- `meta.NewKey` registers a typed key; consumers read it via `Key.Get(bag)`

**Conformance:** `RunSuite` + `RunAnnotatorSuite`.

## Generator — per-source-decl emission

**Pattern:** read a directive-tagged source decl (struct,
interface, function), emit a counterpart in a generated
package. The canonical "for each `+gen:repo` struct emit a
`<Type>Repository` interface + `<Type>Repo` struct" pattern.

**Reference:** [`reference/repogen`](../../reference/repogen) (canonical), [`reference/mockgen`](../../reference/mockgen), [`plugins/generator/builder`](../../plugins/generator/builder) (template-driven variant)

```go
package repogen

import (
    "go.thesmos.sh/eidos/core/opt"
    "go.thesmos.sh/eidos/emit"
    "go.thesmos.sh/eidos/emit/builder"
    "go.thesmos.sh/eidos/node"
    "go.thesmos.sh/eidos/sdk"
)

const (
    Name          = "repogen"
    Capability    = "repository"
    DirectiveName = sdk.DirectiveName("repo")
    Language      = "golang"
)

// Options is the typed configuration the plugin declares through
// sdk.OptionsProvider. Defaults are pre-applied at opt.Bind time.
type Options struct {
    InterfaceSuffix string `eidos:"interface_suffix,default=Repository"`
    StructSuffix    string `eidos:"struct_suffix,default=Repo"`
    Naming          string `eidos:"naming,one_of=Pascal|Camel,default=Pascal"`
}

type Plugin struct {
    *opt.Holder[Options]  // embeds the OptionsSchema / SetOptions methods
    opts Options
}

func New() *Plugin {
    p := &Plugin{}
    p.Holder = opt.Bind(&p.opts)
    return p
}

func (*Plugin) Name() string           { return Name }
func (*Plugin) Priority() sdk.Priority   { return sdk.GeneratorFoundation }
func (*Plugin) Provides() []string     { return []string{Capability} }
func (*Plugin) Requires() []string     { return nil }
func (*Plugin) Outputs(lang string) []sdk.Output {
    if lang == Language {
        return []sdk.Output{{Suffix: "_repo.go"}}
    }
    return nil
}

// Directives declares the +gen:repo schema with the pipeline.
func (*Plugin) Directives() []sdk.DirectiveSchema {
    return []sdk.DirectiveSchema{
        sdk.NewDirective(DirectiveName).
            On(node.KindStruct).
            Describe("Opts the struct into repository emission.").
            Build(),
    }
}

// Generate walks every +gen:repo source struct and emits the
// matching interface + struct + method set.
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
    structs := ctx.Reader.Structs().Where(func(s *node.Struct) bool {
        return s.HasPositiveDirective(DirectiveName)
    }).Slice()

    for _, src := range structs {
        pkg := builder.For(Name, emit.Target{}).
            Package(src.Package, src.Package)
        emitOne(pkg, src, p.opts)
        out, err := pkg.Build()
        if err != nil {
            return err
        }
        if err := ctx.Store.Emit().AddPackage(out); err != nil {
            return err
        }
    }
    return nil
}

func emitOne(pkg *builder.PackageBuilder, src *node.Struct, opts Options) {
    ifaceName := src.Name + opts.InterfaceSuffix       // UserRepository
    structName := src.Name + opts.StructSuffix          // UserRepo
    srcRef := emit.External(src.Package, src.Name)

    pkg.Interface(ifaceName, func(i *builder.InterfaceBuilder) {
        i.Origin(src)
        i.Method("Get", func(m *builder.MethodBuilder) {
            m.Param("ctx", emit.External("context", "Context"))
            m.Param("id", emit.Builtin("string"))
            m.Return(emit.Ptr(srcRef))
            m.Return(emit.Builtin("error"))
        })
        // List, Save, Delete methods elided for brevity
    })

    pkg.Struct(structName, func(s *builder.StructBuilder) {
        s.Origin(src)
        // Implementing-struct stub fields elided
    })
}
```

**Key idioms:**

- `*opt.Holder[Options]` embedding satisfies `sdk.OptionsProvider`
  without per-plugin boilerplate
- `builder.For(Name, emit.Target{})` produces a package builder
  scoped to the plugin's identity; every emit decl auto-stamps
  its SetBy attribution
- `ctx.Reader.Structs().Where(...).Slice()` filters source-side
  structs through the per-plugin read-tracking reader so reads
  contribute to the plugin's cache key
- `i.Origin(src)` back-links every emit decl to its source; the
  layout phase composes `emit.Target` from the origin
- `emit.External(pkg, name)` / `emit.Builtin(name)` / `emit.Ptr(ref)`
  are the canonical type-reference constructors

**Conformance:** `RunSuite` + `RunGeneratorSuite` + `RunOptionsSuite`.

## Generator — cross-cutting slot contributor

**Pattern:** contribute statements / fields / methods to
existing emit decls (typically those another generator already
emitted), without owning your own routable output.

**Reference:** [`reference/auditweaver`](../../reference/auditweaver), [`reference/debugweaver`](../../reference/debugweaver)

```go
package debugweaver

import (
    "go.thesmos.sh/eidos/core/opt"
    "go.thesmos.sh/eidos/emit"
    "go.thesmos.sh/eidos/emit/builder"
    "go.thesmos.sh/eidos/sdk"
)

const (
    Name          = "debugweaver"
    Capability    = "debug-trace"
    DirectiveName = sdk.DirectiveName("debug")
)

type Options struct {
    Package string `eidos:"package,default=log"`
    Func    string `eidos:"func,default=Printf"`
    Format  string `eidos:"format,default=debug: %s entered"`
}

type Plugin struct {
    *opt.Holder[Options]
    opts Options
}

func New() *Plugin {
    p := &Plugin{}
    p.Holder = opt.Bind(&p.opts)
    return p
}

func (*Plugin) Name() string           { return Name }
func (*Plugin) Priority() sdk.Priority   { return sdk.GeneratorCrossCutting }
func (*Plugin) Provides() []string     { return []string{Capability} }
func (*Plugin) Requires() []string     { return nil }
// No FilenameProvider — this plugin emits no routable decls.

// Generate visits every emit method that opts into the +gen:debug
// directive and prepends a logging call to its Prebody slot.
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
    for _, m := range ctx.Reader.EmitMethods().Slice() {
        if !m.HasPositiveDirective(DirectiveName) {
            continue
        }
        logCall := emit.NewCallExpr(
            emit.NewChain(emit.NewIdent(p.opts.Package), p.opts.Func),
            emit.NewStringLit(p.opts.Format),
            emit.NewStringLit(m.Name),
        )
        m.Prebody().Append(emit.NewExprStmt(logCall))
    }
    return nil
}
```

**Key idioms:**

- No `Directives()` / `FilenameProvider()` — the plugin doesn't
  declare a routable decl, so the framework expects only the
  slot-contribution surface
- `m.Prebody().Append(stmt)` writes into the method's pre-body
  slot; the foundation generator owns the host decl, the weaver
  contributes content
- `sdk.GeneratorCrossCutting` runs after `GeneratorFoundation`
  and `GeneratorComposition` so the host decls exist by the time
  the weaver visits them

**Conformance:** `RunSuite` + `RunGeneratorSuite` (empty-store
no-panic; determinism; frozen source nodes).

## Generator — plugin-defined emit kind

**Pattern:** introduce a new emit type outside the `emit.*`
namespace, ship a matching template, and have the backend
render it through the standard template-provider surface.

**Reference:** [`reference/registrygen`](../../reference/registrygen)

**Deep dive:** [templates.md](templates.md) walks through the
full template surface — kind naming, the `Templates` /
`TemplateFuncs` / `TemplateOverrides` capability methods, the
funcmap, and the rendering pipeline — using registrygen as the
canonical end-to-end example.

```go
package registrygen

import (
    "embed"
    "io/fs"
    "text/template"

    "go.thesmos.sh/eidos/core/kind"
    "go.thesmos.sh/eidos/emit"
    "go.thesmos.sh/eidos/sdk"
)

const Name = "registrygen"

// RegistrationKind is the plugin-defined emit kind. The dotted
// spelling keeps it outside emit.* (which is reserved for core
// emit types).
const RegistrationKind kind.Kind = "registrygen.registration"

// Registration is the plugin's emit type. Embeds emit.BaseEmit
// for the shared Node methods (Pos, Docs, Directives, Meta,
// Origin, SetBy).
type Registration struct {
    emit.BaseEmit
    Target emit.Target
    Type   string  // qualified name of the type being registered
}

func (*Registration) Kind() kind.Kind { return RegistrationKind }

// Compile-time confirmation that *Registration satisfies emit.Node.
var _ emit.Node = (*Registration)(nil)

type Plugin struct{ /* opt.Holder elided */ }

func (*Plugin) Name() string { return Name }

// Generate visits every +gen:register-tagged decl and appends a
// Registration to the package-level slot.
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
    // walk + append elided for brevity
    return nil
}

//go:embed templates/golang/*.tmpl
var templatesFS embed.FS

// Templates ships the per-language template through
// sdk.TemplateProvider. The backend's template-collection step
// picks up every TemplateProvider's filesystem automatically.
func (*Plugin) Templates(lang string) (fs.FS, bool) {
    if lang != "golang" {
        return nil, false
    }
    sub, err := fs.Sub(templatesFS, "templates/golang")
    if err != nil {
        return nil, false
    }
    return sub, true
}

// TemplateFuncs returns nil — no funcmap extensions needed.
func (*Plugin) TemplateFuncs(string) template.FuncMap     { return nil }
func (*Plugin) TemplateOverrides(string) template.FuncMap { return nil }
```

**Key idioms:**

- The custom kind is declared as a `kind.Kind` constant outside
  the `emit.*` namespace — `<plugin-name>.<kind-name>` is the
  convention
- `emit.BaseEmit` embedded on the custom type provides the
  shared `Pos`, `Docs`, `Directives`, `Meta`, `Origin`, and
  `SetBy` accessors
- `var _ emit.Node = (*Registration)(nil)` is the compile-time
  interface-satisfaction check
- The template is shipped via `//go:embed` + `fs.Sub` so the
  plugin's templates ride alongside its code

**Conformance:** `RunSuite` + `RunGeneratorSuite` +
`RunOptionsSuite`. The plugin-defined kind doesn't change which
suites apply.

## Frontend — alternative source language

**Pattern:** parse a non-Go input format (proto, OpenAPI, …)
into the language-agnostic `node` graph; downstream annotators
and generators run unchanged.

**Reference:** [`frontend/protobuf`](../../frontend/protobuf) (uses `protocompile` for real proto parsing)

```go
package myfrontend

import (
    "fmt"

    "go.thesmos.sh/eidos/core/opt"
    "go.thesmos.sh/eidos/node"
    "go.thesmos.sh/eidos/sdk"
)

const Name = "myfrontend"

type Options struct {
    Dir string `eidos:"dir,required"`
}

type Plugin struct {
    *opt.Holder[Options]
    opts Options
}

func New() *Plugin {
    p := &Plugin{}
    p.Holder = opt.Bind(&p.opts)
    return p
}

func (*Plugin) Name() string { return Name }

// Version contributes to the cache key. Bump when the frontend's
// output shape changes in a way that should invalidate caches.
func (*Plugin) Version() string         { return "1.0.0" }
func (*Plugin) EmitVersions() []string  { return []string{"1"} }

// Load parses ctx.Pattern from the configured directory and
// populates ctx.Store.Nodes() via AddPackage. Per-input issues
// attach to ctx.Diag; fatal failures return a non-nil error.
func (p *Plugin) Load(ctx *sdk.FrontendContext) error {
    pkg := &node.Package{Name: "example", Path: "example.com/parsed"}
    // ... parsing logic populates pkg.Structs, pkg.Interfaces, etc.
    if err := ctx.Store.Nodes().AddPackage(pkg); err != nil {
        return fmt.Errorf("myfrontend: AddPackage: %w", err)
    }
    return nil
}
```

**Key idioms:**

- Language-specific facts ride on meta keys in a per-language
  namespace (`go.*` for the Go frontend, `proto.*` for proto);
  the node graph itself stays language-agnostic
- `Versioned` + `EmitVersioned` declare the frontend's
  contribution to the cache key — bumping the version
  invalidates downstream caches
- `ctx.Store.Nodes().AddPackage(pkg)` is the canonical way to
  register a parsed package; the store auto-indexes by kind /
  package / directive / meta-key

**Conformance:** `RunSuite` + `RunFrontendSuite` against
representative source-directory fixtures.

## Backend — target language renderer

**Pattern:** consume the emit graph and write rendered files
through a `sink.Sink`. Exactly one backend per pipeline.

**Reference:** [`backend/golang`](../../backend/golang)

```go
package mybackend

import (
    "go.thesmos.sh/eidos/emit"
    "go.thesmos.sh/eidos/sdk"
)

const (
    Name     = "mybackend"
    Language = "mylang"
)

type Plugin struct{}

func New() *Plugin { return &Plugin{} }

func (*Plugin) Name() string     { return Name }
func (*Plugin) Language() string { return Language }

// Render walks every emit entity, groups them by Target, renders
// one file per group, and writes through ctx.Sink.
func (p *Plugin) Render(ctx *sdk.BackendContext) error {
    byTarget := make(map[emit.Target][]emit.Node)
    for _, s := range ctx.Store.Emit().Structs().Items() {
        byTarget[s.Target] = append(byTarget[s.Target], s)
    }
    for target, decls := range byTarget {
        body := renderFile(decls)
        if err := ctx.Sink.Write(target, body); err != nil {
            return err
        }
    }
    return nil
}

func renderFile(decls []emit.Node) []byte {
    // Render decls into your target language. The Go backend uses
    // text/template; a markdown backend might just concatenate
    // declarations; a binary backend might serialise protobuf.
    return nil
}
```

**Key idioms:**

- `ctx.Plugins` and `ctx.Ordered` carry every plugin in the run
  — use them to discover `sdk.TemplateProvider` implementors and
  collect their templates for multi-plugin template merging
- `ctx.Command`, `ctx.SourcesOverride`, `ctx.Brand`, and the
  header / footer slots support reproducible header lines
  (`Code generated by …. DO NOT EDIT.`) for byte-stable goldens
- `ctx.Sink.Write(target, body)` is the only output path; the
  pipeline owns where files actually land (filesystem, archive,
  in-memory)

**Conformance:** `RunSuite` + `RunBackendSuite` against
pre-built emit fixtures.

## Two-role plugins

A plugin may implement multiple role interfaces on one struct.
The framework detects each role via interface assertion and
invokes the matching method in the matching phase.

No reference plugin currently uses this pattern — single-role
plugins are easier to reason about. If you find yourself wanting
two roles, consider splitting into two plugins that share a
capability name via `sdk.CapabilityProvider.Provides` /
`.Requires`.

## Anti-patterns to avoid

- **Reading `Store.Emit` from an annotator.** Annotators run
  before any generator has emitted. The emit view is always
  empty at annotator phase.

- **Mutating `Store.Nodes` from a generator.** The source-side
  store is frozen between the frontend phase and the generator
  phase; the conformance suite's frozen-store check catches this
  via node-count diff.

- **Hand-constructing `emit.Target` literals in a generator.**
  Generators set `Origin` on every emitted decl; the layout
  phase composes `Target.Dir` / `.Filename` / `.Package`
  downstream. Generators that build their own Target hardcode
  routing decisions the framework's layout system is supposed
  to own.

- **Returning a mutated slice from `Provides()` / `Requires()`
  / `Directives()`.** These methods must be deterministic across
  calls; the conformance suite's stability check catches this.

- **Versioning via random / time-derived strings.** The cache
  key composes `Versioned.Version` verbatim; non-deterministic
  versions defeat the cache.
