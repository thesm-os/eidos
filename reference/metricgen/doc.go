// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package metricgen is a reference contributor: a plugin that renders into
// another plugin's file without owning one.
//
// It contributes the metrics entry and must render after authgen, which it declares by naming that plugin's capability in Requires rather than by arranging to append second.
//
// It declares its own emit kind and ships the template that renders
// it, then appends that value into the "chain" slot of every
// [go.thesmos.sh/eidos/reference/middlewaregen] stack in the run. The
// host's template calls `render` on each slot item, which dispatches
// to whichever template owns that item's Kind — so the host never
// learns what this plugin's entry looks like, and this plugin never
// learns where the file lands.
//
// Four properties follow, each deliberate:
//
//   - **No [sdk.FilenameProvider].** Templates say how a value renders;
//     Outputs say where a file lands. This plugin renders inside a file
//     it does not own, so it declares no output. One would make Layout
//     compose a second file for it.
//   - **No [sdk.NodesOnly].** It reads the emit graph to find the
//     stacks. Declaring otherwise would license the pipeline to run it
//     concurrently with the generator writing that graph.
//   - **TemplateFuncs returns nil.** The shared Go helpers are already
//     in the backend's overrideable funcmap; contributing them again is
//     a Build-time collision, and two plugins that both did it could
//     not appear in one pipeline.
//   - **The template file is named for the plugin.** Templates are
//     registered under their base filename as well as their `define`
//     name, so three contributors each shipping `entry.tmpl` collide
//     even though their define names differ.
//
// See [go.thesmos.sh/eidos/reference/tracegen] for the third link in the same chain.
package metricgen
