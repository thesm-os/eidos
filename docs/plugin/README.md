# Plugin authoring guide

eidos plugins are the unit of extension. Every code-generation
behaviour — Go-source parsing, repository emission, mock
generation, debug-trace weaving — runs as a plugin against the
framework's shared store and pipeline.

This guide collects the documents a plugin author reads in
order:

1. **[quickstart.md](quickstart.md)** — Write your first plugin
   from scratch in ten minutes. Targets an annotator (the
   simplest role): the role interfaces, the `Annotate` hook, an
   optional priority declaration, and the conformance test.

2. **[recipes.md](recipes.md)** — Pattern catalog. For each
   common plugin shape (one struct → one emit, cross-cutting
   slot contribution, plugin-defined emit kind, …) points at a
   working reference plugin and summarises its structure.

3. **[conformance.md](conformance.md)** — Testing your plugin
   against the framework conformance suite. What `plugintest`
   provides, which suite applies to which role, how to write
   fixtures.

4. **[templates.md](templates.md)** — Shipping templates from a
   generator plugin via `sdk.TemplateProvider`. Plugin-defined
   emit kinds, the template naming contract, the funcmap, and
   funcmap extensions / overrides — walked through end-to-end
   with `registrygen` as the canonical example.

5. **[composition.md](composition.md)** — Multi-generator
   composition. An ensemble of generators produces a production
   HTTP handler from one annotated source struct, contributing
   into the host's `prebody` / `postbody` slots without
   coordinating with each other; then the same example with the
   host declaring an emit kind and a named slot of its own, so
   contributors depend on the host explicitly.

6. **[routing.md](routing.md)** — Where rendered output goes.
   The three user-facing forms (`+gen:out`, per-directive
   `out=`/`pkg=` keys, defaults), the `_test.go` shift, the
   precedence pipeline, and the cross-package reference
   resolution the backend handles automatically.

7. **[multi-file-output.md](multi-file-output.md)** — How a
   single plugin emits more than one rendered file per source.
   The `Outputs(lang) []Output` contract, per-decl
   `OutputTag` tagging via `pkg.File(tag)`, per-output routing
   overrides (`+gen:out tag=...`, `-o <plugin>:<tag>=...`),
   project-config schema, and the migration from
   `FilenameSuffix`.

## Reference plugins

Every reference plugin in `reference/` is a complete, tested,
production-grade example:

| Plugin                         | Role             | Pattern                            |
|--------------------------------|------------------|------------------------------------|
| [shapewriter](../../reference/shapewriter)   | Annotator        | Infer structural shape; stamp meta |
| [repogen](../../reference/repogen)           | Generator        | Per-source-struct emit (CRUD repo) |
| [mockgen](../../reference/mockgen)           | Generator        | Per-interface emit (mock) — source-side or upstream-emitted |
| [stubgen](../../reference/stubgen)           | Generator        | Two-output emit: recording double + tagged companion test |
| [auditweaver](../../reference/auditweaver), [debugweaver](../../reference/debugweaver) | Cross-cutting | Method `prebody` contribution rendered through the plugin's own template, via `emit.NewRenderStmt` |
| [registrygen](../../reference/registrygen)   | Cross-cutting    | Plugin-defined emit kind + init() registration |
| [handlergen](../../reference/handlergen)     | Generator        | Slot host: own emit kind carrying `prebody` / `postbody` |
| [middlewaregen](../../reference/middlewaregen) | Generator      | Slot host: own emit kind carrying a `chain` slot |
| [validategen](../../reference/validategen)   | Generator        | Owns its own file and contributes into handlergen's `prebody` |
| [authgen](../../reference/authgen), [metricgen](../../reference/metricgen), [tracegen](../../reference/tracegen) | Cross-cutting | Entries into middlewaregen's `chain`, ordered by `Requires` |
| [errorgen](../../reference/errorgen), [auditgen](../../reference/auditgen) | Cross-cutting | Entries into handlergen's `postbody`, ordered by priority bucket |

The production [`plugins/generator/builder`](../../plugins/generator/builder)
is the canonical template-driven generator — copy from it when
your plugin ships per-language templates rather than constructing
emit decls programmatically.

Read the reference plugin matching your intended pattern before
writing your own — every framework idiom appears in at least one
of them. The last five rows are one ensemble, not five
independent examples; [composition.md](composition.md) walks
them end to end.

## The SDK façade

The [`sdk` package](../../sdk) re-exports the plugin contract
surface (roles, capabilities, hooks, priority buckets, directive
schema builders, the structural `Kind` discriminator) under one
import. A typical plugin's framework imports shrink from eight
to five:

```go
import (
    "go.thesmos.sh/eidos/sdk"          // role + capability contracts
    "go.thesmos.sh/eidos/core/opt"     // typed options (when applicable)
    "go.thesmos.sh/eidos/node"         // source-side store (read)
    "go.thesmos.sh/eidos/emit"         // emit-side store (generators / backends)
    "go.thesmos.sh/eidos/emit/builder" // emit construction helpers
)
```

The high-volume packages (`node`, `emit`, `emit/builder`,
`core/opt`) stay as separate imports — flattening them into
`sdk` would clash on common names like `Schema`, `Field`,
`Walk`, and `New`.

What `sdk` does alias from them is the handful nearly every
plugin touches: `BaseEmit`, `EmitNode`, `EmitTarget`, `Ref`,
`Expr` and `NewExternal` from `emit`; `Provenance` and
`NewProvenance` from `emit/builder`; `Holder` and
`BindOptions` from `core/opt`. A pure contributor — one that
appends into another plugin's slots and owns no output file —
needs no framework import beyond `sdk` and `emit`.
