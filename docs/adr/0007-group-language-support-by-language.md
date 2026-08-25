---
adr: 0007
title: Group language support by language, one module apiece
status: Accepted
date: 2026-08-25
supersedes: none
superseded-by: none
---

# ADR-0007: Group language support by language, one module apiece

## Status

Accepted.

## Context

Everything eidos knew about Go was spread across four trees that shared
nothing but a name: `lang/golang` for conventions, `frontend/golang`
for parsing, `backend/golang` for rendering, `sdk/golang` for the
plugin base. The pipeline *phase* was the primary axis and the language
was scattered along it.

That reads well with one language. It stops scaling at the second: a
reader asking "what does eidos know about Go?" had four places to look,
and an author adding Rust had four trees to create, none of which sat
near each other.

Three of the four were also separate Go modules, while the conventions
lived in the root module. So the module every consumer takes carried
Go-specific code, and a consumer wanting only another language would
still compile Go's conventions.

The framework was already prepared for a second language in every other
respect. Detection dispatches per frontend marker
(`Detect map[string]DetectFunc`), query helpers are per-language files
(`helpers_go.go`, whose docblock invites `helpers_<frontend>.go`
beside it), and a generator plugin already binds its language in
`<plugin>_<lang>.go` with templates under `templates/<lang>/`. The
layout was the part that had not caught up.

## Decision

We will group language support by language: `lang/<lang>/` holds a
language's conventions at its root and its adapters beneath —
`frontend/`, `backend/`, `sdk/` — as **one module per language**.

A bridge between two languages stays outside `lang/`, because it
belongs to a pair rather than to either one. `bridge/protogo` is the
existing instance.

## Alternatives Considered

### Keep the phase-primary layout, add `frontend/rust` and `backend/rust`

The cheapest option: no moves, no module churn, no abandoned published
paths.

Rejected because it makes the problem worse at exactly the rate the
project grows. Each language adds a directory to every phase tree, so
the number of places to look when reasoning about one language is the
number of phases — and the trees that must stay in sync are the ones
furthest apart on disk.

### One module per package, keeping the adapters split

`lang/golang`, `lang/golang/frontend`, `lang/golang/backend` and
`lang/golang/sdk` as four modules under one directory. This preserves
the ability to depend on the parser without the renderer, which two
modules genuinely exercised: `bridge/protogo` reads Go and never
renders it, `reference` renders Go and never reads it.

Rejected on cost per unit of benefit. The split bought a smaller
dependency graph for those two, and charged a `replace` directive per
package in every dependent `go.mod` — four of them, maintained by hand,
which is where a stale `replace` had already accumulated. The measured
benefit did not survive contact: `plugins`, `reference` and `eidostest`
import the conventions rather than the adapters, so Go only propagates
what they actually reach and all three still carry zero third-party
dependencies with the modules merged.

### One module for the whole `lang/` tree

Every language in one module. Fewest `go.mod` files.

Rejected because it hands every consumer every language's dependencies:
`golang.org/x/tools` for Go, `protocompile` and `google.golang.org/protobuf`
for proto, whatever Rust support would need. A consumer generating only
Go would resolve the proto toolchain to build.

### Move the adapters but leave the conventions in the root module

The smallest change that groups the trees.

Rejected because it leaves the original defect in place: the root
module — the one every consumer takes — keeps Go-specific code, so
adding Rust means the root carries two languages' conventions.

## Consequences

### Positive

- One place to look, and one to create. Adding a language is a new
  `lang/<lang>/` and nothing else; the framework needs no change.
- The root module carries no language. It has zero requirements and
  zero third-party dependencies.
- Packages take their directories' names, so `frontend.New()` and
  `backend.New()` replace four packages all called `golang` and the
  alias soup that came with them.
- One coverage and mutation entry per language rather than one per
  adapter.

### Negative

- The published module paths `…/frontend/golang`, `…/backend/golang`
  and `…/frontend/protobuf` are abandoned at their final tags. A Go
  module path change is a new module: the old paths keep their history
  and receive nothing further, and every consumer edits imports once.
- Two packages are now named `frontend` — the Go one and the proto
  one — so a file registering both must alias them.
- A language whose adapters have genuinely divergent dependencies pays
  for all of them in one module. Proto's frontend brings
  `protocompile` to anything importing `lang/protobuf`.

### Neutral

- The per-language extension points do not change. Detection still
  dispatches on the frontend marker, helpers are still
  `helpers_<frontend>.go`, and generator templates still live under
  `templates/<lang>/`. This decision moved directories to match a
  convention the code already had.
- Documentation mirrors the code: `docs/lang/<lang>/`.
