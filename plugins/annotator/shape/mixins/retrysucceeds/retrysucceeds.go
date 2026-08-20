// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package retrysucceeds

import (
	"fmt"
	"strconv"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
)

// Name is the canonical mixin name this package stamps.
const Name = "retrysucceeds"

// ParamAttempts is the KV key naming how many attempts the
// convergence claim covers: the first attempts-1 may fail, the last
// must succeed.
//
// The bound is part of what the author asserts, which is why it is a
// parameter here and a check's private choice on `accumulates` — an
// accumulation observed at any N proves the claim, while "converges"
// says nothing checkable until the author says by when. Opaque to the
// resolver: a quantity names neither a callable nor a var.
//
// Optional. The bare form still separates "considered and naturally
// retryable" from "nobody considered it", and a consumer that cannot
// state the law without a bound declines to state it.
const ParamAttempts = "attempts"

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []shape.Param{
	{Key: ParamAttempts, Kind: shape.KindOpaque},
}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{
		Name:     Name,
		Params:   Params,
		Validate: validate,
	}
}

// validate requires a declared bound to be an integer of at least 2.
//
// `attempts=1` is "succeeds on the first try" — the smoke check under
// a number, and wrong for a subject whose first attempts are meant to
// fail. A non-integer stamps like any other opaque value and would
// silently derive nothing, the misspelled-axis shape partition
// validates against.
func validate(attachments []shape.MixinAttachment) []shape.MixinViolation {
	var out []shape.MixinViolation
	for _, attached := range attachments {
		raw, given := attached.Params[ParamAttempts]
		if !given || raw == "" {
			continue
		}
		n, err := strconv.Atoi(raw)
		if err == nil && n >= 2 {
			continue
		}
		out = append(out, shape.MixinViolation{
			Host: attached.Host,
			Message: fmt.Sprintf(
				"%s=%q must be an integer of at least 2: one attempt is no retry",
				ParamAttempts, raw),
		})
	}
	return out
}
