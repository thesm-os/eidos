# multipkg fixture

A multi-package testdata fixture for the import-handling and
generics-rendering paths. Its own `go.mod` keeps the fixture out of
the parent `eidos` module's build graph.

## Layout

```
multipkg/
    go.mod                   module example.com/multipkg
    domain/                  core entities + generics (Result, Page, Filter, Box, Numeric)
        types.go               User, Order, OrderItem, Product, ID
        generics.go            Box[T], Result[T], Page[T, C], Map[K, V], Filter[T], Sum[T Numeric]
    events/                  current events surface
        events.go              Event[T], Handler[T], Dispatcher[T]
    legacy/events/           older events surface — same short name `events`
        events.go              LegacyEvent, LegacyDispatcher
    storage/                 generic Repository[T] surface; mocked into storage_test
        repository.go          Repository[T], Query[T]
    api/                     headline cross-package consumer; imports BOTH `events` packages
        handler.go             Handler, Service
    internal/codec/          generic codec primitives, internal-only
        codec.go               Codec[T], PairCodec[K, V]
```

## What this fixture proves

### Same-package import elision

Files generated alongside a source struct (e.g. `domain/user_repo.go`)
land in the same Go package as the source. The renderer must NOT
emit a self-import for references back to types declared in that
package — verified by every `*_repo.go` / `*_builder.go` /
`registry.go` produced under `domain/`.

### Short-name collisions

`example.com/multipkg/events` and `example.com/multipkg/legacy/events`
both declare `package events`. The `api/handler.go` file imports
both, forcing the writer's ImportSet to detect the alias collision
and resolve the second one as `events2` (or whichever suffix the
collision discipline picks). Generated `Handler` mocks must thread
both aliases through the rendered method signatures.

### Cross-package generic instantiations

`storage.Repository[domain.User]`, `events.Event[*domain.User]`, and
`events.Dispatcher[*domain.Order]` reach the renderer with
non-trivial type arguments. `renderType` for `ExternalRef` must
include the bracketed type-arg list with the correct nested
qualifiers, and any same-package elision must apply per-element.

### `+gen:out` filename override

`domain.Product` carries two `+gen:out` directives:

- `+gen:out product_codegen.go` — applies to every plugin that
  emits decls for Product. repogen and registrygen compose into one
  rendered file.

  Product is deliberately *not* `+gen:builder`. An unscoped override
  pinning a filename cannot apply to a plugin emitting more than one
  file — one name, two outputs — and the pipeline refuses rather than
  dropping one silently. Builder is exercised on six other types
  here, so both the multi-output plugin and the unscoped override
  stay covered.
- `+gen:out product_mock_test.go plugin=mockgen` — plugin-scoped
  variant that overrides only mockgen's filename. mockgen's
  test-package mode (`package <src>_test`) is incompatible with
  the domain-package output sharing a filename — the
  one-file-one-package invariant fires when both target the same
  on-disk path with different package clauses. The scoped form
  routes mockgen's output separately so both layouts coexist
  cleanly.

`+gen:out` accepts a positional filename and two optional
key=value modifiers:

- `plugin=<name>` scopes the override to one plugin's output.
- `pkg=<name>` pins the rendered package clause along with the
  filename.

### Generated test-file output

`storage.Repository[T]` and `events.Handler[T]` are mocked with
mockgen's default routing: every mock lands in a `<srcPkg>_test`
emit.Package and the rendered file ends in `_mock_test.go`. The
Go toolchain compiles `_test.go` files only at test time, so the
generated mocks never reach a production binary. The external
test-package import identity (`<pkg>_test`) differs from the
regular package's, so references back into the regular package
qualify rather than elide.

Per-source overrides flow through the routing surface — a
`+gen:out:mockgen <path> pkg=<name>` directive on the interface,
or `-o <path> -p <name>` on the CLI, reshapes the destination
when a non-default layout is needed (whitebox same-package mocks,
production mocks in a custom location, sibling-directory routing).

## Acceptance assertions

This fixture is forward-looking documentation for the import-handling
and generics-rendering paths. The intended acceptance suite asserts:

- every generated file's import block resolves cleanly
- `go build ./...` inside the fixture exits 0 after generation
- the second run is byte-identical to the first (idempotency)
- mock outputs end in `_mock_test.go` and the rendered package
  matches the `<srcPkg>_test` default
- `events2` (or the equivalent suffixed alias) appears in
  `api/handler_mock_test.go` for the legacy events import
