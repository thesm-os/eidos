// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package sideeffect recognises the side-effect mixin — the
// assertion that the annotated callable has observable
// side-effects beyond its return value (so the test suite must
// observe them externally rather than just inspecting the
// return).
//
// The `observe` param names the callable whose result changes
// across this one — the effect a suite has to read to see anything
// happened.
//
// The param is optional: the bare form still classifies the
// callable, and a consumer wanting only the classification writes
// it. A generated check that has to call the partner needs it
// named, and an unresolvable name is reported by the resolver.
//
// The recognised directive is:
//
//	//+gen:mixin sideeffect observe=Count
package sideeffect
