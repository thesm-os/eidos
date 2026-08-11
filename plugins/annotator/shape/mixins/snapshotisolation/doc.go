// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package snapshotisolation recognises the snapshotisolation mixin — the assertion that
// a transaction observes one consistent snapshot, so no dirty write, dirty read or read skew is visible.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin snapshotisolation
package snapshotisolation
