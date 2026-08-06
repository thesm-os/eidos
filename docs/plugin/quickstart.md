# Quickstart — your first plugin

This walkthrough builds an **annotator** from scratch. We'll
write a plugin that detects every source struct whose name ends
in `Repo` and stamps a `myco.repo` boolean on its metadata bag —
the kind of foundational shape inference downstream generators
read against.

By the end you'll have:

- A working plugin satisfying `plugin.Annotator`
- A meta key declaring its typed shape
- A `_test.go` invoking the framework conformance suite

Total reading: ~10 minutes. Total writing: ~50 lines.

## The plugin contract in one paragraph

Every eidos plugin satisfies `plugin.Plugin` (one method:
`Name() string`) plus one or more **role interfaces**:

- **Frontend** — parses input into the source-side store
- **Annotator** — stamps metadata on existing nodes (no add /
  remove)
- **Generator** — produces emit entities (CRUD repos, builders,
  mocks, …)
- **Backend** — renders emit entities to a target language

A plugin may also opt into **capabilities**: `CapabilityProvider`
(priority + provides/requires), `OptionsProvider` (typed
configuration), `DirectiveProvider` (`+gen:` schema declarations),
`Versioned` (cache invalidation), and so on. Capabilities are
plain interfaces — the pipeline detects each via type assertion.

## Step 1: the package and constructor

Create `myco/reporepo/reporepo.go`:

```go
// Package reporepo stamps myco.repo=true on every source struct
// whose name ends in "Repo". Other plugins read the key via
// reporepo.MetaRepo.Get(node.Meta()).
package reporepo

import (
    "strings"

    "go.thesmos.sh/eidos/core/meta"
    "go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier — used in diagnostics
// and cache-key composition.
const Name = "reporepo"

// MetaRepo is the typed meta key the plugin stamps. Plugin
// authors read via MetaRepo.Get(s.Meta()).
var MetaRepo = meta.NewKey[bool](
    "myco.repo",
    func(raw string) (bool, error) {
        return raw == "true", nil
    },
)

// Plugin satisfies sdk.Annotator. The zero value is usable.
type Plugin struct{}

// New returns a fresh plugin.
func New() *Plugin { return &Plugin{} }

// Name returns the stable identifier.
func (*Plugin) Name() string { return Name }
```

That's the skeleton. We've declared the plugin name, a typed
meta key, and the constructor.

## Step 2: implement Annotate

Add the role method:

```go
// Annotate walks every source struct and stamps MetaRepo=true on
// the ones whose name ends in "Repo".
func (*Plugin) Annotate(ctx *sdk.AnnotatorContext) error {
    for _, s := range ctx.Store.Nodes().Structs().Items() {
        if !strings.HasSuffix(s.Name, "Repo") {
            continue
        }
        MetaRepo.Set(s.Meta(), true, Name)
    }
    return nil
}
```

That's it. The plugin reads source structs from the store, picks
the `*Repo` ones, and stamps their meta. The pipeline already
indexes structs in stable insertion order, so iteration is
deterministic.

## Step 3: declare a priority (optional)

By default the plugin runs in `priority.Default`
(`GeneratorCrossCutting`). Shape annotators conventionally run
earlier, so the framework provides `priority.AnnotatorShape`.
Declaring it requires implementing `CapabilityProvider`:

```go
import "go.thesmos.sh/eidos/priority"

// Priority places the plugin in the shape-detection bucket so
// downstream annotators and generators see populated MetaRepo
// values.
func (*Plugin) Priority() priority.Priority {
    return priority.AnnotatorShape
}

// Provides declares the capability name this plugin produces.
// Generators that depend on the shape declare it as a Requires
// entry to enforce ordering.
func (*Plugin) Provides() []string {
    return []string{"myco.shape.repo"}
}

// Requires returns nil — no upstream dependencies.
func (*Plugin) Requires() []string { return nil }
```

## Step 4: write the conformance test

Create `myco/reporepo/reporepo_test.go`:

```go
package reporepo_test

import (
    "testing"

    "go.thesmos.sh/eidos/eidostest/plugintest"
    "go.thesmos.sh/eidos/eidostest/storefixture"
    "go.thesmos.sh/eidos/store"
    "myco/reporepo"
)

// TestConformance pins every framework contract: stable Name,
// role-interface compliance, deterministic capabilities, plus
// the per-role annotator contracts (no panic on empty store,
// idempotent meta stamping, frozen node count).
func TestConformance(t *testing.T) {
    t.Parallel()

    t.Run("framework", func(t *testing.T) {
        t.Parallel()
        plugintest.RunSuite(t, reporepo.New())
    })

    t.Run("annotator", func(t *testing.T) {
        t.Parallel()
        plugintest.RunAnnotatorSuite(t, reporepo.New(), []plugintest.AnnotatorFixture{
            {
                Name: "package with a Repo-suffixed struct",
                BuildStore: func(t *testing.T) *store.Store {
                    t.Helper()
                    return storefixture.New().
                        Struct("UserRepo", nil).
                        Struct("OrderRepo", nil).
                        Struct("Plain", nil).
                        Build()
                },
            },
        })
    })
}
```

Run it:

```sh
go test ./...
```

The two suites check: `Name()` returns a stable non-empty
string; the plugin satisfies at least one role interface;
`CapabilityProvider` is implemented in full, returns the same
Provides / Requires on every call, and hands back a fresh slice
each time rather than the plugin's own; `Annotate` on an empty
store doesn't panic; node count is unchanged across Annotate;
running Annotate twice produces identical meta state
(idempotency).

A green test means the plugin satisfies every framework
invariant the pipeline relies on at registration / build time.

## What's next

- **[recipes.md](recipes.md)** — pattern catalog for the other
  three roles (Generator, Backend, Frontend) and the
  cross-cutting weaver pattern.
- **[conformance.md](conformance.md)** — full reference for the
  `plugintest` suite, fixture authoring, and per-role contracts.
- **`reference/shapewriter`** — production-grade equivalent of
  the plugin above; reads as the natural next step from this
  quickstart.

## Common next-step questions

**My plugin needs typed options.** Embed `*sdk.Holder[Options]`
on your plugin and assign `sdk.BindOptions(&p.opts)` to it in
`New`. The pipeline calls `SetOptions` at build time; defaults
are pre-applied at bind time, so even un-pipelined uses (tests,
direct invocation) see populated values. See `reference/repogen`
for a worked example.

**My plugin needs to declare a `+gen:` directive.** Implement
`DirectiveProvider`:

```go
import "go.thesmos.sh/eidos/node"

func (*Plugin) Directives() []sdk.DirectiveSchema {
    return []sdk.DirectiveSchema{
        sdk.NewDirective("myco-repo").
            On(node.KindStruct).
            Describe("Opts the struct into MyCo repository emission.").
            Build(),
    }
}
```

A directive name is not a meta-key name: the parser accepts a
letter followed by letters, digits, `_` and `-`, so `myco.repo`
names a key but never a directive. The framework auto-collects
every plugin's schemas at build time and rejects duplicates.

**My plugin emits code.** That's a Generator, not an Annotator.
See `reference/repogen` for the canonical shape and the
`emit/builder` package for the construction helpers.
