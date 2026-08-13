// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package orderafter

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "orderafter"

// ParamFn is the KV key naming the callable this one must run after.
//
// Named because a consumer reaching for it otherwise writes
// `Params[0]`, which is a position rather than a key: reordering
// the list — or adding a second parameter ahead of this one —
// silently changes what every such call site reads.
const ParamFn = "fn"

// ParamUnready is the KV key naming the error the callable reports
// when invoked before its sibling has run.
//
// Without it "calling early fails" is asserted with a bare non-nil
// check, which an implementation failing early for an unrelated reason
// — a nil map, a refused connection — passes as ordering enforcement.
//
// A sentinel is a package-level var, so it resolves through the var
// scope rather than the callable one — see [shape.KindVar].
const ParamUnready = "unready"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamFn, Kind: shape.KindCallable},
	{Key: ParamUnready, Kind: shape.KindVar},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{
		Name:   Name,
		Params: Params,
	}
}
