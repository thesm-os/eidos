# TypeScript frontend

The TypeScript frontend converts `.ts` / `.tsx` / `.mts` / `.cts`
sources — parsed with tree-sitter — into the language-agnostic `node`
model. Language-specific facts ride on metadata keys in the `ts.*`
namespace rather than first-class node fields, keeping `node` and
`emit` portable to other languages.

This document catalogues every `ts.*` key the frontend stamps and the
node kinds it attaches to.

## Parsing

Two grammars, selected by extension. They are not interchangeable:
`<T>value` is a type assertion in `.ts` and the opening of a JSX
element in `.tsx`, so the same bytes parse to different trees. Parsers
are pooled per grammar because a tree-sitter parser is not safe for
concurrent use and binding a grammar crosses into C.

tree-sitter recovers from syntax errors rather than stopping, so a
file with a mistake in it still contributes the declarations it could
read; each unparseable region is reported as a positioned diagnostic.

A package is a directory, named by its basename — TypeScript has no
package clause. A declaration's members are recorded at the file the
declaration was parsed from, which is what routes generated output
beside its source.

## Reading these keys from a plugin

**Read them through `lang/typescript`, never through
`lang/typescript/frontend`.** A plugin importing a specific frontend
fails the build: depguard denies `plugins/…` any dependency on a
frontend or backend, because a plugin that knows which frontend
produced its input can only ever work with that one. Importing the
frontend also means importing tree-sitter, which is a C toolchain a
plugin has no business carrying.

`lang/typescript` owns the key vocabulary and the typed accessors,
and it is language-conventions only — no parsing, no rendering — so
both sides may depend on it:

```go
import "go.thesmos.sh/eidos/lang/typescript"

if typescript.IsUnion(field.Type) {
    for _, member := range typescript.Members(field.Type) { … }
}
if opt, _ := typescript.MetaOptional.Get(field.Meta()); opt {
    // declared `name?: T`
}
for _, d := range typescript.Decorators(field) {
    // in source order, repeats preserved
}
```

Use `Meta()` for reads — it returns `nil` for a node nothing has
stamped and the typed `Get` handles that. A write needs
`EnsureMeta()`, which allocates the bag.

## Structural markers, not keys

Three type shapes stay structural rather than riding metadata: union,
intersection and tuple members sit on a marker ref's `TypeArgs`
(`typescript.RefUnion`, `RefIntersection`, `RefTuple` under the
synthetic package `ts`). They are `node.TypeRef` values, and
`TypeArgs` is the field a generic walker already descends — a
metadata key would hide a union's members from every traversal that
does not know it. Read them with `IsUnion` / `IsIntersection` /
`IsTuple` / `Members`.

A type expression with no structured form at all — a conditional, a
mapped type, `keyof T` — is the fourth marker, `RefOperator`, with
the verbatim source text on `ts.typeText`.

## Key catalogue

### Declaration-level (`convert.go`)

Stamped on whatever declaration the wrapper encloses.

| Key | Type | Stamped when |
|-----|------|--------------|
| `ts.exported` | `bool` | The declaration was reached through an `export` statement. |
| `ts.defaultExport` | `bool` | The `export default` form. |
| `ts.ambient` | `bool` | Inside `declare` — asserted to exist elsewhere, no implementation here. |
| `ts.namespace` | `string` | The dotted namespace path the declaration was hoisted out of. |

### Class-level (`struct.go`, on `*node.Struct`)

| Key | Type | Stamped when |
|-----|------|--------------|
| `ts.abstract` | `bool` | An `abstract` class. |
| `ts.heritage` | `string` | On each `*node.Embed`: `extends` or `implements`. Extending inherits members; implementing only asserts them. |
| `ts.indexSignature` | `string` | `[key: string]: T`, verbatim. |
| `ts.constructSignature` | `string` | `new (…): T` from an interface body, verbatim. |
| `ts.visibility` | `string` | An explicit access modifier on a constructor parameter property. |

### Member-level (`member.go`, on fields and methods)

| Key | Type | Stamped when |
|-----|------|--------------|
| `ts.optional` | `bool` | A `?`-marked property, method or parameter. Distinct from a type union with `undefined` — `exactOptionalPropertyTypes` makes the difference load-bearing. |
| `ts.readonly` | `bool` | A `readonly` property. |
| `ts.static` | `bool` | A `static` class member. |
| `ts.abstract` | `bool` | An `abstract` member. |
| `ts.visibility` | `string` | `public` / `protected` / `private` / `#` (hard). Stamped only where written: absent and `public` are deliberately distinguishable. |
| `ts.definiteAssignment` | `bool` | The `!` assertion — `name!: string`. |
| `ts.async` | `bool` | An `async` method. |
| `ts.generator` | `bool` | A generator — `*name()`. |
| `ts.accessor` | `string` | `get` or `set` — a method whose use site is a property access. |
| `ts.parameterProperty` | `bool` | A constructor parameter that also declares a field. |
| `ts.initialiser` | `string` | A property initialiser, verbatim. |

### Callable-level (`overload.go`, `decorator.go`)

| Key | Type | Stamped when |
|-----|------|--------------|
| `ts.overloads` | list | The overload signatures declared above an implementation, verbatim, in source order — which is resolution order. The displaced signatures fold onto the implementation's node. |
| `ts.decorators` | list | The decorators applied, in source order, repeats preserved. One ordered list rather than a key per name, because `@A @B` and `@B @A` compose differently and a route declares `@ApiResponse` per status code. |

### Type-level (`type_ref.go`, on `*node.TypeRef`)

| Key | Type | Stamped when |
|-----|------|--------------|
| `ts.nullable` | `bool` | The type includes `null` or `undefined`. |
| `ts.literalType` | `string` | A literal type's value — the `'a'` in `type Tag = 'a'`. |
| `ts.typeText` | `string` | The verbatim text of an operator-marker type. |
| `ts.mapped` | `bool` | An `object_type` that is a mapped type rather than a member list. |
| `ts.constructor` | `bool` | A `new () => T` constructor type. |
| `ts.rest` | `bool` | A tuple element carrying `...`. |

### Enum, generic and import keys

| Key | Type | Stamped when |
|-----|------|--------------|
| `ts.constEnum` | `bool` | A `const enum` (`enum.go`). |
| `ts.typeParamDefault` | `string` | The `= string` in `<T = string>`, verbatim (`type_param.go`). |
| `ts.moduleSpecifier` | `string` | An import's specifier exactly as written — `./user` and `user` resolve differently (`import.go`). |
| `ts.typeOnly` | `bool` | `import type` / `export type` — erased at compile time. |
| `ts.reExport` | `bool` | An import produced by `export … from`. |
| `ts.reExportNames` | list | The names a re-export forwards, in source order. |

## What is deliberately not stamped

JSDoc is not duplicated onto a key: `DocLines` already carries it,
and the frontend is generic — a doc comment is not a TypeScript fact.
Implicit enum values are left empty rather than computed; the rule is
"one more than the previous numeric member", which a consumer applies
(`typescript.EnumOf` does) and which recording would turn into a
literal the source never wrote.
