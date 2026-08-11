---
adr: 0003
title: Use metadata as the sole inter-plugin extension mechanism
status: Accepted
date: 2026-08-11
supersedes: none
superseded-by: none
---

# ADR-0003: Use metadata as the sole inter-plugin extension mechanism

## Status

Accepted.

## Context

Plugins have to tell each other things. A shape detector concludes a
struct is a writer; a generator three stages later needs that
conclusion. The two are written by different people, ship in different
packages, and neither may import the other — `plugins/**` cannot reach
`frontend/**` or `backend/**`, and nothing makes one plugin's package a
dependency of another's.

The pipeline also has to let a human overrule any of it. A detector
that infers the wrong thing about one struct must be correctable at
that struct, in source, without a plugin code change and without a
fork.

Whatever carries facts between plugins therefore needs three
properties at once: it must not create a compile-time dependency
between plugins, it must survive a human overriding it, and it must
say who won and why — a fact that silently changed value is
indistinguishable from one that was never stamped.

## Decision

We will make typed metadata the only channel between plugins. Facts
are written into a `meta.Bag` under a namespaced `meta.Key[T]`, each
write carrying an authority (`plugin` < `directive` < `manual`) and
provenance. Source overrides any key with `+gen:meta KEY=value` and
deletes one with `-gen:meta KEY`. Plugins do not call into each other.

## Alternatives Considered

### Direct plugin-to-plugin interfaces

A generator declares the annotator interface it needs and calls it.
Rejected: it makes the plugin graph a compile-time dependency graph, so
a consumer wanting one plugin gets its collaborators, and two plugins
that both want a fact from a third must agree on its interface. It also
has nowhere to put a human override — a call returns what it computes.

### A shared typed context struct

One framework-owned struct with a field per fact, threaded through
every stage. Rejected: every new fact is a framework change, which
makes the framework the bottleneck for work that is supposed to happen
in plugins. It also has no per-fact provenance — a field has a value
and no history of who set it.

### An event bus with typed subscriptions

Plugins publish facts and subscribe to the ones they need. Rejected as
the same mechanism with worse ergonomics: it delivers facts in
publication order, so a generator must handle arriving before the
annotator it depends on, and eidos already orders plugins by capability
topology. What remains after removing the delivery semantics is a
keyed store.

### Metadata without authorities

A plain typed key-value bag, last write wins. This is the closest
alternative and it fails on the override requirement: a `+gen:meta`
directive from source and an inference from a plugin are the same
operation, so whether the human wins depends on plugin ordering. The
authority rank exists to make that outcome independent of order.

## Consequences

### Positive

- Plugins compose without depending on each other, so one can be
  deleted without touching the others.
- Any fact is overridable at its source declaration, so a wrong
  inference is a one-line fix in the user's own file rather than an
  upstream issue.
- `Bag.Winning` and `Bag.History` can explain why a fact holds, which
  is what makes a surprising generated file diagnosable.

### Negative

- The type safety is per key, not per plugin: nothing stops two plugins
  choosing the same key name for different facts, and the collision
  surfaces as a wrong value rather than a build error. Namespacing is a
  convention the framework cannot enforce.
- Reading a fact tells you nothing about who produces it. Following the
  data means grepping for a key name across the plugin set, where a
  direct call would have a definition to jump to.
- A fact that no plugin writes reads as absent rather than as an error,
  so a missing annotator degrades to silently reduced output.

### Neutral

- Directive overrides are parsed and applied by the pipeline's
  override step, not by the plugins that own the keys, so a plugin gets
  override support without writing any.
