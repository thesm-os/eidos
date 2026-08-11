---
adr: 0004
title: Compose generated output through slots, not inheritance
status: Accepted
date: 2026-08-11
supersedes: none
superseded-by: none
---

# ADR-0004: Compose generated output through slots, not inheritance

## Status

Accepted.

## Context

Generated output is rarely owned by one plugin. A repository generator
emits a struct and its constructor; a tracing plugin wants a field on
that struct and a line in that constructor; a metrics plugin wants the
same, independently, without either knowing the other exists.

[ADR-0003](0003-metadata-as-the-extension-mechanism.md) settles how
plugins exchange *facts*. It does not settle how two plugins
contribute to the same *output*, which is a different problem: facts
merge by authority, but two struct fields both belong in the result and
their order is visible in the diff.

The output is committed source under review, so ordering has to be
stable across runs and explainable to whoever reads the diff.

## Decision

We will have generators emit entities carrying named slots, each typed
by the content kind it accepts. Plugins append into the slot they have
something to say about. Slot contents order by capability topology
across plugins, with an alphabetical tie-break.

## Alternatives Considered

### Template inheritance with overridable blocks

The Jinja and Django model: a base template declares blocks and a child
overrides them. Rejected because override is the wrong primitive — a
block has one occupant, so a tracing plugin and a metrics plugin both
overriding `struct_fields` means the second silently wins. What is
needed is accumulation, and expressing accumulation in an override
mechanism means every block body has to call its parent by convention,
with nothing enforcing it.

### AST post-processing passes

Each plugin receives the rendered syntax tree and rewrites it.
Rejected: it gives every plugin the power to change anything, so no
plugin's output is stable under the addition of another, and a
conflict surfaces as mangled code rather than as an error. It also
makes the contract the whole language's AST, which the framework would
then owe compatibility on.

### String append into the output file

Plugins contribute text at named markers. Rejected because the
framework loses all structure: a slot typed by content kind can reject
a method appended into a field list at the point of the mistake, where
text cannot fail until the compiler sees it, if then.

### Visitor over a fixed entity hierarchy

Plugins implement visit methods for each emit type. Rejected as
subclassing in another spelling: adding an entity kind changes an
interface every plugin implements, and it still leaves open where two
visitors' contributions go relative to each other.

## Consequences

### Positive

- Two plugins can contribute to the same entity without either
  importing the other or knowing it exists.
- A slot's element kind is checked when the item is appended, so a
  method appended into a field slot fails with
  `emit.ErrSlotElementType` naming the slot rather than as broken
  output.
- Ordering is derived from declared capabilities rather than from
  registration order, so it is stable across runs and reproducible from
  the plugin set alone.

### Negative

- Slot names are a contract between plugins that the framework cannot
  check. A plugin appending into `"fields"` when the generator declared
  `"field"` gets `store.ErrUnknownSlotName` at run time, and only if
  that path executes.
- The generator that owns an entity has to anticipate where extension
  is wanted. A slot nobody declared is a slot nobody can append to, so
  a cross-cutting plugin can be blocked by an omission upstream.
- Capability-topological ordering is harder to predict than a list. A
  contributor asking why their field landed second has to reason about
  the capability graph rather than read an order.

### Neutral

- The alphabetical tie-break is arbitrary and deliberately so: it makes
  the outcome deterministic for plugins with no declared relationship,
  without implying one exists.
