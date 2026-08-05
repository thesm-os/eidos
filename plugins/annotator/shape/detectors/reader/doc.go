// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package reader recognises the canonical reader shape — a
// callable that takes a key and returns a value plus an error.
//
// The recognised Go signature is:
//
//	func[T any](ctx context.Context, key K) (V, error)
//	func[T any](key K) (V, error)              // ctx-optional
//	func (r *Repo) Get(ctx, key K) (V, error)  // method form
//
// A positive detection stamps the standard three keys on the
// callable's meta bag (via the umbrella shape plugin):
//
//	shape            = "reader"
//	shape.key_type   = qualified type of `key`
//	shape.value_type = qualified type of `V`
//
// Consumers register the [Detector] alongside other shape
// detectors when constructing the umbrella plugin:
//
//	pipe.Use(shape.New().Detectors(reader.Detector()))
//
// Reader is the canonical shape for the "one key in, one value +
// error out" pattern. Dispatch is by [shape.Detector.Priority]
// rather than registration order, and reader sits low in the
// ladder (420) so that more specific shapes sharing its signature —
// `lookup`, `readerwithbool`, `batchreader` — claim their cases
// first.
//
// Ordering is not what keeps reader distinct from `writer`. The two
// predicates are disjoint: writer takes `error` as its only return,
// reader requires exactly one value alongside it. That was not
// always so, and the overlap made reader unreachable; see
// `detectors.TestAll_Reachability` for the guard that pins it.
package reader
