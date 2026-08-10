# Routing — placing rendered output

Routing is the framework concern of deciding **where** a plugin's
emit decls land: which directory, which filename, which `package`
clause. A plugin contributes three things and no more: the
filename suffixes it declares through `Outputs(lang) []Output`,
the source-node anchor it attaches to each decl, and — when it
has an opinion — the `sdk.EmitPackage` name it emits into.
Everything else — dir, import path, test-package shift — is
computed by the pipeline's Layout phase from the directives on
the source and the project's configured policy. Cross-package
qualification is resolved later still, by the backend at render
time.

This guide covers the user-facing surface (the three directive
forms), the precedence pipeline, the `_test.go` shift, and the
cross-package reference resolution. Read the [composition guide][1]
afterwards for slot-based cross-cutting; this document is purely
about where files go.

[1]: composition.md
[2]: multi-file-output.md

## TL;DR

| Form | Anchor | When to reach for it |
|------|--------|---------------------|
| Default (no directive) | source location | alongside source, source package — the common case |
| `+gen:out <path>` | source node | moves every plugin emitting from that source; narrow to one with `plugin=<name>` |
| `+gen:repo out=... pkg=...` | the directive that triggers emission | moves the owning plugin's output only, without naming it |

All three feed the same precedence layer; they differ in the
syntactic anchor and in default scope.

## Three routing forms

### 1. Default — no directive

```go
//+gen:mock
type Store interface {
    Get(ctx context.Context, key string) (Record, error)
}
```

mockgen emits a struct anchored on `Store`. The Layout phase
reads the anchor and resolves placement:

- **Dir** — source dir (where `store.go` lives).
- **Filename** — `<source-basename><sdk.Output.Suffix>` →
  `store_mock_test.go` for mockgen (`_mock_test.go` suffix).
- **Package** — the plugin's `sdk.EmitPackage` name when it set
  one, otherwise the source package, `store`. Either way a
  filename ending in `_test.go` triggers the shift below.
- **ImportPath** — the import path matching that package;
  receives the same `_test` shift when the filename triggers it.

The output lands in **`package store_test`** — Go's external
test convention — with no directive. mockgen reaches it by
emitting into a `<srcPkg>_test` `sdk.EmitPackage` of its own; a
plugin that leaves `sdk.EmitPackage.Name` empty and declares a
`_test.go` suffix reaches it through the framework shift
instead. Neither route double-suffixes: a package already ending
in `_test` is left alone.

### 2. Standalone `+gen:out`

The framework reserves a core routing directive, `+gen:out`, that
any source can carry. Positional path plus optional
`plugin=<name>` scope, `pkg=<name>` package override, and
`tag=<name>` output scope (see [multi-file output][2]):

```go
//+gen:out testkit/
//+gen:repo
type User struct { ... }
```

Unscoped, it moves every plugin emitting against that source.
repogen emits `User`'s repository, and mockgen mocks that
repository against the same origin, so both files travel:

→ `store/testkit/store_repo.go` (`package testkit`) and
`store/testkit/store_mock_test.go` (`package testkit_test`).

Three positional shapes:

```go
//+gen:out filename.go         // pin the rendered filename
//+gen:out subdir/             // place files in a sibling dir
//+gen:out subdir/file.go      // both
```

The path always resolves under the origin's own source
directory. A leading separator is stripped and `..` segments are
clamped, so `+gen:out /abs/file.go` and `+gen:out ../escape/`
land at `store/abs/file.go` and `store/escape/` — the directive
cannot write outside the source tree.

When the path carries a dir and `pkg=` is not set, the package
name is auto-derived from the resolved dir's basename. Use
`pkg=<name>` to override:

```go
//+gen:out testkit/ pkg=storetest
```

→ files land in `store/testkit/` but the `package` clause is
`storetest` (and `storetest_test` for the test file).

`plugin=<name>` is the strict-scope escape hatch — the override
applies **only** to the named plugin's output. Useful when one
plugin should land somewhere distinct from its companions:

```go
//+gen:out mocks/ plugin=mockgen
//+gen:repo
type User struct { ... }
```

→ the mock moves to `store/mocks/store_mock_test.go`
(`package mocks_test`), while `store/store_repo.go` stays in the
source dir following the default rules.

### 3. Per-directive `out=` / `pkg=` keys

Routing keys on any plugin's own directive. The pipeline records
directive ownership at Build time (from each plugin's
`DirectiveProvider.Directives()`) and recognises `out=`, `pkg=`
and `tag=` keys on every owned directive automatically:

```go
//+gen:repo out=testkit/ pkg=storetest
type User struct { ... }
```

Semantically equivalent to the standalone `+gen:out testkit/
pkg=storetest plugin=repogen` on the same source — same
precedence layer, same scope — but anchored at the directive
that actually triggers the emission, so the owning plugin's name
never has to be written down.

**Scope** — per-directive keys are scoped to the directive's
**owning plugin** and to nothing else. Above, repogen's
`store_repo.go` moves to `store/testkit/`; mockgen's
`store_mock_test.go`, anchored on the same `User`, stays where
the default rules put it. That is deliberate: the moment a type
carries two directives, an unscoped reading would let whichever
directive was written first decide where every other plugin's
output went. A plugin that genuinely has to follow another one
uses the standalone form with `plugin=`, which says so.

## Precedence

Each layer overrides the previous when its field is set:

1. **Framework default** — alongside source; `Dir` from the
   origin's source file, `Package` / `ImportPath` from the
   plugin's `sdk.EmitPackage` when it named one, otherwise from the
   origin's source package.
2. **Plugin filename suffix** — appended to the source basename
   (`store.go` + `_mock_test.go` → `store_mock_test.go`).
3. **Resolved layout policy** — the `output.*` block in
   `.eidos.yaml` (project, then per-plugin, then per-tag), with
   the CLI `-layout`, `-p` and `-output-dir` folded in on top.
4. **Per-source routing directives** — `+gen:out` (form 2) and
   per-directive `out=`/`pkg=` keys (form 3). Both feed the same
   layer; both can be present on one source.
5. **CLI `-o`** — `-o <path>`, or the scoped
   `-o <plugin>[:<tag>]=<path>` form.
6. **The `_test.go → <pkg>_test` shift** — last, over whatever
   the layers above resolved.

`-p` sits at layer 3, not above the directives: a `pkg=` written
on a source overrides it, because the source is more specific
than the run.

Higher layers replace whichever fields they touch and leave
others unchanged, with one exception — `Dir` **stacks**. A
directive path and a `-o` path each join onto the directory
below them, so `+gen:out dirdir/` under `-o clidir/cli.go`
resolves to `store/dirdir/clidir/cli.go`.

## The `_test.go → <pkg>_test` shift

When the resolved filename ends `_test.go`, Layout appends
`_test` to the resolved package and import path **after every
other layer has run**. The rule is uniform — never conditional
on the routing form, and never on which layer supplied the
package:

| Resolved filename | Resolved dir | Resolved package |
|-------------------|--------------|------------------|
| `store_mock_test.go` | `store/` | `store_test` |
| `store_mock_test.go` | `store/testkit/` (from `out=testkit/`) | `testkit_test` |
| `store_mock_test.go` | `store/testkit/` (from `out=testkit/ pkg=foo`) | `foo_test` |
| `store_mock_test.go` | `store/` (from `pkg=foo_test`) | `foo_test` — already suffixed |

`pkg=` answers "which package does this output belong to", not
"suppress Go's test convention", so it does not suppress the
shift: honouring `pkg=foo` literally on a `_test.go` file would
turn an external test into an internal one silently. The one way
to stop the shift is to resolve to a package that already ends
in `_test` — by writing `pkg=<name>_test`, or by emitting into a
`<pkg>_test` `sdk.EmitPackage` as mockgen does.

## Cross-package references

When a generator emits an `sdk.Internal(target)` ref, the Go
backend resolves qualification at render time from the target's
resolved `Target.ImportPath`:

- Target's import path **equals** the rendering file's import
  path → bare name (same-package elision).
- Target's import path **differs** → register the import on the
  file and qualify the name with the resulting alias.
- Target has **no** resolved import path (synthetic, or never
  routed) → bare name.

This is what lets mockgen reference repogen's interface via
`sdk.Internal(i)` regardless of whether the mock lands in the
same package, `<pkg>_test`, or a sibling testkit package — the
framework resolves the qualifier post-routing without the plugin
knowing or caring.

## Plugin-side contract

The plugin's entire contribution to routing is:

```go
b := sdk.NewProvenance(p.Name()).Anchor(srcNode)
b.Struct(name, func(s *sdk.StructBuilder) { ... })
out, err := b.Build()
```

`Anchor(srcNode)` derives the emit package path from the
anchor's source package and stamps the anchor as the default
`Origin` on every decl built through `b` — no per-decl
`s.Origin(srcNode)` needed. The plugin separately declares its
filename suffix via the `FilenameProvider` capability
(`Outputs(lang) []Output`).

Plugins leave `sdk.EmitPackage.Name` empty — that is the "no
opinion" signal, and the framework fills `Package` and
`ImportPath` from the origin's source package. A plugin with a
real reason to land elsewhere names the package itself, as
mockgen does with `c.Package(srcPkg.Name+"_test",
srcPkg.Path+"_test")`; the Layout phase honours a non-empty name
under alongside-source layout. What plugins never do is set
`sdk.EmitTarget` on a decl or look at the file's destination. The
framework does all of that.
