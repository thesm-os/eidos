# Plugin authoring guide

eidos plugins are the unit of extension. Every code-generation
behaviour — Go-source parsing, repository emission, mock generation,
debug-trace weaving — runs as a plugin against the framework's shared
store and pipeline.

**Using eidos rather than extending it?** Start at
[docs/README.md](../README.md); this guide assumes you are writing a
plugin.

## Read in this order

1. **[quickstart.md](quickstart.md)** — your first plugin, end to end.
   An annotator: the role interfaces, `Annotate`, a priority
   declaration, and the conformance test.

2. **[recipes.md](recipes.md)** — pattern catalogue. For each common
   shape (one struct → one emit, cross-cutting slot contribution,
   plugin-defined emit kind) it points at a working reference plugin
   and summarises its structure.

3. **[conformance.md](conformance.md)** — testing against the
   framework suites. What `plugintest` provides, which suite applies
   to which role, how to write fixtures.

4. **[templates.md](templates.md)** — shipping templates from a
   generator. Plugin-defined emit kinds, the template naming contract,
   the funcmap and its extension rules, walked through with
   `registrygen`.

5. **[composition.md](composition.md)** — multi-generator composition.
   An ensemble producing one HTTP handler from one annotated struct,
   contributing into the host's slots without coordinating with each
   other.

6. **[routing.md](routing.md)** — where rendered output goes. The
   user-facing forms, the `_test.go` shift, the precedence ladder, and
   automatic cross-package reference resolution.

7. **[multi-file-output.md](multi-file-output.md)** — emitting more
   than one file per source. The `Outputs(lang)` contract, per-value
   output tagging, per-output routing overrides.

## The one rule that costs the most

**Read through `ctx.Reader`; never through `ctx.Store`.**

Reads captured by the Reader compose the plugin's cache key. A read
that goes around it is a read the cache cannot invalidate on: the
source changes, the fingerprint does not, and the next run serves
stale output indistinguishable from current. Nothing reports it.

`ctx.Store` exists for what the Reader does not cover — chiefly the
emit side, where a generator queues its contributions.

Its counterpart on the write side: stamp metadata through
`EnsureMeta()`, never `Meta()`. `Meta()` is the read accessor and
returns `nil` for a node nothing has written to yet, which is every
node on a cold run.

## The façade

A plugin imports [`sdk`](../../sdk), not the framework's layering:

```go
import (
    "go.thesmos.sh/eidos/sdk"         // contracts + the source and emit models
    sdkgo "go.thesmos.sh/eidos/sdk/golang" // the Go plugin base
    "go.thesmos.sh/eidos/lang/golang" // Go conventions and refs
)
```

An annotator needs only the first. A Go generator takes all three.

`sdk` carries everything a plugin *names*: the role and capability
contracts, the source model it reads (`sdk.Struct`, `sdk.Method`, …),
the emit model it writes (`sdk.EmitStruct`, the builders, the leaf
constructors), and the metadata, diagnostic, position and store
vocabulary joining them.

Two spelling rules are worth knowing before you start:

- **The source model takes the bare name; the emit model takes the
  `Emit` prefix.** `sdk.Struct` is a source struct; `sdk.EmitStruct`
  is one you are emitting. Likewise `sdk.NodeKindStruct` against
  `sdk.EmitKindStruct`.
- **Every declaration kind exists on both sides carrying a different
  value.** Confusing them fails silently in both directions: a slot
  constrained on a source kind accepts nothing an emit builder
  produces, and a directive scoped to an emit kind matches no source
  node, so the plugin never fires and nothing reports it.

`sdk/golang` supplies `sdkgo.Base`, the embedded base that answers the
Go declaration methods — outputs, templates, funcmaps — so a generator
declares its suffixes and template tree once and inherits the rest.
[`plugins/generator/enum`](../../plugins/generator/enum) is the
canonical shape.

For what the façade deliberately excludes and why, read
[its own package doc](../../sdk/doc.go). The short version: anything a
plugin has no business touching — the store, reader and diagnostic
constructors the pipeline hands you, pipeline internals, and
renderer-side discriminators a generator never reads back.

## Reference plugins

Every plugin in `reference/` is complete, tested and production-grade.

| Plugin | Role | Pattern |
|---|---|---|
| [shapewriter](../../reference/shapewriter) | Annotator | Infer structural shape; stamp meta |
| [repogen](../../reference/repogen) | Generator | Per-source-struct emit (CRUD repo) |
| [mockgen](../../reference/mockgen) | Generator | Per-interface emit — source-side or upstream-emitted |
| [stubgen](../../reference/stubgen) | Generator | Two outputs: recording double + tagged companion test |
| [auditweaver](../../reference/auditweaver), [debugweaver](../../reference/debugweaver) | Cross-cutting | Method `prebody` contribution through the plugin's own template |
| [registrygen](../../reference/registrygen) | Cross-cutting | Plugin-defined emit kind + `init()` registration |
| [handlergen](../../reference/handlergen) | Generator | Slot host: own emit kind carrying `prebody` / `postbody` |
| [middlewaregen](../../reference/middlewaregen) | Generator | Slot host: own emit kind carrying a `chain` slot |
| [validategen](../../reference/validategen) | Generator | Owns a file *and* contributes into handlergen's `prebody` |
| [authgen](../../reference/authgen), [metricgen](../../reference/metricgen), [tracegen](../../reference/tracegen) | Cross-cutting | Entries into middlewaregen's `chain`, ordered by `Requires` |
| [errorgen](../../reference/errorgen), [auditgen](../../reference/auditgen) | Cross-cutting | Entries into handlergen's `postbody`, ordered by priority bucket |

The last five rows are **one ensemble**, not five independent
examples; [composition.md](composition.md) walks them end to end.

Read the reference plugin matching your intended pattern before
writing your own — every framework idiom appears in at least one of
them.

For a template-driven generator that ships per-language templates
rather than constructing emit values programmatically, copy from
[`plugins/generator/enum`](../../plugins/generator/enum) or
[`plugins/generator/builder`](../../plugins/generator/builder).
