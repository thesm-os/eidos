// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package tracegen is a reference contributor: a plugin that renders into
// another plugin's file without owning one.
//
// It contributes the tracing entry and renders last, completing a three-link chain of declared Requires. The composition test registers all three in reverse and asserts the rendered order regardless, which is what makes the ordering a contract rather than a coincidence.
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
// Together with authgen and metricgen it shows that slot order is a property of the plugin graph, not of the host application's registration sequence.
package tracegen
