// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package writer recognises the canonical writer shape — a
// callable that takes a value and reports success via an error.
//
// The recognised Go signatures are:
//
//	func (r *Repo) Save(ctx context.Context, v V) error
//	func (r *Repo) Save(v V) error                      // ctx-optional
//	func (r *Repo) Save(ctx context.Context, v V) (R, error)
//	func (r *Repo) Save(v V) (R, error)                 // with-result
//
// A positive detection stamps the standard meta triple on the
// callable's bag (via the umbrella shape plugin):
//
//	shape            = "writer"
//	shape.value_type = qualified type of `v`
//
// Consumers register the [Detector] alongside other shape
// detectors when constructing the umbrella plugin:
//
//	pipe.WithAnnotator(shape.New().Detectors(writer.Detector()))
//
// Detector ordering matters when multiple shapes share a
// signature ([shape.New] honours registration order); writer is
// the canonical fallback for the "value in, error out" pattern
// and typically registers after more specific detectors
// (Deleter, CompareAndSwap, Persister) that would otherwise be
// shadowed.
package writer
