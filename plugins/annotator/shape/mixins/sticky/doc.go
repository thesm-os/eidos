// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package sticky recognises the sticky mixin — the assertion that
// inputs sharing the named key are served by the same instance for as long as it is healthy.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The `key` param names the parameter whose value pins the instance.
//
// The recognised directive is:
//
//	//+gen:mixin sticky key=...
package sticky
