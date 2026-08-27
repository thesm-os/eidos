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
// Witnessing it takes the write the two reads straddle, which `write=`
// names. The key is optional — a read answering a consistent snapshot
// is what this classifies either way — but a law that reads the
// partner does not bind without it, so a subject declaring only the
// bare form states the property and derives no check from it.
//
// The recognised directives are:
//
//	//+gen:mixin pointintime
//	//+gen:mixin pointintime write=Store
package pointintime
