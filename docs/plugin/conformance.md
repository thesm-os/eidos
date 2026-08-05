# Conformance testing

The [`plugintest`](../../eidostest/plugintest) package ships the
framework conformance suite plugin authors run against their
plugin instances. The suite enforces the contracts the pipeline
relies on at registration / build time, so a failing suite
means the plugin would either crash the pipeline or produce
non-deterministic output in production.

This document is the reference for which suite applies to which
role and how to write fixtures.

## The five suites

### `RunSuite(t, plugin)` — universal framework contracts

Every plugin runs this. It pins:

- `Name()` returns a non-empty stable identifier
- The plugin satisfies at least one role interface (Frontend /
  Annotator / Generator / Backend)
- `CapabilityProvider.Provides()` / `.Requires()` (when
  implemented) return deterministic, non-empty entries
- `DirectiveProvider.Directives()` (when implemented) declares
  unique non-empty schema names
- `Versioned.Version()` (when implemented) is stable across
  calls
- `EmitVersioned.EmitVersions()` (when implemented) is stable
  and contains no empty entries
- `NodesOnly()` (when implemented) is stable across calls
- `FilenameProvider.Outputs(lang)` (when implemented) is
  stable across calls for each language

Pass any plugin instance — the suite probes for each capability
via interface assertion and skips checks for capabilities the
plugin doesn't implement.

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
- For each fixture: `Generate` doesn't panic, doesn't mutate
  source-side node counts (generators write to `Store.Emit`,
  not `Store.Nodes`), and is deterministic — driving Generate
  against two freshly-built stores produced from the same
  fixture yields identical emit projections.

**Fixture shape:** same as `AnnotatorFixture`. The fixture's
`BuildStore` is called twice for the determinism check.

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

The `plugintest` package also exports three reference plugin
fixtures plugin authors can use directly:

- **`plugintest.FixturePlugin`** — implements every role and
  capability the framework recognises. Useful as a meta-test
  baseline: passing `FixturePlugin` to `RunSuite` should always
  succeed.
- **`plugintest.MinimalPlugin`** — implements `plugin.Plugin`
  only, no role. `RunSuite` against it fails the role probe —
  useful for verifying conformance-test behaviour.
- **`plugintest.OptionsFixturePlugin`** — a generator with a
  small options schema covering required + default + free-text
  - one-of fields. The reference example for the options suite.

## Common conformance failures

**Idempotency fails on the annotator suite.** Your `Annotate`
stamps a value derived from a counter, timestamp, or other
non-input source. Stamp values derived only from the input
node's content; the bag's authority-slot model overwrites in
place when the same value is re-stamped, so deterministic
stamping passes idempotency for free.

**Determinism fails on the generator suite.** Your `Generate`
iterates a map (Go's map iteration order is non-deterministic).
Sort the keys or use the store's order-preserving buckets
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
