// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import "go.thesmos.sh/eidos/plugin"

// Plugin is the marker interface every plugin satisfies. The
// Name method returns a stable identifier the pipeline uses for
// registration, ordering tie-breaks, error attribution in
// diagnostics, and cache-key derivation.
type Plugin = plugin.Plugin

// Frontend is the role for plugins that parse input into the
// source side of the [store.Store]. The pipeline runs every
// Frontend in the frontend phase; multiple frontends may coexist.
type Frontend = plugin.Frontend

// Annotator is the role for plugins that read existing nodes and
// stamp metadata. Annotators run in the annotator phase, ordered
// across priority buckets and topo-sorted within a bucket.
type Annotator = plugin.Annotator

// Generator is the role for plugins that produce emit entities.
// Generators run in the generator phase, ordered across priority
// buckets and topo-sorted within a bucket.
type Generator = plugin.Generator

// Backend is the role for plugins that render emit entities to a
// target language and write the result through a [sink.Sink].
// Exactly one Backend runs per pipeline.
type Backend = plugin.Backend

// FrontendContext is the per-call context handed to every
// [Frontend.Load] invocation. Carries the shared store, the
// diagnostic sink, the directive registry, the directive parser,
// the cache, and the user-supplied pattern.
type FrontendContext = plugin.FrontendContext

// AnnotatorContext is the per-run context handed to every
// [Annotator.Annotate]. Carries the shared store, a per-plugin
// [store.Reader] for read tracking, and the diagnostic sink.
type AnnotatorContext = plugin.AnnotatorContext

// GeneratorContext is the per-run context handed to every
// [Generator.Generate]. Carries the shared store, a per-plugin
// [store.Reader] for read tracking, and the diagnostic sink.
type GeneratorContext = plugin.GeneratorContext

// BackendContext is the per-run context handed to the [Backend].
// Carries the shared store, the diagnostic sink, the destination
// sink, the target-language identifier, the ordered plugin list
// (for template-provider discovery), and header / footer
// composition slots.
type BackendContext = plugin.BackendContext

// CapabilityProvider is the optional capability for plugins that
// participate in pipeline ordering. Implementing plugins return a
// [Priority] bucket plus Provides / Requires capability names;
// the pipeline groups by Priority and topo-sorts within each
// bucket using those names.
type CapabilityProvider = plugin.CapabilityProvider

// DirectiveProvider is the optional plugin role for declaring the
// `+gen:` directive schemas the plugin understands. The pipeline
// collects every DirectiveProvider's schemas at Build time.
type DirectiveProvider = plugin.DirectiveProvider

// OptionsProvider is the optional capability for plugins that
// declare typed configuration. Use [core/opt.Bind] to satisfy
// this interface from a tagged options struct.
type OptionsProvider = plugin.OptionsProvider

// TemplateProvider is the optional capability for plugins that
// ship templates and / or extend a backend's template func-map.
type TemplateProvider = plugin.TemplateProvider

// FilenameProvider is the optional capability generators
// implement to declare the ordered set of outputs the pipeline
// composes into rendered filenames for every decl they emit.
type FilenameProvider = plugin.FilenameProvider

// Output is one entry in a [FilenameProvider]'s ordered output
// set — Tag scopes the output within the plugin's namespace
// (empty for the primary output); Suffix is the per-source-basename
// trailer the Layout phase appends to compose the rendered output
// filename.
type Output = plugin.Output

// Versioned is the optional plugin capability for declaring a
// plugin version string. The version composes into the plugin's
// cache key.
type Versioned = plugin.Versioned

// EmitVersioned is the optional capability for plugins that
// declare which major versions of the [emit] contract they
// support. The pipeline checks compatibility at Build time.
type EmitVersioned = plugin.EmitVersioned

// NodesOnly is the optional capability for [Generator] plugins
// that promise to read only the source-side store. The pipeline
// uses the declaration to parallelise the generator phase.
type NodesOnly = plugin.NodesOnly

// BeforeNodesHook is the optional pre-iteration hook a [Walk]
// target may implement. Invoked once before any per-kind hook
// fires.
type BeforeNodesHook = plugin.BeforeNodesHook

// AfterNodesHook is the optional post-iteration hook. Invoked
// once after every per-kind hook has fired.
type AfterNodesHook = plugin.AfterNodesHook

// StructHook is the optional per-struct hook. [Walk] invokes
// OnStruct once per [node.Struct] in stable insertion order.
type StructHook = plugin.StructHook

// InterfaceHook is the optional per-interface hook. [Walk]
// invokes OnInterface once per [node.Interface] in stable
// insertion order.
type InterfaceHook = plugin.InterfaceHook

// MethodHook is the optional per-method hook. [Walk] invokes
// OnMethod once per [node.Method] across the entire methods
// bucket (struct- and interface-declared methods alike) in stable
// insertion order.
type MethodHook = plugin.MethodHook

// FunctionHook is the optional per-free-function hook. [Walk]
// invokes OnFunction once per [node.Function] in stable insertion
// order. Methods reach [MethodHook] instead — this hook fires
// only for standalone functions.
type FunctionHook = plugin.FunctionHook

// Walk drives target's hook methods over every node in the
// annotator context's store. See [plugin.Walk] for the
// iteration contract.
var Walk = plugin.Walk

// ImportRegistrar re-exports [plugin.ImportRegistrar] — the ability
// to name a package in the file currently rendering.
type ImportRegistrar = plugin.ImportRegistrar

// ImportAwareFuncs re-exports [plugin.ImportAwareFuncs] — the
// optional interface a plugin implements when its template helpers
// emit references to other packages.
//
// The static [LanguageSupport.Funcs] and [LanguageSupport.Overrides]
// are built once for the whole run and cannot register an import, so
// text they return naming another package renders cleanly and fails
// the consumer's build. Implement this instead when a helper spells a
// reference: it is called once per rendered file, against that file's
// imports.
type ImportAwareFuncs = plugin.ImportAwareFuncs
