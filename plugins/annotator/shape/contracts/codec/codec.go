// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package codec

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "codec"

// RoleForward is the encoding direction — the callable a round-trip
// starts from.
const RoleForward = "forward"

// RoleInverse is the decoding direction — the callable that undoes
// [RoleForward].
const RoleInverse = "inverse"

// ParamFidelity declares which equality the pair claims.
//
// Opaque to the resolver: the value names neither a callable nor a
// parameter, only which of two laws applies.
const ParamFidelity = "fidelity"

// FidelityExact claims `inverse(forward(x)) == x`. The default when
// [ParamFidelity] is absent.
const FidelityExact = "exact"

// FidelityLossy claims `forward(inverse(forward(x))) == forward(x)`,
// which is the strongest true statement about an encoding that
// normalises or discards input.
const FidelityLossy = "lossy"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleForward, RoleInverse}

// Params enumerates the directive's opaque KV keys.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []shape.Param{
	{Key: ParamFidelity, Kind: shape.KindOpaque},
}

// Required pins the inverse to the forward.
//
// A forward naming no inverse is the shape this contract exists to
// rule out: the property is unstatable without both halves, so the
// omission fails at the directive rather than producing a suite that
// asserts nothing. The reverse is not required — an inverse may be
// declared on its own and back-stamped by the forward that names it.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Required = map[string][]string{RoleForward: {RoleInverse}}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{
		Name:     Name,
		Roles:    Roles,
		Params:   Params,
		Required: Required,
	}
}
