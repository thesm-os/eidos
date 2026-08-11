// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package streamreflectsmutations

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "streamreflectsmutations"

// ParamMutate is the KV key naming the callable whose mutations the stream must observe.
//
// Named because a consumer reaching for it otherwise writes
// `Params[0]`, which is a position rather than a key: reordering
// the list — or adding a second parameter ahead of this one —
// silently changes what every such call site reads.
const ParamMutate = "mutate"

// ParamDelete is the KV key naming the callable that removes an entry
// mid-iteration.
//
// Distinct from [ParamMutate], which reaches the write half only: a
// stream reflecting an insert and not a removal is the commoner bug,
// and a law that never deletes cannot see it.
const ParamDelete = "delete"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []string{ParamMutate, ParamDelete}

// SiblingParams enumerates the param keys whose values name
// sibling callables the resolver rewrites into qualified names.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var SiblingParams = []string{ParamMutate, ParamDelete}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{
		Name:          Name,
		Params:        Params,
		SiblingParams: SiblingParams,
	}
}
