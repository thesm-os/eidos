// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package deleteremoves recognises the delete-removes mixin —
// the assertion that a delete operation actually removes the
// entity such that a subsequent read returns not-found.
//
// The `read` param names the callable whose not-found proves the
// removal; without it there is nothing to read back.
//
// The param is optional: the bare form still classifies the
// callable, and a consumer wanting only the classification writes
// it. A generated check that has to call the partner needs it
// named, and an unresolvable name is reported by the resolver.
//
// The recognised directive is:
//
//	//+gen:mixin deleteremoves read=Get
package deleteremoves
