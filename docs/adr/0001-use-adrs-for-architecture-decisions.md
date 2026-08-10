---
adr: 0001
title: Use ADRs for architecture decisions
status: Accepted
date: 2026-08-10
supersedes: none
superseded-by: none
---

# ADR-0001: Use ADRs for architecture decisions

## Status

Accepted.

## Context

eidos carries several load-bearing architectural choices whose
rationale lives only in prose: plugins are static Go imports rather
than dynamically loaded, metadata is the sole inter-plugin extension
mechanism, composition happens through slots rather than inheritance,
templates are owned per backend and per plugin, and a run targets one
backend. Each trades something away deliberately.

Until now that rationale sat in a `## Design decisions` section of
`README.md`. Two forces made that inadequate.

The section states conclusions without the alternatives that lost.
A contributor who thinks dynamic plugin loading is obviously better
finds no record that it was considered, so the question returns.
Nothing in the file distinguishes "we chose this over X and Y" from
"nobody thought about it".

It also has no room to grow. A README answers *what is this and should
I use it*; a reader in that state does not want five architectural
essays, and a reader who does want them will not think to look in a
README. Every subsequent decision either bloats a document with the
wrong audience or goes unrecorded — and the second is what has been
happening.

The repository now has a documentation tree with an audience-routed
index, so there is somewhere for these to live.

## Decision

We will record architectural decisions as ADRs under `docs/adr/`, one
decision per file, numbered monotonically and never renumbered. An
Accepted ADR is not edited in place: when a decision changes, a new
ADR supersedes it, and only the `status` and `superseded-by`
frontmatter of the old one change.

## Alternatives Considered

None — this is a process bootstrap.

## Consequences

### Positive

- The alternatives that lost are recorded, so a settled question stops
  being re-opened by contributors who cannot tell it was settled.
- Each decision has one home and one number, so it can be cited from
  code comments, RFCs and other ADRs.
- Superseding is explicit and additive, so the history of a decision
  survives the decision changing.

### Negative

- Every architectural change now carries a writing cost, and the
  Alternatives section is the expensive part — it demands that
  rejected options be described fairly enough to convince, which is
  harder than asserting a conclusion.
- A tree of ADRs decays quietly if the index is not maintained. An
  index claiming `Accepted` for a superseded decision is worse than no
  index, because it is trusted.
- There is now a judgement call on every change: implementation detail
  or architectural decision. Getting it wrong in the permissive
  direction produces an archive nobody searches.

### Neutral

- The five decisions currently in `README.md` are not yet converted.
  They remain there until each is written up with the alternatives it
  beat, which is the work this ADR makes possible rather than work it
  performs.
