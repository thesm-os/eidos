# Go frontend

The Go frontend converts Go source packages — loaded via
`golang.org/x/tools/go/packages` — into the language-agnostic `node`
model. Language-specific facts ride on metadata keys in the `go.*`
namespace rather than first-class node fields, keeping `node/` and
`emit/` portable to other languages.

This document catalogues every `go.*` metadata key the frontend
stamps and the node kinds it attaches to. It is the contract
plugins read against; in source these keys live on the
`golang.Meta*` package-level vars.

## Reading and writing keys

Plugin code reads via the typed `Key[T].Get` accessor:

```go
import "go.thesmos.sh/eidos/frontend/golang"

if isCtx, _ := golang.MetaIsContext.Get(typeRef.Meta()); isCtx {
    // ...
}
```

Templates read via the funcmap helpers — they're string-keyed
because templates are text:

```
{{ if metaBool . "go.isContext" }} ctx {{ end }}
{{ metaStr . "go.iterValueType" }}
```

Every stamp records full provenance — author `"golang"`, authority
`meta.AuthorityPlugin`, and the source position of the type
expression. `eidos explain` surfaces this chain.

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
