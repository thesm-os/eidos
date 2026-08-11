// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sideeffect

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "sideeffect"

// ParamObserve is the KV key naming the callable whose result changes across this one's call.
//
// Named because a consumer reaching for it otherwise writes
// `Params[0]`, which is a position rather than a key: reordering
// the list — or adding a second parameter ahead of this one —
// silently changes what every such call site reads.
const ParamObserve = "observe"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []string{ParamObserve}

// SiblingParams enumerates the param keys whose values name
// sibling callables the resolver rewrites into qualified names.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var SiblingParams = []string{ParamObserve}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{
		Name:          Name,
		Params:        Params,
		SiblingParams: SiblingParams,
	}
}
