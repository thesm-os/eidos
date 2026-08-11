// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package xsssafe recognises the xsssafe mixin — the assertion that
// output carrying untrusted input is escaped for an HTML context before it leaves the callable.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin xsssafe
package xsssafe
