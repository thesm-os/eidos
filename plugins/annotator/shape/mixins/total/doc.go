// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package total recognises the total mixin — the assertion that
// the callable returns a defined result for every input in the named domain rather than failing on some.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// The `domain` param names the input set the callable is total over,
// as prose for a reader; `edge=` names a declared input at its
// boundary, which is the one worth testing and the one prose cannot
// produce. Optional.
//
// The recognised directives are:
//
//	//+gen:mixin total domain=...
//	//+gen:mixin total domain=... edge=DomainEdge
package total
