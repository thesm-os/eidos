// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package transaction recognises the transaction contract — a
// single-role marker declaring that the annotated callable runs
// inside (or owns) a transactional scope.
//
// `notfound=` names the error a read reports for work the rollback
// undid, and `write=` / `read=` name the pair that reaches it: run
// the scope with a body that writes and then errors, read the key
// from outside, require the sentinel. Neither callable can be
// derived — the scope's whole signature is the body it runs, so
// every error-answering sibling is equally qualified — which is why
// the author names them.
//
// All three are optional. The bare form still classifies, and a
// consumer that cannot state the law without them declines to state
// it rather than aiming a read at state the body never touched.
//
// The recognised directives are:
//
//	//+gen:contract transaction role=fn
//	//+gen:contract transaction role=fn notfound=ErrNotFound write=Put read=Get
//
// One role, and partners named as params rather than roles: the
// write and the read are siblings the scope acts through, not
// members of the contract. The downstream codegen reads the contract
// membership to wrap the call in begin / commit / rollback
// scaffolding.
package transaction
