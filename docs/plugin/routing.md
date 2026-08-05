# Routing — placing rendered output

Routing is the framework concern of deciding **where** a plugin's
emit decls land: which directory, which filename, which `package`
clause. Plugins contribute nothing to this decision beyond the
filename suffix they declare and the source-node anchor they
attach to each decl. Everything else — package name, dir,
import path, test-package shift, cross-package qualification —
is computed by the pipeline's Layout phase from the directives on
the source and the project's configured policy.

This guide covers the user-facing surface (the three directive
forms), the precedence pipeline, the `_test.go` shift, and the
cross-package reference resolution. Read the [composition guide][1]
afterwards for slot-based cross-cutting; this document is purely
about where files go.

[1]: composition.md

## TL;DR

| Form | Anchor | When to reach for it |
|------|--------|---------------------|
| Default (no directive) | source location | alongside source, source package — the common case |
| `+gen:out <path>` | source node | any plugin without an obvious owning directive, or strict per-plugin scope via `plugin=` |
| `+gen:mock out=... pkg=...` | the directive that triggers emission | one-line override that propagates to companion plugins |

All three feed the same precedence pipeline; only the syntactic
anchor differs.

## Three equivalent forms

### 1. Default — no directive

```go
//+gen:mock
type Store interface {
    Get(ctx context.Context, key string) (Record, error)
}
```

The mock plugin emits a struct anchored on `Store`. The Layout
phase reads the anchor and resolves placement:

- **Dir** — source dir (where `store.go` lives).
- **Filename** — `<source-basename><plugin.Output.Suffix>` →
  `store_mock.go` for the mock plugin (`_mock.go` suffix) and
  `store_mock_test.go` for the mocktest companion.
- **Package** — source package, `store`, unless the filename ends
  in `_test.go` (the next section).
- **ImportPath** — source import path; receives the same `_test`
  shift when the filename triggers it.

The mocktest output lands in **`package store_test`** — Go's
external test convention — automatically. No directive needed.
Plugins that already opt into the convention themselves (their
`emit.Package` ends in `_test`) bypass the shift; the framework
does not double-suffix.

### 2. Standalone `+gen:out`

The framework reserves one core directive, `+gen:out`, that any
source can carry. Positional path plus optional `plugin=<name>`
scope and `pkg=<name>` package override:

```go
//+gen:out testkit/
//+gen:mock
type Store interface { ... }
```

→ `store/testkit/store_mock.go` (`package testkit`) and
`store/testkit/store_mock_test.go` (`package testkit_test`).

Three positional shapes:

```go
//+gen:out filename.go         // pin the rendered filename
//+gen:out subdir/             // place files in a sibling dir
//+gen:out subdir/file.go      // both
```

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
//+gen:out mocks/ plugin=mock
//+gen:mock
type Store interface { ... }
```

→ the mock file moves to `store/mocks/`, but mocktest stays in
the source dir following the default rules.

### 3. Per-directive `out=` / `pkg=` keys

Routing keys on any plugin's own directive. The pipeline records
directive ownership at Build time (from each plugin's
`DirectiveProvider.Directives()`) and recognises `out=` and
`pkg=` keys on every owned directive automatically:

```go
//+gen:mock out=testkit/ pkg=storetest
type Store interface { ... }
```

Semantically equivalent to the standalone `+gen:out testkit/
pkg=storetest` on the same source — same precedence layer, same
companion-aware propagation — but anchored at the directive
that actually triggers the emission. The natural form for the
common "this directive's products travel together" case.

**Scope** — per-directive keys produce an **unscoped** spec
(applies to every plugin emitting against the same origin), so
sibling generators that discover output via meta (mocktest
discovers mock output via `mock.iface`) inherit the override
without restating it. To restrict to one plugin, fall back to
the standalone form with `plugin=`.

## Precedence

Each layer overrides the previous when its field is set:

1. **Framework default** — alongside source, source package,
   plugin's filename suffix.
2. **Plugin filename suffix** — appended to the source basename
   (`store.go` + `_mock.go` → `store_mock.go`).
3. **Project layout policy** — `output.*` block in `.eidos.yaml`.
4. **Per-source routing directives** — `+gen:out` (form 2) and
   per-directive `out=`/`pkg=` keys (form 3). Both feed the same
   layer; both can be present on one source.
5. **CLI flags** — `-o`, `-p`, `-output-dir`, `-layout`.

Higher layers replace whichever fields they touch and leave
others unchanged. The `_test.go → <pkg>_test` shift runs at the
framework-default layer and is skipped when any higher layer
pinned `Package` (or when the resolved package already ends in
`_test`).

## The `_test.go → <pkg>_test` shift

When the resolved filename ends `_test.go`, Layout appends
`_test` to the resolved package and import path **at the
framework-default layer only**. The rule is uniform — never
conditional on the routing form:

| Resolved filename | Resolved dir | Resolved package |
|-------------------|--------------|------------------|
| `store_mock_test.go` | `store/` | `store_test` |
| `store_mock_test.go` | `store/testkit/` (from `out=testkit/`) | `testkit_test` |
| `store_mock_test.go` | `store/` (from `pkg=foo`) | `foo` — shift suppressed |

The shift gives Go's external test convention by default and
stays out of the way when the user explicitly sets `pkg=`.

## Cross-package references

When a generator emits an `emit.Internal(target)` ref, the Go
backend resolves qualification at render time from the target's
resolved `Target.ImportPath`:

- Target's import path **equals** the rendering file's import
  path → bare name (same-package elision).
- Target's import path **differs** → register the import on the
  file and qualify the name with the resulting alias.

This is what lets mocktest reference the mock struct via
`emit.Internal(s)` regardless of whether the test file lands in
the same package, `<pkg>_test`, or a sibling testkit package —
the framework resolves the qualifier post-routing without the
plugin knowing or caring.

## Plugin-side contract

The plugin's entire contribution to routing is:

```go
b := builder.For(p.Name()).Anchor(srcNode)
b.Struct(name, func(s *builder.StructBuilder) { ... })
out, err := b.Build()
```

`Anchor(srcNode)` derives the emit package path from the
anchor's source package and stamps the anchor as the default
`Origin` on every decl built through `b` — no per-decl
`s.Origin(srcNode)` needed. The plugin separately declares its
filename suffix via the `FilenameProvider` capability
(`Outputs(lang) []Output`).

Plugins **never** call `Package(name, path)` with non-empty
arguments (that signals "no opinion"; the framework fills both
in), never set `emit.Target` on a decl, and never look at the
file's destination. The framework does all of that.
