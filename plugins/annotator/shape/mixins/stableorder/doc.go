// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package stableorder recognises the stableorder mixin — the assertion that
// repeated iteration yields elements in the same order, so a suite may compare sequences rather than sets.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin stableorder
package stableorder
