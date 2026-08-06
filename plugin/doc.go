// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package plugin defines the contracts every eidos plugin satisfies.
// A plugin is anything that implements [Plugin] (a single Name()
// accessor) plus one or more role interfaces:
//
//   - [Frontend]: parses input into the source-side store
//   - [Annotator]: stamps metadata on existing nodes
//   - [Generator]: produces emit entities into the output-side store
//   - [Backend]: renders emit to a target language and writes to a
//     [sink.Sink]
//
// Plugins may also opt into capability interfaces:
//
//   - [CapabilityProvider]: priority + Provides/Requires for
//     pipeline ordering
//   - [TemplateProvider]: ships templates / func-map extensions for
//     a backend
//   - [OptionsProvider]: declares typed configuration the pipeline
//     populates at Build()
//
// One plugin may implement any combination of role and capability
// interfaces. The roles and capabilities are deliberately small so
// plugin authors can compose them without ceremony.
//
// Capabilities are detected by interface assertion on whatever is
// registered. Roles are not: each is registered explicitly through
// the matching Builder method, so a plugin implementing two roles is
// registered once per role. `cli`'s registerPlugin does this for
// every role a plugin implements; a library caller writes
// `WithAnnotator(p).WithGenerator(p)` with the same instance.
// Registering only one role leaves the other's method uncalled,
// silently — the pipeline iterates the role slices rather than
// re-asserting.
//
// One constraint comes with the composition:
// [CapabilityProvider.Priority] is read once and serves every phase
// the plugin participates in, and the annotator and generator bucket
// ladders share their integers — [priority.AnnotatorRefinement] and
// [priority.GeneratorCrossCutting] are both 300. A dual-role plugin
// therefore cannot choose its annotator and generator buckets
// independently; AnnotatorShape plus GeneratorFoundation is not
// expressible. Pick the value whose generator placement matters and
// say so in the plugin's docblock, or split the plugin in two so
// each half gets its own priority and capability set.
//
// State must not travel on the plugin struct between phases. The
// node graph freezes after the frontend, so an annotator's result
// belongs on the nodes as metadata, which is where the generator
// half reads it from through ctx.Reader — a field on the plugin
// bypasses the read set and produces output that is stale but looks
// current.
//
// Directive schemas are NOT a plugin capability — directives are
// shared contracts (multiple plugins may consume the same directive),
// so schemas register directly with the pipeline via
// `pipeline.Builder.WithDirective`.
package plugin
