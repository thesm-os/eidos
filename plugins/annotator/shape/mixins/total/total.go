// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package total

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "total"

// ParamDomain is the KV key naming the input set the callable is total over.
//
// Named because a consumer reaching for it otherwise writes
// `Params[0]`, which is a position rather than a key.
const ParamDomain = "domain"

// ParamEdge is the KV key naming a declared input at the domain's
// boundary.
//
// The law is that the callable answers for every input in the named
// domain, and the input worth testing is the edge — which
// [ParamDomain], being prose for a reader, cannot produce. A derived
// sample sits comfortably inside the domain, so a subject that fails
// at the boundary passes on it.
//
// A package-level var, resolved through the var scope and stamped
// qualified — see [shape.KindVar]. Optional on the terms the
// validates mixin's `invalid=` is: a total callable without one is
// still what the mixin names, and a law handing the edge to the call
// simply does not bind.
const ParamEdge = "edge"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamDomain, Kind: shape.KindOpaque},
	{Key: ParamEdge, Kind: shape.KindVar, Counterexample: true},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Params: Params}
}
