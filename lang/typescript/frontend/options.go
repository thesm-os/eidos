// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"go.thesmos.sh/eidos/core/opt"
)

// Options carries the TypeScript frontend's user-tunable settings,
// populated at pipeline Build time either programmatically or from a
// config-file entry scoped to the frontend's name.
//
// Every field has a default; the typical pipeline registers the
// frontend with no options at all.
type Options struct {
	// Dir is the directory patterns resolve against. Empty lets the
	// frontend inherit the process working directory, which is what a
	// CLI run wants. Test fixtures and embedded callers targeting a
	// tree elsewhere supply an absolute path.
	Dir string `json:"dir" eidos:"dir,default="`

	// IncludeTests controls whether `*.test.ts` and `*.spec.ts` files
	// contribute declarations. Defaults to false so generators see
	// only production-facing types.
	IncludeTests bool `json:"include_tests" eidos:"include_tests,default=false"`

	// IncludeDeclarations controls whether `.d.ts` files are parsed.
	//
	// Defaults to true, the opposite of what the name might suggest
	// at a glance: a `.d.ts` file is often the only place a type is
	// declared at all — it is how a package published as JavaScript
	// describes itself — so excluding them by default would make the
	// frontend blind to most third-party types.
	IncludeDeclarations bool `json:"include_declarations" eidos:"include_declarations,default=true"`

	// SkipGeneratedFiles drops files whose first lines carry the
	// canonical `Code generated … DO NOT EDIT.` marker.
	//
	// The framework's own output carries that header, so the default
	// keeps a second run from re-parsing what the first one emitted.
	SkipGeneratedFiles bool `json:"skip_generated_files" eidos:"skip_generated_files,default=true"`

	// SkipNodeModules drops any path with a `node_modules` segment.
	//
	// Defaults to true. A recursive pattern rooted at a project
	// directory would otherwise walk the whole dependency tree, which
	// is routinely tens of thousands of files and is never what the
	// pattern meant.
	SkipNodeModules bool `json:"skip_node_modules" eidos:"skip_node_modules,default=true"`
}

// defaultOptions returns the values that hold when no overrides
// reach the frontend. Mirrors the `default=…` tags one for one; the
// duplication is the trade for keeping [New] panic-free.
func defaultOptions() Options {
	return Options{
		Dir:                 "",
		IncludeTests:        false,
		IncludeDeclarations: true,
		SkipGeneratedFiles:  true,
		SkipNodeModules:     true,
	}
}

// optionsSchema is the reflected schema, cached at package init and
// reused across pipeline invocations.
var optionsSchema = opt.Reflect(
	Options{},
) //nolint:gochecknoglobals // schema is stateless and a reflection result
