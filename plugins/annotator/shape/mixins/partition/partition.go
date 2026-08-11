// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package partition

import (
	"fmt"
	"slices"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the canonical mixin name this package stamps.
const Name = "partition"

// ParamRead is the KV key naming the callable that reads a partition back.
//
// Named because a consumer reaching for it otherwise writes
// `Params[0]`, which is a position rather than a key: reordering
// the list — or adding a second parameter ahead of this one —
// silently changes what every such call site reads.
const ParamRead = "read"

// ParamAxis is the KV key naming the annotated callable's parameter
// that identifies the partition.
//
// The claim this mixin makes is that two writes under different
// partitions do not interfere, so a check has to vary one parameter
// and hold the rest fixed. Which one is not derivable: a callable
// taking `(ctx, partition, key, value)` offers two strings of the same
// type, and a check that varies both writes to two different keys —
// passing against an implementation that ignores partitions entirely.
// That check cannot fail, which is worse than the one this names.
//
// Unlike [ParamRead] the value is a parameter of the annotated
// callable rather than a callable in scope, so it is not resolved into
// a qualified name. It is validated instead: see [Mixin].
const ParamAxis = "axis"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []string{ParamRead, ParamAxis}

// SiblingParams enumerates the param keys whose values name
// sibling callables the resolver rewrites into qualified names.
//
// [ParamAxis] is deliberately absent: a parameter has no qualified
// name, and asking the resolver to look one up in scope would report
// every correct axis as not found.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var SiblingParams = []string{ParamRead}

// Mixin returns the [shape.Mixin] this package contributes.
//
// The validator checks [ParamAxis] against the host's own parameter
// list, which is the half the resolver cannot do. Absence is not a
// violation: the bare form remains a classification, and a consumer
// that cannot build a sound check without an axis declines to build
// one — that judgement belongs to the generator rather than here.
func Mixin() shape.Mixin {
	return shape.Mixin{
		Name:          Name,
		Params:        Params,
		SiblingParams: SiblingParams,
		Validate:      validateAxis,
	}
}

// validateAxis reports an axis naming no parameter of its host.
//
// A misspelled axis is the failure worth catching. It stamps like any
// other value, so without this the generated check varies nothing and
// passes against every implementation — the silent shape this mixin's
// axis exists to prevent.
func validateAxis(attachments []shape.MixinAttachment) []shape.MixinViolation {
	var out []shape.MixinViolation
	for _, attached := range attachments {
		axis, given := attached.Params[ParamAxis]
		if !given || axis == "" {
			continue
		}
		params, _ := shape.GoCallable(attached.Host)
		named := func(p *sdk.Param) bool { return p != nil && p.Name == axis }
		if slices.ContainsFunc(params, named) {
			continue
		}
		out = append(out, shape.MixinViolation{
			Host: attached.Host,
			Message: fmt.Sprintf(
				"%s=%q names no parameter of the annotated callable", ParamAxis, axis),
		})
	}
	return out
}
