---
adr: 0002
title: Compile plugins into the binary as static imports
status: Accepted
date: 2026-08-11
supersedes: none
superseded-by: none
---

# ADR-0002: Compile plugins into the binary as static imports

## Status

Accepted.

## Context

A run is assembled from a `[]plugin.Plugin` handed to `cli/build.go`
and registered into `pipeline.Builder`. Three forces constrain how
plugins may arrive.

The plugin contract is Go interfaces carrying generics. A typed
`meta.Key[T]` is what stops a generator reading a bool out of a slot an
annotator wrote a string into, and that guarantee exists only
in-process.

The contract passes a mutable graph. Annotators write into a `Bag`
hanging off nodes the frontend built; generators read that same graph
and emit values that other plugins append into.

Output is committed source. A run that differs between two machines is
a defect rather than an inconvenience, so plugin identity and ordering
must be fixed at build time.

## Decision

We will require plugins to be compiled into the generator binary as
ordinary Go imports. A different pipeline is a different binary.

## Alternatives Considered

### Go's native `plugin` package (`-buildmode=plugin`)

Load `.so` files at startup. Rejected: it requires the host and every
plugin to be built with an identical toolchain, identical build flags,
and identical versions of every shared dependency — a constraint
strictly harder to satisfy than rebuilding. It has no Windows support,
and nothing loaded can be unloaded.

### Subprocess plugins over RPC (`hashicorp/go-plugin`)

Mature, proven at scale in Terraform, and it buys process isolation and
language independence. Rejected on the shape of our contract rather
than on its quality: plugins mutate a shared graph, so each call would
serialise the working set across the boundary, and typed metadata keys
degrade to strings on the wire — discarding the type parameter that is
the mechanism's main safety property (see [ADR-0003](0003-metadata-as-the-extension-mechanism.md)).

### WASM plugins

Carries the same serialisation boundary as RPC, since a node graph
cannot cross the host boundary by reference, and adds a less mature Go
host story. The isolation it buys is not a property we need: plugins
are trusted code chosen by whoever builds the binary.

### Run-time module resolution

Declare plugins in config and fetch their modules on demand. Rejected
because it makes generated output depend on network state at run time,
contradicting the reproducibility the tool exists to provide.

## Consequences

### Positive

- Plugin contracts are checked by the compiler, including the generic
  metadata keys that carry the type safety.
- The plugin set and its ordering are fixed at build time, so two runs
  of the same binary over the same source produce the same output.
- Deployment is one binary, with no loader path, version manifest, or
  subprocess lifecycle to operate.

### Negative

- Changing the plugin set requires a Go toolchain and a rebuild, which
  shuts out users who do not write Go — a real population for a code
  generator.
- A third-party plugin cannot be adopted without building a bespoke
  generator, so there is no ecosystem of drop-in plugins and there will
  not be one under this decision.
- The binary carries every plugin anyone might enable, so its size
  grows with the union of everyone's needs rather than with any one
  user's.

### Neutral

- `filterEnabledPlugins` selects at run time from the compiled set, so
  "rebuild" is the cost of changing which plugins *exist*, not of
  changing which of them run for a given project.
