---
adr: 0008
title: Map TypeScript interfaces to node.Struct
status: Proposed
date: 2026-08-25
supersedes: none
superseded-by: none
---

# ADR-0008: Map TypeScript interfaces to `node.Struct`

## Status

Proposed.

## Context

`node.Interface` is described in its own docblock as "a method-set
type — Go's interface, Rust's trait, and similar abstractions at the
model level" (`node/interface.go:15`). It carries `Methods`, `Embeds`
and `TypeParams`. It has no field list.

A TypeScript interface is not primarily a method set. Parsing
`interface User { readonly id: string; greet(): void }` yields an
`interface_body` whose members are `property_signature`,
`method_signature`, `index_signature` and `construct_signature`. Data
properties are the common case — the majority of interfaces in
practice declare no methods at all — and `node.Interface` has nowhere
to put them.

The choice is forced rather than stylistic: a TypeScript frontend
cannot represent its most common declaration without answering it, and
the answer either constrains the frontend or changes the
language-agnostic model that every frontend, backend and plugin
depends on.

The framework has already faced the general shape of this question.
The Go frontend does not map declarations by keyword: a `type Status
int` paired with a typed `const` block becomes `node.Enum`, not
`node.Alias`, because the model kind follows what a declaration *is*
rather than which keyword introduced it.

## Decision

TypeScript `interface` and `class` declarations both convert to
`node.Struct`, discriminated by a `ts.declKind` metadata key holding
`"interface"` or `"class"`.

`node.Struct` already carries `Fields`, `Methods`, `Embeds` and
`TypeParams`, which covers every member an interface body admits.
Members with no `node` equivalent ride on metadata: `index_signature`
on `ts.indexSignature`, `construct_signature` on
`ts.constructSignatures`.

`node.Interface` is left unused by the TypeScript frontend.

## Alternatives Considered

### Add a `Fields` slice to `node.Interface`

"Method-set type" encodes a Go-shaped assumption. TypeScript
interfaces carry data; so do Swift protocols and Rust traits with
associated constants. On this reading the model is wrong, and
TypeScript is the first language to expose it rather than an awkward
case to work around.

This is the strongest alternative and it may yet be right. It was
rejected here on blast radius rather than on principle: `node` is the
package every frontend, backend and plugin depends on, and widening a
core kind to admit a member that exactly one frontend populates makes
every consumer handle a field that is empty for Go and proto. If a
second language arrives wanting the same thing, the argument changes
and this ADR should be superseded rather than worked around.

### Map by shape: property-bearing interfaces to Struct, method-only to Interface

Preserves `node.Interface` for the declarations that genuinely are
method sets, and uses `node.Struct` only where properties force it.

Rejected because it makes one source construct convert to two model
kinds depending on its contents, so adding a property to an interface
silently changes its node kind and every plugin keyed on that kind
stops matching it. A generator would have to query both kinds to find
"TypeScript interfaces", which is worse than querying one kind with a
discriminator.

### Model properties as zero-argument methods

Fits properties into `node.Interface` unchanged, on the grounds that a
property read resembles a getter.

Rejected because it is false to the source. A plugin walking `Methods`
would find members that are not methods, could not tell them apart
without consulting metadata anyway, and would render them as methods
in any backend that took the model at face value. The metadata key
that repairs it is the same key this ADR proposes, without the
falsehood.

### Introduce a `node.Record` kind

A third structural kind, distinct from both, for structural types with
properties and methods.

Rejected as a larger version of the first alternative: it changes the
core model, and it adds a kind that every existing consumer must learn
in order to serve one frontend. `node.Struct` already has the shape;
a new kind would duplicate it.

## Consequences

### Positive

- No change to the language-agnostic model. `node`, and every consumer
  of it, is untouched.
- One source construct maps to one model kind, stably, regardless of
  what the interface body contains.
- Interface and class share a representation, which matches how
  TypeScript itself treats them — both are structural types, and a
  class name is usable as a type.

### Negative

- A plugin asking "what interfaces exist" finds none. Mock generators
  and similar shapes must query `node.Struct` filtered on
  `ts.declKind == "interface"`. This is discoverable only from
  documentation, which is a real cost against a plugin author's first
  attempt.
- `node.Interface` is dead weight in a TypeScript-only pipeline.
- The distinction between a type that can be implemented and a type
  that can be instantiated is carried by metadata rather than by the
  model, so a backend rendering `node.Struct` must consult
  `ts.declKind` before choosing a keyword.

### Neutral

- Nothing prevents the first alternative later. Adding `Fields` to
  `node.Interface` and remapping is a frontend change plus a model
  addition; the metadata key stays useful either way as the
  interface-versus-class discriminator, which no model change removes
  the need for.
