// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package errorgen is a reference contributor: a plugin that renders
// into another plugin's file without owning one.
//
// It appends a recover-and-respond entry into the "postbody" slot of
// every [go.thesmos.sh/eidos/reference/handlergen] handler in the run,
// declaring its own emit kind and shipping the template that renders
// it. The host's template calls `render` on each slot item, which
// dispatches to whichever template owns that item's Kind — so the host
// never learns what this entry looks like, and this plugin never learns
// where the file lands.
//
// # Why this one exists
//
// errorgen and [go.thesmos.sh/eidos/reference/auditgen] both append to
// the same handlergen slot, and the pair is the worked example of
// ordering ACROSS priority buckets. errorgen runs in
// [sdk.GeneratorCrossCutting]; auditgen runs in the later
// [sdk.GeneratorFinalize]. The recover entry therefore renders before
// the audit entry.
//
// The mechanism matters more than the result. `Requires` resolves only
// within a bucket, so neither plugin could have named the other to
// achieve this — a `Requires` edge pointing across a bucket boundary is
// silently inert, not an error. Contributors that must interleave in a
// known order either share a bucket and name each other, as the
// authgen / metricgen / tracegen chain does, or sit in different
// buckets and let the bucket decide. This plugin is half of the second
// case; see [go.thesmos.sh/eidos/reference/authgen] for the first.
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
package errorgen
