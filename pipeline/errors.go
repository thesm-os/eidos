// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline

import "errors"

// ErrDuplicatePlugin is returned by [Builder.Build] when two plugins
// register under the same [plugin.Plugin.Name]. Names must be unique
// across every role; collisions are programmer errors.
var ErrDuplicatePlugin = errors.New("pipeline: duplicate plugin name")

// ErrNoFrontend is returned by [Builder.Build] when no frontend was
// registered. A pipeline without a frontend has no source to load
// and so cannot run.
var ErrNoFrontend = errors.New("pipeline: no frontend registered")

// ErrNoBackend is returned by [Builder.Build] when no backend was
// registered. A pipeline without a backend cannot render emit
// values to output and so cannot run.
var ErrNoBackend = errors.New("pipeline: no backend registered")

// ErrMultipleBackends is returned by [Builder.Build] when more than
// one backend is registered. Exactly one backend is supported per
// pipeline; multi-language output is achieved by running multiple
// pipelines.
var ErrMultipleBackends = errors.New("pipeline: multiple backends registered")

// ErrInvalidOptions is returned by [Builder.Build] (wrapped with
// the offending plugin name and the underlying validation error)
// when an [plugin.OptionsProvider]'s SetOptions returns a
// validation failure.
var ErrInvalidOptions = errors.New("pipeline: invalid plugin options")

// ErrCycle is returned by [Builder.Build] when a priority bucket
// contains a cycle in its [plugin.CapabilityProvider.Requires]
// graph. The wrapped message lists the plugins involved in the
// cycle in alphabetical order.
var ErrCycle = errors.New("pipeline: cycle in plugin requires")

// ErrDuplicateProvider is returned by [Builder.Build] when two
// plugins in the same priority bucket both declare the same
// capability name in [plugin.CapabilityProvider.Provides]. Each
// capability must have a single provider per bucket so topo
// resolution stays deterministic.
var ErrDuplicateProvider = errors.New("pipeline: duplicate capability provider")

// ErrTemplateFuncCollision is returned by [Builder.Build] when two
// plugins both register a [text/template] func of the same name in
// [plugin.TemplateProvider.TemplateFuncs] for the backend's
// language. Intentional overrides go through
// [plugin.TemplateProvider.TemplateOverrides] and bypass this
// check.
var ErrTemplateFuncCollision = errors.New("pipeline: template func collision")

// ErrNoSink is returned by [Pipeline.Run] when no [sink.Sink] was
// configured at Build time. The backend has nowhere to write so the
// run cannot complete.
var ErrNoSink = errors.New("pipeline: no sink configured")

// ErrRunHadErrors is returned by [Pipeline.Run] when one or more
// plugins emitted a [diag.Error] diagnostic. The pipeline runs to
// completion regardless of intermediate errors; ErrRunHadErrors is
// the post-hoc signal that the user's code or a plugin reported
// problems. Inspect the [diag.Sink] (via [Pipeline.Diag]) for
// per-error details.
var ErrRunHadErrors = errors.New("pipeline: run completed with error diagnostics")

// ErrDuplicateDirective is returned by [Builder.Build] when two
// directive schemas register under the same name. Schemas are
// shared contracts (multiple plugins may consume one), so the
// pipeline rejects duplicate registrations rather than guessing
// which definition wins.
var ErrDuplicateDirective = errors.New("pipeline: duplicate directive schema")

// ErrIncompatibleEmitVersion is returned by [Builder.Build] when a
// plugin implementing [plugin.EmitVersioned] declares a list of
// supported emit majors that does not include the in-tree
// [emit.Major]. The wrapping message names the plugin and lists
// both the declared and the in-tree majors so the upgrade path is
// obvious.
var ErrIncompatibleEmitVersion = errors.New("pipeline: incompatible emit version")

// ErrInvalidDirectivePrefix is returned by [Builder.Build] when the
// prefix configured via [Builder.WithDirectivePrefix] is rejected
// by the underlying [directive.NewParser] (empty, contains
// whitespace, or contains the reserved `:` / `+` / `-` runes). The
// wrapping error chain preserves [directive.ErrInvalidPrefix] so
// callers can match on it via [errors.Is].
var ErrInvalidDirectivePrefix = errors.New("pipeline: invalid directive prefix")

// ErrInvalidOutputs is returned by [Builder.Build] when a plugin
// implementing [plugin.FilenameProvider] returns a malformed
// Outputs slice for the active backend's language. The shape
// rules — every Suffix is non-empty, tags within one slice are
// unique, at most one Output declares an empty Tag, and the
// empty-Tag Output is at index 0 when present — protect every
// downstream consumer (Layout dispatch, directive resolution,
// CLI scoping, manifest attribution) from ambiguous routing
// states. The wrapping message names the plugin and describes
// the specific rule that fired.
var ErrInvalidOutputs = errors.New("pipeline: invalid plugin Outputs")

// ErrMissingFilenameProvider is surfaced by the Layout phase when a
// plugin emits a routable decl or a File-level slot contribution
// but does not implement [plugin.FilenameProvider]. The Layout
// phase wraps the sentinel with the offending plugin's name and
// the kind that triggered the lookup; consumers compare with
// [errors.Is].
//
// The check fires at Layout-phase runtime rather than at
// [Builder.Build] time because routability is data-dependent —
// the framework cannot statically tell which generators will
// emit. Plugins that legitimately emit nothing routable (method-
// slot weavers contributing only to other plugins' decls) do not
// trigger the check and need not implement the capability.
var ErrMissingFilenameProvider = errors.New(
	"pipeline: plugin emitted routable output without declaring a filename suffix",
)

// ErrUnknownOutputTag is surfaced by the Layout phase when a decl
// carries a non-empty [emit.BaseEmit.OutputTag] that does not
// match any of the [plugin.Output.Tag] values its owning plugin
// declares for the active backend's language. The wrapping
// message names the plugin, the offending tag, and the set of
// declared tags so authors can locate the typo or stray
// `pkg.File(tag)` call quickly.
var ErrUnknownOutputTag = errors.New(
	"pipeline: decl carries unknown OutputTag for its plugin's declared outputs",
)

// ErrNoDefaultOutput is surfaced by the Layout phase when a decl
// arrives with empty [emit.BaseEmit.OutputTag] but its owning
// plugin declares no empty-Tag output for the active backend's
// language. The plugin's intent — "every decl must carry an
// explicit tag" — is honoured by refusing to silently route the
// decl to the slice's first entry.
var ErrNoDefaultOutput = errors.New(
	"pipeline: decl carries empty OutputTag but plugin declares no default output",
)

// ErrUnscopedMultiOutputOverride is surfaced by the Layout phase
// when an unscoped routing directive (`+gen:out <path>` without
// `tag=`, or a form-3 emitter directive carrying `out=<path>`
// without `tag=`) pins a filename component against a plugin
// that declares multiple [plugin.Output] entries. Applying the
// override uniformly would force every output to share one
// filename — silently collapsing the plugin's per-output
// distinction. Authors scope to one output with `tag=<tag>`,
// or relax the override to a directory-only path so the
// per-output suffixes keep filenames distinct.
var ErrUnscopedMultiOutputOverride = errors.New(
	"pipeline: unscoped routing override pins a filename against a multi-output plugin",
)

// ErrPartialCapability is returned by [Builder.Build] when a plugin
// implements some but not all methods of a multi-method optional
// capability ([plugin.TemplateProvider],
// [plugin.CapabilityProvider]).
//
// Go's interface assertion is all-or-nothing, so a plugin declaring
// two of three methods satisfies neither the capability nor any
// consumer's check for it. Before this error the whole contribution
// vanished silently: the pipeline skipped the plugin, its templates
// were never registered, and the rendered output came out short
// with nothing reported. The conformance suite made the same
// assertion and skipped its checks, so both sides stayed green.
//
// The wrapped message names the plugin and every missing method,
// because the method name is the only actionable content — "partial
// capability" alone tells an author nothing they can fix.
var ErrPartialCapability = errors.New("pipeline: partial capability implementation")
