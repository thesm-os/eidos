# Go frontend

The Go frontend converts Go source packages — loaded via
`golang.org/x/tools/go/packages` — into the language-agnostic `node`
model. Language-specific facts ride on metadata keys in the `go.*`
namespace rather than first-class node fields, keeping `node` and
`emit` portable to other languages.

This document catalogues every `go.*` key the frontend stamps and the
node kinds it attaches to.

## Reading these keys from a plugin

**Read them through `lang/golang`, never through `frontend/golang`.**
A plugin importing a specific frontend fails the build: depguard
denies `plugins/…` any dependency on `frontend/…` or `backend/…`,
because a plugin that knows which frontend produced its input can
only ever work with that one.

`lang/golang` exists for exactly this. It owns the key vocabulary and
the typed accessors, and it is language-conventions only — no parsing,
no rendering — so both sides may depend on it.

The accessors are the shortest path:

```go
import "go.thesmos.sh/eidos/lang/golang"

if golang.IsContext(param.Type) {
    // first parameter is a context.Context
}
if golang.ReceiverIsPointer(method) {
    // ...
}
```

They answer the common questions — `IsError`, `IsContext`,
`IsStringer`, `IsComparable`, `IsInterface`, `EmbedsInterface`,
`IsEmptyInterface`, `IsConstraintInterface`, `ReceiverIsPointer`,
`UnderlyingKind`, `IotaValue` — without the caller handling a
two-value read.

For a key with no accessor, read the exported `Meta*` var directly:

```go
if dir, ok := golang.MetaChanDir.Get(typeRef.Meta()); ok && dir == "recv" {
    // <-chan T
}
```

Use `Meta()` for reads. It returns `nil` for a node nothing has
stamped, and the typed `Get` handles that — but a *write* needs
`EnsureMeta()`, which allocates the bag.

Templates read string-keyed, because templates are text:

```
{{ if metaBool . "go.isContext" }} ctx {{ end }}
{{ metaStr . "go.iterValueType" }}
```

Every stamp records full provenance — author `"golang"`, authority
`meta.AuthorityPlugin`, and the source position of the type
expression. `eidos explain` surfaces that chain.

## Key catalogue

### TypeRef-level

Stamped on the `*node.TypeRef` produced by the converter when the
type warrants it. Refs reach these keys via `typeRef.Meta()`.

| Key | Type | Stamped when |
|-----|------|--------------|
| `go.isChannel` | `bool` | The ref models a Go channel (Named ref with package `"go"` / name `"chan"`). |
| `go.chanDir` | `string` | Channel directionality: `"both"`, `"send"`, or `"recv"`. |
| `go.chanElem` | `string` | Printed source form of the channel's element type. (The element type also rides on the ref's first type argument.) |
| `go.isContext` | `bool` | The ref points at `context.Context`. |
| `go.isError` | `bool` | The ref is the predeclared `error` interface. |
| `go.isStringer` | `bool` | The underlying type implements `fmt.Stringer` (on either value or pointer form). |
| `go.isComparable` | `bool` | The underlying type satisfies Go's `comparable` constraint. |
| `go.isInterface` | `bool` | The underlying type is an interface, type parameters excluded. Lets a plugin tell a collaborator or stream from a plain value without resolving names, which it cannot do for types outside the loaded packages. |

### Struct-level

Stamped on the `*node.Struct`.

| Key | Type | Stamped when |
|-----|------|--------------|
| `go.embedsInterface` | `bool` | At least one embedded type's underlying type is an interface — Go's promotion-by-embedding case. |

### Interface-level

Stamped on the `*node.Interface`.

| Key | Type | Stamped when |
|-----|------|--------------|
| `go.isEmptyInterface` | `bool` | The interface declares no methods and no embeds. |
| `go.isConstraintInterface` | `bool` | The interface declares at least one type-set entry or `~T` approximate term. |

### Alias-level

Stamped on the `*node.Alias`.

| Key | Type | Stamped when |
|-----|------|--------------|
| `go.underlyingKind` | `string` | Short identifier for the underlying kind: `"basic"`, `"func"`, `"map"`, `"slice"`, `"array"`, `"pointer"`, or `"chan"`. |

### Method-level

Stamped on the `*node.Method`.

| Key | Type | Stamped when |
|-----|------|--------------|
| `go.receiverIsPointer` | `bool` | The method is declared on a pointer receiver (`func (*T) Foo()`). |

### Function-level

Stamped on the `*node.Function` when the function returns an `iter`
sequence type.

| Key | Type | Stamped when |
|-----|------|--------------|
| `go.isIterSeq` | `bool` | The single return type is `iter.Seq[T]`. |
| `go.isIterSeq2` | `bool` | The single return type is `iter.Seq2[K, V]`. |
| `go.iterKeyType` | `string` | Printed source form of an `iter.Seq2`'s key-type parameter. |
| `go.iterValueType` | `string` | Printed source form of an `iter.Seq` / `iter.Seq2`'s value-type parameter. |

### Constant / enum-variant

Stamped on the `*node.Constant` and forwarded to its promoted
`*node.EnumVariant`.

| Key | Type | Stamped when |
|-----|------|--------------|
| `go.iotaValue` | `int` | The constant has an integer-typed value (covers iota-driven enum variants). |

### Type-parameter constraints

Stamped on `*node.TypeParam` for generic-constraint type-set terms.

| Key | Type | Stamped when |
|-----|------|--------------|
| `go.constraintTerms` | `[]golang.ConstraintTerm` | The type-parameter's constraint declares at least one type-set term (e.g. `~int \| ~string`). Each `ConstraintTerm` carries a `Type *node.TypeRef` and an `Approximate bool`. |

### Struct-tag entries

Tag keys are dynamically named, registered through
`meta.EnsureKey`, and stamped on `*node.Field`.

| Namespace | Type | Stamped when |
|-----------|------|--------------|
| `go.tag.<key>` | `string` | The field carries a struct tag entry `<key>:"<value>"`. One key per declared tag entry, e.g. `go.tag.json`, `go.tag.db`, `go.tag.yaml`. |

## Provenance

Every key is stamped through `Key.SetAt(bag, value,
meta.AuthorityPlugin, "golang", pos)`. `pos` is the type's source
position — overlaid where possible, falling back to the type's
declaration position via `declPosOf` for refs the converter has not
yet position-overlaid. `eidos explain` surfaces the trail:

```
$ eidos explain typeref ctx
go.isContext
  ↳ golang set true (plugin) at user.go:14:18
```

## Where these are produced

Stamping happens in `frontend/golang/stamp.go` via per-kind helpers:

- `stampTypeRefMeta` — TypeRef-level facts (context, error, stringer, comparable).
- `stampChanMeta` — channel direction + element.
- `stampStructMeta` — interface embedding.
- `stampInterfaceMeta` — empty / constraint variants.
- `stampAliasMeta` — underlying kind.
- `stampMethodMeta` — pointer-receiver flag.
- `stampFunctionMeta` — iter.Seq / iter.Seq2 detection.
- `stampConstantMeta` — iota value.
- `stampVariantMeta` — forwards iota value to enum variants.
- `stampFieldTagMeta` — struct-tag entries.

Type-set constraint terms are stamped on `*node.TypeParam` from
`typeParamsFromList` via `MetaConstraintTerms.SetAt`.

## Cross-language bridge keys

These are not stamped by the Go frontend. A bridge annotator — the
proto→Go bridge, and any future one targeting Go — stamps them on a
*source* node so the Go backend renders a Go-shaped spelling for a
type that came from another language.

| Key | On | Meaning |
|---|---|---|
| `go.type` | TypeRef | Verbatim Go type spelling to render instead of the derived one |
| `go.name` | Field, Method | Go identifier to render instead of the source name |
| `go.import` | TypeRef | Import path to register for a `go.type` override |

They are listed here because the Go backend *reads* them, so a
plugin inspecting why a rendered type does not match its source node
will find the answer among them. A plugin generating from Go source
alone never sets them.
