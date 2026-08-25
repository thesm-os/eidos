# Go backend

The Go backend renders `emit` graphs to gofmt-clean, byte-deterministic Go
source. It implements `plugin.Backend` and exposes an extension surface
plugin authors interact with: a canonical template set, a canonical funcmap,
slot-composition contracts, an envelope (header / footer) with extension
points, and a stable error-sentinel vocabulary.

This document is the contract reference plugins build against.

## Identity

| Constant | Value |
|---|---|
| `backend.Name` | `backend.golang` |
| `backend.Language` | `golang` |

The package is `backend`, imported from
`go.thesmos.sh/eidos/lang/golang/backend`. A file importing both it and
the Go frontend aliases one of them, since the protobuf frontend is
also `package frontend`.

Plugin authors target `backend.Language` when implementing
`plugin.TemplateProvider.Templates(lang)` and the other lang-keyed
methods — or, better, `golang.Language` from `lang/golang`, which is
the same string and carries no dependency on the backend.

## Canonical template set

Eight kind-templates ship with the backend, one per renderable top-level
emit kind. Templates are named verbatim from `emit.Node.Kind()`; the
`render` funcmap entry routes dispatch by kind.

| Template name | Source file | Renders |
|---|---|---|
| `emit.struct` | `templates/struct.tmpl` | Struct decl, fields, embeds, inline method block |
| `emit.interface` | `templates/interface.tmpl` | Interface decl, methods, embeds |
| `emit.function` | `templates/function.tmpl` | Top-level function decl + body |
| `emit.method` | `templates/method.tmpl` | Method decl with receiver clause |
| `emit.enum` | `templates/enum.tmpl` | Underlying-type decl + `const ( … )` variant block |
| `emit.alias` | `templates/alias.tmpl` | Both `type X = Y` and `type X Y` forms |
| `emit.variable` | `templates/variable.tmpl` | `var Name Type = Value` |
| `emit.constant` | `templates/constant.tmpl` | Single-line `const Name Type = Value` |

`emit.File` is composed in Go (header, imports, slot ordering, footer) and
never goes through `render` — see [Render pipeline](#render-pipeline) and
[Canonical file layout](#canonical-file-layout).

Sub-element kinds (Field, EnumVariant, Param, TypeParam, Embed, Import) have
no standalone template — they're inlined by their parent template via the
funcmap. Type refs (TypeRef, ExternalRef, BuiltinRef, CompositeRef) route
through `renderType`. Stmt / Expr route through `renderStmt` / `renderExpr`.

The `fragment.` template-name prefix is reserved for future shared partials;
plugin-defined templates using it are rejected at parse time with
`ErrReservedTemplatePrefix`.

Plugin-defined kinds follow the same convention. A plugin whose emit type
returns `Kind() = "sagagen.saga"` ships a template defining `sagagen.saga`;
the `render` dispatch looks up by `Kind()` and finds it in the merged tree.

## Canonical funcmap

The funcmap is shipped by the backend and merged with plugin contributions
on every `Render` call. Reserved entries cannot be overridden.

### Reserved — dispatch helpers

| Entry | Signature | Purpose |
|---|---|---|
| `render` | `func(emit.Node) (string, error)` | Universal dispatch by `Kind()`. Resolves to either a template (structured kinds) or the appropriate `renderXxx` helper. |
| `renderType` | `func(emit.Ref) (string, error)` | Type-expression spelling. Switches on the four `Ref` kinds: BuiltinRef, ExternalRef (registers via `imp`), TypeRef (same-package by contract), CompositeRef (every shape). |
| `renderStmt` | `func(*emit.Stmt) (string, error)` | Statement-level dispatch on `StmtKind` — covers block, expression-statement, assignment, short-var, return, if/else, for, range-for, switch, defer, go, send, receive, break/continue. |
| `renderExpr` | `func(*emit.Expr) (string, error)` | Expression-level dispatch on `ExprKind` — literal, identifier, binary, unary, paren, call, method-call, field selector, index, slice, type-assert, composite-literal, function-literal, addr, deref. |

### Reserved — canonical render

| Entry | Signature | Purpose |
|---|---|---|
| `renderParams` | `func([]*emit.Param) (string, error)` | Parenthesised param list. Rejects mixed-named with `ErrMixedNamedParams`. |
| `renderReturns` | `func([]*emit.Return) (string, error)` | Return clause: empty / bare-type / parenthesised list per the three-case rule. |
| `renderReceiver` | `func(*emit.Method) (string, error)` | Receiver clause: empty / `(name Type)` / `(Type)` per the three shapes. |
| `renderDocs` | `func([]string) string` | Doc-comment block with `//` prefix, passing directive lines (`//go:build`, `//nolint:`, …) through verbatim. |
| `renderTypeParams` | `func([]*emit.TypeParam) (string, error)` | Generic bracket clause `[T any, U comparable]`. Empty input → empty string. |
| `renderEnumVariants` | `func(*emit.Enum) (string, error)` | Enum variant block (typed-iota or explicit values). |

### Reserved — slot composition

| Entry | Signature | Purpose |
|---|---|---|
| `renderStructFields` | `func(*emit.Struct) (string, error)` | Merges typed `Fields` + `FieldsSlot()` contributions; rejects duplicates with `ErrDuplicateEntity`. |
| `renderStructEmbeds` | `func(*emit.Struct) (string, error)` | Merges typed embeds + `EmbedsSlot()` contributions. |
| `renderStructMethods` | `func(*emit.Struct) ([]*emit.Method, error)` | Returns the merged method slice for the struct template to range. |
| `renderInterfaceMethods` | `func(*emit.Interface) ([]*emit.Method, error)` | Merges typed methods + `MethodsSlot()` contributions; rejects duplicates. |
| `renderInterfaceEmbeds` | `func(*emit.Interface) (string, error)` | Merges typed embeds + `EmbedsSlot()` contributions. |
| `renderFunctionBody` | `func(*emit.Function) (string, error)` | Composes `Prebody()` + typed body + `Postbody()` in plugin-topo order. |
| `renderMethodBody` | `func(*emit.Method) (string, error)` | Same composition for methods. |
| `renderFunctionParams` | `func(*emit.Function) (string, error)` | Appends `ParamsSlot()` to the typed Params then routes through `renderParams`. |
| `renderMethodParams` | `func(*emit.Method) (string, error)` | Same for methods. |

### Reserved — collision handling & metadata

| Entry | Signature | Purpose |
|---|---|---|
| `imp` | `func(path string) (string, error)` | Registers an import path with the per-file `writer.ImportSet`. Returns the local alias to use (`bar`, `bar2`, `bar3`, …) so collisions resolve deterministically. |
| `slot` | `func(host emit.Node, name string) (*emit.Slot, error)` | Per-host slot accessor: `{{ range (slot . "fields").Items }} … {{ end }}`. Returns `ErrSlotHostUnsupported` when host doesn't implement `emit.SlotHost`. |
| `provenance` | `func(emit.Node) string` | Provenance attribution string for an emit value (`emit.struct from pkg/user.go:42`, `emit.function (synthetic)`, `(nil)` for nil hosts). |

### Overrideable — leaf utilities

These are available for plugin extension (registered via `TemplateFuncs`) or
override (via `TemplateOverrides`). The canonical bucket lives in the
`core/naming` package and stdlib strings:

| Names | Source |
|---|---|
| `pascal`, `camel`, `snake`, `screaming`, `exported` | Case conversion via `core/naming`. |
| `meta`, `metaBool`, `metaStr`, `hasMeta`, `metaEq` | Meta-bag readers for templates. |
| `join`, `title`, `upper`, `lower`, `trim`, `split`, `default`, `coalesce` | String / fallback utilities. |
| `origin`, `explain` | Provenance-debug helpers. |

## Plugin extension surface

Plugin authors implement `plugin.TemplateProvider` to contribute templates
and funcmap entries:

```go
type TemplateProvider interface {
    Templates(lang string) (fs.FS, bool)
    TemplateFuncs(lang string) template.FuncMap
    TemplateOverrides(lang string) template.FuncMap
}
```

The backend's two-pass merge runs per `Render` call:

| Pass | Source | Walks | Failure |
|---|---|---|---|
| 1 — Extensions | `TemplateFuncs(lang)` | `ctx.Plugins` in registration order | Name collides with another plugin's extension or with any core canonical entry → `ErrTemplateFuncCollision` |
| 2 — Overrides | `TemplateOverrides(lang)` | `ctx.Ordered` in capability-topo | Name in the reserved set → `ErrReservedFuncName`. Successful override emits an `Info` diagnostic: `plugin <id> overrode <name> (was: <previous owner>)` |

Plugin templates merge in alongside core via `AddParseTree`. Cross-plugin
template-name collisions surface as `ErrTemplateNameCollision`. Plugin-vs-core
template overrides are recorded silently (the canonical extension story).

## Plugin-defined emit kinds

A plugin shipping its own emit kind:

1. Defines a Go type with a `Kind()` method returning a `directive.Kind` outside the `emit.*` namespace (e.g. `directive.Kind("sagagen.saga")`).
2. Ships a template `define "<kind>"` under `templates/<lang>/` in its filesystem.
3. Implements `TemplateProvider.Templates(lang)` to surface the FS.

```
plugins/sagagen/
    saga.go              # emit.Saga, emit.SagaStep
    plugin.go            # implements Generator + TemplateProvider
    templates/golang/
        saga.tmpl        # define "sagagen.saga" { ... }
        saga_step.tmpl   # define "sagagen.saga_step" { ... }
```

Plugin templates use the same merged funcmap — `imp`, `pascal`, `slot`, the
slot-composition helpers — so plugin kinds compose cleanly alongside core
kinds in the same Target. See `lang/golang/backend/testdata/pluginfixture/`
for a working
end-to-end fixture.

## Slot composition

The backend supports two complementary file-composition mechanisms.

**Mechanism 1 — Free-floating decls with the same Target.** Multiple
plugins emit decls (Struct, Interface, Function, Method, Variable, Constant,
Alias, Enum) with their `Target` set to the same file. The backend renders
them at layout item 6, sorted by capability-topo of the producing plugin
then by `QName()`. Same-`QName()` duplicates → `ErrDuplicateEntity`.

**Mechanism 2 — Origin-anchored slots on `emit.File`.** Plugins attach
file-level contributions through `EmitView.AppendOriginSlot(origin,
slotName, item, prov)`. The Layout phase resolves each origin to its
target file using the standard precedence model, then materialises
the contribution into the named slot on that file:

| Slot | Renders at | Item Kind |
|---|---|---|
| `File.Top()` | Layout item 5 (after imports) | Decls |
| `File.Init()` | Layout item 7 (rendered as `func init() { … }` body) | Stmts |
| `File.Bottom()` | Layout item 8 (after decls) | Decls |
| `File.ImportsSlot()` | Drained into `writer.ImportSet` before any template fires | Imports |

`AppendOriginSlot` rejects nil origins, nil items, and empty slot
names synchronously. Non-empty custom slot names are accepted —
plugin-defined emit kinds may declare their own slots through
`emit.File.Slot`. Multiple plugins anchoring contributions to the
same origin compose into one file in capability-topological order
across plugins, FIFO of registration order within each plugin.

Per-host slots on structured kinds merge similarly: typed direct content
first, then slot contributions re-grouped by plugin topo:

| Host | Slots |
|---|---|
| `*emit.Struct` | `FieldsSlot()`, `MethodsSlot()`, `EmbedsSlot()` |
| `*emit.Interface` | `MethodsSlot()`, `EmbedsSlot()` |
| `*emit.Enum` | `VariantsSlot()` |
| `*emit.Function` | `Prebody()`, `Postbody()`, `ParamsSlot()` |
| `*emit.Method` | `Prebody()`, `Postbody()`, `ParamsSlot()` |
| `*emit.Field` | `Tags()` |

Slot entries that carry a `QName()` (Methods, Fields, Variants) enforce
`ErrDuplicateEntity` on cross-plugin name collisions. Unnamed entries
(Stmts in `Prebody`, Tags) render in append order without duplicate check.

## Render pipeline

`Backend.Render(ctx)` groups emit entities by `emit.Target` and produces one
file per non-empty Target. Each Target flows through:

| Step | Operation | Failure handling |
|---|---|---|
| 1 | Render body via merged templates → raw bytes | Template execution error → `Error` diagnostic on Target; loop continues |
| 2 | `go/format.Source(raw)` | Failure → `Warn` with `line:col` from the format error; falls back to raw bytes |
| 3 | `golang.org/x/tools/imports` library pass | Failure → `Warn`; falls back to step-2 bytes. Imports added beyond `ImportSet` → one `Warn` per uncovered reference |
| 4 | SHA-256 over body bytes (= provenance hash) | — |
| 5 | Compose header + body + footer-with-hash | — |
| 6 | `ctx.Sink.Write(target, bytes)` | Failure → wrapped error from `Render` |

Empty Targets (no decls, every slot empty after pre-render) are filtered
before render — they never reach the sink or the manifest.

## Canonical file layout

Every rendered file follows this structure:

```
1. Header                  (writer.Header.Render, trailing blank line)
2. // Package <Name> ...    (emit.Package.DocLines, omitted when empty)
3. package <Name>
4. imports block            (writer.ImportSet.Imports(); ungrouped at
                             template time, regrouped by goimports)
5. Top slot                 (File.Top(), capability-topo order)
6. Free-floating decls      (Structs, Interfaces, Functions, Methods,
                             Aliases, Enums, Variables, Constants —
                             capability-topo, then QName())
7. Init slot                (rendered as body of func init() { … };
                             whole block omitted when slot empty)
8. Bottom slot              (File.Bottom(), capability-topo order)
9. Footer                   (writer.Footer with provenance hash)
```

## Envelope extension points

Library embedders customise the envelope via `plugin.BackendContext` fields:

| Field | Type | Purpose |
|---|---|---|
| `Brand` | `string` | Replaces `"eidos"` in the header marker and footer label. Empty defaults to `"eidos"`. |
| `Command` | `string` | The `Command:` line in the header (typically `os.Args[1:]` joined). Empty → line omitted. |
| `HeaderPrefix` | `[]string` | Lines emitted verbatim before the standard header block (copyright lines, build-tag constraints). |
| `HeaderSuffix` | `[]string` | Lines emitted verbatim after the standard header block. |
| `FooterSuffix` | `[]string` | Lines emitted verbatim after the standard footer's two lines. |
| `SourcesOverride` | `[]string` | Replaces the auto-aggregated `Source:` list. Used for programmatic invocations where no source path applies. |

The header carries no timestamp — `Generated:` lines are intentionally
absent so two runs over the same emit graph produce byte-identical output,
header and footer included.

## Error sentinels

Backend-emitted errors are compared via `errors.Is`:

| Sentinel | Fires when |
|---|---|
| `ErrTemplateMissing` | `render` dispatched for an `emit.Node` with no registered template. |
| `ErrUnsupportedRef` | `renderType` called on an `emit.Ref` kind it can't render, or `internalTargetName` called on an unsupported `TypeRef` target. |
| `ErrUnsupportedExpr` | `renderExpr` called on an `ExprKind` or `LiteralKind` variant it can't render. |
| `ErrUnsupportedStmt` | `renderStmt` called on a `StmtKind` variant it can't render. |
| `ErrMixedNamedParams` | A parameter list mixes named and unnamed entries (forbidden by Go's grammar). |
| `ErrDuplicateEntity` | Same-`QName()` collision either between free-floating decls in the same Target or between slot contributions to a kinded slot. Aliased to `store.ErrDuplicateEntity` so both layers chain to the same sentinel. |
| `ErrNilHost` | A slot-composition funcmap helper received a nil host entity. |
| `ErrEmptyTarget` | A Target produced no renderable content after the pre-render pass — the loop continues to the next Target. |
| `ErrTemplateFuncCollision` | Funcmap Pass 1: a plugin extension collides with another plugin's extension or with a core canonical entry. |
| `ErrReservedFuncName` | Funcmap Pass 2: a plugin tried to override a reserved canonical entry. |
| `ErrTemplateNameCollision` | Template parse: two plugins both define a template under the same name. |
| `ErrReservedTemplatePrefix` | Template parse: a plugin defines a template under a reserved name prefix (currently `fragment.`). |
| `ErrSlotHostUnsupported` | The `slot` funcmap helper was invoked against a value that doesn't implement `emit.SlotHost` (typical cause: calling `slot` on a sub-element like a Field). |

## Determinism contract

Output is byte-identical across runs for identical input, header and footer
included:

- Headers carry no timestamp; the `Generated:` line is intentionally absent.
- Free-floating decls sort by capability-topo + `QName()`.
- Slot contributions sort by capability-topo + append-sequence.
- `imp` collision resolution is deterministic (`bar`, `bar2`, `bar3`, …).
- `format.Source` and goimports are deterministic per input; the
  success-vs-failure path is itself a property of the input, so hash
  equality across runs is preserved regardless of which path the format
  pass took.
- The provenance hash is over the body bytes actually written to the sink
  (layout items 2–8) — gofmt'd if step 2 succeeded, goimports-formatted if
  step 3 also succeeded, unformatted if step 2 failed.

## Concurrency

A `*Backend` instance is safe for concurrent use. The parent template tree
is parsed once at construction and never mutated. Per-Target rendering
clones the parent, binds per-Target funcmap closures, and accumulates per-
Target `ImportSet` state in isolation. Plugin templates merge into the
per-Render clone, not the parent, so concurrent `Render` calls remain
race-free.
