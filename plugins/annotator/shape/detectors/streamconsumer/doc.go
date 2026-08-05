// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package streamconsumer detects callables that consume a stream —
// a context, one interface-typed parameter, and a value plus error
// out:
//
//	func (l *Loader) Load(ctx context.Context, r io.Reader) (int, error)
//	func (d *Decoder) Decode(ctx context.Context, r io.Reader) (Document, error)
//
// It is the inverse of `streamreader`, which recognises callables
// that *return* an `iter.Seq`. Nothing else in the catalog covers
// consumption.
//
// Without this shape such a callable falls to `reader` and its
// stream is stamped as `shape.key_type`. That is not merely
// imprecise: a key promises same-key-same-value, read-after-write
// and deterministic re-read, and a drained stream satisfies all
// three vacuously, returning the zero value and no error. Generated
// assertions then pass without testing anything.
//
// The consumed type is recorded under `shape.stream_type` rather
// than `shape.key_type`, so a generator branching on key_type never
// sees it.
//
// # Frontend dependency
//
// Detection of a *named* interface parameter requires the Go
// frontend's `go.isInterface` stamp; the node IR alone cannot tell
// `io.Reader` from `time.Duration`. Inline `interface{...}`
// parameters are decidable without it. A frontend that does not
// stamp the key detects only the inline form — the detector is
// registered per language, so other frontends supply their own
// entry rather than inheriting this one's assumptions.
package streamconsumer
