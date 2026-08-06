// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package auditgen is a reference contributor: a plugin that renders
// into another plugin's file without owning one.
//
// It appends an audit-log entry into the "postbody" slot of every
// [go.thesmos.sh/eidos/reference/handlergen] handler in the run,
// declaring its own emit kind and shipping the template that renders
// it. The host's template calls `render` on each slot item, which
// dispatches to whichever template owns that item's Kind — so the host
// never learns what this entry looks like, and this plugin never learns
// where the file lands.
//
// # Why this one exists
//
// auditgen is the second half of the cross-bucket ordering example that
// [go.thesmos.sh/eidos/reference/errorgen] opens: both append to the
// same handlergen slot, errorgen from [sdk.GeneratorCrossCutting] and
// auditgen from the later [sdk.GeneratorFinalize], so the recover entry
// renders before the audit entry. `Requires` could not have expressed
// that — it resolves only within a bucket, and an edge across a
// boundary is silently inert rather than an error. See errorgen's
// documentation for the full comparison against the same-bucket case.
//
// It is also the ensemble's example of a plugin that publishes NO
// capability. [Plugin.Provides] returns nil because nothing orders
// against this plugin and it needs no label to be ordered — the
// finalize bucket already places it after everything else. A capability
// nobody names is noise in the topo graph, and the conformance suite
// does not require one.
//
// # Properties, each deliberate
//
//   - **No [sdk.FilenameProvider].** Templates say how a value renders;
//     Outputs say where a file lands. This plugin renders inside a file
//     it does not own, so it declares no output. One would make Layout
//     compose a second file for it.
//   - **No [sdk.NodesOnly].** It reads the emit graph to find the
//     handlers. Declaring otherwise would license the pipeline to run
//     it concurrently with the generator writing that graph.
//   - **It appends to the host's emit value, not by origin.** A run
//     without handlergen leaves nothing to append to, so this plugin
//     degrades to contributing nothing rather than stranding an orphan
//     file.
//   - **TemplateFuncs returns nil.** The shared Go helpers are already
//     in the backend's overrideable funcmap; contributing them again is
//     a Build-time collision.
//   - **The template file is named for the plugin.** Templates are
//     registered under their base filename as well as their `define`
//     name, so two contributors each shipping `entry.tmpl` collide even
//     though their define names differ.
package auditgen
