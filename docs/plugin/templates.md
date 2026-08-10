# Templates with generator plugins

When a generator emits a plugin-defined emit kind — a custom
type outside the `emit.*` namespace, like `Registration` from
`registrygen` or `Saga` from a hypothetical workflow plugin —
the backend needs to know how to render it. The
`sdk.TemplateProvider` capability is the bridge: a plugin ships
templates alongside its code, the backend picks them up, and
the rendered output flows through the same finalisation passes
(import resolution, `gofmt`, header stamping) as the core emit
kinds.

This guide walks through the full pattern end-to-end using
[`reference/registrygen`](../../reference/registrygen) as the
canonical example.

## The capability

```go
type TemplateProvider interface {
    Templates(lang string) (fs.FS, bool)
    TemplateFuncs(lang string) template.FuncMap
    TemplateOverrides(lang string) template.FuncMap
}
```

All three methods are language-scoped via the `lang` argument
(`"golang"`, `"rust"`, `"ts"`, …). A plugin that contributes
templates only to the Go backend returns `(nil, false)` from
`Templates` for every other language.

## Step 1: define the emit kind

A plugin-defined emit kind is a Go struct embedding
`sdk.BaseEmit` plus a `Kind()` method returning a namespaced
`sdk.Kind` constant. From registrygen:

```go
package registrygen

import (
    "go.thesmos.sh/eidos/sdk"
)

// Kind keeps the registration kind outside the emit.* namespace
// reserved for core emit types.
const Kind sdk.Kind = "registrygen.registration"

type Registration struct {
    sdk.BaseEmit

    Name         string
    NameLit      *sdk.Expr  // pre-built string literal
    Init         *sdk.Expr  // value passed to register call
    RegisterFunc *sdk.Expr  // the register call's callee
}

func (*Registration) Kind() sdk.Kind { return Kind }

// Compile-time confirmation that *Registration is a valid
// sdk.EmitNode.
var _ sdk.EmitNode = (*Registration)(nil)
```

`sdk.Kind` aliases `core/sdk.Kind` and `sdk.EmitNode` aliases
`sdk.EmitNode`, so the declaration reads through `sdk` without a
`core/kind` import.

**Naming convention**: the kind string is `<plugin>.<entity>`.
The dotted spelling matches the template-naming convention
below and keeps the kind discoverable in diagnostics.

## Step 2: write the template

Templates live in a per-language subdirectory the plugin owns,
typically `templates/<lang>/<entity>.tmpl`. Use `//go:embed`
to ship them as part of the plugin's binary.

From `reference/registrygen/templates/golang/registration.tmpl`:

```
{{- define "registrygen.registration" -}}
{{ renderExpr .RegisterFunc }}({{ renderExpr .NameLit }}, {{ renderExpr .Init }})
{{- end -}}
```

That's the entire template. It defines a Go `text/template`
named `registrygen.registration` — **matching the `Kind`
constant verbatim** — that emits a single line:

```go
log.Print("Article", Article{})
```

**Naming contract**: every emit kind needs a template whose
name equals its `Kind()` value. The backend's main render
function (`render` in the funcmap) converts `Node.Kind()` to a
string and looks the template up under it, so template
selection is a string match.

**Reserved prefix**: template names beginning with `fragment.`
are reserved for future shared partials; using one fails Build
with `ErrReservedTemplatePrefix`.

## Step 3: ship the template

A Go generator does not answer the template methods itself. It hands
its embedded tree to `sdkgo.NewGenerator`, and the base answers
`Templates`, `TemplateFuncs` and `TemplateOverrides` for Go — and
reports *not provided* for every other language, which is what makes
Layout report a missing provider rather than composing Go-shaped
filenames for a non-Go backend.

```go
//go:embed templates/golang/*.tmpl
var goTemplates embed.FS

func New() *Plugin {
    return &Plugin{Base: sdkgo.NewGenerator(
        Name, goTemplates, sdk.Output{Suffix: FilenameSuffix},
    ).
        Version(Version).
        Priority(sdk.GeneratorCrossCutting).
        Provides(Capability).
        Build()}
}
```

The backend walks every plugin's tree for `*.tmpl` files, parses each,
and adds the defined names to the rendering tree. A plugin may ship
several files; each may define several templates.

**Filenames are template names too.** `text/template` registers each
parsed file under its own path as well as the names its `define`
blocks declare. Two plugins shipping `registration.tmpl` collide at
Build with `ErrTemplateNameCollision` even when their `define` names
differ — and the whole run then writes nothing. The core tree already
occupies the bare names `alias.tmpl`, `constant.tmpl`, `enum.tmpl`,
`function.tmpl`, `interface.tmpl`, `method.tmpl`, `struct.tmpl` and
`variable.tmpl`; a plugin file reusing one displaces the core entry
and records an override diagnostic that reads as a mistake.

Name the file after the emit kind that owns it — `registrygen.entry.tmpl`,
not `entry.tmpl`.

## Step 4: emit the entity in `Generate`

A generator constructing a plugin-defined emit kind looks the
same as one constructing a core kind. From registrygen's
`Generate`:

```go
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
    c := sdk.NewProvenance(Name, sdk.EmitTarget{})
    for _, s := range ctx.Reader.Structs().Slice() {
        if !s.HasPositiveDirective(DirectiveName) {
            continue
        }
        reg := &Registration{
            BaseEmit: sdk.BaseEmit{
                OriginNode: s,
                SetByName:  c.SetBy(),
                SourcePos:  s.Pos(),
            },
            Name:         s.Name,
            NameLit:      sdk.NewLiteralString(s.Name),
            Init:         sdk.NewComposite(sdk.External(s.Package, s.Name), nil),
            RegisterFunc: sdk.NewExternal(p.registerPackage(), p.registerFunc()),
        }
        // Append into the file-level init slot — the layout phase
        // routes the slot's host file based on the source struct's
        // origin.
        if err := ctx.Store.Emit().AppendOriginSlot(
            s, SlotName, reg, c.Provenance("registry."+s.Name),
        ); err != nil {
            return err
        }
    }
    return nil
}
```

`sdk.NewExternal` is the key: it produces an `Expr` referencing
an identifier in a specific package, and the backend
automatically registers that package as an import on the
rendered file. The plugin never touches the file's import set
directly.

## Step 5: render

At render time, the backend:

1. Groups every emit entity by its `sdk.EmitTarget`
2. For each target, calls the canonical template for each entity
   — `render <entity>` dispatches by `Node.Kind()` to the
   appropriately-named template
3. Composes the rendered fragments into one file body, with
   imports resolved and the standard header / footer stamped

For a source struct named `Article`, the registration template
renders as:

```go
log.Print("Article", Article{})
```

The Go backend then groups all registrations for the same file
inside one `func init() { ... }` block (via the file-level
`init` slot), runs `gofmt` + `goimports`, and writes the result
through the backend context's `Sink`. `sdk.GeneratorContext`
carries no sink — a generator hands emit values to the store and
never writes a file.

## The funcmap

Templates have access to a funcmap of helper functions. The
core funcmap, exposed by the Go backend, includes:

- **Dispatch helpers** — `render`, `renderType`, `renderStmt`,
  `renderExpr` — route to the appropriate sub-template based on
  the value's kind. Most plugin templates use `renderExpr` to
  render `*sdk.Expr` values and `renderType` for type
  references.
- **Slot-composition helpers** — `renderStructFields`,
  `renderStructEmbeds`, `renderStructMethods`,
  `renderInterfaceEmbeds`, `renderInterfaceMethods`,
  `renderFunctionBody`, `renderMethodBody`,
  `renderFunctionParams`, `renderMethodParams`,
  `renderFunctionReturns`, `renderMethodReturns`,
  `renderEnumVariants` — merge a host's typed content with the
  contributions cross-cutting plugins appended to the matching
  slot, in plugin-topo order. The two body helpers compose
  prebody + typed body + postbody in one call; there is no
  separate prebody or postbody helper.
- **Render helpers** — `renderParams`, `renderReturns`,
  `renderReceiver`, `renderDocs`, `renderTypeParams` — render
  canonical param lists, return clauses, receiver forms, doc
  blocks, and generic bracket clauses.
- **Collision helpers** — `imp` (import path → alias), `slot`
  (named slot accessor on a host), `external` (build an
  `*sdk.Expr` referencing another package's identifier).
- **Metadata** — `provenance` (an emit value's attribution
  string, `emit.struct from pkg/user.go:42`).

These are **reserved**. A plugin cannot override them; doing so
fails Build with `ErrReservedFuncName`, and registering one as
an extension fails with `ErrTemplateFuncCollision`.

Overrideable leaf utilities sit alongside — case conversion,
meta-bag readers, string helpers, origin-debug helpers, and the
shared `lang/golang` identifier-convention helpers — and are
extended / overridden through the other two methods on
`TemplateProvider`:

## Funcmap extensions: `Funcs`

Returns funcmap entries the plugin contributes. The backend
merges every plugin's returned map at Build time; cross-plugin
name collisions and collisions with a reserved canonical entry
fail Build with `ErrTemplateFuncCollision`. Collisions with an
overrideable entry are not caught — the extension wins, with no
diagnostic — which is the second reason to prefix.

```go
sdkgo.NewGenerator(Name, goTemplates, outputs...).
    Funcs(template.FuncMap{
        "myco_camelCase": camelCase,
        "myco_snakeCase": snakeCase,
    }).
    Build()
```

**Register nothing unless the plugin owns a helper of its own.** The
shared `lang/golang.FuncMap()` — `isExported`, `isByteSlice`,
`selfType`, … — is already merged into the backend's
overrideable bucket, so plugin templates call it without
registering anything. Returning it from `TemplateFuncs` adds
nothing and collides with the next plugin that does the same:
`ErrTemplateFuncCollision`, on `selfType`, from a plugin that
never wrote it. registrygen registers none, which is why its builder chain has no `Funcs` call at all.

**Convention**: prefix extension names with your plugin
identifier (`myco_camelCase`, not `camelCase`) to keep
cross-plugin collisions rare. Reserved names (the dispatch
helpers above) are off-limits regardless.

## Funcmap overrides: `Overrides`

Returns funcmap entries that **intentionally replace**
previously-registered names. The backend records each override
as a diagnostic, naming the winning plugin and the previous
owner, so users can see which plugin's definition won.

```go
func (*Plugin) TemplateOverrides(lang string) template.FuncMap {
    if lang != "golang" {
        return nil
    }
    return template.FuncMap{
        // Replace the core `camel` helper with our locale-aware
        // variant.
        "camel": ourLocaleAwareCamel,
    }
}
```

The override pass runs after the extension pass in capability
topological order, so a downstream plugin can replace an
upstream plugin's funcmap entry. Reserved-name overrides still
fail with `ErrReservedFuncName`.

## Anti-patterns

- **Template name doesn't match `Kind()`.** The backend
  dispatches by `Node.Kind()` value as a string; a mismatched
  template name produces `ErrTemplateMissing` at render time
  (`golang: no template registered for kind: <kind>`), and the
  Target's file is not written.

- **Using `fragment.*` template names.** Reserved for future
  shared partials. Plugin-defined templates using the prefix are
  rejected at parse time.

- **Reusing another plugin's `.tmpl` filename.** The file path
  is a template name in its own right, so `registration.tmpl`
  from two plugins is `ErrTemplateNameCollision` however the
  `define` blocks inside are named.

- **Manually composing imports inside the template.** The
  backend resolves imports from `sdk.NewExternal` /
  `sdk.External` references on emit entities; templates should
  `renderExpr` against the entity, never embed raw package
  paths.

- **Bare funcmap names without a plugin prefix.** Cross-plugin
  collisions happen the moment two plugins ship the same name.
  Prefixing keeps the collision surface small and makes the
  intent ("I am extending the funcmap with these specific
  utilities") explicit.

- **Re-implementing rendering logic the canonical funcmap
  already provides.** `renderExpr`, `renderType`, `renderStmt`
  cover the common cases. A template that hand-builds Go syntax
  has likely missed a helper.

## Quick reference: registrygen end-to-end

The full picture, in one diff against a fresh project:

1. **Source file**: `reg/article.go`

   ```go
   package reg

   // +gen:register
   type Article struct { ID string }
   ```

2. **Plugin runs**: registrygen sees the `+gen:register`
   directive, appends a `Registration{Name: "Article", ...}` to
   the file-level `init` slot.

3. **Backend renders**: template `registrygen.registration`
   emits `log.Print("Article", Article{})`. The Go backend wraps
   it in a file-level `func init()` block.

4. **Output file**: `reg/article_registry.go`

   ```go
   // Code generated by <brand>. DO NOT EDIT.
   //
   // Source:    reg/article.go
   // Plugins:   golang 1.0.0, registry-gen, backend.golang
   // Command:   <brand> run ./...

   package reg

   import (
       "log"
   )

   func init() {
       log.Print("Article", Article{})
   }

   // <brand>: end of generated content.
   // <brand>:provenance <sha256-of-body-bytes>
   ```

The plugin author wrote ~100 lines of Go + a 3-line template;
the framework handled routing, import resolution, slot
composition, header stamping, and `gofmt` finalisation.

## Conformance

A plugin shipping templates satisfies the framework conformance
suite via the standard role + options suites (`RunSuite`,
`RunGeneratorSuite`, `RunOptionsSuite`). Two of `RunSuite`'s
checks are `TemplateProvider`-specific:

- **Templates return stable, well-formed contributions** —
  `Templates(lang)` answers the same way twice, never reports
  `ok` with a nil filesystem, and no name appears in both
  `TemplateFuncs` and `TemplateOverrides`.
- **Shipped templates parse and claim no reserved name** — every
  `*.tmpl` at the filesystem root is parsed with its function
  calls stubbed out, so a syntax error surfaces on the commit
  that introduced it rather than midway through someone else's
  `Render`. Any `define` under the `fragment.` prefix fails here
  too.

Neither check executes a template. Rendered output is exercised
end-to-end through `pipelinetest` or `backendtest` once the
plugin lands in a project that consumes it.
