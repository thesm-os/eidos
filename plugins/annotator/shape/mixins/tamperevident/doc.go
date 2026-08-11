// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package tamperevident recognises the tamperevident mixin — the assertion that
// modification of previously accepted data is detectable rather than silent.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin tamperevident
package tamperevident
