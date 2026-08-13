// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package closer recognises the bare teardown shape — a callable
// taking nothing, returning only an error, and named for release.
//
// The recognised Go signatures are:
//
//	func (c *Conn) Close() error
//	func (s *Server) Shutdown() error
//
// This is the catalogue's only name-gated detector, and the exception
// is deliberate. `Close() error` and `Err() error` are the same
// signature, so no structural rule separates a teardown from a poison
// probe — and the two carry opposite semantics. A teardown is expected
// to answer differently the second time, which is precisely what a
// read-purity law over a poison accessor forbids, so classifying one
// as the other reddens correct code.
//
// Go's convention is strong enough to key on where structure is
// silent: `io.Closer` is a standard-library interface, and a method
// named Close returning only an error is that interface's whole
// contract. The wider set — Shutdown, Stop, Disconnect, Terminate —
// covers the spellings a release method takes when Close is taken or
// wrong.
//
// The name gate cuts both ways and both are escapable. A latch
// genuinely spelled Stop pins itself with `+gen:shape poisonaccessor`;
// a teardown spelled something this list does not hold pins itself
// with `+gen:shape closer`. Pinning to another shape is allowed —
// only suppression is not.
//
// A positive detection stamps:
//
//	shape = "closer"
package closer
