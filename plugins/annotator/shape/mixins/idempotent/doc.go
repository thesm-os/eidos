// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package idempotent recognises the idempotent mixin — the
// assertion that invoking the callable N times with the same
// input has the same observable effect as invoking it once.
// Composes with any structural shape.
//
// The recognised directive is:
//
//	//+gen:mixin idempotent
//
// No parameters; presence is the entire signal. A positive
// detection appends `"idempotent"` to the callable's
// [shape.MetaMixins] list.
//
// Consumers register the [Mixin] alongside other mixins when
// constructing the umbrella plugin:
//
//	pipe.WithAnnotator(shape.New().Mixins(idempotent.Mixin()))
package idempotent
