// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package codec recognises the codec contract — a pair of callables
// where one undoes the other.
//
// The `forward` role is the encoding direction and the `inverse` role
// is the decoding one, so a suite can assert `inverse(forward(x)) == x`
// over generated inputs. The inverse is required: a forward with none
// declared states a property with no way to check it, which is the one
// outcome worth failing at the directive rather than discovering in a
// suite that quietly asserts nothing.
//
// `fidelity` says which equality the pair claims. The default `exact`
// is the round-trip above. `lossy` weakens it to
// `forward(inverse(forward(x))) == forward(x)` — idempotence after the
// first pass — which is the honest claim for an encoding that
// normalises, truncates or drops unrepresentable input. Stating
// `exact` for one of those produces a check that fails on correct
// code, so the parameter exists to be declared rather than inferred.
//
// The recognised directives are:
//
//	//+gen:contract codec role=forward inverse=Decode
//	//+gen:contract codec role=forward inverse=Decode fidelity=lossy
package codec
