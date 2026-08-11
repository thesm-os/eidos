// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package defaultonerror recognises the defaultonerror mixin — the assertion that
// a failed read returns the type's default beside the error rather than an undefined value.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin defaultonerror
package defaultonerror
