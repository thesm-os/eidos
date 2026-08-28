// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package transaction

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "transaction"

// ParamNotFound is the KV key naming the error a read reports for work the rollback undid.
//
// A sentinel is a package-level var, so the resolver rewrites it
// through the var scope rather than the callable one — see
// [shape.KindVar]. Absence is not an error: the bare
// form still classifies, and a suite that cannot state the law
// without a sentinel declines to state it.
const ParamNotFound = "notfound"

// ParamWrite is the KV key naming the callable whose effect the
// scope is expected to undo.
//
// [ParamNotFound] presumes a read and, until these keys, the
// contract named neither it nor the write it reads after. The law is
// that a rolled-back run leaves nothing behind, and checking it takes
// three steps: run the scope with a body that writes and then errors,
// read that key from outside, require the sentinel. This is step
// one's declaration.
//
// It cannot be derived, and that is a fact about the contract's shape
// rather than about any fixture. The `fn` host's whole signature is
// the body it runs — `Run(ctx, body func(ctx) error) error` declares
// no roles — so a walk looking for the establishing write finds every
// error-answering sibling equally qualified. Guessing aims the read
// at state the body never touched, where a correct subject and a
// broken one report the same nothing.
//
// [shape.KindCallable], resolved through the host's own scope like
// `atomic`'s and `crdtmerge`'s partners, and scoped to [RoleFn] since
// that is the only role there is to host it.
const ParamWrite = "write"

// ParamRead is the KV key naming the callable that looks for what the
// rollback undid.
//
// The observation [ParamNotFound]'s own wording assumes. Nothing in
// the claim is checkable without one: the assertion is about what is
// not there after a failure, and a check that cannot look asserts
// only that a successful call succeeded — `atomic`'s docblock puts it
// that way about the same gap, and `crdtmerge` declares the same pair
// for the same reason.
//
// [shape.KindCallable] and [RoleFn]-scoped, on [ParamWrite]'s terms.
const ParamRead = "read"

// Params enumerates the directive's KV keys.
//
// All three optional: the bare form still classifies, and a consumer
// that cannot state the law without them declines to state it rather
// than guessing at a partner.
//
// None is a counterexample. `write=` and `read=` name observations
// rather than an input no derivation can invent, which is the
// question [shape.Param.Counterexample] exists to ask — and a
// transaction's failing input is the body's own error, which the
// check supplies rather than the author.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []shape.Param{
	{Key: ParamNotFound, Kind: shape.KindVar},
	{Key: ParamRead, Kind: shape.KindCallable, Role: RoleFn},
	{Key: ParamWrite, Kind: shape.KindCallable, Role: RoleFn},
}

// RoleFn is the callable that runs inside the transaction.
const RoleFn = "fn"

// Roles enumerates the contract's role vocabulary — a single
// "fn" role since the contract is a per-callable marker.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleFn}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{
		Name:   Name,
		Roles:  Roles,
		Params: Params,
	}
}
