// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package leakfree recognises the leakfree mixin — the assertion that
// every resource the callable acquires is released, including on the failure paths.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin leakfree
package leakfree
