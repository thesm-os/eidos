# Pattern catalogue

For each common plugin pattern this catalogue walks the matching
reference plugin under `reference/`. Each section carries a working
snippet so you can see the shape without leaving the page; follow the
link for the full plugin, with its helpers and edge cases.

Every reference plugin satisfies the conformance suite and is
production-grade. The snippets here are trimmed from the real source —
where one differs from the plugin it names, the plugin is right.

## Before the patterns: two rules

**Read through `ctx.Reader`, never `ctx.Store`.** Captured reads
compose the plugin's cache key. A read that bypasses the Reader is a
read the cache cannot invalidate on: the source changes, the
fingerprint does not, and the next run serves stale output
indistinguishable from current. `ctx.Store` is for the emit side,
where a generator queues its contributions.

**Write metadata through `EnsureMeta()`, never `Meta()`.** `Meta()` is
the read accessor and returns `nil` for a node nothing has stamped,
which is every node on a cold run. Passing that to a setter panics.

## Annotator — shape inference

**Pattern:** read source nodes, stamp typed metadata, run before the
generator phase. Other plugins read the stamp to decide whether their
path applies.

**Reference:** [`reference/shapewriter`](../../reference/shapewriter)

Detect every struct satisfying the `io.Writer` shape and stamp a typed
key:

```go
package shapewriter

import (
    "go.thesmos.sh/eidos/lang/golang"
    "go.thesmos.sh/eidos/sdk"
    sdkgo "go.thesmos.sh/eidos/lang/golang/sdk"
)

const (
    Name          = "shape-writer"
    Version       = "1.0.0"
    DirectiveName = sdk.DirectiveName("writer")
)

// Detected is the key the plugin stamps. Consumers read it with
// shapewriter.Detected.Get(s.Meta()).
var Detected = sdk.NewKey("shape.writer.detected", sdk.BoolParser)

type Plugin struct{ *sdk.Base }

func New() *Plugin {
    return &Plugin{Base: sdk.NewPlugin(Name).
        Version(Version).
        Priority(sdk.AnnotatorShape).
        Directives(
            sdk.NewDirective(DirectiveName).
                On(sdk.NodeKindStruct).
                Describe("Forces (+) or suppresses (-) writer-shape detection.").
                Build(),
        ).
        Build()}
}

func (p *Plugin) Annotate(ctx *sdk.AnnotatorContext) error {
    return sdk.Walk(ctx, p)
}

// OnStruct runs once per struct in stable insertion order. The
// heuristic decides first; a +gen:writer / -gen:writer directive
// overrides it.
func (*Plugin) OnStruct(_ *sdk.AnnotatorContext, s *sdk.Struct) {
    _, detected := matchSignature(s)
    if d := s.Directive(DirectiveName); d != nil {
        detected = !d.Negated
    }
    Detected.Set(s.EnsureMeta(), detected, Name)
}
```

**Key idioms:**

- `sdk.NewPlugin(name)` returns a fluent builder for the embedded
  `sdk.Base`, which answers the declaration methods — version,
  priority, capabilities, directives — so the plugin body holds only
  its behaviour. `Build()` closes it.
- This annotator declares **no language**. It ships no templates and
  emits no files, so it is language-agnostic by construction: what it
  stamps is metadata, and metadata carries no language. A plugin that
  *does* render declares a `LanguageSupport` bundle per language with
  `For(...)` — see [templates.md](templates.md).
- `sdk.Walk(ctx, p)` dispatches to whichever hooks the plugin
  implements: `OnStruct`, `OnInterface`, `OnMethod`, `OnFunction`,
  `BeforeNodes`, `AfterNodes`.
- `sdk.AnnotatorShape` is the earliest annotator bucket, so refinement
  and validation annotators see the inferred shapes.
- `sdk.NewKey(name, parser)` declares a typed key and registers it
  globally; `sdk.EnsureKey` is the variant for a key several packages
  may declare, since `NewKey` panics on a duplicate.
- The directive is **scoped with `sdk.NodeKindStruct`**, not
  `sdk.EmitKindStruct`. Both exist, they carry different values, and a
  directive scoped to an emit kind matches no source node — so the
  plugin never fires and nothing reports it.
- A `+gen:writer` overriding the heuristic is deliberate and
  undetectable from inside the plugin: the user's statement is final.

**Conformance:** `RunSuite` + `RunAnnotatorSuite`.

## Generator — per-source-decl emission

**Pattern:** read a directive-tagged source decl, emit a counterpart.
The canonical "for each `+gen:repo` struct emit a `<Type>Repository`
interface and a `<Type>Repo` struct".

**Reference:** [`reference/repogen`](../../reference/repogen)
(canonical), [`reference/mockgen`](../../reference/mockgen),
[`plugins/generator/enum`](../../plugins/generator/enum)
(template-driven variant)

```go
package repogen

import (
    "go.thesmos.sh/eidos/sdk"
    sdkgo "go.thesmos.sh/eidos/lang/golang/sdk"
)

const (
    Name           = "repogen"
    Version        = "1.0.0"
    Capability     = "repository"
    DirectiveName  = sdk.DirectiveName("repo")
    FilenameSuffix = "_repo.go"
)

// Options is the typed configuration, declared through the embedded
// holder. Defaults apply at bind time, so tests and direct calls see
// populated values too.
type Options struct {
    InterfaceSuffix string `eidos:"interface_suffix,default=Repository"`
    StructSuffix    string `eidos:"struct_suffix,default=Repo"`
}

type Plugin struct {
    *sdk.Base
    *sdk.Holder[Options]
    opts Options
}

// repogen_go.go — it owns a Go file but renders it through the
// backend's own kind templates, so it ships no tree of its own.
func goSupport() (string, sdk.LanguageSupport) {
    return sdkgo.Builtin(sdk.Output{Suffix: FilenameSuffix})
}

func New() *Plugin {
    p := &Plugin{Base: sdk.NewPlugin(Name).
        For(goSupport()).
        Version(Version).
        Priority(sdk.GeneratorFoundation).
        Provides(Capability).
        Directives(
            sdk.NewDirective(DirectiveName).
                On(sdk.NodeKindStruct).
                Describe("Forces (+) or suppresses (-) repository emission.").
                Build(),
        ).
        Build()}
    p.Holder = sdk.BindOptions(&p.opts)
    return p
}

// Generate emits one package of output per source package holding at
// least one +gen:repo struct.
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
    structs := ctx.Reader.Structs().Where(func(s *sdk.Struct) bool {
        return s.HasPositiveDirective(DirectiveName)
    }).Slice()

    for _, src := range structs {
        // Struct.Package is the import path, not the package name;
        // the package node carries both.
        srcPkg, ok := ctx.Reader.Store().Nodes().Packages().ByQName(src.Package)
        if !ok {
            continue
        }
        c := sdk.NewProvenance(Name)
        pkg := c.Package(srcPkg.Name, srcPkg.Path)
        p.emitOne(pkg, src, sdk.External(src.Package, src.Name))
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

func (p *Plugin) emitOne(pkg *sdk.PackageBuilder, src *sdk.Struct, srcRef sdk.Ref) {
    pkg.Interface(src.Name+p.opts.InterfaceSuffix, func(i *sdk.InterfaceBuilder) {
        i.Origin(src)
        i.Method("Get", func(m *sdk.MethodBuilder) {
            m.Param("ctx", sdk.External("context", "Context"))
            m.Param("id", sdk.Builtin("string"))
            m.Return(sdk.Ptr(srcRef))
            m.Return(sdk.Builtin("error"))
        })
    })

    pkg.Struct(src.Name+p.opts.StructSuffix, func(s *sdk.StructBuilder) {
        s.Origin(src)
    })
}
```

**Key idioms:**

- Embedding `*sdk.Holder[Options]` and assigning `sdk.BindOptions`
  in `New` satisfies the options contract with no per-plugin
  boilerplate.
- `sdk.NewProvenance(Name)` opens a provenance context scoped to the
  plugin. Everything built through its `Package(name, path)` builder
  carries that attribution automatically, which is what the backend
  orders slot contributions by and what the manifest records.
- `ctx.Reader.Structs().Where(...).Slice()` filters through the
  read-tracking reader, so the filter lands in the cache key. The same
  query off `ctx.Store` would not.
- `i.Origin(src)` back-links each emitted declaration to its source.
  **Layout composes the output path from that origin** — which is why
  a generator never computes its own filename.
- `sdk.External(pkg, name)`, `sdk.Builtin(name)`, `sdk.Ptr(ref)` are
  the type-reference constructors. Cross-package references travel as
  `External`, never as a rendered string: the backend registers the
  import automatically, so templates carry no import block.

**A note on the origin.** Because Layout routes from the origin, an
output carrying *methods* cannot be moved out of its type's own
package — Go permits a method declaration only there. An `+gen:out` on
such a source produces a file naming an undefined type. If your
primary output carries methods, say so in the package doc.

**Conformance:** `RunSuite` + `RunGeneratorSuite` + `RunOptionsSuite`.

## Generator — cross-cutting slot contributor

**Pattern:** contribute statements / fields / methods to
existing emit decls (typically those another generator already
emitted), without owning your own routable output.

**Reference:** [`reference/auditweaver`](../../reference/auditweaver), [`reference/debugweaver`](../../reference/debugweaver) — both take Option B below.

```go
package debugweaver

import (
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
            On(sdk.NodeKindMethod).
            Describe("Suppresses (-) the debug-entry trace on the host method.").
            Build(),
    }
}

```

### Spelling the contribution: two valid options

The skeleton above is the same either way. What differs is how the
contributed statement gets its rendered form. Both options are
supported; pick per contribution, not per project.

The `prebody` / `postbody` slots are constrained to `sdk.EmitKindStmt`,
so whatever you append has to *be* an `*sdk.Stmt`. That constraint
is what the two options work with.

| | Option A — build it in Go | Option B — render your own template |
|---|---|---|
| Ships a template | no | yes |
| Declares an emit kind | no | yes |
| Implements `TemplateProvider` | no | yes (3 methods) |
| Spelling lives in | the `sdk.Stmt` union | a `.tmpl` file |
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
        stmt := sdk.NewExprStmt(sdk.NewCall(
            sdk.NewExternal(p.opts.Package, p.opts.Func),
            sdk.NewLiteralString(p.opts.Format),
            sdk.NewLiteralString(m.OwnerName()+"."+m.Name),
        ))
        // AppendPrebody only fails on a nil or unsupported host,
        // neither of which EmitMethods can yield.
        _ = c.AppendPrebody(m, stmt, EntryID)
    }
    return nil
}
```

The cost shows up when the contribution grows. Anything the
`sdk.Stmt` union does not model has to become `sdk.NewRawStmt`
text, and at that point the plugin is formatting Go source by hand.

#### Option B — render through your own template

Declare an emit kind, ship its template, and wrap the value in
`sdk.NewRenderStmt`. The wrapper reports `sdk.EmitKindStmt`, so it
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

// Trace is the plugin's own emit value. The fields are sdk.Expr
// rather than strings so the backend does the literal escaping and
// registers the import for FuncRef on the host file.
type Trace struct {
    sdk.BaseEmit

    FuncRef *sdk.Expr
    Format  *sdk.Expr
    Subject *sdk.Expr
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
            Format:   sdk.NewLiteralString(p.opts.Format),
            Subject:  sdk.NewLiteralString(m.OwnerName() + "." + m.Name),
        }
        _ = c.AppendPrebody(m, sdk.NewRenderStmt(trace), EntryID)
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
  position itself with `sdk.InsertBefore` / `sdk.InsertAfter`. The raw
  `sdk.Slot.Append` takes an `sdk.EmitProvenance` and returns an
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
    "go.thesmos.sh/eidos/sdk"
)

const Name = "registry-gen"

// Kind is the plugin-defined emit kind. The dotted spelling keeps
// it outside emit.* (which is reserved for core emit types).
const Kind sdk.Kind = "registrygen.registration"

// SlotName is the file-level slot the contributions land in, so
// every registration routed to one file shares its func init().
const SlotName = "init"

// Registration is the plugin's emit type. Embeds sdk.BaseEmit
// for the shared Node methods (Pos, Docs, Directives, Meta,
// Origin, SetBy).
type Registration struct {
    sdk.BaseEmit
    Name    string      // the key the registry records the value under
    NameLit *sdk.Expr  // that name, pre-quoted for renderExpr
    Init    *sdk.Expr  // the value expression evaluated at init time
}

func (*Registration) Kind() sdk.Kind { return Kind }

// Compile-time confirmation that *Registration satisfies sdk.EmitNode.
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
- `sdk.BaseEmit` embedded on the custom type provides the
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
    HandlerRef sdk.Ref

    chain *sdk.Slot
}

func (*MiddlewareStack) Kind() sdk.Kind { return Kind }

// Chain returns the slot contributors append into, creating it on
// first use. The empty ElemKind leaves the slot unconstrained —
// each contributor brings its own emit kind and template, so no
// single kind could describe the contents.
func (s *MiddlewareStack) Chain() *sdk.Slot {
    if s.chain == nil {
        s.chain = sdk.NewSlot(ChainSlot, "")
        s.chain.Owner = s
    }
    return s.chain
}

// Slot satisfies sdk.SlotHost so the backend's `slot` template
// helper reaches the chain by name. An unknown name returns an
// empty slot rather than nil, so a template asking for a slot this
// kind lacks renders nothing instead of failing.
func (s *MiddlewareStack) Slot(name string) *sdk.Slot {
    if name == ChainSlot {
        return s.Chain()
    }
    return sdk.NewSlot(name, "")
}

var _ sdk.SlotHost = (*MiddlewareStack)(nil)
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
  `slot "chain" .`. It returns an `*sdk.Slot`, which a template
  cannot range over — iterate `.Items`
- `render` dispatches an item to the template registered under its
  `Kind()`. `renderExpr` works only on a slot constrained to
  `sdk.EmitKindExpr`; a heterogeneous slot needs `render`
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

**Reference:** [`lang/protobuf/frontend`](../../lang/protobuf/frontend) (uses `protocompile` for real proto parsing)

```go
package myfrontend

import (
    "fmt"
    "go.thesmos.sh/eidos/sdk"
)

const Name = "myfrontend"

type Options struct {
    Dir string `eidos:"dir,required"`
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

func (*Plugin) Name() string { return Name }

// Version contributes to the cache key. Bump when the frontend's
// output shape changes in a way that should invalidate caches.
func (*Plugin) Version() string         { return "1.0.0" }
func (*Plugin) EmitVersions() []string  { return []string{"1"} }

// Load parses ctx.Pattern from the configured directory and
// populates ctx.Store.Nodes() via AddPackage. Per-input issues
// attach to ctx.Diag; fatal failures return a non-nil error.
func (p *Plugin) Load(ctx *sdk.FrontendContext) error {
    pkg := &sdk.Package{Name: "example", Path: "example.com/parsed"}
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
  declaration: the pipeline checks it against the emit
  model's own major version at Build time and rejects an incompatible plugin;
  it does not enter the cache key
- `ctx.Store.Nodes().AddPackage(pkg)` is the canonical way to
  register a parsed package; the store auto-indexes by kind /
  package / directive / meta-key

**Conformance:** `RunSuite` + `RunFrontendSuite` against
representative source-directory fixtures.

## Backend — target language renderer

**Pattern:** consume the emit graph and write rendered files
through a `sink.Sink`. Exactly one backend per pipeline.

**Reference:** [`lang/golang/backend`](../../lang/golang/backend)

```go
package mybackend

import (
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
    byTarget := make(map[sdk.EmitTarget][]sdk.EmitNode)
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

func renderFile(decls []sdk.EmitNode) []byte {
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

- **Hand-constructing `sdk.EmitTarget` literals in a generator.**
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
