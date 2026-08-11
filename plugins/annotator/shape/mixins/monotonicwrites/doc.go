// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package monotonicwrites recognises the monotonicwrites mixin — the assertion that
// writes by one client are applied in the order that client issued them.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin monotonicwrites
package monotonicwrites
