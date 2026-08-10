// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package lifecycle recognises the canonical lifecycle shape — a
// callable that performs a side-effecting state transition,
// taking only a context and reporting success via an error.
//
// The recognised Go signature is:
//
//	func (s *Service) Start(ctx context.Context) error
//	func (s *Service) Stop(ctx context.Context) error
//
// A positive detection stamps the structural shape on the
// callable's meta bag (via the umbrella shape plugin):
//
//	shape = "lifecycle"
//
// No key or value type is stamped — lifecycle callables have
// neither.
//
// Consumers register the [Detector] alongside other shape
// detectors when constructing the umbrella plugin:
//
//	pipe.WithAnnotator(shape.New().Detectors(lifecycle.Detector()))
//
// Detector ordering matters when multiple shapes share a
// signature ([shape.New] honours registration order); the bare
// `(ctx) error` pattern is unambiguous, so lifecycle's position
// in the registration list usually does not matter.
package lifecycle
