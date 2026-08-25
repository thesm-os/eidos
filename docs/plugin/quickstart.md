# Quickstart — your first plugin

This walkthrough builds an **annotator**: a plugin that finds every
source struct whose name ends in `Repo` and stamps a `myco.repo`
boolean on its metadata. That is the foundational shape inference
downstream generators read against.

By the end you will have a plugin satisfying `sdk.Annotator`, a typed
metadata key, and a `_test.go` running the framework conformance
suites.

About fifty lines of code.

## One import

A plugin imports the façade, not the framework's layering:

```go
import "go.thesmos.sh/eidos/sdk"
```

`sdk` re-exports everything a plugin *names* — the role contracts, the
source model it reads, the emit model it writes, and the metadata,
diagnostic and position vocabulary joining them. A Go-generating
plugin adds `lang/golang/sdk` for the plugin base and `lang/golang` for Go
conventions; an annotator needs neither.

Reach past the façade only for something it deliberately excludes —
see [the façade's own doc](../../sdk/doc.go) for what those are and
why.

## The plugin contract

Every plugin satisfies `sdk.Plugin` — one method, `Name() string` —
plus at least one **role**:

| Role | Method | Does |
|---|---|---|
| `sdk.Frontend` | `Load` | Parses input into the source store |
| `sdk.Annotator` | `Annotate` | Stamps metadata on existing nodes |
| `sdk.Generator` | `Generate` | Produces emit values |
| `sdk.Backend` | `Language`, `Render` | Renders emit values to a language |

An annotator adds and removes nothing: the node graph is frozen
between the frontend and generator phases, and annotators run inside
that window. Its whole output is metadata.

Beyond the roles, a plugin opts into **capabilities** — typed options,
directive schemas, priority, versioning. Each is a plain interface the
pipeline detects by assertion.

## Step 1 — the package

Create `myco/reporepo/reporepo.go`:

```go
// Package reporepo stamps myco.repo=true on every source struct whose
// name ends in "Repo". Downstream plugins read it with
// reporepo.MetaRepo.Get(s.Meta()).
package reporepo

import (
    "strings"

    "go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier. It appears in diagnostics,
// in every generated file's provenance header, and in the cache key.
const Name = "reporepo"

// MetaRepo is the typed key this plugin stamps.
//
// The parser turns the serialised form back into a bool: metadata
// survives a cache round trip as text, so a key that cannot parse
// itself back cannot be read from a warm cache.
var MetaRepo = sdk.NewKey[bool](
    "myco.repo",
    func(raw string) (bool, error) { return raw == "true", nil },
)

// Plugin satisfies sdk.Annotator. The zero value is usable.
type Plugin struct{}

// New returns a fresh plugin.
func New() *Plugin { return &Plugin{} }

// Name returns the stable identifier.
func (*Plugin) Name() string { return Name }
```

Use `sdk.NewKey` when this plugin owns the key. Use `sdk.EnsureKey`
when several packages declare the same key and any of them may be
first — `NewKey` panics on a duplicate, which is what you want for a
key you own and not for one you share.

## Step 2 — implement Annotate

```go
// Annotate stamps MetaRepo on every source struct named *Repo.
func (*Plugin) Annotate(ctx *sdk.AnnotatorContext) error {
    for s := range ctx.Reader.Structs().All() {
        if !strings.HasSuffix(s.Name, "Repo") {
            continue
        }
        MetaRepo.Set(s.EnsureMeta(), true, Name)
    }
    return nil
}
```

Two details in those four lines are the whole difference between a
plugin that works and one that fails silently.

**Read through `ctx.Reader`, never `ctx.Store`.** Reads captured by
the Reader compose the plugin's cache key. A read that goes around it
is a read the cache cannot invalidate on: the source changes, the
fingerprint does not, and the next run serves stale output that is
indistinguishable from current. Nothing reports it. `ctx.Store` is
present for what the Reader does not cover — chiefly the emit side.

**Write through `EnsureMeta()`, never `Meta()`.** `Meta()` is the read
accessor and returns `nil` for a node nothing has stamped yet, which
is every node on a cold run. Passing that to a setter panics.
`EnsureMeta()` allocates the bag once, on first write.

Iteration order is stable insertion order, so the pass is
deterministic without sorting.

## Step 3 — declare a priority

Without one, the plugin lands in `sdk.DefaultPriority` (300 — the same
value as `GeneratorCrossCutting`). Shape annotators run earlier so
that refinement annotators and generators see populated values:

```go
// Priority places the plugin in the shape-detection bucket.
func (*Plugin) Priority() sdk.Priority { return sdk.AnnotatorShape }

// Provides names the capability this plugin produces. A generator
// depending on the shape lists it in Requires.
func (*Plugin) Provides() []string { return []string{"myco.shape.repo"} }

// Requires is nil — nothing upstream.
func (*Plugin) Requires() []string { return nil }
```

**All three methods or none.** `CapabilityProvider` is a single
interface, so a plugin declaring `Priority` alone satisfies neither it
nor any consumer's check for it: the assertion fails, the plugin
collapses to the default priority, and the ordering its author wrote
down is discarded without a diagnostic. Returning `nil` from
`Provides` and `Requires` is fine and common. Omitting them is the
bug. The conformance suite fails a partial implementation.

`Requires` resolves only *within* a priority bucket. A dependency
across buckets is expressed by choosing the later bucket, not by
naming a capability the sorter will not consult.

## Step 4 — the conformance test

Create `myco/reporepo/reporepo_test.go`:

```go
package reporepo_test

import (
    "testing"

    "go.thesmos.sh/eidos/eidostest/plugintest"
    "go.thesmos.sh/eidos/eidostest/storefixture"
    "go.thesmos.sh/eidos/sdk"
    "myco/reporepo"
)

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
                Name: "package with Repo-suffixed structs",
                BuildStore: func(t *testing.T) *sdk.Store {
                    t.Helper()
                    return storefixture.New().
                        Package("shop", "example.com/shop").
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

```sh
go test ./...
```

`RunSuite` pins the universal contracts: a stable non-empty `Name`; at
least one role implemented; `CapabilityProvider` whole or absent;
deterministic `Provides`/`Requires` returned as fresh slices rather
than the plugin's own backing array; directive schemas uniquely named;
declared outputs well-formed; shipped templates parsing and claiming
no reserved name.

`RunAnnotatorSuite` adds the per-role contracts: `Annotate` does not
panic on an empty store, the node count is unchanged across the pass,
and running twice produces identical metadata.

That last one is worth dwelling on. Annotators re-run against warm
caches and partially annotated graphs, so a pass that is not
idempotent produces different output on the second run than the first
— and the second run is the one your users get.

If you want to confirm the harness is wired up at all,
`plugintest.BrokenPlugin(v)` returns a plugin violating exactly one
contract. Watching your suite fail against it is the cheapest way to
catch a test that is passing because it is not running.

## Where to go next

- **[recipes.md](recipes.md)** — the other three roles and the
  cross-cutting weaver pattern.
- **[conformance.md](conformance.md)** — the full `plugintest`
  reference and fixture authoring.
- **[`reference/shapewriter`](../../reference/shapewriter)** — the
  production-grade equivalent of the plugin above.

## Common next questions

**My plugin needs typed options.** Embed `*sdk.Holder[Options]` and
assign `sdk.BindOptions(&p.opts)` in `New`. The pipeline calls
`SetOptions` at build time; defaults apply at bind time, so tests and
direct invocation see populated values too. Worked example:
[`reference/repogen`](../../reference/repogen).

**My plugin needs a `+gen:` directive.** Implement
`DirectiveProvider`:

```go
func (*Plugin) Directives() []sdk.DirectiveSchema {
    return []sdk.DirectiveSchema{
        sdk.NewDirective("myco-repo").
            On(sdk.NodeKindStruct).
            Describe("Opts the struct into MyCo repository emission.").
            Build(),
    }
}
```

Scope with `sdk.NodeKind*`, not `sdk.EmitKind*`. Every declaration
kind exists on both sides carrying a different value, and a directive
scoped to an emit kind matches no source node — so the plugin never
fires and nothing reports it.

A directive name is not a metadata key name. The parser accepts a
letter followed by letters, digits, `_` and `-`, so `myco.repo` names
a key and can never name a directive. Names are collected at build
time and duplicates rejected, so one plugin owns a directive and the
rest read the stamp.

**My plugin emits code.** That is a Generator. Start at
[recipes.md](recipes.md): embed `sdk.Base` rather than answering the
declaration methods yourself, and declare one `LanguageSupport`
bundle per language you target — `lang/golang/sdk.Support` builds
Go's.
