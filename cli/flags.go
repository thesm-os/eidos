// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cli

// Flag-name constants. The cli package registers flags into a
// caller-supplied [flag.FlagSet] using these names; consumers
// using other CLI frameworks (cobra, kong, urfave-cli) reference
// the same constants when declaring equivalent flags so the
// canonical spelling stays consistent across binaries.
//
// Per-command flag-name sets:
//
//   - Every command: FlagConfig, FlagDiagFormat, FlagVerbose, FlagQuiet
//   - Run: + FlagNoCache, FlagTarget, FlagOutput
//   - Plan, Version: (none beyond the common set)
//   - Check, Prune: (none beyond the common set; Prune adds FlagDryRun)
//   - Explain: (none beyond the common set; selector is positional)
const (
	// FlagConfig is the path to the config file (overrides
	// .<brand>.yaml discovery).
	FlagConfig = "config"

	// FlagDiagFormat selects the diagnostic output format
	// ("text" or "json").
	FlagDiagFormat = "diag-format"

	// FlagVerbose surfaces Info diagnostics.
	FlagVerbose = "verbose"

	// FlagQuiet suppresses Warn diagnostics.
	FlagQuiet = "quiet"

	// FlagNoCache disables the build cache for a single invocation
	// (overrides the file's cache.enabled).
	FlagNoCache = "no-cache"

	// FlagTarget restricts the pipeline to a single source-decl
	// scope: a bare unqualified name (`-target Article`) or a
	// qualified `pkg.Name` form (`-target blog.Article`) for the
	// disambiguator case across packages.
	FlagTarget = "target"

	// FlagOutput pins the rendered filename for every emitted
	// decl in scope. Used by `//go:generate` one-shot invocations
	// where the rendered output collapses into a single named
	// file. Requires [FlagTarget] (or [GOFILE]-driven inference)
	// since pairing it with a multi-symbol scope produces
	// undefined per-decl behaviour.
	FlagOutput = "o"

	// FlagPackage pins the rendered file's package name for
	// every emitted decl in scope. Required when
	// [FlagLayout]=centralised and the config supplies no
	// `output.package`.
	FlagPackage = "p"

	// FlagLayout overrides the project-default layout policy
	// for the run. Accepts [pipeline.LayoutAlongsideSource] or
	// [pipeline.LayoutCentralised].
	FlagLayout = "layout"

	// FlagOutputDir sets the rendered output directory under
	// centralised layout. Ignored — with a configuration warning
	// — under alongside-source layout.
	FlagOutputDir = "output-dir"

	// FlagDryRun reports planned actions without performing them
	// (currently used by `prune`).
	FlagDryRun = "dry-run"
)

// Usage strings. Centralised so the wording stays consistent
// across the reference binary and any downstream binary that
// re-declares flags with another CLI framework.
const (
	UsageConfig     = "Path to the config file. Defaults to .<brand>.yaml discovered upward from the working directory."
	UsageDiagFormat = "Diagnostic output format. One of: text, json."
	UsageVerbose    = "Surface Info diagnostics."
	UsageQuiet      = "Suppress Warn diagnostics."
	UsageNoCache    = "Disable the build cache for this invocation."
	UsageTarget     = "Restrict the run to source decls whose unqualified name equals VALUE, or whose qualified name ends with .VALUE (pkg.Name disambiguates across packages)."
	UsageOutput     = "Pin the rendered filename for every emitted decl in scope. Requires -target (or GOFILE inference)."
	UsagePackage    = "Pin the rendered file's package name for every emitted decl in scope. Required when -layout=centralised has no config-side output.package."
	UsageLayout     = "Override the project-default layout policy (alongside-source or centralised)."
	UsageOutputDir  = "Output directory under -layout=centralised. Ignored under alongside-source."
	UsageDryRun     = "Report planned actions without performing them."
)
