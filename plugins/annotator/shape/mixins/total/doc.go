// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package total recognises the total mixin — the assertion that
// the callable returns a defined result for every input in the named domain rather than failing on some.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The `domain` param names the input set the callable is total over.
//
// The recognised directive is:
//
//	//+gen:mixin total domain=...
package total
