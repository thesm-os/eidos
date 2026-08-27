// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sticky

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "sticky"

// ParamKey is the KV key naming the parameter whose value pins the instance.
//
// Named because a consumer reaching for it otherwise writes
// `Params[0]`, which is a position rather than a key.
//
// [shape.KindParam]: the resolver checks the name against the host's
// own signature, which is the check `partition`'s axis carried in a
// hand-rolled Validate hook while this key carried nothing. A name
// the signature does not declare is reported where the author is;
// the stamp stays as written, since a parameter has no qualified
// form.
const ParamKey = "key"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamKey, Kind: shape.KindParam},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Params: Params}
}
