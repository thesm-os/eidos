# Conformance testing

The [`plugintest`](../../eidostest/plugintest) package ships the
framework conformance suite plugin authors run against their
plugin instances. The suite enforces the contracts the pipeline
relies on at registration / build time, so a failing suite
means the plugin would either crash the pipeline or produce
non-deterministic output in production.

This document is the reference for which suite applies to which
role and how to write fixtures.

## The six suites

### `RunSuite(t, plugin)` — universal framework contracts

Every plugin runs this. It pins thirteen contracts, in this
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
  defines a name claiming the reserved `fragment.` prefix
- Declaration accessors (`Provides`, `Requires`, `Directives`,
  `Outputs`) return a fresh slice on each call rather than the
  plugin's own backing array

Pass any plugin instance — the suite probes for each capability
via interface assertion and skips the checks for capabilities
the plugin doesn't implement. The all-or-nothing
`CapabilityProvider` check is the exception: it fires precisely
when `Priority()` is declared and the interface is not
satisfied.

Every per-language lookup runs against
`plugintest.ConformanceLanguage` (`"golang"`) and against a
language no backend claims, so the negative path is exercised
rather than assumed. A plugin that branches `Outputs` or
`Templates` on any other spelling answers no probe, and the
per-language checks validate an empty slice.

### `RunAnnotatorSuite(t, annotator, fixtures)`

For plugins satisfying `plugin.Annotator`. Pins:

- `Annotate` on an empty store doesn't panic
- For each fixture: `Annotate` doesn't panic, doesn't change
  the node count (the source-side store is frozen during the
  annotator phase), and is idempotent (running twice produces
  identical meta state).

**Fixture shape:**

```go
plugintest.AnnotatorFixture{
    Name: "package with three structs",
    BuildStore: func(t *testing.T) *store.Store {
        t.Helper()
        return storefixture.New().
            Struct("User", nil).
            Struct("Order", nil).
            Struct("Invoice", nil).
            Build()
    },
}
```

Each fixture's `BuildStore` is called once per subtest; return a
fresh store each call.

### `RunGeneratorSuite(t, generator, fixtures)`

For plugins satisfying `plugin.Generator`. Pins:

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
  - every `OutputTag` on an origin-anchored slot contribution
    corresponds to an `Output` the plugin declares (skipped for
    a plugin declaring none)
  - every `emit.OutputPackageSetter` among those contributions
    tolerates a partial routing map — an empty one, one holding
    only foreign tags, one holding the primary tag with no
    derivable path — without panicking

The determinism check compares a sorted projection of identity
tuples (kind, qualified name, target), so a generator emitting
the same set of decls in a different order still passes.
Per-entity content is out of scope: golden-file assertions
through `pipelinetest` / `backendtest` cover that.

**Fixture shape:** same as `AnnotatorFixture`. `BuildStore` is
called once per subtest — twice each for the determinism and
`NodesOnly` checks.

### `RunBackendSuite(t, backend, fixtures)`

For plugins satisfying `plugin.Backend`. Pins:

- `Render` on an empty emit graph doesn't panic
- For each fixture: `Render` doesn't panic, doesn't emit
  Error-severity diagnostics on valid input, and is byte-stable
  across two independent runs of the same fixture.

**Fixture shape:**

```go
plugintest.BackendFixture{
    Name: "single struct in one package",
    BuildEmitPackages: func(t *testing.T) []*emit.Package {
        t.Helper()
        return []*emit.Package{{
            Name: "demo",
            Path: "example.com/demo",
            Structs: []*emit.Struct{{
                Name: "User",
                Target: emit.Target{
                    Dir: "demo", Filename: "user_gen.go", Package: "demo",
                },
            }},
        }}
    },
    Command: "test-fixture",
}
```

Backend fixtures supply pre-built `emit.Target` values on every
decl — the suite skips the routing layer. The `Command` field
stamps a stable string into the rendered file's `Command:`
header line; pin it explicitly for reproducibility.

### `RunFrontendSuite(t, frontend, fixtures)`

For plugins satisfying `plugin.Frontend`. Pins:

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
  `opt.ErrMissingRequired`)
- `SetOptions(Valid)` succeeds
- `SetOptions(Valid + UnknownKey)` returns an error wrapping
  `opt.ErrUnknownField`

**Fixture shape:**

```go
plugintest.OptionsFixture{
    Valid: map[string]string{
        "output_package": "main",
        "mode":           "fast",
    },
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
- **`NewMinimalPlugin(name)`** — implements `plugin.Plugin`
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
