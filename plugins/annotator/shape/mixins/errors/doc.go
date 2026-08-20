// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package errors recognises the errors mixin — the assertion that
// the annotated callable's error returns are part of its contract
// (callers are expected to inspect and handle them) rather than
// "shouldn't happen" sentinels.
//
// The mixin owes documentation, not a check. It changes how a
// reader treats the error returns and licenses nothing falsifiable
// by itself: which condition answers which sentinel is not stated
// here, and deliberately so. A condition-to-sentinel mapping in one
// KV value would be an encoded graph the resolver cannot check,
// where the per-condition mixins — `notfound sentinel=` for the
// miss, and siblings named as corpora ask — are one fact per
// declaration, each resolved through the var scope and each
// selecting its own law. A consumer should not report this mixin
// as an underivable gap; it derives nothing by design.
//
// The recognised directive is:
//
//	//+gen:mixin errors
package errors
