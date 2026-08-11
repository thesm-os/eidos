---
adr: 0006
title: Target one backend per pipeline run
status: Accepted
date: 2026-08-11
supersedes: none
superseded-by: none
---

# ADR-0006: Target one backend per pipeline run

## Status

Accepted.

## Context

A backend renders emit values to text and owns everything downstream of
that: import resolution, formatting, file naming, and the target
language's conventions.

Those are not independent per output file. Import resolution is a
whole-file property computed from every type the file references;
formatting is a language-wide pass; the layout step routes an emit
value to a path using conventions that only make sense within one
language. A run that produced Go and TypeScript together would have to
carry both sets through every one of those steps and pick between them
per value.

Nothing about generating two languages requires it to happen in one
process. The frontend and the source graph are language-agnostic
already, so a second run over the same source is not a second parse of
a different thing — it is the same input with a different renderer.

## Decision

We will support exactly one backend per pipeline. `Builder.Build`
rejects zero backends with `ErrNoBackend` and more than one with
`ErrMultipleBackends`. Multi-language output is multiple runs.

## Alternatives Considered

### Multiple backends fanning out in one run

Register several backends; each renders every emit value it can.
Rejected because the stages after generation stop having one answer.
Layout routes a value to an output path per language, and the sink's
staleness sweep — which deletes generated files no longer produced —
would have to distinguish "this file's language was not generated this
run" from "this file is stale", or delete another language's output.

### One backend, multiple output languages

A single backend that renders more than one target. Rejected as
relocating the problem rather than solving it: the backend then holds
the per-language import resolution and formatting the pipeline was
trying to avoid holding, with no framework support for keeping them
apart.

### Backend selected per emit value

Each emit value declares its language and the pipeline dispatches.
Rejected because it makes every generator language-aware, which is the
coupling the frontend/backend split exists to remove, and it gives a
plugin author a decision they have no basis to make.

## Consequences

### Positive

- Import resolution, formatting, and file layout each have exactly one
  target language to answer for, so none of them carries a per-value
  branch.
- The staleness sweep can treat everything it did not produce this run
  as stale, because the run covers one language completely.
- A generator plugin never knows which language it targets, so the same
  plugin works against any backend.

### Negative

- A project generating two languages runs the tool twice, so its build
  wiring carries two invocations and two configurations rather than
  one.
- Each run reparses the source, so the frontend cost is paid per
  language. For a large source tree generated to several languages that
  is real duplicated work.
- Cross-language consistency is the user's problem. Nothing checks that
  the Go and TypeScript outputs of the same source agree, because
  nothing sees both.

### Neutral

- The `backends` field on `Builder` is a slice that must hold exactly
  one element. It is registration-shaped rather than arity-shaped so
  that registering a second backend fails with a diagnostic naming both
  rather than silently replacing the first.
