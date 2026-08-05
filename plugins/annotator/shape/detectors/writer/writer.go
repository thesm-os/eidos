// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package writer

import (
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
)

// Name is the canonical shape name this detector stamps. Consumers
// switching on [shape.Get] compare against this constant rather
// than the literal string so renames surface as compile errors.
const Name = "writer"

// Detector returns the [shape.Detector] this package contributes
// to the umbrella shape plugin. Register one instance per
// pipeline:
//
//	pipe.Use(shape.New().Detectors(writer.Detector()))
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: 500,
		Detect: map[string]shape.DetectFunc{
			"golang": detectGolang,
		},
	}
}

// detectGolang recognises the canonical Go writer signature:
// exactly one non-context input parameter and `error` as the only
// return. The leading `context.Context` parameter is optional.
//
// The non-error return count must be zero, not "at most one". An
// earlier form accepted `<= 1`, which made every signature [reader]
// recognises — one input, one value, one error — a strict subset of
// this one. Running first at the higher priority, this detector
// claimed all of them, and reader never won a dispatch. The harm
// was not the dead rule but the stamp that replaced it: this
// detector records its *parameter* as ValueType, so a read like
// `Get(ctx, id string) (Document, error)` was labelled a writer of
// `string` and the Document went unrecorded.
//
// A write that returns a receipt (`Insert(ctx, Doc) (ID, error)`)
// consequently falls to reader rather than here. That shape is
// genuinely neither and wants a detector of its own; recording both
// its types under the wrong label is recoverable, which the
// previous behaviour was not.
func detectGolang(n node.Node) (shape.Match, bool) {
	params, returns := shape.GoCallable(n)
	if !shape.GoHasError(returns) {
		return shape.Match{}, false
	}
	values := shape.GoStripContext(params)
	results := shape.GoStripError(returns)
	if len(values) != 1 || len(results) != 0 {
		return shape.Match{}, false
	}
	return shape.Match{
		ValueType: shape.QName(values[0].Type),
	}, true
}
