// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package partition recognises the partition mixin — the
// assertion that the annotated callable observes a partition
// boundary (e.g. tenant, shard) and never serves data from a
// different partition.
//
// The `read` param names the callable that reads a partition back,
// which is how a suite proves data from another one never appears.
// The `axis` param names the annotated callable's own parameter
// carrying the partition, which is the one a check varies while
// holding the rest fixed.
//
// Both are needed for a check that can fail. Writing twice while
// varying every parameter writes to two different keys, so it passes
// against an implementation that ignores partitions entirely — a
// check that cannot fail is worse than none, and the axis is what
// separates the two writes onto one key.
//
// The axis has to name a parameter on both halves, because a check
// writes through the annotated callable and reads through the partner
// while carrying one partition value across both calls. The validator
// requires the partner to spell it identically, which is what lets a
// generator match the two by name rather than guess — the invariant is
// checked here so it need not be assumed there.
//
// With the pair pinned, the rest follows from the signatures: a
// parameter the reader also takes is identity and is held fixed, and
// one only the writer takes is payload and is varied. Both writes must
// differ in payload or the read cannot tell them apart.
//
// Both params are optional: the bare form still classifies the
// callable, and a consumer wanting only the classification writes it.
// A read naming nothing in scope is reported by the resolver; an axis
// naming no parameter of the callable, or none of its partner, is
// reported by this mixin's validator.
//
// The recognised directive is:
//
//	//+gen:mixin partition read=Read axis=partition
package partition
