// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package sample recognises the sample mixin — the assertion
// that the annotated callable's test suite uses sampled
// fixtures rather than exhaustive enumeration (intended for
// callables whose input space is too large for full coverage).
//
// The `builder` param names the callable producing a value this one
// accepts, so a suite can sample inputs rather than enumerate them.
//
// The param is optional: the bare form still classifies the
// callable, and a consumer wanting only the classification writes
// it. A generated check that has to call the partner needs it
// named, and an unresolvable name is reported by the resolver.
//
// The recognised directive is:
//
//	//+gen:mixin sample builder=NewFixture
package sample
