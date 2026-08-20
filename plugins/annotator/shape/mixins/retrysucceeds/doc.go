// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package retrysucceeds recognises the retry-succeeds mixin —
// the assertion that a transient failure followed by a retry
// converges to a successful outcome (the operation is naturally
// retryable).
//
// `attempts=` bounds the convergence: the first attempts-1 calls may
// fail, the attempts-th must succeed. Without it the claim licenses
// no check — a test either retries forever or picks a number the
// declaration did not authorise — so a consumer declines to state
// the law and the bare form serves as classification alone.
//
// The recognised directives are:
//
//	//+gen:mixin retrysucceeds
//	//+gen:mixin retrysucceeds attempts=3
package retrysucceeds
