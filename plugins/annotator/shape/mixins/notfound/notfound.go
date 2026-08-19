// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package notfound

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "notfound"

// ParamSentinel is the KV key naming the error a draw of an absent
// key reports.
//
// A sentinel is a package-level var, so it resolves through the var
// scope rather than the callable one — see [shape.KindVar]. Spelled
// `sentinel` after [deleteremoves], not `notfound`, which would put
// the mixin's own name on both sides of the `=`.
const ParamSentinel = "sentinel"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamSentinel, Kind: shape.KindVar},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{
		Name:     Name,
		Params:   Params,
		Validate: validate,
	}
}

// validate requires the sentinel, because it is the mixin's entire
// content.
//
// Unlike [deleteremoves], whose bare form still classifies the delete,
// a bare `notfound` classifies nothing: the callable was already a
// reader by structure, and "a miss is an error of unspecified
// identity" can only lower to a check that some error came back —
// which passes against every implementation, including one reporting
// the wrong error. Refusing it here names the offending line; refusing
// it at derivation names a missing check nobody asked about.
func validate(attachments []shape.MixinAttachment) []shape.MixinViolation {
	var out []shape.MixinViolation
	for _, attached := range attachments {
		if attached.Params[ParamSentinel] != "" {
			continue
		}
		out = append(out, shape.MixinViolation{
			Host: attached.Host,
			Message: "notfound requires " + ParamSentinel +
				"=, naming the error a draw of an absent key reports",
		})
	}
	return out
}
