// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"strings"

	"go.thesmos.sh/eidos/core/opt"
)

// Options carries the protobuf frontend's user-tunable settings.
// The fields populate via the [plugin.OptionsProvider] surface at
// pipeline Build time — either programmatically through
// [pipeline.Builder.WithPluginOptions] or from a config-file entry
// scoped to the frontend's [plugin.Plugin.Name].
//
// All fields have sensible defaults; the typical pipeline registers
// the frontend with no options at all and points it at a proto
// source root via the standard `dir` key (matching the Go
// frontend's `dir` convention).
type Options struct {
	// Dir is the proto-source search root. Empty (the default)
	// lets protocompile inherit the process working directory.
	// Test fixtures and embedded callers targeting a separate
	// source tree supply an absolute path so resolution happens
	// against that tree rather than the parent process's CWD.
	Dir string `json:"dir" eidos:"dir,default="`

	// ImportPaths is a comma-separated list of additional proto
	// import search roots. The resolver always includes [Dir];
	// ImportPaths supplements it with paths used to resolve
	// `import "path/to/file.proto";` declarations that aren't
	// under [Dir]. Empty (the default) restricts resolution to
	// [Dir] plus the bundled well-known descriptors (when
	// [IncludeWellKnown] is true).
	ImportPaths string `json:"import_paths" eidos:"import_paths,default="`

	// IncludeWellKnown controls whether protocompile's bundled
	// well-known descriptors satisfy `import
	// "google/protobuf/timestamp.proto";` and friends without
	// requiring users to stage the files locally. Defaults to
	// true. When false, references to unstaged well-knowns
	// surface as protocompile resolution errors funnelled through
	// the frontend's diagnostic sink.
	IncludeWellKnown bool `json:"include_well_known" eidos:"include_well_known,default=true"`
}

// optionsSchema is the reflected option schema cached at package
// init time. Each [Frontend] instance reuses it across pipeline
// invocations.
//
//nolint:gochecknoglobals // schema is stateless and a reflection result.
var optionsSchema = opt.Reflect(Options{})

// defaultOptions returns the [Options] value the frontend uses
// before [Frontend.SetOptions] is called. Mirrors the `default=…`
// tags on the [Options] struct one-for-one; the small amount of
// duplication is the trade-off for keeping [New] panic-free and
// side-effect-free.
func defaultOptions() Options {
	return Options{
		Dir:              "",
		ImportPaths:      "",
		IncludeWellKnown: true,
	}
}

// importPathList splits the comma-separated [Options.ImportPaths]
// field into the slice form the resolver consumes. Empty values
// (and an entirely-empty [Options.ImportPaths]) produce a nil
// slice rather than a `[""]` singleton.
func importPathList(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
