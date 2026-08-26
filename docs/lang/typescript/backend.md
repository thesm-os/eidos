# TypeScript backend

The TypeScript backend renders emit graphs to TypeScript source. It
implements `plugin.Backend` under the name `backend.typescript` and
answers for the language `typescript`.

A pipeline takes exactly one backend, so a TypeScript-targeting
binary registers this one in place of the Go backend — see
`cmd/eidos-reference`'s `defaultPlugins` for the swap, and
`TestTypeScriptTargetE2E` beside it for the proof.

## Declarations, not bodies

The backend renders no statement and no method body. That single fact
decides most of its shape:

- **Classes are ambient.** A bodiless method in a plain class is
  TS2391 and an uninitialised property TS2564 under strict, so every
  class renders `export declare class`. Interfaces, aliases and enums
  need no bodies and render plain.
- **Functions, and value-less variables and constants, are ambient**
  for the same reason.
- **A constant or variable with a value drops `declare` and carries
  it** — `export const MAX: number = 100;`. An ambient declaration
  admits no initialiser, and a value the expression renderer can
  spell (a literal, an identifier, a reference) is not the runtime
  code the ambient spelling exists to avoid.
- **Runtime expressions are refused.** A generator emitting a call
  gets `ErrUnsupportedExpr` and a diagnostic naming the shape, not a
  plausible mis-rendering.

## Formatting without a formatter

The Go backend ends in `go/format.Source`, which both canonicalises
and validates. No Go-native TypeScript formatter exists, so the two
jobs are answered separately: canonical form comes from the templates
plus a normaliser (single quotes, semicolons always, two-space
indent, one blank line between declarations, single trailing
newline), and validity is checked in the acceptance tier —
`typescripttest`'s round trip parses every rendered file through
tree-sitter and type-checks it under strict `tsc`. Line wrapping is
absent by decision: matching Prettier means implementing its printer,
and a consumer's own formatter rewrites generated line breaks anyway.

## The ts.* vocabulary, and what becomes of it

Rendered:

| Keys | Rendering |
|------|-----------|
| `ts.visibility`, `ts.static`, `ts.abstract`, `ts.readonly` | Member keywords, in grammar order. A stamped `public` renders — the key's contract is that a stamp means the author wrote it. `#`-hard visibility rides the member name, unquoted. |
| `ts.optional` | `?` on properties, parameters and methods. |
| `ts.accessor` | `get` / `set` before the member name. |
| `ts.overloads` | The overload signatures render *instead of* the derived one — the implementation signature is a body fact. Functions render one `export declare function` per overload. |
| `ts.indexSignature`, `ts.constructSignature` | Verbatim lines opening the interface body. |
| `ts.constEnum` | `export const enum`. |
| `ts.typeParamDefault` | `<T = Def>`, verbatim. |
| `ts.initialiser` | `= text` appended to variables and constants, when no structured `Init`/`Value` expression was built; the expression wins where both exist. |
| `ts.exported` | The `export` prefix. Absent means exported: a generated type nothing can import is a type nothing can use. |

Absorbed before the backend runs: the union / intersection / tuple
markers, operator and literal type text, nullability and mapped types
all ride the ref shapes `typescript.FromNode` projects; the import
keys (`ts.moduleSpecifier`, `ts.typeOnly`, `ts.reExport*`) describe
the source module graph, and the backend derives its own import block
from what it spells.

Not rendered, deliberately:

- **`ts.async` and `ts.generator`** — illegal on an interface method
  and in an ambient class alike. Both say how a body produces its
  result; the `Promise` or iterator return type is the contract.
- **`ts.decorators`** — runtime metadata on runtime classes, which an
  ambient declaration cannot carry.
- **`ts.definiteAssignment`** — an assertion about a body's
  initialisation order, rejected in ambient contexts.
- **`ts.defaultExport` and `ts.namespace`** — authoring-style facts
  recorded for readers; generated modules use named exports at the
  top level.

## Canonical template set

One template per emit kind, resolved by the kind's string value:
`emit.interface`, `emit.struct` (a class), `emit.enum`, `emit.alias`,
`emit.function`, `emit.variable`, `emit.constant`. Methods, fields,
params, variants and heritage render through funcmap helpers rather
than sub-templates, so a plugin overriding one kind's template keeps
the shared member spelling.

## Canonical funcmap

Dispatch: `render`, `renderType`. Declaration parts: `renderDocs`
(JSDoc), `renderMembers`, `renderMethods`, `renderParams`,
`renderReturn`, `renderTypeParams`, `renderHeritage`,
`renderVariants`. Metadata: `abstractKw`, `constKw`, `signatures`,
`renderInit`, `annotation`, `overloadLines`, plus string-keyed `meta`
/ `metaBool`. Spelling: `quote` (single quotes), `ident` (reserved
words gain a trailing `_`), `propKey` (quotes what an identifier
cannot carry), `exported`, `indent`. Case: `camel`, `pascal`,
`scream`.

## Types

`renderType` spells every `emit.Ref` shape: builtins map through the
TypeScript names (`int` → `number`, `error` → `Error`, unknown names
pass through); pointers become `T | null`; slices and arrays `T[]`
(parenthesised when the element is compound); maps
`Record<K, V>`; funcs `(arg0: A) => R` with several returns as a
tuple; unions with `never` for the empty one. A ref the walk cannot
spell fails the file with `ErrUnsupportedRef` from wherever it sits —
`Box<>` is output that fails in the consumer's build, not here.

## Imports

The backend keeps its own import set rather than `writer.ImportSet`:
that models one alias per path — the whole of a Go import — while
TypeScript binds a set of names per specifier, plus an optional
default and namespace, any of which may be type-only. Spelling a type
is what registers its import, so the block is assembled after the
body and prepended; a reference into the rendered module registers
nothing, because a module cannot import itself.

## Determinism

Two runs over one graph produce identical bytes: no timestamps,
imports sorted (package specifiers before relative ones, then
lexically), declarations rendered in qualified-name order per target.
The conformance rung (`plugintest.RunBackendSuite`) pins it.

## Error sentinels

`ErrTemplateMissing` — an emit kind with no registered template,
named per kind. `ErrUnsupportedRef` — a type shape this backend
cannot spell as TypeScript. `ErrUnsupportedExpr` — a runtime
expression. `ErrDuplicateEntity` — two contributions declaring the
same member on one host. A render failure reports the file and
continues to the next; a sink failure aborts the run.
