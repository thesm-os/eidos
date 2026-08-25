// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package atomic recognises the atomic mixin — the assertion
// that the annotated callable either fully completes or has no
// observable side-effect. Composes with any structural shape.
//
// The recognised directive is:
//
//	//+gen:mixin atomic
//
// No parameters; presence is the entire signal. A positive
// detection appends `"atomic"` to the callable's
// [shape.MetaMixins] list.
//
// Consumers register the [Mixin] alongside other mixins when
// constructing the umbrella plugin:
//
//	pipe.WithAnnotator(shape.New().Mixins(atomic.Mixin()))
//
// `read=` names the callable a check reads the state back through.
// Nothing in an atomic claim is observable without one: the assertion
// is about what is *not* there after a failure, and a check that
// cannot look asserts only that a successful call succeeded.
//
// Optional, like every partner param in this vocabulary — the bare
// form still classifies, and a consumer that cannot state the law
// without an observer declines to state it.
package atomic
