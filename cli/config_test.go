// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cli_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/cli"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/manifest"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/plugin"
)

// TestLoadConfig_Defaults covers the empty-path entry point: an
// empty string returns a *Config seeded with the documented
// defaults, never touching disk.
func TestLoadConfig_Defaults(t *testing.T) {
	t.Parallel()

	t.Run("empty path returns DefaultConfig", func(t *testing.T) {
		t.Parallel()
		c, err := cli.LoadConfig("")
		if err != nil {
			t.Fatalf("LoadConfig(\"\") returned %v", err)
		}
		if c.Version != cli.ConfigVersion {
			t.Fatalf("default Version = %d, want %d", c.Version, cli.ConfigVersion)
		}
		if c.Directives.Prefix != directive.DefaultPrefix {
			t.Fatalf(
				"default Directives.Prefix = %q, want %q",
				c.Directives.Prefix,
				directive.DefaultPrefix,
			)
		}
		if c.Sink.Kind != "disk" {
			t.Fatalf("default Sink.Kind = %q, want %q", c.Sink.Kind, "disk")
		}
	})

	t.Run("DefaultConfig has empty Plugins / Sources slices but is non-nil", func(t *testing.T) {
		t.Parallel()
		c := cli.DefaultConfig()
		if c.Sources == nil || len(c.Sources) != 0 {
			t.Fatalf("Sources should be non-nil empty slice; got %v", c.Sources)
		}
	})
}

// TestLoadConfig_YAML covers the on-disk happy path: a valid YAML
// file populates the *Config field-by-field, defaults fill in for
// omitted fields.
func TestLoadConfig_YAML(t *testing.T) {
	t.Parallel()

	t.Run("populated YAML hydrates every field", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, ".eidos.yaml")
		body := []byte(`version: 1
sources:
  - frontend: golang
    patterns: ["./..."]
plugins:
  - name: repogen
    options:
      output_package: repo
  - name: validation
    enabled: false
sink:
  kind: disk
cache:
  enabled: true
  dir: ./build/cache
manifest:
  path: ./.eidos/manifest.json
directives:
  prefix: gen
parallel:
  - annotator
envelope:
  header_prefix: ["// Copyright X"]
verbose: true
`)
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		c, err := cli.LoadConfig(path)
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if len(c.Sources) != 1 || c.Sources[0].Frontend != "golang" {
			t.Fatalf("Sources not populated; got %+v", c.Sources)
		}
		if len(c.Plugins) != 2 || c.Plugins[0].Name != "repogen" {
			t.Fatalf("Plugins not populated; got %+v", c.Plugins)
		}
		if c.Plugins[0].IsEnabled() != true {
			t.Fatalf("repogen should default to enabled")
		}
		if c.Plugins[1].IsEnabled() != false {
			t.Fatalf("validation should be disabled per the file")
		}
		if !c.Verbose {
			t.Fatalf("Verbose should be true per the file")
		}
		if len(c.Envelope.HeaderPrefix) != 1 {
			t.Fatalf("HeaderPrefix not populated; got %v", c.Envelope.HeaderPrefix)
		}
		if c.Cache.Dir != "./build/cache" {
			t.Fatalf("Cache.Dir = %q, want %q", c.Cache.Dir, "./build/cache")
		}
	})

	t.Run("missing file returns ConfigError", func(t *testing.T) {
		t.Parallel()
		_, err := cli.LoadConfig(filepath.Join(t.TempDir(), "missing.yaml"))
		var ce *cli.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("expected *ConfigError; got %T %v", err, err)
		}
	})

	t.Run("unsupported version is rejected", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, ".eidos.yaml")
		if err := os.WriteFile(path, []byte("version: 999\n"), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		_, err := cli.LoadConfig(path)
		var ce *cli.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("expected *ConfigError; got %T %v", err, err)
		}
	})

	t.Run("malformed YAML surfaces as ConfigError", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, ".eidos.yaml")
		if err := os.WriteFile(path, []byte("not: [valid: yaml"), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		_, err := cli.LoadConfig(path)
		var ce *cli.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("expected *ConfigError; got %T %v", err, err)
		}
	})

	t.Run("unknown sink kind is rejected", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, ".eidos.yaml")
		if err := os.WriteFile(path, []byte("version: 1\nsink:\n  kind: invalid\n"), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		_, err := cli.LoadConfig(path)
		var ce *cli.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("expected *ConfigError; got %T %v", err, err)
		}
	})
}

// TestConfigPlugin_IsEnabled covers the ConfigPlugin.IsEnabled
// default: omitted Enabled field treats the plugin as enabled;
// an explicit false disables it.
func TestConfigPlugin_IsEnabled(t *testing.T) {
	t.Parallel()

	t.Run("nil Enabled defaults to true", func(t *testing.T) {
		t.Parallel()
		p := cli.ConfigPlugin{Name: "repogen"}
		if !p.IsEnabled() {
			t.Fatalf("ConfigPlugin without Enabled should default to enabled")
		}
	})

	t.Run("explicit false disables", func(t *testing.T) {
		t.Parallel()
		off := false
		p := cli.ConfigPlugin{Name: "repogen", Enabled: &off}
		if p.IsEnabled() {
			t.Fatalf("ConfigPlugin with Enabled=false should report disabled")
		}
	})
}

// TestConfigCache_IsEnabled mirrors [TestConfigPlugin_IsEnabled]
// for the cache toggle.
func TestConfigCache_IsEnabled(t *testing.T) {
	t.Parallel()

	t.Run("nil Enabled defaults to true", func(t *testing.T) {
		t.Parallel()
		c := cli.ConfigCache{}
		if !c.IsEnabled() {
			t.Fatalf("ConfigCache without Enabled should default to enabled")
		}
	})

	t.Run("explicit false disables", func(t *testing.T) {
		t.Parallel()
		off := false
		c := cli.ConfigCache{Enabled: &off}
		if c.IsEnabled() {
			t.Fatalf("ConfigCache with Enabled=false should report disabled")
		}
	})
}

// TestLoadConfig_ValidationFailures covers each validation-failure
// arm not already exercised by TestLoadConfig_YAML: a source
// missing its frontend name, a plugin missing its name. Each
// surfaces as a *ConfigError naming the offending field.
func TestLoadConfig_ValidationFailures(t *testing.T) {
	t.Parallel()

	t.Run("source without frontend is rejected", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, ".eidos.yaml")
		body := []byte("version: 1\nsources:\n  - patterns: [\"./...\"]\n")
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		_, err := cli.LoadConfig(path)
		var ce *cli.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("expected *ConfigError; got %T %v", err, err)
		}
	})

	t.Run("plugin without name is rejected", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, ".eidos.yaml")
		body := []byte("version: 1\nplugins:\n  - enabled: true\n")
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		_, err := cli.LoadConfig(path)
		var ce *cli.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("expected *ConfigError; got %T %v", err, err)
		}
	})

	t.Run("default values fill in for omitted fields", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, ".eidos.yaml")
		// File omits version, directives.prefix, sink.kind — all
		// should be filled by validateConfig.
		if err := os.WriteFile(path, []byte("verbose: true\n"), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		c, err := cli.LoadConfig(path)
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if c.Version != cli.ConfigVersion {
			t.Fatalf("Version default = %d, want %d", c.Version, cli.ConfigVersion)
		}
		if c.Directives.Prefix != "gen" {
			t.Fatalf("Directives.Prefix default = %q, want %q", c.Directives.Prefix, "gen")
		}
		if c.Sink.Kind != "disk" {
			t.Fatalf("Sink.Kind default = %q, want %q", c.Sink.Kind, "disk")
		}
	})
}

// TestDiscoverConfig covers the walk-up discovery routine: it
// finds the config file in a parent directory, stops at the
// filesystem root.
func TestDiscoverConfig(t *testing.T) {
	t.Parallel()

	t.Run("walks up to find a config file in a parent directory", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		nested := filepath.Join(root, "a", "b", "c")
		if err := os.MkdirAll(nested, 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		cfgPath := filepath.Join(root, ".eidos.yaml")
		if err := os.WriteFile(cfgPath, []byte("version: 1\n"), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		got, ok := cli.DiscoverConfig(nested, ".eidos.yaml")
		if !ok {
			t.Fatalf("DiscoverConfig should have found the config")
		}
		gotAbs, _ := filepath.Abs(got)
		wantAbs, _ := filepath.Abs(cfgPath)
		if gotAbs != wantAbs {
			t.Fatalf("DiscoverConfig got %q, want %q", gotAbs, wantAbs)
		}
	})

	t.Run("returns false when no config is found up to the root", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		nested := filepath.Join(root, "a", "b", "c")
		if err := os.MkdirAll(nested, 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		_, ok := cli.DiscoverConfig(nested, ".nonexistent.yaml")
		if ok {
			t.Fatalf("DiscoverConfig should have returned false for a missing config")
		}
	})
}

// extendedConfig models the embedder pattern [LoadConfigInto]
// supports: inline-embed [cli.Config] under the same YAML
// namespace, add caller-defined extension fields alongside.
type extendedConfig struct {
	cli.Config `yaml:",inline"`

	App appExtras `yaml:"app"`
}

// appExtras is the embedder-side configuration surface — arbitrary
// shape, owned by the embedder.
type appExtras struct {
	Region   string   `yaml:"region"`
	Replicas int      `yaml:"replicas"`
	Tags     []string `yaml:"tags,omitempty"`
}

// TestLoadConfigInto covers the generic-loader path: embedders
// compose their own typed configuration around [cli.Config], parse
// through [LoadConfigInto], then run [ValidateConfig] on the
// embedded portion to share the framework's validation pass.
func TestLoadConfigInto(t *testing.T) {
	t.Parallel()

	t.Run("inline-embedded extension parses alongside the framework keys", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, ".app.yaml")
		body := []byte(`version: 1
sources:
  - frontend: golang
    patterns: ["./..."]
plugins:
  - name: repogen
    options:
      output_package: gen
app:
  region: eu-west-1
  replicas: 3
  tags: ["alpha", "beta"]
`)
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		cfg := &extendedConfig{Config: *cli.DefaultConfig()}
		if err := cli.LoadConfigInto(path, cfg); err != nil {
			t.Fatalf("LoadConfigInto: %v", err)
		}
		if _, err := cli.ValidateConfig(&cfg.Config, path); err != nil {
			t.Fatalf("ValidateConfig: %v", err)
		}
		if cfg.App.Region != "eu-west-1" || cfg.App.Replicas != 3 {
			t.Fatalf("embedder extension not populated: %+v", cfg.App)
		}
		if len(cfg.Sources) != 1 || cfg.Sources[0].Frontend != "golang" {
			t.Fatalf("framework section not populated: %+v", cfg.Sources)
		}
		if cfg.Plugins[0].Name != "repogen" {
			t.Fatalf("framework plugins section not populated: %+v", cfg.Plugins)
		}
	})

	t.Run("empty path leaves the seeded target untouched", func(t *testing.T) {
		t.Parallel()
		cfg := &extendedConfig{
			Config: *cli.DefaultConfig(),
			App:    appExtras{Region: "us-east-1", Replicas: 1},
		}
		if err := cli.LoadConfigInto("", cfg); err != nil {
			t.Fatalf("LoadConfigInto(\"\"): %v", err)
		}
		if cfg.App.Region != "us-east-1" || cfg.App.Replicas != 1 {
			t.Fatalf("seeded App should be preserved; got %+v", cfg.App)
		}
		if cfg.Sink.Kind != "disk" {
			t.Fatalf(
				"seeded framework defaults should be preserved; got Sink.Kind=%q",
				cfg.Sink.Kind,
			)
		}
	})

	t.Run("missing file surfaces a *ConfigError", func(t *testing.T) {
		t.Parallel()
		cfg := &extendedConfig{Config: *cli.DefaultConfig()}
		err := cli.LoadConfigInto(filepath.Join(t.TempDir(), "nope.yaml"), cfg)
		var ce *cli.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("expected *cli.ConfigError; got %T (%v)", err, err)
		}
	})

	t.Run("malformed YAML surfaces a *ConfigError", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, ".bad.yaml")
		if err := os.WriteFile(path, []byte("not: [valid: yaml"), 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		cfg := &extendedConfig{Config: *cli.DefaultConfig()}
		err := cli.LoadConfigInto(path, cfg)
		var ce *cli.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("expected *cli.ConfigError; got %T (%v)", err, err)
		}
	})
}

// TestValidateConfig_OutputBlock covers the routing-layer config
// validation: the layout-enum check, the centralised-requires-
// package rule, and the dir-without-centralised warning. Each
// rule is exercised at project level and at per-plugin level.
func TestValidateConfig_OutputBlock(t *testing.T) {
	t.Parallel()

	t.Run("unknown project layout value is rejected", func(t *testing.T) {
		t.Parallel()
		c := cli.DefaultConfig()
		c.Output = cli.ConfigOutput{Layout: "bogus"}
		_, err := cli.ValidateConfig(c, "")
		var ce *cli.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("expected *cli.ConfigError; got %v", err)
		}
		if !strings.Contains(ce.Reason, "output.layout") {
			t.Fatalf("error should name output.layout; got %q", ce.Reason)
		}
	})

	t.Run("centralised without package fails at project level", func(t *testing.T) {
		t.Parallel()
		c := cli.DefaultConfig()
		c.Output = cli.ConfigOutput{Layout: pipeline.LayoutCentralised}
		_, err := cli.ValidateConfig(c, "")
		var ce *cli.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("expected *cli.ConfigError; got %v", err)
		}
		if !strings.Contains(ce.Reason, "output.package") {
			t.Fatalf("error should name output.package; got %q", ce.Reason)
		}
	})

	t.Run("centralised with package validates clean", func(t *testing.T) {
		t.Parallel()
		c := cli.DefaultConfig()
		c.Output = cli.ConfigOutput{
			Layout: pipeline.LayoutCentralised, Package: "gen",
		}
		warnings, err := cli.ValidateConfig(c, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(warnings) != 0 {
			t.Fatalf("unexpected warnings: %v", warnings)
		}
	})

	t.Run("dir without centralised surfaces a warning", func(t *testing.T) {
		t.Parallel()
		c := cli.DefaultConfig()
		c.Output = cli.ConfigOutput{Dir: "internal/gen"}
		warnings, err := cli.ValidateConfig(c, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], "output.dir") {
			t.Fatalf("expected output.dir warning; got %v", warnings)
		}
	})

	t.Run("per-plugin centralised inherits project package", func(t *testing.T) {
		t.Parallel()
		c := cli.DefaultConfig()
		c.Output = cli.ConfigOutput{Package: "gen"}
		c.Plugins = []cli.ConfigPlugin{
			{Name: "mockgen", Output: &cli.ConfigOutput{Layout: pipeline.LayoutCentralised}},
		}
		warnings, err := cli.ValidateConfig(c, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(warnings) != 0 {
			t.Fatalf("unexpected warnings: %v", warnings)
		}
	})

	t.Run("per-plugin centralised without inherited package fails", func(t *testing.T) {
		t.Parallel()
		c := cli.DefaultConfig()
		c.Plugins = []cli.ConfigPlugin{
			{Name: "mockgen", Output: &cli.ConfigOutput{Layout: pipeline.LayoutCentralised}},
		}
		_, err := cli.ValidateConfig(c, "")
		var ce *cli.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("expected *cli.ConfigError; got %v", err)
		}
		if !strings.Contains(ce.Reason, "plugins[0].output") {
			t.Fatalf("error should name plugins[0].output; got %q", ce.Reason)
		}
	})

	t.Run("per-plugin dir without centralised surfaces a warning", func(t *testing.T) {
		t.Parallel()
		c := cli.DefaultConfig()
		c.Plugins = []cli.ConfigPlugin{
			{Name: "mockgen", Output: &cli.ConfigOutput{Dir: "internal/mocks"}},
		}
		warnings, err := cli.ValidateConfig(c, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(warnings) != 1 || !strings.Contains(warnings[0], "plugins[0].output.dir") {
			t.Fatalf("expected plugins[0].output.dir warning; got %v", warnings)
		}
	})

	t.Run("per-plugin unknown layout value is rejected", func(t *testing.T) {
		t.Parallel()
		c := cli.DefaultConfig()
		c.Plugins = []cli.ConfigPlugin{
			{Name: "mockgen", Output: &cli.ConfigOutput{Layout: "weird"}},
		}
		_, err := cli.ValidateConfig(c, "")
		var ce *cli.ConfigError
		if !errors.As(err, &ce) {
			t.Fatalf("expected *cli.ConfigError; got %v", err)
		}
		if !strings.Contains(ce.Reason, "plugins[0].output.layout") {
			t.Fatalf("error should name plugins[0].output.layout; got %q", ce.Reason)
		}
	})
}

// TestBuildPipeline_OutputConfigThreaded pins the wiring contract:
// project-level and per-plugin output blocks loaded from a Config
// reach the constructed [pipeline.Pipeline] and surface through
// [pipeline.Pipeline.LayoutPolicyFor] for both the run-wide
// default and per-plugin lookups.
func TestBuildPipeline_OutputConfigThreaded(t *testing.T) {
	t.Parallel()

	t.Run("project + per-plugin output config flow to LayoutPolicyFor", func(t *testing.T) {
		t.Parallel()
		env, _, _ := freshEnv(t, "eidos")
		cfg := cli.DefaultConfig()
		cfg.Output = cli.ConfigOutput{
			Layout: pipeline.LayoutCentralised, Package: "gen", Dir: "internal/gen",
		}
		cfg.Plugins = []cli.ConfigPlugin{
			{Name: "fe"},
			{Name: "mockgen", Output: &cli.ConfigOutput{
				Layout: pipeline.LayoutCentralised, Package: "mocks", Dir: "internal/mocks",
			}},
			{Name: "repogen"},
			{Name: "be"},
		}
		p, err := cli.BuildPipeline(env, cfg, []plugin.Plugin{
			stubFrontend{name: "fe"},
			stubGenerator{name: "mockgen"},
			stubGenerator{name: "repogen"},
			stubBackend{name: "be", lang: "stub"},
		})
		if err != nil {
			t.Fatalf("BuildPipeline: %v", err)
		}
		// Mockgen has a per-plugin override → its Layout policy
		// carries the per-plugin fields (under per-plugin
		// attribution).
		got := p.LayoutPolicyFor("mockgen")
		if got.Package != "mocks" || got.PackageFrom != manifest.LayerPerPlugin {
			t.Errorf(
				"mockgen Package = %q from %q, want mocks from per-plugin",
				got.Package,
				got.PackageFrom,
			)
		}
		if got.Dir != "internal/mocks" || got.DirFrom != manifest.LayerPerPlugin {
			t.Errorf(
				"mockgen Dir = %q from %q, want internal/mocks from per-plugin",
				got.Dir,
				got.DirFrom,
			)
		}
		// Repogen has no per-plugin override → its policy is the
		// project-level merge.
		got = p.LayoutPolicyFor("repogen")
		if got.Package != "gen" || got.PackageFrom != manifest.LayerProject {
			t.Errorf(
				"repogen Package = %q from %q, want gen from project",
				got.Package,
				got.PackageFrom,
			)
		}
		if got.Dir != "internal/gen" || got.DirFrom != manifest.LayerProject {
			t.Errorf(
				"repogen Dir = %q from %q, want internal/gen from project",
				got.Dir,
				got.DirFrom,
			)
		}
	})

	t.Run("per-tag output config flows to LayoutPolicyForTag", func(t *testing.T) {
		t.Parallel()
		env, _, _ := freshEnv(t, "eidos")
		cfg := cli.DefaultConfig()
		cfg.Plugins = []cli.ConfigPlugin{
			{Name: "fe"},
			{Name: "mockgen", Output: &cli.ConfigOutput{
				Layout:  pipeline.LayoutCentralised,
				Package: "mocks",
				Dir:     "internal/mocks",
				Tags: map[string]cli.ConfigOutput{
					"test": {
						Layout:  pipeline.LayoutCentralised,
						Package: "mockstest",
						Dir:     "internal/mockstest",
					},
				},
			}},
			{Name: "be"},
		}
		p, err := cli.BuildPipeline(env, cfg, []plugin.Plugin{
			stubFrontend{name: "fe"},
			stubGenerator{name: "mockgen"},
			stubBackend{name: "be", lang: "stub"},
		})
		if err != nil {
			t.Fatalf("BuildPipeline: %v", err)
		}
		// Primary output keeps the per-plugin block.
		primary := p.LayoutPolicyForTag("mockgen", "")
		if primary.Package != "mocks" || primary.Dir != "internal/mocks" {
			t.Errorf("primary policy = %+v, want Package=mocks Dir=internal/mocks", primary)
		}
		// Tagged output picks up the per-tag block.
		tagged := p.LayoutPolicyForTag("mockgen", "test")
		if tagged.Package != "mockstest" || tagged.Dir != "internal/mockstest" {
			t.Errorf("tagged policy = %+v, want Package=mockstest Dir=internal/mockstest", tagged)
		}
		// A tag not declared in the config falls back to the
		// per-plugin block.
		other := p.LayoutPolicyForTag("mockgen", "other")
		if other.Package != "mocks" || other.Dir != "internal/mocks" {
			t.Errorf("undeclared-tag policy = %+v, want fallback to per-plugin", other)
		}
	})

	t.Run("empty output config leaves the framework default in place", func(t *testing.T) {
		t.Parallel()
		env, _, _ := freshEnv(t, "eidos")
		cfg := cli.DefaultConfig()
		p, err := cli.BuildPipeline(env, cfg, []plugin.Plugin{
			stubFrontend{name: "fe"},
			stubBackend{name: "be", lang: "stub"},
		})
		if err != nil {
			t.Fatalf("BuildPipeline: %v", err)
		}
		got := p.LayoutPolicyFor("anything")
		switch {
		case got.Layout != pipeline.LayoutAlongsideSource,
			got.LayoutFrom != manifest.LayerFramework:
			t.Fatalf("default policy = %+v, want framework alongside-source", got)
		}
	})
}

// TestConfig_OutputBlock_RoundTrip pins the YAML serialisation
// contract: a config carrying every documented output-block
// field marshals, re-loads, and re-marshals byte-identically.
// Embedders and tools that round-trip configs through YAML rely
// on this stability — a field rename or tag drift would surface
// here immediately.
func TestConfig_OutputBlock_RoundTrip(t *testing.T) {
	t.Parallel()

	t.Run("project + per-plugin output round-trip preserves every field", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, ".eidos.yaml")
		body := []byte(`version: 1
output:
  layout: centralised
  package: gen
  dir: internal/gen
plugins:
  - name: mockgen
    output:
      layout: centralised
      package: mocks
      dir: internal/mocks
  - name: repogen
    output:
      package: repos
`)
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		cfg, err := cli.LoadConfig(path)
		if err != nil {
			t.Fatalf("LoadConfig: %v", err)
		}
		if cfg.Output.Layout != pipeline.LayoutCentralised {
			t.Errorf("Output.Layout = %q, want centralised", cfg.Output.Layout)
		}
		if cfg.Output.Package != "gen" {
			t.Errorf("Output.Package = %q, want gen", cfg.Output.Package)
		}
		if cfg.Output.Dir != "internal/gen" {
			t.Errorf("Output.Dir = %q, want internal/gen", cfg.Output.Dir)
		}
		if len(cfg.Plugins) != 2 {
			t.Fatalf("Plugins len = %d, want 2", len(cfg.Plugins))
		}
		mock := cfg.Plugins[0]
		if mock.Output == nil {
			t.Fatalf("mockgen.Output should be non-nil")
		}
		switch {
		case mock.Output.Layout != pipeline.LayoutCentralised,
			mock.Output.Package != "mocks",
			mock.Output.Dir != "internal/mocks":
			t.Errorf("mockgen.Output = %+v", *mock.Output)
		}
		repo := cfg.Plugins[1]
		if repo.Output == nil {
			t.Fatalf("repogen.Output should be non-nil")
		}
		if repo.Output.Package != "repos" {
			t.Errorf("repogen.Output.Package = %q, want repos", repo.Output.Package)
		}
	})
}

// TestBuildPipeline_PluginOverlay pins the overlay semantics for
// the config's `plugins:` block: plugins not mentioned stay enabled
// (the consumer's static slice is the default-enabled universe);
// only entries with `enabled: false` disable a plugin. This lets a
// user attach per-plugin options or output overrides without
// implicitly disabling every other statically-compiled plugin.
func TestBuildPipeline_PluginOverlay(t *testing.T) {
	t.Parallel()

	t.Run(
		"per-plugin output override leaves unlisted plugins enabled",
		func(t *testing.T) {
			t.Parallel()
			env, _, _ := freshEnv(t, "eidos")
			cfg := cli.DefaultConfig()
			// The config only mentions one plugin — attaching an output
			// override. Other registered plugins should remain enabled.
			cfg.Plugins = []cli.ConfigPlugin{
				{Name: "mockgen", Output: &cli.ConfigOutput{
					Layout: pipeline.LayoutCentralised, Package: "mocks",
				}},
			}
			p, err := cli.BuildPipeline(env, cfg, []plugin.Plugin{
				stubFrontend{name: "fe"},
				stubGenerator{name: "repogen"},
				stubGenerator{name: "mockgen"},
				stubBackend{name: "be", lang: "stub"},
			})
			if err != nil {
				t.Fatalf("BuildPipeline: %v", err)
			}
			gens := p.Generators()
			names := make([]string, 0, len(gens))
			for _, g := range gens {
				names = append(names, g.Name())
			}
			wantSet := map[string]bool{"repogen": true, "mockgen": true}
			for _, n := range names {
				if !wantSet[n] {
					t.Errorf("unexpected generator enabled: %q", n)
				}
				delete(wantSet, n)
			}
			if len(wantSet) != 0 {
				t.Fatalf("generators not enabled under overlay: %v (got %v)", wantSet, names)
			}
		},
	)

	t.Run("enabled:false disables a plugin while others stay on", func(t *testing.T) {
		t.Parallel()
		env, _, _ := freshEnv(t, "eidos")
		cfg := cli.DefaultConfig()
		off := false
		cfg.Plugins = []cli.ConfigPlugin{
			{Name: "mockgen", Enabled: &off},
		}
		p, err := cli.BuildPipeline(env, cfg, []plugin.Plugin{
			stubFrontend{name: "fe"},
			stubGenerator{name: "repogen"},
			stubGenerator{name: "mockgen"},
			stubBackend{name: "be", lang: "stub"},
		})
		if err != nil {
			t.Fatalf("BuildPipeline: %v", err)
		}
		for _, g := range p.Generators() {
			if g.Name() == "mockgen" {
				t.Fatalf("mockgen should be disabled by enabled:false; found in Generators")
			}
		}
		var sawRepogen bool
		for _, g := range p.Generators() {
			if g.Name() == "repogen" {
				sawRepogen = true
			}
		}
		if !sawRepogen {
			t.Fatalf("repogen should stay enabled (not mentioned in config)")
		}
	})
}

// FuzzValidateConfig drives the hand-rolled config validator over
// the routing-layer rule matrix.
//
// The YAML layer itself is a plain `yaml.Unmarshal` into a struct
// and carries no bespoke grammar, so it is not the interesting
// surface. [cli.ValidateConfig] is: it hand-rolls a precedence
// merge (per-plugin layout and package fall back to the
// project-level block) on top of an enum check, and it doubles as
// the normaliser that fills Version, Sink.Kind, and
// Directives.Prefix. A wrong merge does not crash — it silently
// accepts a `centralised` plugin with no resolvable package, which
// the Layout phase then routes into a package named "".
//
// Four properties run per input:
//
//   - Differential. [refValidateOutputRules] re-derives the
//     documented rules independently; its accept verdict must match
//     ValidateConfig's on every combination.
//   - Warning count on the accepted path, which is the only path
//     where the count is part of the contract rather than an
//     artefact of where validation stopped.
//   - Normalisation. Every accepted config leaves with a known
//     Version, a non-empty Sink.Kind, and a non-empty directive
//     prefix — the guarantee the doc comment makes to callers that
//     skip the loader.
//   - Idempotence. Re-validating the config ValidateConfig just
//     normalised must reach the same verdict with the same
//     warnings. A default filled on the first pass that flips a
//     rule on the second would make `LoadConfig` order-dependent.
//
// The typed arguments stand in for the fields the rules read; the
// seeds walk the enum values, the empty string, the merge's
// fall-through cases, and the one-off garbage values that must be
// rejected.
func FuzzValidateConfig(f *testing.F) {
	for _, seed := range []struct {
		version                          int
		projLayout, projPackage, projDir string
		plugLayout, plugPackage, plugDir string
	}{
		{1, "", "", "", "", "", ""},
		{0, "", "", "", "", "", ""},
		{2, "", "", "", "", "", ""},
		{-1, "", "", "", "", "", ""},
		{1, "centralised", "gen", "", "", "", ""},
		{1, "centralised", "", "", "", "", ""},
		{1, "alongside-source", "", "out", "", "", ""},
		{1, "", "", "", "centralised", "", ""},
		{1, "", "gen", "", "centralised", "", ""},
		{1, "centralised", "gen", "", "alongside-source", "", "d"},
		{1, "weird", "", "", "", "", ""},
		{1, "", "", "", "weird", "", ""},
		{1, "", "", "d", "", "", "e"},
		{1, "CENTRALISED", "gen", "", "", "", ""},
		{1, " centralised", "gen", "", "", "", ""},
	} {
		f.Add(seed.version, seed.projLayout, seed.projPackage, seed.projDir,
			seed.plugLayout, seed.plugPackage, seed.plugDir)
	}

	f.Fuzz(func(
		t *testing.T,
		version int,
		projLayout, projPackage, projDir string,
		plugLayout, plugPackage, plugDir string,
	) {
		cfg := cli.DefaultConfig()
		cfg.Version = version
		cfg.Output = cli.ConfigOutput{Layout: projLayout, Package: projPackage, Dir: projDir}
		// The plugin name is pinned non-empty so the "plugins[i]:
		// name is required" rule never fires and the only rejection
		// this target observes comes from the output-block rules.
		cfg.Plugins = []cli.ConfigPlugin{{
			Name:   "p",
			Output: &cli.ConfigOutput{Layout: plugLayout, Package: plugPackage, Dir: plugDir},
		}}

		wantWarnings, wantOK := refValidateOutputRules(
			version, projLayout, projPackage, projDir, plugLayout, plugPackage, plugDir,
		)

		warnings, err := cli.ValidateConfig(cfg, "fuzz.yaml")
		if (err == nil) != wantOK {
			t.Fatalf(
				"ValidateConfig(version=%d, proj=%q/%q/%q, plug=%q/%q/%q): err = %v, reference accepts = %v",
				version, projLayout, projPackage, projDir,
				plugLayout, plugPackage, plugDir, err, wantOK,
			)
		}
		if !wantOK {
			var ce *cli.ConfigError
			if !errsAs(err, &ce) {
				t.Fatalf("rejection should be a *cli.ConfigError; got %T (%v)", err, err)
			}
			return
		}
		if len(warnings) != wantWarnings {
			t.Fatalf("warnings = %d (%v), reference expects %d", len(warnings), warnings, wantWarnings)
		}
		// Normalisation contract for the accepted path.
		if cfg.Version != cli.ConfigVersion {
			t.Fatalf("accepted config left Version = %d, want %d", cfg.Version, cli.ConfigVersion)
		}
		if cfg.Sink.Kind == "" {
			t.Fatalf("accepted config left Sink.Kind empty")
		}
		if cfg.Directives.Prefix == "" {
			t.Fatalf("accepted config left Directives.Prefix empty")
		}
		// Idempotence over the config the first pass normalised.
		again, err := cli.ValidateConfig(cfg, "fuzz.yaml")
		if err != nil {
			t.Fatalf("re-validating a normalised config failed: %v", err)
		}
		if len(again) != len(warnings) {
			t.Fatalf("re-validation produced %d warnings (%v), first pass produced %d (%v)",
				len(again), again, len(warnings), warnings)
		}
	})
}

// refValidateOutputRules is the deliberately naive re-derivation of
// the routing-layer config rules used by [FuzzValidateConfig].
//
// It reads as a flat transcription of the documented rule list
// rather than as the production function's structure — no shared
// enum helper, no shared merge helper — so a bug in the precedence
// merge shows up as a disagreement instead of being mirrored.
//
// Returns the number of warnings the accepted path must produce and
// whether the combination is accepted at all. The warning count is
// only meaningful when ok is true: the production validator returns
// whatever warnings it had accumulated when a plugin-level rule
// rejected, which is a stopping artefact rather than a contract.
func refValidateOutputRules(
	version int,
	projLayout, projPackage, projDir string,
	plugLayout, plugPackage, plugDir string,
) (warnings int, ok bool) {
	// Version 0 is normalised to the current version before the
	// comparison, so only a non-zero mismatch rejects.
	if version != 0 && version != cli.ConfigVersion {
		return 0, false
	}
	if !refKnownLayout(projLayout) {
		return 0, false
	}
	if projLayout == pipeline.LayoutCentralised && projPackage == "" {
		return 0, false
	}
	if projDir != "" && projLayout != pipeline.LayoutCentralised {
		warnings++
	}
	if !refKnownLayout(plugLayout) {
		return warnings, false
	}
	effLayout := plugLayout
	if effLayout == "" {
		effLayout = projLayout
	}
	effPackage := plugPackage
	if effPackage == "" {
		effPackage = projPackage
	}
	if effLayout == pipeline.LayoutCentralised && effPackage == "" {
		return warnings, false
	}
	if plugDir != "" && effLayout != pipeline.LayoutCentralised {
		warnings++
	}
	return warnings, true
}

// refKnownLayout reports whether layout is one of the two documented
// selectors or the empty string that defers to the layer below.
func refKnownLayout(layout string) bool {
	return layout == "" ||
		layout == pipeline.LayoutAlongsideSource ||
		layout == pipeline.LayoutCentralised
}
