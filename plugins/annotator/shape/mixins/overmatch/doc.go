// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package overmatch recognises the overmatch mixin — the assertion that
// iteration may yield more elements than the query strictly selected, so a suite asserts containment rather than equality.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin overmatch
package overmatch
