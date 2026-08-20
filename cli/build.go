// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cli

import (
	"fmt"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
)

// BuildPipeline composes a [pipeline.Pipeline] from the supplied
// Config and plugin universe. The Config selects which plugins
// from `plugins` are active and supplies their options; the
// function constructs the appropriate sink and cache, registers
// every active plugin in its correct role, applies the envelope /
// parallel / directive-prefix settings, and threads the routing-
// layer config (project-level and per-plugin output blocks) onto
// the Builder.
//
// Returns:
//   - the constructed *pipeline.Pipeline ready for Run.
//   - any [*ConfigError] describing a configuration fault (e.g. a
//     plugin named in Config.Plugins but absent from the slice).
//   - any sentinel error from pipeline.Build (ErrNoFrontend etc.).
//
// Shared by every command that needs a live pipeline rather than
// just the parsed config: [RunCommand], [PlanCommand],
// [CheckCommand], [PruneCommand], [ExplainCommand]. Exposed so
// embedders and integration tests can construct a Pipeline from
// a Config without going through a Command.
//
// Runtime overrides (`--no-cache`, the check command's in-memory
// sink swap) are CLI-internal — they live on the unexported
// pipelineOverride channel passed by each Command. Embedders
// configure equivalent behaviour through the on-disk config:
// [Config.Cache.Enabled] toggles the cache, [Config.Sink.Kind]
// selects the sink implementation, [Config.Manifest.Path] pins
// the manifest path. The exported entry deliberately omits the
// override channel so embedder code stays declarative.
func BuildPipeline(
	env *Env,
	cfg *Config,
	plugins []plugin.Plugin,
) (*pipeline.Pipeline, error) {
	return buildPipeline(env, cfg, plugins, pipelineOverride{})
}

func buildPipeline(
	env *Env,
	cfg *Config,
	plugins []plugin.Plugin,
	override pipelineOverride,
) (*pipeline.Pipeline, error) {
	if env.Brand == "" {
		return nil, &ConfigError{Reason: "Env.Brand is required (the consumer's hardcoded tool identity)"}
	}
	preflightWorkspaceCheck(env)
	enabled, err := filterEnabledPlugins(cfg, plugins)
	if err != nil {
		return nil, err
	}
	b := pipeline.New().WithBrand(env.Brand).WithDryRun(override.DryRun)
	for _, p := range enabled {
		registerPlugin(b, p)
	}
	for _, schema := range pluginOptionsFromConfig(cfg) {
		b.WithPluginOptions(schema.name, schema.options)
	}
	s, err := buildSink(env, cfg, override)
	if err != nil {
		return nil, err
	}
	b.WithSink(s)
	// Unconditional by buildCache's own contract: the disabled path
	// answers a NoneCache, never nil.
	b.WithCache(buildCache(env, cfg, override))
	if cfg.Directives.Prefix != "" {
		b.WithDirectivePrefix(cfg.Directives.Prefix)
	}
	for _, ph := range parsePhases(cfg.Parallel) {
		b.WithParallel(ph)
	}
	if mp := manifestPath(env, cfg); mp != "" {
		b.WithManifestPath(mp)
	}
	if env.Diag != nil {
		b.WithDiag(env.Diag)
	}
	if cfg.Verbose || override.Verbose {
		b.WithVerbose(true)
	}
	if env.Workdir != "" {
		// SourceRoot determines how the backend renders origin paths
		// in the rendered "Source:" header line. The CLI defaults it
		// to the working directory so generated headers are
		// repository-relative and byte-stable across machines.
		b.WithSourceRoot(env.Workdir)
	}
	applyOutputConfig(b, cfg)
	override.Routing.Apply(b)
	p, err := b.Build()
	if err != nil {
		return nil, fmt.Errorf("cli: build pipeline: %w", err)
	}
	return p, nil
}

// pipelineOverride bundles the runtime overrides CLI flags supply
// on top of the file-loaded Config — `--no-cache` overrides
// `cache.enabled: true`, `--verbose` wins over the file's value,
// and a non-empty SinkOverride substitutes for the file's sink
// kind (used by [CheckCommand] to swap the disk sink for an
// in-memory sink without touching the file).
type pipelineOverride struct {
	NoCache bool
	Verbose bool
	// SinkOverride, when non-nil, replaces the sink built from the
	// config file. Used by `check` and `prune --dry-run` to swap
	// for an in-memory sink.
	SinkOverride sink.Sink
	// Routing carries the resolved routing-layer flag overrides
	// (post-Infer, post-Validate) that the Command's Execute path
	// applies onto the Builder before [pipeline.Builder.Build]
	// runs.
	Routing RoutingFlags
	// DryRun threads [pipeline.Builder.WithDryRun] through. The
	// `prune --dry-run` flag sets this so the pipeline computes
	// what it WOULD produce without persisting the manifest. Pair
	// with SinkOverride = sink.NewMemory() to also suppress file
	// writes from the backend.
	DryRun bool
}

// filterEnabledPlugins returns the subset of plugins enabled per
// the config. The config's `plugins:` block is an overlay onto
// the consumer's static plugin slice, not an allow-list:
//
//   - A plugin not mentioned in cfg.Plugins is enabled by default
//     (the consumer compiled it in; the absence of a config entry
//     leaves it on).
//   - A plugin mentioned with `enabled: false` is disabled.
//   - A plugin mentioned with no `enabled` field, or `enabled: true`,
//     stays enabled; the config entry exists to attach options or
//     per-plugin output overrides.
//   - A plugin named in cfg.Plugins but missing from the static
//     slice surfaces a [*ConfigError] — the consumer's binary
//     doesn't statically import that plugin.
//
// Preserves the order of the static slice, which the resolver and
// the rendered file's `Plugins:` header observe.
func filterEnabledPlugins(cfg *Config, plugins []plugin.Plugin) ([]plugin.Plugin, error) {
	overlay := make(map[string]ConfigPlugin, len(cfg.Plugins))
	for _, entry := range cfg.Plugins {
		overlay[entry.Name] = entry
	}
	available := make(map[string]struct{}, len(plugins))
	for _, p := range plugins {
		available[p.Name()] = struct{}{}
	}
	for name := range overlay {
		if _, ok := available[name]; !ok {
			return nil, &ConfigError{
				Reason: fmt.Sprintf(
					"plugins[%q]: not registered in the consumer's static plugin slice",
					name,
				),
			}
		}
	}
	out := make([]plugin.Plugin, 0, len(plugins))
	for _, p := range plugins {
		if entry, ok := overlay[p.Name()]; ok && !entry.IsEnabled() {
			continue
		}
		out = append(out, p)
	}
	return out, nil
}

// registerPlugin dispatches p to the Builder method matching its
// role interface(s). One plugin may implement multiple roles
// (e.g. an Annotator that also provides directive schemas via
// DirectiveProvider); each implemented role is registered.
func registerPlugin(b *pipeline.Builder, p plugin.Plugin) {
	if fe, ok := p.(plugin.Frontend); ok {
		b.WithFrontend(fe)
	}
	if ann, ok := p.(plugin.Annotator); ok {
		b.WithAnnotator(ann)
	}
	if gen, ok := p.(plugin.Generator); ok {
		b.WithGenerator(gen)
	}
	if be, ok := p.(plugin.Backend); ok {
		b.WithBackend(be)
	}
}

// pluginOptionsEntry pairs a plugin name with its decoded options
// map. Used by [pluginOptionsFromConfig] to flow file-supplied
// options into the Builder's [Builder.WithPluginOptions] calls.
type pluginOptionsEntry struct {
	name    string
	options map[string]string
}

// pluginOptionsFromConfig walks cfg.Plugins and returns the
// (name, options) pairs for plugins whose entries carry an
// `options:` map. Empty options skip the corresponding
// WithPluginOptions call so the Builder doesn't see noise.
func pluginOptionsFromConfig(cfg *Config) []pluginOptionsEntry {
	out := make([]pluginOptionsEntry, 0, len(cfg.Plugins))
	for _, entry := range cfg.Plugins {
		if !entry.IsEnabled() || len(entry.Options) == 0 {
			continue
		}
		kv := make(map[string]string, len(entry.Options))
		for k, v := range entry.Options {
			kv[k] = fmt.Sprint(v)
		}
		out = append(out, pluginOptionsEntry{name: entry.Name, options: kv})
	}
	return out
}

// buildSink returns the sink the pipeline writes through. When
// override.SinkOverride is non-nil it wins unconditionally (used
// by `check`); otherwise the function constructs one from the
// config file's sink.kind.
func buildSink(env *Env, cfg *Config, override pipelineOverride) (sink.Sink, error) {
	if override.SinkOverride != nil {
		return override.SinkOverride, nil
	}
	switch cfg.Sink.Kind {
	case SinkKindDisk, "":
		return sink.NewDisk(env.Workdir), nil
	case SinkKindMemory:
		return sink.NewMemory(), nil
	case SinkKindStdout:
		return sink.NewStdout(env.Stdout), nil
	default:
		return nil, &ConfigError{
			Reason: fmt.Sprintf("sink.kind %q has no constructor wired in cli; supported: %s, %s, %s",
				cfg.Sink.Kind, SinkKindDisk, SinkKindMemory, SinkKindStdout),
		}
	}
}

// buildCache returns the cache implementation the pipeline uses.
// override.NoCache disables the cache regardless of file settings.
// The pipeline tolerates a NoneCache; no error path exists today,
// so the caller treats the result as unconditional.
func buildCache(env *Env, cfg *Config, override pipelineOverride) cache.Cache {
	if override.NoCache || !cfg.Cache.IsEnabled() {
		return cache.NewNone()
	}
	dir := cfg.Cache.Dir
	if dir == "" {
		dir = env.CacheDir()
	}
	return cache.NewDisk(dir)
}

// applyOutputConfig threads the project-level and per-plugin
// routing configuration from cfg onto b. Project-level fields
// land via [pipeline.Builder.WithProjectOutput]; every plugin
// entry with a non-nil Output block lands via
// [pipeline.Builder.WithPluginOutput] keyed by the plugin name.
// Empty fields short-circuit so no-op layers don't perturb the
// merge's per-field attribution.
//
// CLI-level overrides (`-layout` / `-p` / `-output-dir` /
// `-o` / `-target`) are applied separately by the CLI command
// flag wiring; this helper covers only the config-file
// contribution.
func applyOutputConfig(b *pipeline.Builder, cfg *Config) {
	if !cfg.Output.IsEmpty() {
		b.WithProjectOutput(cfg.Output.Layout, cfg.Output.Package, cfg.Output.Dir)
	}
	for _, p := range cfg.Plugins {
		if p.Output == nil || p.Output.IsEmpty() {
			continue
		}
		b.WithPluginOutput(p.Name, p.Output.Layout, p.Output.Package, p.Output.Dir)
		for tag, tagOut := range p.Output.Tags {
			if tagOut.IsEmpty() {
				continue
			}
			b.WithPluginTagOutput(p.Name, tag, tagOut.Layout, tagOut.Package, tagOut.Dir)
		}
	}
}

// manifestPath returns the path the pipeline writes the manifest
// to. Empty disables manifest writes. The file's
// manifest.path wins when set; otherwise the brand-derived default
// applies.
func manifestPath(env *Env, cfg *Config) string {
	if cfg.Manifest.Path != "" {
		return cfg.Manifest.Path
	}
	return env.ManifestPath()
}

// parsePhases translates the config's parallel-phase names into
// [pipeline.Phase] values, silently ignoring unknown names — the
// validator already enforces known values at LoadConfig time, so
// any unknown here means caller-side hand-crafted Config.
func parsePhases(names []string) []pipeline.Phase {
	out := make([]pipeline.Phase, 0, len(names))
	for _, n := range names {
		switch n {
		case "frontend":
			out = append(out, pipeline.PhaseFrontend)
		case "annotator":
			out = append(out, pipeline.PhaseAnnotator)
		case "generator":
			out = append(out, pipeline.PhaseGenerator)
		}
	}
	return out
}
