// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package causal recognises the causal mixin — the assertion that
// effects are observed in an order consistent with the causal order in which they were produced.
//
// The `version` param names the field of the read or written value
// carrying the ordering stamp, the same one the session guarantees
// take. A law reading a trace per client orders operations by it, so
// without the stamp no operation carries an ordering and the law has
// nothing to compare.
//
// The claim itself is declared rather than detected: no signature
// reveals it.
//
// The recognised directive is:
//
//	//+gen:mixin causal version=Version
package causal
