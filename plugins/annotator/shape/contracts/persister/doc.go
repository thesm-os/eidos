// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package persister recognises the persister contract — a
// two-member protocol pairing a writer callable with a reader
// callable that retrieves the just-persisted entity. Persister
// is purely directive-driven: the structural shape of each
// member is detected independently by the [writer] /
// [shape/reader] detectors; persister adds the protocol-binding
// metadata on top.
//
// The recognised directive is:
//
//	//+gen:contract persister role=writer reader=GetByID
//
// applied to the writer-side callable. The `reader=` partner
// names a sibling callable (in the same struct, interface, or
// package) that returns the just-persisted entity. The
// refinement resolver rewrites the raw name into
// a qualified name and back-stamps the persister contract on
// the resolved reader.
//
// A positive stamp lays down these meta keys on the writer:
//
//	shape.contracts                        = [..., "persister"]
//	shape.contract.persister.role          = "writer"
//	shape.contract.persister.partner.reader = "GetByID"  (raw name)
//
// And after the refinement resolver runs, the same keys land on
// the reader with `role = "reader"` and the partner key pointing
// back to the writer's qname.
//
// Consumers register the [Contract] alongside other contracts
// when constructing the umbrella plugin:
//
//	pipe.WithAnnotator(shape.New().Contracts(persister.Contract()))
//
// Persister is the canonical N=2 contract — every plugin author
// writing a multi-member contract should read this package's
// source as the smallest possible reference implementation.
package persister
