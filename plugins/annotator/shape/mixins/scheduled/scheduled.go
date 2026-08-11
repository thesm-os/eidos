// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package scheduled

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "scheduled"

// ParamSchedule is the KV key naming the callable that registers a
// task to run at a future offset.
const ParamSchedule = "schedule"

// ParamFired is the KV key naming the accessor reporting how many
// scheduled tasks have run.
//
// The law compares it across a clock advance, so without it there is
// nothing to observe: a suite that cannot count firings reports every
// scheduler as correct, including one that never fires at all.
const ParamFired = "fired"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []string{ParamSchedule, ParamFired}

// SiblingParams enumerates the param keys whose values name sibling
// callables the resolver rewrites into qualified names.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var SiblingParams = []string{ParamSchedule, ParamFired}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{
		Name:          Name,
		Params:        Params,
		SiblingParams: SiblingParams,
	}
}
