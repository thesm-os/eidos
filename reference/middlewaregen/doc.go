// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package middlewaregen is the reference implementation of a plugin
// that other plugins compose into.
//
// Most cross-cutting generators append to a slot the framework already
// defines — `prebody` on a method, `fields` on a struct. Those slots
// live on core emit kinds, so any plugin can use them without knowing
// who else does. This package demonstrates the other arrangement: a
// plugin declaring its own emit kind with its own named slot, so that
// contributors depend on it explicitly.
//
// # What it emits
//
// One [MiddlewareStack] per struct carrying `+gen:handler`, rendered
// as a chain variable:
//
//	var UsersMiddleware = []func(http.Handler) http.Handler{
//		auth.RequireAuth,
//		metrics.RecordLatency,
//	}
//
// The entries are not this plugin's. They are contributed by
// [go.thesmos.sh/eidos/reference/authgen] and
// [go.thesmos.sh/eidos/reference/metricgen] into the value's "chain"
// slot, and rendered by this package's template.
//
// # Why a custom slot rather than a plain slice
//
// A slice would work if this plugin knew its contributors. It does
// not, and the slot is what makes that absence workable:
//
//   - Each entry carries provenance, so the framework can attribute a
//     contribution to the plugin that made it.
//   - Ordering is the pipeline's resolved capability topology, not
//     append order — metricgen renders after authgen because it names
//     authgen's capability in Requires, whatever order the host
//     application registered them in.
//   - A later contributor can position itself relative to an earlier
//     one by [emit.Provenance] ID.
//   - The slot carries no element kind, because each contributor
//     brings its own emit kind and the template that renders it. See
//     [MiddlewareStack.Chain] for what that costs at diagnosis time.
//
// # The dependency direction, and what it buys
//
// Contributors append to a value this plugin created. A pipeline that
// registers a contributor without registering this plugin therefore
// emits nothing: there is no stack to append to, so no file is
// written and no partial chain reaches disk.
//
// That is worth contrasting with the other way two plugins can share a
// file. A contributor can instead declare an [sdk.Output] whose Suffix
// equals this package's [GoSuffix] and route by origin — Layout then
// composes the same Target for both and their contributions land in
// the same file. It works, and it is the right mechanism when the two
// plugins are genuinely peers. But Layout's file lookup is
// lookup-or-create, so under that arrangement a contributor running
// without its host conjures an orphan file containing only its own
// half. Depending on the host's value rather than on its filename is
// what makes the absent-host case degrade to nothing instead of to
// something wrong.
package middlewaregen
