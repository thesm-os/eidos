// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package writer is the language-agnostic text-emission layer
// backends compose to assemble per-file output. It exposes:
//
//   - [ImportSet]: per-file import deduplication, deterministic
//     alias resolution, and the `imp` registration API the backend's
//     template func-map exposes to template authors.
//   - [Writer]: per-output-file buffer that bundles an ImportSet
//     with the rendered body and finalises the bytes a sink
//     receives.
//   - [ExtractProvenance] / [IsProvenanceAtTail]: readers for the
//     provenance trailer a backend stamps at the end of every
//     generated file, used by the disk sink to decide whether an
//     on-disk file still matches what the pipeline produced.
//
// The writer is intentionally language-agnostic: alias derivation
// defaults to the last "/"-delimited path segment (Go convention)
// but accepts a custom derivation function for other targets.
// Formatting, template engines, and language-specific naming live
// in the backend that wraps the writer.
package writer
