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
package atomic
