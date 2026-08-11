// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package partition recognises the partition mixin — the
// assertion that the annotated callable observes a partition
// boundary (e.g. tenant, shard) and never serves data from a
// different partition.
//
// The `read` param names the callable that reads a partition back,
// which is how a suite proves data from another one never appears.
//
// The param is optional: the bare form still classifies the
// callable, and a consumer wanting only the classification writes
// it. A generated check that has to call the partner needs it
// named, and an unresolvable name is reported by the resolver.
//
// The recognised directive is:
//
//	//+gen:mixin partition read=Get
package partition
