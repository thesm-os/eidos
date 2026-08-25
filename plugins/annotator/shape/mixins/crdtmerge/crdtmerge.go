// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package crdtmerge

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "crdtmerge"

// ParamWrite is the KV key naming the callable that adds an element
// to one replica, which is how a check makes two replicas diverge
// before merging them.
//
// The host is the merge itself, so without this the check has no way
// to create the conflict the claim is about.
const ParamWrite = "write"

// ParamRead is the KV key naming the callable that reads the merged
// state, which is what determinism is asserted about.
const ParamRead = "read"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamWrite, Kind: shape.KindCallable},
	{Key: ParamRead, Kind: shape.KindCallable},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Params: Params}
}
