// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package chain recognises the chain contract — an append-only log
// whose history can be read back and, where the implementation
// supports it, checked for tampering.
//
// The `append` role adds an entry and the `replay` role reads the
// history back, which is what makes append-only checkable at all: a
// suite compares successive replays to assert the sequence only ever
// extends, and that an entry accepted by the append is present in the
// replay rather than silently dropped. The replay is required, since
// an append with no read surface states a property nothing can
// observe.
//
// The `verify` role is optional and names an explicit integrity check.
// A chain that has none is not thereby unchecked: a poison-accessor
// shape reports the same corruption through its error surface, which
// is the commoner Go spelling. Declaring verify says an implementation
// offers the direct form, not that it is required to.
//
// The recognised directives are:
//
//	//+gen:contract chain role=append replay=Replay
//	//+gen:contract chain role=append replay=Replay verify=Verify
package chain
