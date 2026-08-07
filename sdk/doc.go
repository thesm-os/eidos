// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package sdk is the plugin-author façade. It re-exports the
// contract-shaped surface from the framework's layered packages —
// [plugin], [priority], [core/directive], and [core/kind] — under
// one import so a new plugin's `import` block stays compact.
//
// A typical plugin imports:
//
//	import (
//	    "go.thesmos.sh/eidos/sdk"          // role + capability contracts
//	    "go.thesmos.sh/eidos/core/opt"     // typed options (plugins that have any)
//	    "go.thesmos.sh/eidos/node"         // source-side store (annotators / generators)
//	    "go.thesmos.sh/eidos/emit"         // emit-side store (generators / backends)
//	    "go.thesmos.sh/eidos/emit/builder" // emit construction helpers (generators)
//	)
//
// Without the façade the same plugin needs `core/directive`,
// `priority`, `plugin`, `core/kind`, plus the four high-volume
// packages above — eight imports before the plugin's own code
// starts. sdk halves that to four.
//
// # What's in scope
//
// Every contract-shaped concern — anything the framework asserts
// against a plugin, or that a plugin must name to declare itself:
// the role and capability interfaces, per-phase contexts,
// annotator visitor hooks, priority buckets, directive schema
// declaration + validation, the structural kind discriminator and
// the source kinds a directive scopes against, and the emit-side
// interfaces the framework calls back on a plugin's own values.
//
//   - [plugin.go]: roles, capabilities, contexts, hooks
//   - [priority.go]: Priority type + canonical buckets
//   - [directive.go]: schemas, parsing, validation, registry, errors
//   - [kind.go]: Kind type + the NodeKind source kinds
//   - [emit.go]: emit contracts — base value, refs, provenance,
//     slots, output-package dispatch
//   - [options.go]: typed options binding
//
// The test is whether a plugin can be *declared* without the
// symbol, not how often it is used. [OutputPackageSetter] appears
// in two in-tree plugins and belongs here; [emit.NewCall] appears
// in many more and does not.
//
// # What's intentionally out of scope
//
// The construction surfaces — the bulk of [node], [emit] and
// [emit/builder], plus [core/meta] — stay as separate imports.
// Flattening them would balloon this package to several hundred
// symbols and shadow common Go names (`Field`, `Schema`, `Type`,
// `Walk`, `New`) with conflicting meanings. That hazard is not
// hypothetical: all eighteen source kinds share a name with an
// emit kind carrying a *different* value, which is why the source
// set is re-exported prefixed and the emit set not at all. Bulk
// volume stays where [node], [emit] and [emit/builder] already own
// a coherent namespace.
package sdk
