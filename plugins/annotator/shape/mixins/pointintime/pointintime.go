// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pointintime

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "pointintime"

// ParamWrite is the KV key naming the callable whose landing the two
// reads straddle.
//
// The law is that two reads agree even with a write between them, so
// witnessing it takes that write — and until this key existed the
// directive had no way to say which method it is. A subject carrying
// the classification and nothing else states the property and leaves
// every derivation to report it unclaimed, which is honest and
// permanent: no derivation can find the partner, because which write
// matters is a choice only the author knows.
//
// Optional, on the terms `pool`'s `stats` role is: a read answering a
// consistent snapshot is what the mixin names whether or not the
// author points at the write, and a law reading the partner simply
// does not bind without one. Requiring it would retire the
// classification for every subject already carrying it.
const ParamWrite = "write"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamWrite, Kind: shape.KindCallable},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Params: Params}
}
