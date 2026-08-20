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
var Params = []shape.Param{
	{Key: ParamRead, Kind: shape.KindCallable},
	{Key: ParamAxis, Kind: shape.KindOpaque},
}

// Mixin returns the [shape.Mixin] this package contributes.
//
// The validator checks [ParamAxis] against the host's own parameter
// list, which is the half the resolver cannot do. Absence is not a
// violation: the bare form remains a classification, and a consumer
// that cannot build a sound check without an axis declines to build
// one — that judgement belongs to the generator rather than here.
func Mixin() shape.Mixin {
	return shape.Mixin{
		Name:     Name,
		Params:   Params,
		Validate: validateAxis,
	}
}

// validateAxis reports an axis that does not name a parameter on both
// halves of the pair.
//
// A misspelled axis is the first failure worth catching. It stamps
// like any other value, so without this the generated check varies
// nothing and passes against every implementation — the silent shape
// this mixin's axis exists to prevent.
//
// The second is the correspondence. A check writes through the host
// and reads through the partner, so it has to carry one partition
// value across both calls, and the axis names a parameter of the host
// only. Requiring the partner to spell it identically is what lets a
// generator match the two by name without guessing: the invariant is
// checked here rather than assumed there.
//
// Only reachable for a method, whose Owner carries the sibling list. A
// free function records a package path rather than a node, so the
// partner cannot be reached from the attachment and the pair goes
// unchecked rather than falsely reported.
func validateAxis(attachments []shape.MixinAttachment) []shape.MixinViolation {
	var out []shape.MixinViolation
	for _, attached := range attachments {
		axis, given := attached.Params[ParamAxis]
		if !given || axis == "" {
			continue
		}
		hostParams, _ := shape.GoCallable(attached.Host)
		if !hasParam(hostParams, axis) {
			out = append(out, shape.MixinViolation{
				Host: attached.Host,
				Message: fmt.Sprintf(
					"%s=%q names no parameter of the annotated callable", ParamAxis, axis,
				),
			})
			continue
		}
		read := attached.Params[ParamRead]
		partnerParams, found := partnerParams(attached.Host, read)
		if !found || hasParam(partnerParams, axis) {
			continue
		}
		out = append(out, shape.MixinViolation{
			Host: attached.Host,
			Message: fmt.Sprintf(
				"%s=%q names no parameter of %s=%q, so a check cannot carry one partition across the pair",
				ParamAxis, axis, ParamRead, read,
			),
		})
	}
	return out
}

// hasParam reports whether params carries one named name.
func hasParam(params []*sdk.Param, name string) bool {
	return slices.ContainsFunc(params, func(p *sdk.Param) bool {
		return p != nil && p.Name == name
	})
}

// partnerParams returns the parameter list of the callable qname
// names, and whether it could be reached from host at all.
//
// The false return is "not reachable", never "not found": a function
// host has no owner node to search, and reporting that as a missing
// partner would fail a directive that is correct.
func partnerParams(host sdk.Node, qname string) ([]*sdk.Param, bool) {
	if qname == "" {
		return nil, false
	}
	method, isMethod := host.(*sdk.Method)
	if !isMethod {
		return nil, false
	}
	var siblings []*sdk.Method
	var owner string
	switch declarer := method.Owner.(type) {
	case *sdk.Struct:
		siblings, owner = declarer.Methods, declarer.QName()
	case *sdk.Interface:
		siblings, owner = declarer.Methods, declarer.QName()
	default:
		return nil, false
	}
	if owner == "" {
		return nil, false
	}
	for _, sibling := range siblings {
		if sibling == nil || owner+"."+sibling.Name != qname {
			continue
		}
		params, _ := shape.GoCallable(sibling)
		return params, true
	}
	return nil, false
}
