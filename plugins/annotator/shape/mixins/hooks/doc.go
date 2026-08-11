// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package hooks recognises the hooks mixin — the assertion that
// the annotated callable supports before/after lifecycle hooks
// the test suite must exercise.
//
// The `register` param names the callable a hook is registered
// through, without which a suite has no way to install one.
//
// The param is optional: the bare form still classifies the
// callable, and a consumer wanting only the classification writes
// it. A generated check that has to call the partner needs it
// named, and an unresolvable name is reported by the resolver.
//
// The recognised directive is:
//
//	//+gen:mixin hooks register=OnEvent
package hooks
