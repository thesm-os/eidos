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

const DirectiveName sdk.DirectiveName = "writer"

// Detected is the meta key the plugin stamps. Consumers read via
// shapewriter.Detected.Get(node.Meta()).
var Detected = meta.NewKey("shape.writer.detected", meta.BoolParser)

type Plugin struct{}

func New() *Plugin { return &Plugin{} }

func (*Plugin) Name() string           { return Name }
func (*Plugin) Priority() sdk.Priority { return sdk.AnnotatorShape }
func (*Plugin) Provides() []string     { return nil }
func (*Plugin) Requires() []string     { return nil }

// Directives declares the schema for the directive OnStruct reads.
func (*Plugin) Directives() []sdk.DirectiveSchema {
    return []sdk.DirectiveSchema{
        sdk.NewDirective(DirectiveName).
            On(node.KindStruct).
            Describe("Forces (+) or suppresses (-) writer-shape detection on the host struct.").
            Build(),
    }
}

// Annotate dispatches to the per-kind hooks via sdk.Walk.
func (p *Plugin) Annotate(ctx *sdk.AnnotatorContext) error {
    return sdk.Walk(ctx, p)
}

// OnStruct is the sdk.StructHook entry point — invoked once per
// struct in stable insertion order. The heuristic runs first; a
// +gen:writer / -gen:writer directive overrides its outcome.
func (*Plugin) OnStruct(_ *sdk.AnnotatorContext, s *node.Struct) {
    _, detected := matchSignature(s)
    if d := s.Directive(DirectiveName); d != nil {
        detected = !d.Negated
    }
    Detected.Set(s.Meta(), detected, Name)
}
```

**Key idioms:**

- `sdk.Walk(ctx, p)` dispatches to whichever hook interfaces (`OnStruct`, `OnInterface`, `OnMethod`, `OnFunction`, `BeforeNodes`, `AfterNodes`) the plugin implements
- `sdk.AnnotatorShape` is the earliest annotator priority; refinement / validation annotators see the inferred shapes
- `sdk.CapabilityProvider` is all-or-nothing — a plugin declaring `Priority` without `Provides` **and** `Requires` fails the suite's "CapabilityProvider is implemented in full or not at all" check and silently runs in the default bucket
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
// sdk.OptionsProvider. Defaults are pre-applied at bind time.
type Options struct {
    InterfaceSuffix string `eidos:"interface_suffix,default=Repository"`
    StructSuffix    string `eidos:"struct_suffix,default=Repo"`
    Naming          string `eidos:"naming,one_of=Pascal|Camel,default=Pascal"`
}

type Plugin struct {
    *sdk.Holder[Options]  // embeds the OptionsSchema / SetOptions methods
    opts Options
}

func New() *Plugin {
    p := &Plugin{}
    p.Holder = sdk.BindOptions(&p.opts)
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
            Describe("Forces (+) or suppresses (-) repository emission for the host struct.").
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
        // node.Struct.Package is the import path, not the package
        // name; the package node carries both.
        srcPkg, ok := ctx.Reader.Store().Nodes().Packages().ByQName(src.Package)
        if !ok {
            continue
        }
        c := sdk.NewProvenance(Name, sdk.EmitTarget{})
        pkg := c.Package(srcPkg.Name, srcPkg.Path)
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

- `*sdk.Holder[Options]` embedding satisfies `sdk.OptionsProvider`
  without per-plugin boilerplate
- `sdk.NewProvenance(Name, sdk.EmitTarget{})` returns a provenance
  context scoped to the plugin's identity; its `Package(name, path)`
  method opens a package builder, and every emit decl built through
  it auto-stamps its SetBy attribution
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

**Reference:** [`reference/auditweaver`](../../reference/auditweaver), [`reference/debugweaver`](../../reference/debugweaver) — both take Option B below.

```go
package debugweaver

import (
    "go.thesmos.sh/eidos/emit"
    "go.thesmos.sh/eidos/node"
    "go.thesmos.sh/eidos/sdk"
)

const (
    Name          = "debug-weaver"
    Capability    = "trace"
    DirectiveName = sdk.DirectiveName("debug")
    EntryID       = "trace.entry"
)

type Options struct {
    Package string `eidos:"package,default=log"`
    Func    string `eidos:"func,default=Printf"`
    Format  string `eidos:"format,default=debug: %s entered"`
}

type Plugin struct {
    *sdk.Holder[Options]
    opts Options
}

func New() *Plugin {
    p := &Plugin{}
    p.Holder = sdk.BindOptions(&p.opts)
    return p
}

func (*Plugin) Name() string           { return Name }
func (*Plugin) Priority() sdk.Priority { return sdk.GeneratorCrossCutting }
func (*Plugin) Provides() []string     { return []string{Capability} }
func (*Plugin) Requires() []string     { return nil }
// No FilenameProvider — this plugin emits no routable decls.

// Directives declares the suppression directive the plugin reads.
func (*Plugin) Directives() []sdk.DirectiveSchema {
    return []sdk.DirectiveSchema{
        sdk.NewDirective(DirectiveName).
            On(node.KindMethod).
            Describe("Suppresses (-) the debug-entry trace on the host method.").
            Build(),
    }
}

```

### Spelling the contribution: two valid options

The skeleton above is the same either way. What differs is how the
contributed statement gets its rendered form. Both options are
supported; pick per contribution, not per project.

The `prebody` / `postbody` slots are constrained to `emit.KindStmt`,
so whatever you append has to *be* an `*emit.Stmt`. That constraint
is what the two options work with.

| | Option A — build it in Go | Option B — render your own template |
|---|---|---|
| Ships a template | no | yes |
| Declares an emit kind | no | yes |
| Implements `TemplateProvider` | no | yes (3 methods) |
| Spelling lives in | the `emit.Stmt` union | a `.tmpl` file |
| Best when | the contribution is one call and its shape will not change | the contribution has structure, or you want to restyle it without touching Go |

#### Option A — build the statement in Go

Smallest surface: no kind, no template, no `TemplateProvider`. The
statement shape is right there in the generator.

```go
// Generate appends a logging call to every emit method's Prebody
// slot, skipping the methods that carry -gen:debug.
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
    c := sdk.NewProvenance(Name, sdk.EmitTarget{})
    for _, m := range ctx.Reader.EmitMethods().Slice() {
        if m.HasNegatedDirective(DirectiveName) {
            continue
        }
        stmt := emit.NewExprStmt(emit.NewCall(
            sdk.NewExternal(p.opts.Package, p.opts.Func),
            emit.NewLiteralString(p.opts.Format),
            emit.NewLiteralString(m.OwnerName()+"."+m.Name),
        ))
        // AppendPrebody only fails on a nil or unsupported host,
        // neither of which EmitMethods can yield.
        _ = c.AppendPrebody(m, stmt, EntryID)
    }
    return nil
}
```

The cost shows up when the contribution grows. Anything the
`emit.Stmt` union does not model has to become `emit.NewRawStmt`
text, and at that point the plugin is formatting Go source by hand.

#### Option B — render through your own template

Declare an emit kind, ship its template, and wrap the value in
`emit.NewRenderStmt`. The wrapper reports `emit.KindStmt`, so it
satisfies the slot; the backend then renders it by dispatching to
the template registered under the wrapped node's `Kind`.

This is what the in-tree weavers do, and it is the same mechanism
every slot-host contributor uses — the difference is only that a
core `prebody` slot needs the wrapper, where a plugin-declared slot
(see [composition.md](composition.md)) can be left unconstrained and
take the node directly.

```go
// Kind must equal the `define` name in the shipped template.
const Kind sdk.Kind = "debugweaver.trace"

//go:embed templates/golang/*.tmpl
var goTemplates embed.FS

// Trace is the plugin's own emit value. The fields are emit.Expr
// rather than strings so the backend does the literal escaping and
// registers the import for FuncRef on the host file.
type Trace struct {
    sdk.BaseEmit

    FuncRef *emit.Expr
    Format  *emit.Expr
    Subject *emit.Expr
}

func (*Trace) Kind() sdk.Kind { return Kind }

func (*Plugin) Templates(lang string) (fs.FS, bool) {
    if lang != "golang" {
        return nil, false
    }
    sub, err := fs.Sub(goTemplates, "templates/golang")
    if err != nil {
        return nil, false
    }
    return sub, true
}

// Return nil from both: the shared Go helpers are already in the
// backend's funcmap, and re-registering them is a Build-time
// ErrTemplateFuncCollision.
func (*Plugin) TemplateFuncs(string) template.FuncMap     { return nil }
func (*Plugin) TemplateOverrides(string) template.FuncMap { return nil }

func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
    c := sdk.NewProvenance(Name, sdk.EmitTarget{})
    for _, m := range ctx.Reader.EmitMethods().Slice() {
        if m.HasNegatedDirective(DirectiveName) {
            continue
        }
        trace := &Trace{
            BaseEmit: sdk.BaseEmit{SetByName: c.SetBy(), SourcePos: m.Pos()},
            FuncRef:  sdk.NewExternal(p.opts.Package, p.opts.Func),
            Format:   emit.NewLiteralString(p.opts.Format),
            Subject:  emit.NewLiteralString(m.OwnerName() + "." + m.Name),
        }
        _ = c.AppendPrebody(m, emit.NewRenderStmt(trace), EntryID)
    }
    return nil
}
```

`templates/golang/debugweaver.trace.tmpl`:

```gotemplate
{{- define "debugweaver.trace" -}}
{{ renderExpr .FuncRef }}({{ renderExpr .Format }}, {{ renderExpr .Subject }})
{{- end -}}
```

Two naming rules bite here, and both fail at render time rather
than at build time:

- The `define` name must equal the declared `Kind`. The backend
  looks the template up by kind; a mismatch finds nothing.
- The template *filename* must be unique across every plugin in the
  pipeline. Templates are registered under their base filename as
  well as their define name, so two plugins each shipping
  `entry.tmpl` collide even with different define names. Name the
  file for the plugin.

**Key idioms:**

- No `FilenameProvider` — the plugin declares no routable decl, so
  the framework expects only the slot-contribution surface. It
  still declares `Directives()`: the suppression directive is the
  plugin's own, and one plugin owns a directive name for the run
- Contributions go through a `sdk.Provenance`, not through the
  slot directly: `c.AppendPrebody(host, stmt, id)` stamps SetBy
  attribution and the per-contribution ID that lets a later weaver
  position itself with `builder.Before` / `builder.After`. The raw
  `emit.Slot.Append` takes an `emit.Provenance` and returns an
  error, so bypassing the context loses the attribution the
  manifest and file headers read
- Opt-out beats opt-in for an unconditional cross-cutting concern:
  the weaver applies to every emit method and `-gen:debug`
  suppresses it, so the source needs no annotation to get traced
- `sdk.GeneratorCrossCutting` runs after `GeneratorFoundation`
  and `GeneratorComposition` so the host decls exist by the time
  the weaver visits them

**Conformance:** `RunSuite` + `RunGeneratorSuite` (empty-store
no-panic; determinism; unchanged source-side node counts) +
`RunOptionsSuite`.

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

    "go.thesmos.sh/eidos/emit"
    "go.thesmos.sh/eidos/sdk"
)

const Name = "registry-gen"

// Kind is the plugin-defined emit kind. The dotted spelling keeps
// it outside emit.* (which is reserved for core emit types).
const Kind sdk.Kind = "registrygen.registration"

// SlotName is the file-level slot the contributions land in, so
// every registration routed to one file shares its func init().
const SlotName = "init"

// Registration is the plugin's emit type. Embeds emit.BaseEmit
// for the shared Node methods (Pos, Docs, Directives, Meta,
// Origin, SetBy).
type Registration struct {
    emit.BaseEmit
    Name    string      // the key the registry records the value under
    NameLit *emit.Expr  // that name, pre-quoted for renderExpr
    Init    *emit.Expr  // the value expression evaluated at init time
}

func (*Registration) Kind() sdk.Kind { return Kind }

// Compile-time confirmation that *Registration satisfies emit.Node.
var _ sdk.EmitNode = (*Registration)(nil)

type Plugin struct{ /* sdk.Holder elided */ }

func (*Plugin) Name() string { return Name }

// Generate visits every +gen:register-tagged source struct and
// appends a Registration to the origin's "init" slot. The plugin
// sets no Target; Layout resolves the origin to a file.
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
    c := sdk.NewProvenance(Name, sdk.EmitTarget{})
    for _, s := range ctx.Reader.Structs().Slice() {
        if !s.HasPositiveDirective(DirectiveName) {
            continue
        }
        reg := &Registration{ /* BaseEmit + fields elided */ }
        err := ctx.Store.Emit().AppendOriginSlot(
            s, SlotName, reg, c.Provenance("registry."+s.Name),
        )
        if err != nil {
            return err
        }
    }
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

- The custom kind is declared as an `sdk.Kind` constant outside
  the `emit.*` namespace — `<package>.<kind>` is the convention
  (`registrygen.registration`, `middlewaregen.stack`,
  `handlergen.handler`), and the string must match the `define`
  name in the shipped template or the value renders as nothing
- `emit.BaseEmit` embedded on the custom type provides the
  shared `Pos`, `Docs`, `Directives`, `Meta`, `Origin`, and
  `SetBy` accessors
- `var _ sdk.EmitNode = (*Registration)(nil)` is the compile-time
  interface-satisfaction check
- The template is shipped via `//go:embed` + `fs.Sub` so the
  plugin's templates ride alongside its code. Template *filenames*
  must be unique across every plugin in the run, not only their
  `define` names
- `TemplateFuncs` returns `nil` unless the plugin has a helper of
  its own — the shared `lang/golang.FuncMap()` is already merged
  into the backend's overrideable funcmap, and re-registering it is
  a Build-time `ErrTemplateFuncCollision`

**Conformance:** `RunSuite` + `RunGeneratorSuite` +
`RunOptionsSuite`. The plugin-defined kind doesn't change which
suites apply.

## Generator — custom-slot composition

**Pattern:** declare a plugin-defined emit kind that owns a *named
slot*, so other plugins contribute into a structure this plugin
owns rather than into a core slot the framework already defines.
The host runs in `GeneratorFoundation`; contributors run in
`GeneratorComposition` and reach the host value through
`Store.Emit().PendingOriginSlots()` before Layout has routed it.

**Reference:** [`reference/middlewaregen`](../../reference/middlewaregen)
(the slot owner), [`reference/authgen`](../../reference/authgen) and
[`reference/metricgen`](../../reference/metricgen) (contributors)

**Deep dive:** [composition.md](composition.md) walks the whole
ensemble — the host, both contributors, the ordering, and the
template that renders a heterogeneous slot.

```go
package middlewaregen

// The directive is declared and owned by handlergen; this plugin
// only reads the stamp. A directive name may be registered once
// per run, so redeclaring it is ErrDuplicateDirective at Build.
const DirectiveName = handlergen.DirectiveName

// ChainSlot is exported because it is the coupling: contributors
// name it, so it is part of the published contract.
const ChainSlot = "chain"

const Kind sdk.Kind = "middlewaregen.stack"

type MiddlewareStack struct {
    sdk.BaseEmit
    VarName    string
    TypeName   string
    HandlerRef emit.Ref

    chain *emit.Slot
}

func (*MiddlewareStack) Kind() sdk.Kind { return Kind }

// Chain returns the slot contributors append into, creating it on
// first use. The empty ElemKind leaves the slot unconstrained —
// each contributor brings its own emit kind and template, so no
// single kind could describe the contents.
func (s *MiddlewareStack) Chain() *emit.Slot {
    if s.chain == nil {
        s.chain = emit.NewSlot(ChainSlot, "")
        s.chain.Owner = s
    }
    return s.chain
}

// Slot satisfies emit.SlotHost so the backend's `slot` template
// helper reaches the chain by name. An unknown name returns an
// empty slot rather than nil, so a template asking for a slot this
// kind lacks renders nothing instead of failing.
func (s *MiddlewareStack) Slot(name string) *emit.Slot {
    if name == ChainSlot {
        return s.Chain()
    }
    return emit.NewSlot(name, "")
}

var _ emit.SlotHost = (*MiddlewareStack)(nil)
```

The contributor side reaches the host value through the pending
origin-slot list, because Layout has not run yet:

```go
func (*Plugin) Generate(ctx *sdk.GeneratorContext) error {
    c := sdk.NewProvenance(Name, sdk.EmitTarget{})
    for _, pending := range ctx.Store.Emit().PendingOriginSlots() {
        stack, ok := pending.Item.(*middlewaregen.MiddlewareStack)
        if !ok {
            continue
        }
        entry := &Entry{ /* the contributor's own emit kind */ }
        if err := stack.Chain().Append(entry, c.Provenance(EntryID)); err != nil {
            return fmt.Errorf("%s: append chain entry: %w", Name, err)
        }
    }
    return nil
}
```

The host's template ranges the slot and dispatches each item to
whichever template owns it:

```gotemplate
var {{ .VarName }} = []func({{ renderType .HandlerRef }}) {{ renderType .HandlerRef }}{
{{- range (slot . "chain").Items }}
    {{ render . }},
{{- end }}
}
```

**Key idioms:**

- `slot` takes the host first: `slot . "chain"`, not
  `slot "chain" .`. It returns an `*emit.Slot`, which a template
  cannot range over — iterate `.Items`
- `render` dispatches an item to the template registered under its
  `Kind()`. `renderExpr` works only on a slot constrained to
  `emit.KindExpr`; a heterogeneous slot needs `render`
- `Slot.Append` appends to the end, so slot order is append order —
  and what decides append order is the plan, not registration
  order. The plan groups plugins into priority buckets and
  topo-sorts by capability *within* each bucket, so metricgen
  appends after authgen because it names authgen's capability in
  `Requires`, while the bucket alone is what orders both after
  their foundation-bucket host
- A contributor that ships a template needs no `FilenameProvider`:
  templates say how a value renders, outputs say where a file
  lands, and the contributor renders inside a file it does not own
- Depending on the host's *value* is what makes the absent-host
  case degrade to nothing. The alternative — a contributor
  declaring an `sdk.Output` with the host's suffix and routing by
  origin — also lands both halves in one file, but Layout's
  `FileFor` is lookup-**or-create**, so a contributor running
  without its host conjures an orphan file holding only its own
  half

**Conformance:** `RunSuite` + `RunGeneratorSuite` for host and
contributors alike. The cross-plugin behaviour is not something the
per-plugin suites can see — assert it with a pipeline test that
registers the ensemble and reads the rendered file.

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
- `Versioned.Version` is the frontend's contribution to the
  run's composition fingerprint — bumping it invalidates
  downstream caches. `EmitVersioned.EmitVersions` is a separate
  declaration: the pipeline checks it against the in-tree
  `emit.Major` at Build time and rejects an incompatible plugin;
  it does not enter the cache key
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

- **Mutating `Store.Nodes` from a generator.** The pipeline
  freezes the node view at the end of the frontend phase, so
  `AddPackage` returns `store.ErrFrozen` from the annotator phase
  onward. `RunGeneratorSuite`'s "source-side node counts unchanged
  by Generate" check catches a generator that got past that by
  mutating the buckets some other way.

- **Hand-constructing `emit.Target` literals in a generator.**
  Generators set `Origin` on every emitted decl; the layout
  phase composes `Target.Dir` / `.Filename` / `.Package`
  downstream. Generators that build their own Target hardcode
  routing decisions the framework's layout system is supposed
  to own.

- **Returning the plugin's own slice from `Provides()` /
  `Requires()` / `Directives()` / `Outputs()`.** These methods must
  be deterministic across calls — `RunSuite`'s
  "CapabilityProvider returns deterministic Provides + Requires"
  and "FilenameProvider returns stable Outputs per language" checks
  cover that. They must also hand back a slice the caller may keep:
  the pipeline stores what it gets, so a returned field aliasing
  plugin state lets any consumer that sorts or filters in place
  rewrite the declaration for every later caller. `RunSuite`'s
  "declaration accessors return slices the caller may keep" check
  catches the aliasing; `slices.Clone` is the fix.

- **Versioning via random / time-derived strings.** The cache
  key composes `Versioned.Version` verbatim; non-deterministic
  versions defeat the cache.
