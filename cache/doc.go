// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package cache is the content-addressed store used to memoise work
// between runs. A cache key is the hash of every input that
// determined the cached value, so a hit is equivalent to a fresh
// recompute.
//
// Two consumers exist today, and they differ in what a hit buys:
//
//   - Frontends store their converted node graph under a key derived
//     from the source inputs, and skip conversion entirely on a hit.
//     This is the only place a hit avoids work.
//   - The pipeline records a per-plugin fingerprint over the reads,
//     routing and scope a plugin observed. Nothing consults it to
//     skip a phase; skipping a generator would mean reconstructing
//     its emit contributions, and the emit graph is a live object
//     graph with owner and slot back-pointers rather than a byte
//     payload. Treat the entry as observability.
//
// The package exposes the [Cache] interface and two implementations:
//
//   - [None] is a no-op cache (every Get is a miss; Put is a no-op).
//     Use for hermetic CI runs or to disable caching entirely.
//   - [Disk] is a filesystem-backed cache rooted at a configurable
//     directory. Entries are immutable and written atomically via
//     temp+rename so concurrent writers cannot observe a partial
//     file.
//
// Entries never change in place — different inputs produce different
// keys, so the cache grows monotonically. Periodic eviction by
// retention policy lives at a higher layer (the pipeline run loop).
package cache
