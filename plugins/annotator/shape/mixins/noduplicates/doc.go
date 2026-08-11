// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package noduplicates recognises the noduplicates mixin — the assertion that
// a single drain emits each element at most once.
//
// The fourth stream-ordering claim, beside [stableorder], [permutation]
// and [overmatch]. Independent of ordering: a stream may emit in any
// order and still repeat, and the repeat is what this rules out.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The recognised directive is:
//
//	//+gen:mixin noduplicates
package noduplicates
