// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package workflow recognises the workflow contract — a callable
// whose execution follows a declared state-transition graph.
//
// `transitions=` names the package-level var holding the graph, not
// an encoding of it: the author's own model is what a consumer
// references, so no notation and no shape is fixed here. `observe=`
// names the read a check compares the state across a call through,
// without which a move can be permitted and not seen.
//
// The recognised directives are:
//
//	//+gen:contract workflow role=fn transitions=Transitions
//	//+gen:contract workflow role=fn transitions=Transitions observe=State
//
// where the graph is an ordinary declaration beside the subject:
//
//	var Transitions = map[State][]State{
//	    StateStart: {StateStep1},
//	    StateStep1: {StateDone},
//	}
package workflow
