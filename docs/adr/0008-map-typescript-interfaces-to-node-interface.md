---
adr: 0008
title: Give node.Interface a field list
status: Accepted
date: 2026-08-25
supersedes: none
superseded-by: none
---

# ADR-0008: Give `node.Interface` a field list

## Status

Accepted.

## Context

`node.Interface` was described in its own docblock as "a method-set
type — Go's interface, Rust's trait, and similar abstractions at the
model level". It carried `Methods`, `Embeds` and `TypeParams`. It had
no field list.

A TypeScript interface is not a method set. Parsing
`interface User { readonly id: string; greet(): void }` yields an
`interface_body` whose members are `property_signature`,
`method_signature`, `index_signature` and `construct_signature`. Data
properties are the common case — the majority of interfaces in
practice declare no methods at all — so a model that could not hold
them would lose most of what most TypeScript interfaces declare.

The definition was written from one language's vantage point.
TypeScript is the first language to test it, and Swift protocols and
Rust traits with associated constants would test it the same way.

The framework already accepts that a model kind carries what some
language needs and others leave empty. `node.Alias.Methods` exists
because Go allows methods on named types; no other frontend populates
it, and nobody reads its presence as a claim that an alias has methods
in general.

## Decision

`node.Interface` gains a `Fields []*Field` slice, mirrored on
`emit.Interface` with a matching `FieldsSlot()` for slot composition.
TypeScript `interface` declarations convert to `node.Interface` and
`class` declarations to `node.Struct`.

What separates the two kinds is instantiability, not which members
they may declare: a Struct is a type values are made of, an Interface
is a shape values are checked against. That line holds in Go,
TypeScript, Rust and Swift alike — a class can be constructed, an
interface cannot — and it survives both kinds carrying fields, just as
it already survives both carrying methods.

## Alternatives Considered

### Map TypeScript interfaces to `node.Struct`

Struct already had every member an interface body can hold, and its
docblock already named TypeScript's class. Both TS `interface` and TS
`class` would convert to Struct, told apart by a `ts.declKind`
metadata key. This needed no change to the core model, and the
precedent existed: the Go frontend picks a model kind by shape rather
than by keyword when it coalesces a typed-const group into
`node.Enum`.

This was the original proposal in this ADR and it was rejected on
consumer experience. A plugin asking "what interfaces are there" —
which is the shape a mock generator has — would find none, and would
have to learn to ask for Structs filtered on a metadata key. That is
discoverable only from documentation, `Package.Interfaces` and
`InterfaceByName` would be permanently empty for TypeScript, and the
kind that exists to model a contract would go unused by a language
whose contracts are everywhere.

### Map by shape: property-bearing interfaces to Struct, method-only to Interface

Preserves `node.Interface` for the declarations that genuinely are
method sets and uses Struct only where properties force it.

Rejected because it makes one source construct convert to two model
kinds depending on its contents, so adding a property to an interface
silently changes its node kind and every plugin keyed on that kind
stops matching it.

### Model properties as zero-argument methods

Fits properties into the existing `node.Interface` unchanged, on the
grounds that a property read resembles a getter.

Rejected because it is false to the source. A plugin walking `Methods`
would find members that are not methods, could not tell them apart
without consulting metadata anyway, and would render them as methods
in any backend that took the model at face value.

### Introduce a `node.Record` kind

A third structural kind for types with properties and methods,
distinct from both.

Rejected as a larger version of the accepted decision that also
duplicates it: it changes the core model, and it adds a kind every
existing consumer must learn in order to serve one frontend, when
`Interface` already means the right thing once its member list is
complete.

## Consequences

### Positive

- One source construct maps to one model kind, stably, regardless of
  what the body contains.
- `Package.Interfaces` and `InterfaceByName` work for TypeScript, so a
  generator looking for contracts finds them by kind rather than by
  filtering another kind on a metadata key.
- The `Interface` / `Struct` distinction now rests on what the kinds
  mean rather than on which members they happen to admit, which is
  what makes it portable to the next language.
- No metadata key is needed to recover the declaring keyword, so there
  is no second, staler answer to a question the node kind already
  settles.

### Negative

- `node` and `emit` both changed, and they are the packages every
  frontend, backend and plugin depends on. The change is additive —
  every existing frontend leaves the slice empty — but it widens a
  core type on one language's evidence.
- A consumer written against the old definition may assume an
  interface has no fields. Nothing enforced that assumption, but
  nothing contradicted it either until now.
- `Walk` and `RewireOwners` each gained an arm. A future kind added to
  `Interface` has three places to update rather than two.

### Neutral

- Go and protobuf frontends are unaffected: they populate no interface
  fields and their output is unchanged, which the full suite across
  every module confirms.
- `emit.Interface.FieldsSlot()` follows the same slot contract as the
  Struct equivalent, so cross-cutting field injection works on both
  hosts through the existing `AppendField` / `InsertField` helpers.
