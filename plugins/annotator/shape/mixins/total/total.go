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

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []string{ParamDomain}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Params: Params}
}
