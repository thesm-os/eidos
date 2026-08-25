// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package detectors is the convenience aggregator for every
// [shape.Detector] shipped under
// `plugins/annotator/shape/detectors/...`.
//
// One axis of three. [All] is every detector eidos ships and nothing
// else — no contracts or mixins:
//
//	pipe.WithAnnotator(shape.New().Detectors(detectors.All()...))
//
// Reach for `shape/full` where the whole vocabulary is wanted. A
// pipeline registering this axis alone stamps nothing for the other
// two, and what a consumer sees is a callable with no contract or mixin
// stamps — indistinguishable from one that legitimately has none.
//
// Consumers wanting a curated subset import the per-detector
// sub-packages directly and pick what they need.
package detectors

import (
	"slices"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/aggregator"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/answeringwriter"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/batchreader"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/closer"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/compositewriter"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/deleter"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/lifecycle"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/lookup"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/multiaggregator"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/multiargwriter"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/multireader"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/mutator"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/pointerreader"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/poisonaccessor"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/predicate"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/pure"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/reader"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/readernoerror"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/readerwithbool"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/streamconsumer"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/streamreader"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/voidlifecycle"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors/writer"
)

// All returns every [shape.Detector] shipped in this repository,
// alphabetised by [shape.Detector.Name]. The returned slice is
// freshly allocated on each call; callers may mutate it without
// affecting future invocations.
//
// Note: the umbrella shape plugin dispatches detectors by
// [shape.Detector.Priority] (higher first), not slice position —
// alphabetical ordering here is for stable diagnostics and
// test-output readability, not for runtime semantics.
func All() []shape.Detector {
	out := []shape.Detector{
		aggregator.Detector(),
		answeringwriter.Detector(),
		batchreader.Detector(),
		closer.Detector(),
		compositewriter.Detector(),
		deleter.Detector(),
		lifecycle.Detector(),
		lookup.Detector(),
		multiaggregator.Detector(),
		multiargwriter.Detector(),
		multireader.Detector(),
		mutator.Detector(),
		pointerreader.Detector(),
		poisonaccessor.Detector(),
		predicate.Detector(),
		pure.Detector(),
		reader.Detector(),
		readernoerror.Detector(),
		readerwithbool.Detector(),
		streamconsumer.Detector(),
		streamreader.Detector(),
		voidlifecycle.Detector(),
		writer.Detector(),
	}
	slices.SortFunc(out, func(a, b shape.Detector) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return out
}
