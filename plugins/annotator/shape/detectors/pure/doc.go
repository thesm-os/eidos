// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package pure recognises the canonical pure shape — a callable
// that computes a single value from its inputs without an error
// return or context dependency.
//
// The recognised Go signature is:
//
//	func Sum(xs ...int) int
//	func Add(a, b int) int
//	func Hash(s string) string
//
// A positive detection stamps the structural shape and the
// computed value type on the callable's meta bag (via the
// umbrella shape plugin):
//
//	shape            = "pure"
//	shape.value_type = qualified type of the single return
//
// No key type is stamped — pure callables may take many
// parameters or none, and any "key" interpretation is
// caller-specific.
//
// Consumers register the [Detector] alongside other shape
// detectors when constructing the umbrella plugin:
//
//	pipe.WithAnnotator(shape.New().Detectors(pure.Detector()))
//
// Detector ordering matters when multiple shapes share a
// signature ([shape.New] honours registration order); pure is
// permissive (any single-value-no-error callable matches) so it
// typically registers late — after more specific single-return
// detectors (ReaderNoError, PointerReader, Aggregator) that
// constrain the signature further.
package pure
