// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package validates recognises the validates mixin — the
// assertion that the annotated callable's inputs are screened by
// the named validator before any business logic runs. The `fn`
// param names the validator sibling; the resolver rewrites it
// into a qualified name.
//
// `invalid=` names a declared value the validator refuses, which is
// what lets the law bind in the direction that can fail: hand the
// value to the call and require a refusal. Without it the only
// stateable reading — whatever the call accepted, the validator must
// accept too — engages and stays green with the screening deleted
// from the subject, because a derived value the validator happens to
// accept answers nil whatever the call did. Optional; a validated
// callable without one is still what the mixin names.
//
// The recognised directives are:
//
//	//+gen:mixin validates fn=ValidateInput
//	//+gen:mixin validates fn=ValidateInput invalid=BadPayload
package validates
