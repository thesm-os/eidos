// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"go.thesmos.sh/eidos/core/opt"
)

// Options carries the Go frontend's user-tunable settings. The
// fields are populated via the [plugin.OptionsProvider] surface at
// pipeline Build time — either programmatically through
// [pipeline.Builder.WithPluginOptions] or from a config-file entry
// scoped to the frontend's [plugin.Plugin.Name].
//
// All fields have sensible defaults; the typical pipeline registers
// the frontend with no options at all.
type Options struct {
	// IncludeTests controls whether `*_test.go` files contribute
	// declarations to the loaded packages. Defaults to false so
	// generators only see production-facing types.
	IncludeTests bool `json:"include_tests" eidos:"include_tests,default=false"`

	// BuildTags is a space-separated list of build tags to apply
	// when loading packages. Empty by default — Go's standard build
	// tag rules apply.
	BuildTags string `json:"build_tags" eidos:"build_tags,default="`

	// SkipCgoFiles drops cgo-synthesized declarations from the
	// loaded packages. Cgo wrappers rarely participate in
	// generator-driven workflows; defaulting to true keeps the
	// store free of noise. Set to false when a generator legitimately
	// needs visibility into cgo types.
	SkipCgoFiles bool `json:"skip_cgo_files" eidos:"skip_cgo_files,default=true"`

	// SkipGeneratedFiles drops files carrying the canonical
	// `// Code generated ... DO NOT EDIT.` marker (Go's
	// `go/build.IsGenerated` shape) before they reach the converter.
	// The framework's own outputs always carry this header, so the
	// default — true — keeps a subsequent run from re-parsing
	// previously-emitted code as fresh source. Set to false when a
	// pipeline legitimately needs to consume generated files (for
	// instance, an annotator that walks third-party generated
	// output to attach metadata).
	SkipGeneratedFiles bool `json:"skip_generated_files" eidos:"skip_generated_files,default=true"`

	// Dir is the working directory passed to `golang.org/x/tools/go/packages`
	// during Load. Empty (the default) lets `go/packages` inherit the
	// process working directory — the typical setup for CLI runs. Test
	// fixtures and embedded callers targeting a separate Go module
	// supply an absolute path so the loader resolves patterns against
	// that module's build graph rather than the parent process's.
	Dir string `json:"dir" eidos:"dir,default="`

	// IgnoreWorkspace, when true, sets `GOWORK=off` in the loader's
	// environment so `packages.Load` respects the loaded directory's
	// own go.mod boundary rather than any enclosing go.work. The
	// default keeps workspace awareness on — consumers loading
	// in-workspace packages (the common case) get replace-directive
	// resolution and cross-module visibility for free. Harnesses
	// driving the frontend at a self-contained fixture module that
	// intentionally lives outside the workspace flip this to true so
	// the workspace doesn't reject the fixture's path.
	IgnoreWorkspace bool `json:"ignore_workspace" eidos:"ignore_workspace,default=false"`
}

// optionsSchema is the reflected option schema cached at package
// init time. Each [Frontend] instance reuses it across pipeline
// invocations.
var optionsSchema = opt.Reflect(
	Options{},
) //nolint:gochecknoglobals // schema is stateless and reflection result
