// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package deleteremoves

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "deleteremoves"

// ParamRead is the KV key naming the callable whose not-found proves the removal.
//
// Named because a consumer reaching for it otherwise writes
// `Params[0]`, which is a position rather than a key: reordering
// the list — or adding a second parameter ahead of this one —
// silently changes what every such call site reads.
const ParamRead = "read"

// ParamSentinel is the KV key naming the error the read reports once the delete has removed the entity.
//
// A sentinel is a package-level var, so it resolves through the var
// scope rather than the callable one — see [shape.Mixin.SiblingVars].
// Spelled `sentinel` rather than a word describing the condition
// because the two keys on this directive would otherwise sit one
// letter apart.
const ParamSentinel = "sentinel"

// SiblingVars enumerates the param keys whose values name
// package-level vars the resolver rewrites into qualified names.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var SiblingVars = []string{ParamSentinel}

// Params enumerates the KV parameter names this mixin accepts.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var Params = []string{ParamRead, ParamSentinel}

// SiblingParams enumerates the param keys whose values name
// sibling callables the resolver rewrites into qualified names.
//
//nolint:gochecknoglobals // intentionally exported as a per-mixin constant set
var SiblingParams = []string{ParamRead}

// Mixin returns the [shape.Mixin] this package contributes.
func Mixin() shape.Mixin {
	return shape.Mixin{
		Name:          Name,
		Params:        Params,
		SiblingParams: SiblingParams,
		SiblingVars:   SiblingVars,
	}
}
