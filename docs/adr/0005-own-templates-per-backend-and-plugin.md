---
adr: 0005
title: Own templates per backend and per plugin
status: Accepted
date: 2026-08-11
supersedes: none
superseded-by: none
---

# ADR-0005: Own templates per backend and per plugin

## Status

Accepted.

## Context

Something has to turn emit values into text, and that step is where all
the target language lives — receiver syntax, import blocks, zero
values, what a comment looks like.

Two groups need to control it, and they are not the same group. A
backend author adding a target language needs every construct of that
language spellable. A plugin author needs their own output to look the
way they intend, in whatever language the backend renders, without
patching the backend to get it.

The framework is in neither group. It has no view on how a Go method
renders, and any opinion it held would have to be re-litigated for
every language added after it.

## Decision

We will have each backend own its core templates and each plugin ship
its own, merging into a shared funcmap. The Go backend's live in
`backend/golang/templates/`; a plugin's live with the plugin. Override
resolution follows capability topology, the same ordering
[ADR-0004](0004-compose-output-through-slots.md) uses for slots.

## Alternatives Considered

### Framework-owned templates with per-language variables

One template set parameterised by a language description — comment
syntax, receiver form, and so on. Rejected: it assumes target languages
differ by substitution when they differ by structure. Go's error
returns and Rust's `Result`, or Java's checked exceptions, are not the
same template with different tokens, and each language added would push
another conditional into a file every other language shares.

### No templates — build syntax trees and print them

Each backend constructs the target language's AST directly. Rejected
for the plugin half rather than the backend half: it is defensible for
a backend, but it gives a plugin author no way to shape their output
without writing Go against the backend's node types, so every
presentational change becomes a backend change.

### One global template directory

All templates in one tree, keyed by name. Rejected because it makes
template names a global namespace across independently authored
plugins, so two plugins shipping a `header.tmpl` collide, and the tree
becomes something a consumer maintains rather than something a plugin
brings with it.

### Plugin templates that cannot override backend ones

Plugins contribute only new templates; core rendering is closed.
Rejected as too strict for the case that motivates overriding at all —
a plugin whose whole purpose is to change how an existing construct
renders has no route, and the workaround is forking the backend, which
is worse than the override being available.

## Consequences

### Positive

- Adding a target language is mostly authoring templates plus a
  format-and-imports pass, rather than a framework change.
- A plugin ships its own presentation, so its output can change without
  a backend release.
- Backend and plugin templates share one funcmap, so a plugin's
  template can use the backend's helpers instead of reimplementing
  them.

### Negative

- A shared funcmap is a shared namespace. Two plugins registering the
  same function name collide, which is why `sdk/golang` composes a
  per-plugin prefix — a mitigation, not a fix, since a plugin
  registering directly can still collide.
- An override resolved by capability topology means the winning
  template depends on the plugin set. Adding an unrelated plugin can
  change which template renders a construct.
- Template errors surface at execute time. A template calling a
  function nobody registered parses and ships, which is why
  `plugintest` asserts resolution as a conformance check rather than
  leaving it to a run.

### Neutral

- Templates being data rather than code means a plugin's rendering can
  be reviewed without reading Go, and equally that it is not
  type-checked. Neither is a win or a loss on its own; it moves where
  the errors are found.
