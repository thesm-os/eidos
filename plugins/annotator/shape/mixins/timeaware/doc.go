// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package timeaware recognises the timeaware mixin — the assertion that
// the result depends on the current time, so a suite must control the clock to get a repeatable answer.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin timeaware
package timeaware
