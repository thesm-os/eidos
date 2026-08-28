// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package workflow

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical contract name this package stamps.
const Name = "workflow"

// RoleFn is the callable the workflow advances.
const RoleFn = "fn"

// Roles enumerates the contract's role vocabulary.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Roles = []string{RoleFn}

// ParamTransitions is the KV key naming the declaration that holds
// the state moves the workflow permits.
//
// The value is a package-level var, not the graph itself:
//
//	//+gen:contract workflow role=fn transitions=Transitions
//
//	var Transitions = map[State][]State{
//	    StateStart: {StateStep1},
//	    StateStep1: {StateDone},
//	}
//
// # Why not the graph encoded in the value
//
// It was opaque, carrying an encoding, and the reasoning was sound as
// far as it went: a parser here would fix a notation for every
// consumer. What that missed is that declining to fix one does not
// leave a consumer free — it leaves every consumer to invent one, and
// two of them already had. This package's own example wrote
// `start->step1->done`, chained with arrows; the only consumer wrote
// `>Draft,Draft>Live`, comma-separated pairs with a leading edge for
// the initial state. Two notations for one key across the two
// repositories using it, and nothing parsing either.
//
// A graph is also not one thing, which is what every other param in
// this catalog names. It is a set of pairs, and the vocabulary has no
// way to hold one — but it has a way to hold the *declaration* of
// one, which is the move `validates`' `invalid=` makes for a value
// no derivation can invent.
//
// So the author's model stays theirs. Naming it fixes no notation and
// no shape, and it gives a consumer more than an encoding could: a
// reference usable at the moment a check needs it. A judge asking
// "was this move permitted" indexes the graph with a state that came
// out of the run, and a named var is already there to index — where a
// parsed notation would first have to be lifted into emitted code as
// a literal.
//
// [shape.KindVar], so it resolves through the package's var scope and
// stamps qualified. Which graph shapes a generator accepts is that
// generator's own decision, and refusing an unsupported shape is a
// sentence it can write — where refusing an unsupported notation is a
// parser it cannot finish.
const ParamTransitions = "transitions"

// ParamObserve is the KV key naming the read a check compares the
// state across a call through.
//
// The graph says which moves are permitted and nothing said how to
// see a move happen. Even holding the transitions, a check had no
// callable to read the state through, so driving one meant guessing
// which sibling was the observation — the shape `sideeffect` was in
// before `observe=` and `partition` before `axis=`.
//
// [shape.KindCallable], resolved through the host's own scope.
// Optional: a workflow callable without one is still what the
// contract names, and a law comparing the state simply does not bind.
const ParamObserve = "observe"

// Params enumerates the directive's KV keys.
//
//nolint:gochecknoglobals // intentionally exported as a per-contract constant set
var Params = []shape.Param{
	{Key: ParamObserve, Kind: shape.KindCallable},
	{Key: ParamTransitions, Kind: shape.KindVar},
}

// Contract returns the [shape.Contract] this package contributes.
func Contract() shape.Contract {
	return shape.Contract{Name: Name, Roles: Roles, Params: Params}
}
