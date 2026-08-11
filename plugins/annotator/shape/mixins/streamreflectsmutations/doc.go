// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package streamreflectsmutations recognises the
// stream-reflects-mutations mixin — the assertion that an
// already-iterating stream observes mutations applied
// concurrently to the underlying data source.
//
// The `mutate` param names the callable whose mutations the stream
// must observe, which is the half a suite applies mid-iteration.
//
// The param is optional: the bare form still classifies the
// callable, and a consumer wanting only the classification writes
// it. A generated check that has to call the partner needs it
// named, and an unresolvable name is reported by the resolver.
//
// The recognised directive is:
//
//	//+gen:mixin streamreflectsmutations mutate=Put
package streamreflectsmutations
