// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package pointintime recognises the pointintime mixin — the assertion that
// two reads of one key agree even when a write lands between them.
//
// Stronger than [cacheable], which claims agreement within one iteration
// and permits a concurrent write to be observed by the second read.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin pointintime
package pointintime
