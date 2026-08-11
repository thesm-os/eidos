// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package conservative recognises the conservative mixin — the assertion that
// the named field is neither created nor destroyed by the operation, only moved between subjects.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The `field` param names the quantity the operation conserves.
//
// The recognised directive is:
//
//	//+gen:mixin conservative field=...
package conservative
