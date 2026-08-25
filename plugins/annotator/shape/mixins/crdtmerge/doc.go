// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package crdtmerge recognises the crdt-merge mixin — the
// assertion that concurrent writes to the annotated callable
// merge deterministically (CRDT-style) without conflict.
//
// The recognised directive is:
//
//	//+gen:mixin crdtmerge
//
// The claim needs three callables and the directive names two of
// them: `write=` makes two replicas diverge, the annotated merge
// reconciles them, and `read=` observes the result. Determinism is
// the assertion that the read agrees whichever order the merges
// happened in, which is unstateable without both.
package crdtmerge
