// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package causal recognises the causal mixin — the assertion that
// effects are observed in an order consistent with the causal order in which they were produced.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin causal
package causal
