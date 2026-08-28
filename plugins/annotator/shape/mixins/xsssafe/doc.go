// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package xsssafe recognises the xsssafe mixin — the assertion that
// output carrying untrusted input is escaped for an HTML context before it leaves the callable.
//
// Presence is the whole signal: nothing in a signature reveals it, so
// it is declared rather than detected.
//
// `unsafe=` names a declared value carrying markup that must not
// survive — the witness a derivation cannot produce, since drawn
// samples carry nothing dangerous and a subject that escapes
// nothing passes on them. Optional.
//
// The recognised directives are:
//
//	//+gen:mixin xsssafe
//	//+gen:mixin xsssafe unsafe=HostileMarkup
package xsssafe
