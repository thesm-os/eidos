// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package injectionsafe recognises the injectionsafe mixin — the assertion that
// untrusted input reaches an interpreter as data rather than as syntax.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin injectionsafe
package injectionsafe
