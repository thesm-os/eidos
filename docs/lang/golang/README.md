# Go language support (`lang/golang`)

Everything eidos knows about Go lives under `lang/golang`, in one
module. Four packages share the path and they are not
interchangeable: picking the wrong one is a build error at best and a
coupling you cannot undo at worst.

| Package | Owns | May be imported by |
|---|---|---|
| `lang/golang` | Go *conventions* — no parsing, no rendering | Everyone: the frontend, the backend, and every plugin |
| `lang/golang/frontend` | Parsing Go source into the node graph | The pipeline. **Not plugins.** |
| `lang/golang/backend` | Rendering the emit graph into Go files | The pipeline. **Not plugins.** |
| `lang/golang/sdk` | The base a Go-generating plugin embeds | Plugins that generate Go |

`.golangci.yml` enforces the two denials: depguard refuses `plugins/…`
any dependency on a `frontend/…` or `backend/…` package, because a
plugin that knows which frontend produced its input can only ever work
with that one, and a plugin that reaches into a backend has chosen its
output language on its consumers' behalf.

`lang/golang` itself is what remains once both are excluded — the
vocabulary and the conventions — which is precisely why every
Go-speaking consumer can share it.

## Why one module

The four are one module rather than four, so a consumer states a
single dependency to speak Go. The alternative — a module per package
— bought the ability to take the parser without the renderer, and the
cost was a `replace` directive per package in every dependent
`go.mod`.

The same shape holds for any language added later:
`lang/<lang>/{frontend,backend,sdk}` under one module, with the
conventions at its root. `lang/protobuf` is the second instance,
carrying a frontend and no backend — proto is read, never written.

## What lives there

The package doc is the catalogue, and it is organised the way a
generator actually runs: **ask** questions of a source declaration,
**project** it into renderable data, **spell** that data as Go.

Read [`lang/golang/doc.go`](../../../lang/golang/doc.go) for the full
surface. The groups, in brief:

- **Signature queries** — `Callable`, `HasContext`, `StripContext`,
  `ErrorReturn`, `TrailingVariadic`, and the element accessors.
- **Type predicates** — `IsExported`, `IsSlice`, `IsMap`,
  `IsByteSlice`, `Nilable`.
- **Well-known shapes** — `IsErrorMethod`, `IsStringMethod`,
  `IsWriteMethod`, the codec pairs. Matched on *types*, not on a
  return's binding name, so a method called `String` that does not
  have `String`'s signature is not mistaken for one.
- **Embedding and satisfaction** — Go's promotion rules in full.
  A generator reading `s.Fields` reads what the source *typed*, not
  what the struct *has*.
- **Enums** — including the fact that catches every enum generator
  once: a string enum's textual form is its declared value, not its
  identifier.
- **Metadata accessors** — typed readers over the `go.*` vocabulary.
- **Identifiers and imports** — `SafeIdent`, `IsKeyword`,
  `ImportAlias`, `ExternalTestPackage`.
- **Funcmap bundles** — `FuncMap()` and the per-concern bundles the
  Go backend merges for templates.
- **Samples and zero values** — `SampleRefFor`, `ZeroRefFor`, and the
  `Sample` type a generated fixture is written from.

## Reading `go.*` metadata

This is the package a plugin reads frontend-stamped facts through:

```go
import "go.thesmos.sh/eidos/lang/golang"

if golang.IsContext(param.Type) {
    // first parameter is a context.Context
}
```

The keys themselves are catalogued in [frontend.md](frontend.md) —
that page documents what the frontend *stamps*; this package is how
you *read* it. The split is deliberate: a plugin reads the vocabulary
without importing the frontend that wrote it.

## The one rule

**Do not return `FuncMap()` from a plugin's template funcs.** The
whole bundle is already merged into the backend's overrideable
bucket, so plugin templates call it without registering anything.
Registering it again collides with the next plugin that does the
same, and the build fails on a name neither of them wrote. See
[plugin/templates.md](../../plugin/templates.md).
