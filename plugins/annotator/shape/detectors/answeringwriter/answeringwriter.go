// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package answeringwriter

import (
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical shape name this package stamps.
const Name = "answeringwriter"

// Priority places this detector above [reader] and below
// [pointerreader].
//
// Above reader because every signature this draws is one reader would
// otherwise claim, recording the written value as a key. Below
// pointerreader because that shape returns a bare pointer with no
// error and cannot collide. Nothing between the two is disturbed.
const Priority = 440

// Detector returns the [shape.Detector] this package contributes.
func Detector() shape.Detector {
	return shape.Detector{
		Name:     Name,
		Priority: Priority,
		Detect: map[string]shape.DetectFunc{
			golang.Language: detectGolang,
		},
	}
}

// detectGolang recognises one non-context parameter of a
// package-qualified type, answered by that same type beside an error.
//
// Type identity alone is too wide, and the corpus says so: a
// `Get(ctx, key string) (string, error)` cache, a codec's
// `Encode`/`Decode`, a classifier, a loader — every one has its
// parameter type equal to its first result, and every one is a read or
// a transformation rather than a write answering what was stored.
// Twenty-one fixtures reclassified before this guard existed.
//
// The qualifier is what separates them. A predeclared type carries no
// package, so `string` fails here while `x.Value` passes; the answered
// stored state this detector exists for is a declared type carrying
// identity and stamps, never a bare builtin. A struct-keyed read
// returning its own key type would still be claimed, and that shape is
// genuinely ambiguous from the signature alone.
//
// Both halves are needed. Dropping identity would take every reader,
// which is the failure the [writer] detector's own comment records
// from the other direction: a permissive rule at a higher priority
// claimed reads and labelled the key as the written value.
func detectGolang(n sdk.Node) (shape.Match, bool) {
	params, returns := golang.Callable(n)
	if !golang.HasError(returns) {
		return shape.Match{}, false
	}
	values := golang.StripContext(params)
	results := golang.StripErrorTypes(returns)
	if len(values) != 1 || len(results) != 1 {
		return shape.Match{}, false
	}
	param := values[0].Type
	if param == nil || param.Package == "" {
		return shape.Match{}, false
	}
	written := golang.QName(param)
	if written != golang.QName(results[0]) {
		return shape.Match{}, false
	}
	return shape.Match{ValueType: written}, true
}
