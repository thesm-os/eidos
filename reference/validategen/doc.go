// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package validategen is the ensemble's hybrid: the one reference
// plugin that BOTH owns a file and contributes into another plugin's.
//
// It emits a `Validate(*<Type>) error` function into its own
// `_validate.go` output, then appends a call to that function into the
// "prebody" slot of the matching
// [go.thesmos.sh/eidos/reference/handlergen] handler. Two emit kinds
// carry the two halves: `validategen.validator` for the function it
// owns, `validategen.entry` for the call it contributes. Each ships its
// own template.
//
// # Why this one exists
//
// Owning an output and contributing into someone else's file at the
// same time creates a problem the pure contributors never hit: the
// contributed call has to name the validator, but the package the
// validator lands in is not known when the entry is built. Layout
// assigns output packages after generation.
//
// [Entry.SetOutputPackages] is the resolution. The framework calls it
// on every emit value implementing [emit.OutputPackageSetter] once
// packages are assigned, passing tag → package path; the entry retains
// its function name so it can rebuild the qualified reference against
// the real package at that point. Building the reference eagerly would
// hard-code whatever package the plugin guessed, and the guess is wrong
// whenever a consumer routes the output elsewhere with `+gen:out`.
//
// This is the pattern any self-referencing multi-output plugin needs,
// which is why it is demonstrated here rather than described.
//
// # Properties, each deliberate
//
//   - **It declares [sdk.FilenameProvider], unlike the pure
//     contributors.** It owns a file, so it says where that file lands.
//     The contributed entry needs no output of its own — it renders
//     inside handlergen's file.
//   - **No [sdk.NodesOnly].** It reads the emit graph to find the
//     handlers. Declaring otherwise would license the pipeline to run
//     it concurrently with the generator writing that graph.
//   - **It runs in [sdk.GeneratorComposition].** After handlergen's
//     foundation bucket, so the handler exists to contribute to; before
//     the cross-cutting and finalize contributors, so validation
//     precedes them in the rendered prebody.
//   - **The contributed call degrades to nothing without handlergen.**
//     The entry is appended to the host's emit value rather than routed
//     by origin, so a run without the host contributes nothing. The
//     `_validate.go` output still lands — it is this plugin's own, and
//     depends on no one.
//   - **TemplateFuncs returns nil.** The shared Go helpers are already
//     in the backend's overrideable funcmap; contributing them again is
//     a Build-time collision.
package validategen
