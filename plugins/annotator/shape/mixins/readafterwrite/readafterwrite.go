// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package readafterwrite

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "readafterwrite"

// ParamWrite is the KV key naming the write a subsequent read is guaranteed to see.
//
// Named because a consumer reaching for it otherwise writes
// `Params[0]`, which is a position rather than a key: reordering
// the list — or adding a second parameter ahead of this one —
// silently changes what every such call site reads.
const ParamWrite = "write"

// Params enumerates the KV parameter names this mixin accepts.
// Exported so downstream consumers can read the canonical
// parameter list without importing the [shape.Mixin] value.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []string{ParamWrite}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Params: Params}
}
