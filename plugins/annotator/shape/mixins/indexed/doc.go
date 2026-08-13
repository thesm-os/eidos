// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package indexed recognises the indexed mixin — the assertion that
// the annotated callable's integer parameters are positions into the
// collection a named method sizes.
//
// Most integers in a Go interface are positions or sizes, and the
// sample derivation cannot tell which: it draws a magnitude, which is
// right for a value set through a setter and read straight back, and
// wrong for anything handed to a subject. `Less(i, j int) bool`
// against a five-element slice given 42 panics — not a failed claim, a
// broken harness, which reads as the generator being at fault.
//
// The `by` param names the callable reporting the collection's size.
// Per callable rather than per parameter: `Less(i, j int)` and
// `Swap(i, j int)` are wholly indices, and a key holds one value, so
// naming them individually would want a list for the common case to
// spare the rare one. A callable mixing an index with an unrelated
// integer — `Page(offset, limit int)` — over-applies this to the
// count, which draws a small limit rather than a large one. That costs
// a little coverage in the direction of exercising a paginator harder;
// the mistake it replaces costs a panic on correct code.
//
// The bound is a runtime fact, so this states it and does not compute
// it. `by=Len` resolves and qualifies like any sibling callable, which
// is what makes a misspelling fail at the directive — but a consumer
// honouring the stamp has to call the method on the seeded subject and
// draw inside what it reports, including the empty case, where no
// index exists and no check should be generated. That knowledge is the
// consumer's; this vocabulary only says which method holds it.
//
// Absence of `by` leaves the fact stated without a bound: a consumer
// that must size the collection declines rather than guessing.
//
// The recognised directive is:
//
//	//+gen:mixin indexed by=Len
package indexed
