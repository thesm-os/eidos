# Conformance testing

The [`plugintest`](../../eidostest/plugintest) package ships the
framework conformance suite plugin authors run against their
plugin instances. The suite enforces the contracts the pipeline
relies on at registration / build time, so a failing suite
means the plugin would either crash the pipeline or produce
non-deterministic output in production.

This document is the reference for which suite applies to which
role and how to write fixtures.

## Where this sits

`plugintest` is the first of five harness rungs and the narrowest.
It drives one plugin in isolation and **renders nothing** — the emit
graph is the last artifact any check here inspects. A template that
parses but cannot execute clears every check in this document.

| Rung | Proves | Does not prove |
|---|---|---|
| `plugintest` | declarations, determinism, diagnostic discipline | that anything renders |
| `backendtest` | one backend over a hand-built emit graph | that a generator's templates merge |
| `pipelinetest` | several plugins + real backend → rendered files | that a real frontend parses your source |
| `frontendtest` | a real frontend with the plugin chain behind it | process-level behaviour |
| `acceptancetest` | the binary; the only gate that runs `go build` on generated output | (in-tree only) |

## Running the suite

Run with `-count=2` at minimum. Two determinism checks compare passes
inside one process, and Go randomises map-iteration order per range
statement, so a "first key wins" defect over a two-entry map is a coin
flip per invocation. This repository's own gate runs `count: 3`.

Run with `-race` if your plugin holds state — the pipeline dispatches
frontends concurrently and nothing here exercises that.

## The six suites

### `RunSuite(t, plugin)` — universal framework contracts

Every plugin runs this. It pins fourteen contracts, in this
order:

- `Name()` returns a non-empty identifier, stable across calls
- The plugin satisfies at least one role interface (Frontend /
  Annotator / Generator / Backend)
- `CapabilityProvider` is implemented in full or not at all — a
  plugin declaring `Priority()` without `Provides()` and
  `Requires()` does not satisfy the interface, so the pipeline
  drops it into the default bucket and discards the ordering it
  declared
- `CapabilityProvider` (when implemented) returns a stable
  `Priority()` and deterministic `Provides()` / `Requires()`
  holding no empty labels
- `DirectiveProvider.Directives()` (when implemented) declares
  unique non-empty schema names
- `Versioned.Version()` (when implemented) is stable across
  calls — the empty string is legal and opts out of the cache
  key
- `EmitVersioned.EmitVersions()` (when implemented) is stable
  and contains no empty entries
- `NodesOnly()` (when implemented) is stable across calls
- `FilenameProvider.Outputs(lang)` (when implemented) is
  stable across calls for each language
- `FilenameProvider.Outputs(lang)` returns a well-formed
  slice: every `Suffix` non-empty, tags unique, at most one
  empty-tag output, and that one at index 0
- `TemplateProvider` (when implemented) returns stable
  `Templates` / `TemplateFuncs` / `TemplateOverrides`, and no
  name appears in both funcmaps
- `TemplateProvider`'s shipped `*.tmpl` files parse, and none
  defines a name claiming the reserved `fragment.` prefix. The
  filesystem is walked recursively, so templates below the root —
  the shape `//go:embed templates/golang/*.tmpl` produces without a
  matching `fs.Sub` — are validated rather than silently skipped
- `TemplateProvider`'s `TemplateFuncs` and `TemplateOverrides`
  claim none of the backend's 25 reserved funcmap names. The backend
  rejects a collision while merging, before it renders anything, so
  the whole run writes zero files for every plugin in the
  composition — not only the one at fault
- Declaration accessors (`Provides`, `Requires`, `Directives`,
  `Outputs`) return a fresh slice on each call rather than the
  plugin's own backing array

Pass any plugin instance — the suite probes for each capability
via interface assertion and **reports a skip** for capabilities the
plugin doesn't implement, so "validated" and "examined nothing" are
distinguishable in the output. It also logs each role it detects and
names the per-role suite that covers it, which `RunSuite` alone does
not run. The all-or-nothing
`CapabilityProvider` check is the exception: it fires precisely
when `Priority()` is declared and the interface is not
satisfied.

Every per-language lookup runs against
`plugintest.ConformanceLanguage` (`"golang"`) and against a
language no backend claims. Plugins targeting another backend call
`RunSuiteFor(t, p, "rust")` — under `RunSuite` those five checks
validate an empty slice and report green having examined nothing, so the negative path is exercised
rather than assumed. A plugin that branches `Outputs` or
`Templates` on any other spelling answers no probe, and the
per-language checks validate an empty slice.

### `RunAnnotatorSuite(t, annotator, fixtures)`

For plugins satisfying `sdk.Annotator`. Pins:

- `Annotate` on an empty store doesn't panic
- For each fixture: `Annotate` doesn't panic, doesn't change
  the node count (the source-side store is frozen during the
  annotator phase), is idempotent (running twice on one store
  produces identical meta state), and is **deterministic** (two
  independently built stores produce identical meta state).

The last two catch different defects. A stamp derived from a counter
or a map iteration passes idempotency — the usual already-stamped
guard makes the second pass a no-op — and fails determinism.

**Fixture shape:**

```go
plugintest.AnnotatorFixture{
    Name: "package with three structs",
    BuildStore: func(t *testing.T) *sdk.Store {
        t.Helper()
        return gofixture.New().
            Struct("User", nil).
            Struct("Order", nil).
            Struct("Invoice", nil).
            Build()
    },
}
```

Each fixture's `BuildStore` is called once per subtest; return a
fresh store each call.

`gofixture` is `go.thesmos.sh/eidos/lang/golang/golangtest/gofixture`,
which builds Go declarations. The suites take a store and never ask
what produced it, so a plugin targeting another language substitutes
that language's fixture and every check above still applies.

### `RunGeneratorSuite(t, generator, fixtures)`

For plugins satisfying `sdk.Generator`. Pins:

- `Generate` on an empty store doesn't panic
- For each fixture, six checks:
  - `Generate` doesn't panic
  - `Generate` doesn't mutate source-side node counts
    (generators write to `Store.Emit`, not `Store.Nodes`)
  - `Generate` is deterministic — driving it against two
    freshly-built stores produced from the same fixture yields
    identical emit projections
  - a `NodesOnly() == true` declaration is truthful: the
    generator neither reads the emit graph through `ctx.Reader`
    nor produces different output when the emit graph is
    pre-seeded
  - every emitted value carries the generator's own `Name()` in
    `SetBy` — Layout looks a plugin's declared Outputs up under it,
    slot rendering orders by it, and the file header attributes by
    it, so a foreign value drops the declaration at routing
  - every `OutputTag` **anywhere in the emit graph** corresponds to
    an `Output` the plugin declares (skipped for a plugin declaring
    none). The empty primary tag is no longer exempt: Layout reports
    `ErrNoDefaultOutput` when a decl carries no tag and the plugin
    declares no empty-tag `Output`
  - every `sdk.OutputPackageSetter` among those contributions
    tolerates a partial routing map — an empty one, one holding
    only foreign tags, one holding the primary tag with no
    derivable path — without panicking

The determinism check compares an **ordered** projection produced by
walking the emit graph the way Layout does, followed by the queued
origin-slot contributions in registration order. Emission order and
slot contributions are both inside the oracle — a generator emitting
exclusively through `AppendOriginSlot` used to have its determinism
checked against two empty projections. A sorted identity-only diff is
still printed alongside a failure, because it is the readable one.

**Fixture shape:** same as `AnnotatorFixture`. `BuildStore` is
called once per subtest — twice each for the determinism and
`NodesOnly` checks.

### `RunBackendSuite(t, backend, fixtures)`

For plugins satisfying `sdk.Backend`. Pins:

- `Render` on an empty emit graph doesn't panic
- For each fixture: `Render` doesn't panic, doesn't emit
  Error-severity diagnostics on valid input, and is byte-stable
  across two independent runs of the same fixture.

**Fixture shape:**

```go
plugintest.BackendFixture{
    Name: "single struct in one package",
    BuildEmitPackages: func(t *testing.T) []*sdk.EmitPackage {
        t.Helper()
        return []*sdk.EmitPackage{{
            Name: "demo",
            Path: "example.com/demo",
            Structs: []*sdk.EmitStruct{{
                Name: "User",
                Target: sdk.EmitTarget{
                    Dir: "demo", Filename: "user_gen.go", Package: "demo",
                },
            }},
        }}
    },
    Command: "test-fixture",
}
```

Backend fixtures supply pre-built `sdk.EmitTarget` values on every
decl — the suite skips the routing layer. The `Command` field
stamps a stable string into the rendered file's `Command:`
header line; pin it explicitly for reproducibility.

### `RunFrontendSuite(t, frontend, fixtures)`

For plugins satisfying `sdk.Frontend`. Pins:

- `Load` on an empty pattern doesn't panic
- For each fixture: `Load` doesn't panic, and is deterministic
  — two invocations with the same `Pattern` and `Options`
  produce equivalent node-graph projections.

**Fixture shape:**

```go
plugintest.FrontendFixture{
    Name:    "basic-struct fixture",
    Pattern: "./...",
    Options: map[string]string{"dir": "/path/to/source"},
}
```

The suite calls `SetOptions` before each Load with the fixture's
Options when the frontend implements `OptionsProvider`. Pin
`dir` (or whatever input-root option the frontend declares) to
a stable testdata path.

### `RunOptionsSuite(t, plugin, fixture)`

For plugins satisfying `plugin.OptionsProvider`. Pins:

- `OptionsSchema()` returns a stable field set across calls
- The fixture's `Valid` map covers every required field (a
  fixture-shape check — if Valid misses a required field,
  rejection-path probes downstream get masked by
  `sdk.ErrMissingRequired`)
- `SetOptions(Valid)` succeeds
- `SetOptions(Valid + UnknownKey)` returns an error wrapping
  `sdk.ErrUnknownField`. `UnknownKey` is now optional — the suite
  synthesises a name the schema does not declare when the fixture
  omits one, so the probe is unconditional
- `SetOptions` with every required field omitted returns an error
  wrapping `sdk.ErrMissingRequired`

The suite asserts these itself; the sentinels are named here because a
plugin writing its own option tests wants the same ones. `sdk`
re-exports the whole set — `ErrMissingRequired`, `ErrUnknownField`,
`ErrInvalidValue`, `ErrInvalidDecodeTarget`, and the two schema-time
ones below — so a plugin never imports `core/opt` to branch on a
failure.

`sdk.BindOptions` panics on a malformed `eidos:` tag, which is right in
production and wrong in a test: the panic fails the suite before any
subtest names the field at fault. `sdk.ReflectOptions` returns that
same failure as an error wrapping `sdk.ErrInvalidTag` or
`sdk.ErrUnsupportedFieldType`, so a plugin can pin its own options
struct the way it pins the rest of its contract:

```go
func TestOptionsStructIsWellFormed(t *testing.T) {
    if _, err := sdk.ReflectOptions(Options{}); err != nil {
        t.Fatalf("options struct does not reflect: %v", err)
    }
}
```

**Fixture shape:**

```go
plugintest.OptionsFixture{
    Valid: map[string]string{
        "output_package": "main",
        "mode":           "fast",
    },
    // Optional: the suite synthesises one when omitted.
  UnknownKey: "no_such_field",
}
```

## Wiring it all up

The canonical `TestConformance` for a generator plugin with
options looks like this:

```go
func TestConformance(t *testing.T) {
    t.Parallel()

    t.Run("framework", func(t *testing.T) {
        t.Parallel()
        plugintest.RunSuite(t, myplugin.New())
    })

    t.Run("generator", func(t *testing.T) {
        t.Parallel()
        plugintest.RunGeneratorSuite(t, myplugin.New(), []plugintest.GeneratorFixture{
            // ...
        })
    })

    t.Run("options", func(t *testing.T) {
        t.Parallel()
        plugintest.RunOptionsSuite(t, myplugin.New(), plugintest.OptionsFixture{
            // ...
        })
    })
}
```

Each suite owns its own plugin instance — the role suites mutate
plugin state (calling SetOptions before each Load, for example),
so sharing one instance across suites would let test order
affect results.

## Reference fixture plugins

The `plugintest` package also exports reference plugin fixtures
plugin authors can use directly:

- **`NewFixturePlugin()`** — a generator implementing every
  optional capability the framework suite probes:
  `CapabilityProvider`, `DirectiveProvider`, `Versioned`,
  `EmitVersioned`, `FilenameProvider`, `NodesOnly`. Useful as a
  meta-test baseline: passing it to `RunSuite` always succeeds.
- **`NewMinimalPlugin(name)`** — implements `sdk.Plugin`
  only, no role. `RunSuite` against it fails the role probe and
  nothing else.
- **`NewOptionsFixturePlugin(name)`** — a generator whose
  schema covers the three field shapes: required
  (`output_package`), one-of with a default (`mode`), and free
  text (`label`). The reference example for the options suite.
- **`BrokenPlugin(v)`** — a plugin breaking exactly the one
  contract the `Violation` `v` names and satisfying every
  other; `Violations()` returns the full set. Running
  `RunSuite` against one and watching it fail is the cheapest
  confirmation that a harness is wired up at all.
- **`LyingNodesOnlyGenerator()`** — declares `NodesOnly` and
  reads the emit graph anyway. Exported separately from
  `BrokenPlugin` because catching it needs `RunGeneratorSuite`,
  which drives Generate against a store.

## Common conformance failures

**Idempotency fails on the annotator suite.** Your `Annotate`
stamps a value derived from a counter, timestamp, or other
non-input source. Stamp values derived only from the input
node's content; the bag's authority-slot model overwrites in
place when the same value is re-stamped, so deterministic
stamping passes idempotency for free.

**Determinism fails on the generator suite.** Your `Generate`
iterates a map (Go's map iteration order is non-deterministic)
and lets that order reach an identity — a synthesised index in
a name, a first-one-wins pick, a filename derived from
position. Order on its own is invisible to the check, because
the projection is sorted; order that decides what a decl is
called or where it lands is not. Sort the keys or use the
store's order-preserving buckets
(`ctx.Store.Nodes().Structs().Items()` etc.) instead of
synthesising your own indices.

**Byte-stability fails on the backend suite.** Your rendered
output includes a non-deterministic value — `time.Now()`,
`os.Getenv("USER")`, an unsorted import set. Audit every part of
the rendered output for non-deterministic inputs; the
`SourcesOverride` and `Command` fixture fields pin the two
header lines that most commonly trip this.

**`RunOptionsSuite` fails on missing required.** Your fixture's
`Valid` map doesn't include every required field declared in
the schema. The suite reports the offending field name — add it
to `Valid`.
