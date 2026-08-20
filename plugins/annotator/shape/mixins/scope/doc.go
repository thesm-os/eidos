// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package scope recognises the scope mixin — the assertion that
// the annotated callable's effect is confined to the named
// scope (request, session, tenant, etc.) and never leaks into
// another scope.
//
// The claim has two halves and the mixin states one. Isolation —
// an effect in one scope is invisible from another — is derivable
// once `axis=` names the parameter carrying the scope, so a check
// can vary it and hold the rest fixed. Authorisation — an
// unauthorised caller is refused — is a property of the caller's
// identity rather than of the signature, and no signature-level
// vocabulary can state it; this mixin does not claim it.
//
// The recognised directives are:
//
//	//+gen:mixin scope name=request
//	//+gen:mixin scope name=tenant axis=tenantID
package scope
