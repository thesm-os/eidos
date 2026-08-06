// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package handlergen is the foundation of the composition ensemble:
// the plugin that emits the value every other plugin extends.
//
// It declares the emit kind [Handler], ships the template that renders
// it, and exposes two slots — [PrebodySlot] and [PostbodySlot] —
// rendered before and after its own statements. Contributors append
// into those slots; handlergen never learns what they hold, because
// its template calls `render` on each item and the backend dispatches
// to whichever template owns that item's kind.
//
// # The ensemble
//
//	handlergen     GeneratorFoundation    emits Handler, owns +gen:handler
//	validategen    GeneratorComposition   owns a file AND fills prebody
//	errorgen       GeneratorCrossCutting  fills postbody
//	auditgen       GeneratorFinalize      fills postbody, last
//
// Ordering across those four is by bucket. Ordering *within* a bucket
// is by capability topo-sort over Provides/Requires — see
// [go.thesmos.sh/eidos/reference/middlewaregen] and its three
// contributors for a chain that exercises that path instead.
//
// # Why the slots are declared here rather than reused
//
// The framework already offers `prebody` and `postbody` on
// [emit.Method]. This plugin does not use them, because it declares
// its own emit kind — and a plugin-defined kind gets no slots for
// free. Owning them is what lets this plugin decide where in the
// rendered body a contribution lands, and it is the price of rendering
// through a template of its own rather than through the core
// `emit.method` template.
//
// Both slots are unconstrained. Every contributor declares its own
// emit kind and ships its own template, so the content is
// heterogeneous by design and no single element kind describes it. The
// trade is where a mistake surfaces: an open slot accepts anything and
// fails at render with a missing template naming a *kind*, where a
// constrained one would reject at the append call naming a *plugin*.
//
// # The dependency direction
//
// Contributors append to a value this plugin created, so a pipeline
// that registers a contributor without registering handlergen emits
// nothing at all — no orphan file, no partial handler. That is not
// configuration; it falls out of appending to a value rather than
// routing to a filename. A contributor that instead declared an Output
// with this package's [GoSuffix] would share the file, and would also
// conjure a file containing only its own half when handlergen was
// absent, because Layout's file lookup is lookup-or-create.
package handlergen
