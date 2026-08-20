// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package scope recognises the scope mixin — the naming of the
// scope discipline the annotated callable participates in: its
// effects belong to the scope `name=` names (request, session,
// tenant), carried by the parameter `axis=` points at.
//
// The mixin owes documentation, not a check. The checkable form of
// the boundary claim is [partition]'s: proving an effect in one
// scope invisible from another needs an observer to read the first
// scope back, and varying the axis alone writes twice without ever
// looking — the check that cannot fail, which partition's own
// docblock rules out for exactly this shape. A scoped reader is no
// escape: it still needs a writer to seed through, so no host makes
// the axis alone sufficient.
//
// What this mixin adds is the thing partition deliberately lacks:
// the discipline's name. `partition axis=tenantID read=Get` says
// which parameter separates writes and how to observe them; nothing
// in it says tenant. The two compose on a callable that wants both
// the name and the check:
//
//	//+gen:mixin scope name=tenant axis=tenantID
//	//+gen:mixin partition axis=tenantID read=Get
//
// Authorisation — an unauthorised caller refused — is a property of
// the caller's identity rather than of the signature, and no
// signature-level vocabulary can state it; this mixin does not
// claim it either.
//
// The recognised directives are:
//
//	//+gen:mixin scope name=request
//	//+gen:mixin scope name=tenant axis=tenantID
//
// [partition]: go.thesmos.sh/eidos/plugins/annotator/shape/mixins/partition
package scope
