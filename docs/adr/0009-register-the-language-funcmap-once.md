---
adr: 0009
title: Register a language's funcmap once, in its backend
status: Accepted
date: 2026-08-25
supersedes: none
superseded-by: none
---

# ADR-0009: Register a language's funcmap once, in its backend

## Status

Accepted.

## Context

[ADR-0005](0005-own-templates-per-backend-and-plugin.md) decided that
backend and plugin templates share one funcmap, and recorded the cost
in its own Negative section: a shared funcmap is a shared namespace,
so two plugins registering one name collide. It named the mitigation
in use at the time — the Go plugin base composed a per-plugin prefix.

That mitigation had a cause worth stating plainly, because it is not
the one the wording suggests. The base did not prefix a plugin's *own*
helpers to keep them apart from another plugin's. It prefixed them
because it handed **every plugin its own copy of the entire Go
vocabulary** — the signature, query, convention, enum, shape, embed
and generics bundles, a hundred-odd helpers. Two plugins reaching for
`promotedMethods` therefore registered the same helper twice, and the
backend rejected the pair at Build. The prefix made the copies
distinct.

The cost landed on templates. A helper was addressed as
`mygen_promotedMethods` — a name appearing in no declaration, derived
at Build from the plugin's own name, and different in every plugin
for the same underlying function. A plugin author reading
`lang/golang` to find out what was available then had to guess at the
spelling. The duplication was also invisible: nothing reported that a
pipeline of six plugins held six copies of one vocabulary.

The Go backend already merged a small subset — twelve
identifier-convention helpers — into its overrideable bucket, where
plugin templates called them unprefixed and without registering
anything. So the framework already had the shape; it applied it to a
twelfth of the vocabulary.

## Decision

We will register a language's whole shared funcmap **once**, by that
language's backend, into its overrideable bucket. A plugin registers
only helpers it wrote itself, under the names it declared, and
replaces a shared name through `TemplateOverrides`.

The per-plugin funcmap prefix is removed, along with the `prefix`
parameter on every `lang/golang` funcmap constructor.

## Alternatives Considered

### Keep the per-plugin prefix

The state before this decision, and it works — a pipeline of any size
builds.

Rejected because it solves a duplication problem by renaming the
duplicates. The template-facing cost is permanent and falls on every
plugin author: a helper has no stable name, so no documentation can
name one, and a template cannot be moved between plugins without
rewriting every call. Removing the duplication removes the need for
the rename.

### Let each plugin choose which bundles to register, unprefixed

A plugin registers `EnumFuncMap` only if its templates use enums, so
collisions are rarer.

Rejected because "rarer" is the whole problem. Two plugins both
generating from enums is ordinary, not exotic, and the failure is a
Build-time collision on a helper neither plugin wrote — an error whose
text names a function the author has never heard of. It also makes a
plugin's registration list a thing to maintain against its own
templates.

### Let plugin extensions silently shadow shared names

Drop the collision check for the shared bundles, so the last plugin to
register wins.

Rejected because it converts a loud failure into a silent one. Two
plugins disagreeing about what `promotedMethods` means would render
different output depending on plugin order, which is the class of
defect capability topology exists to make explicit rather than
incidental.

### Give each plugin its own template set, so funcmaps isolate

The only arrangement that makes a funcmap genuinely per-plugin.
`text/template` scopes functions to a set, not to a template within
one, so per-plugin funcmaps means per-plugin sets and slot
contributions spliced in as rendered strings.

Rejected because it gives up most of what the backend does. Imports
are the clearest loss: one `writer.ImportSet` exists per target file,
and `renderExpr` registers into it as a side effect of rendering, so a
plugin contributing an external reference into another plugin's file
gets its import into that file only because both render through one
state. Separate sets render `time.Unix(42, 0)` into a file that never
imports `time`.

The rest goes with it. `render` resolves a template by `Node.Kind()`,
so a host cannot recurse into a kind it does not own; slot
composition ([ADR-0004](0004-compose-output-through-slots.md)) works
only for leaf content once a contribution can no longer see its host;
and an override ([ADR-0005](0005-own-templates-per-backend-and-plugin.md))
stops affecting anything outside the plugin that wrote it.

The shared namespace costs collisions on names a plugin writes
itself — a small class, caught at Build with both plugin names in the
message. That is a fair price for composition, correct imports,
cross-plugin dispatch and overrides.

### Namespace by language rather than by plugin

`go.promotedMethods`, registered once.

Rejected as the prefix problem in a smaller costume. The backend
already answers for exactly one language per run
([ADR-0006](0006-one-backend-per-pipeline-run.md)), so the qualifier
distinguishes nothing at the point of use and every template pays for
it.

## Consequences

### Positive

- A helper has one name, the one its declaration gives it. Templates
  are portable between plugins, and documentation can name a helper
  without qualifying which plugin is asking.
- One copy of the vocabulary per run instead of one per plugin.
- A plugin's registered funcmap is only what the plugin wrote, so
  reading it tells you what the plugin adds.

### Negative

- Two plugins that both write a helper called `render` still collide,
  and now nothing hides it. That is the intended behaviour and the
  reason to name a helper for what it does in the plugin's terms, but
  it is a real constraint that the prefix used to absorb.
- The overrideable bucket is much larger, so a plugin's override can
  now silently replace a name it did not expect to be there. The
  conformance suite mirrors the overrideable set for exactly this
  reason, and that mirror grew from 20 names to 132.

### Neutral

- [ADR-0005](0005-own-templates-per-backend-and-plugin.md) is not
  superseded. Its decision — each backend owns its core templates,
  each plugin ships its own, merged into a shared funcmap — is
  unchanged and is what makes this possible. What changes is the
  mitigation its Negative section names, which is a consequence rather
  than the decision.
- Nothing here is Go-specific. A second language's backend registers
  its own vocabulary the same way, which is what
  [ADR-0007](0007-group-language-support-by-language.md) puts in one
  place per language.
