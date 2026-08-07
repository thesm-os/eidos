// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package-internal tests for the config → pipeline translation.
//
// [buildSink], [buildCache], [parsePhases], [manifestPath] and
// [pluginOptionsFromConfig] each pick one thing out of the config
// and hand it to the pipeline builder. The choice is not readable
// back off the constructed *pipeline.Pipeline, so a blackbox test
// can only observe that a pipeline was built — not that it was
// built from the settings the user wrote. Those decisions are
// pinned here; command behaviour stays blackbox.
package cli

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/sink"
)

// testEnv returns an Env rooted at a fresh temp dir.
func testEnv(t *testing.T) *Env {
	t.Helper()
	return &Env{
		Brand:   "eidos",
		Workdir: t.TempDir(),
		Stdout:  &bytes.Buffer{},
		Stderr:  &bytes.Buffer{},
	}
}

func TestBuildSink(t *testing.T) {
	t.Parallel()

	t.Run("an unset kind defaults to the disk sink", func(t *testing.T) {
		t.Parallel()
		got, err := buildSink(testEnv(t), &Config{}, pipelineOverride{})
		if err != nil {
			t.Fatalf("buildSink: %v", err)
		}
		if _, ok := got.(*sink.Disk); !ok {
			t.Fatalf("buildSink = %T, want *sink.Disk", got)
		}
	})

	t.Run("an explicit disk kind selects the disk sink", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{Sink: ConfigSink{Kind: SinkKindDisk}}
		got, err := buildSink(testEnv(t), cfg, pipelineOverride{})
		if err != nil {
			t.Fatalf("buildSink: %v", err)
		}
		if _, ok := got.(*sink.Disk); !ok {
			t.Fatalf("buildSink = %T, want *sink.Disk", got)
		}
	})

	t.Run("the memory kind selects the memory sink", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{Sink: ConfigSink{Kind: SinkKindMemory}}
		got, err := buildSink(testEnv(t), cfg, pipelineOverride{})
		if err != nil {
			t.Fatalf("buildSink: %v", err)
		}
		if _, ok := got.(*sink.Memory); !ok {
			t.Fatalf("buildSink = %T, want *sink.Memory", got)
		}
	})

	t.Run("the stdout kind selects the stdout sink", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{Sink: ConfigSink{Kind: SinkKindStdout}}
		got, err := buildSink(testEnv(t), cfg, pipelineOverride{})
		if err != nil {
			t.Fatalf("buildSink: %v", err)
		}
		if _, ok := got.(*sink.Stdout); !ok {
			t.Fatalf("buildSink = %T, want *sink.Stdout", got)
		}
	})

	t.Run("an unknown kind is a config error naming the supported set", func(t *testing.T) {
		t.Parallel()
		// LoadConfig rejects unknown kinds, so reaching here means a
		// hand-built Config. The error still has to name the options
		// rather than fail with a nil sink further downstream.
		cfg := &Config{Sink: ConfigSink{Kind: "s3"}}
		_, err := buildSink(testEnv(t), cfg, pipelineOverride{})
		var cfgErr *ConfigError
		if !errors.As(err, &cfgErr) {
			t.Fatalf("buildSink error = %v, want *ConfigError", err)
		}
		if !bytes.Contains([]byte(cfgErr.Reason), []byte(SinkKindMemory)) {
			t.Fatalf("error must name the supported kinds; got %q", cfgErr.Reason)
		}
	})

	t.Run("an override wins over the configured kind", func(t *testing.T) {
		t.Parallel()
		// `check` swaps in an in-memory sink so it can compare
		// rendered bytes against disk without writing. A config that
		// said "disk" must not defeat that.
		mem := sink.NewMemory()
		cfg := &Config{Sink: ConfigSink{Kind: SinkKindDisk}}
		got, err := buildSink(testEnv(t), cfg, pipelineOverride{SinkOverride: mem})
		if err != nil {
			t.Fatalf("buildSink: %v", err)
		}
		if got != mem {
			t.Fatalf("buildSink = %T, want the override instance", got)
		}
	})
}

func TestBuildCache(t *testing.T) {
	t.Parallel()

	t.Run("an unset config enables the disk cache", func(t *testing.T) {
		t.Parallel()
		got := buildCache(testEnv(t), &Config{}, pipelineOverride{})
		if _, ok := got.(*cache.Disk); !ok {
			t.Fatalf("buildCache = %T, want *cache.Disk (enabled by default)", got)
		}
	})

	t.Run("the no-cache override disables the cache", func(t *testing.T) {
		t.Parallel()
		// `--no-cache` has to win over a config that enables it,
		// otherwise the flag silently does nothing.
		cfg := &Config{Cache: ConfigCache{Enabled: new(true)}}
		got := buildCache(testEnv(t), cfg, pipelineOverride{NoCache: true})
		if _, ok := got.(*cache.None); !ok {
			t.Fatalf("buildCache = %T, want *cache.None", got)
		}
	})

	t.Run("an explicitly disabled cache is disabled", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{Cache: ConfigCache{Enabled: new(false)}}
		got := buildCache(testEnv(t), cfg, pipelineOverride{})
		if _, ok := got.(*cache.None); !ok {
			t.Fatalf("buildCache = %T, want *cache.None", got)
		}
	})

	t.Run("an explicitly enabled cache is the disk cache", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{Cache: ConfigCache{Enabled: new(true)}}
		got := buildCache(testEnv(t), cfg, pipelineOverride{})
		if _, ok := got.(*cache.Disk); !ok {
			t.Fatalf("buildCache = %T, want *cache.Disk", got)
		}
	})

	t.Run("a configured directory is honoured", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{Cache: ConfigCache{Dir: t.TempDir()}}
		got := buildCache(testEnv(t), cfg, pipelineOverride{})
		if _, ok := got.(*cache.Disk); !ok {
			t.Fatalf("buildCache = %T, want *cache.Disk", got)
		}
	})
}

func TestManifestPath(t *testing.T) {
	t.Parallel()

	t.Run("a configured path wins over the brand default", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{Manifest: ConfigManifest{Path: "custom/manifest.json"}}
		if got := manifestPath(testEnv(t), cfg); got != "custom/manifest.json" {
			t.Fatalf("manifestPath = %q, want the configured path", got)
		}
	})

	t.Run("an unset path falls back to the brand default", func(t *testing.T) {
		t.Parallel()
		env := testEnv(t)
		want := filepath.Join(env.Workdir, ".eidos", "manifest.json")
		if got := manifestPath(env, &Config{}); got != want {
			t.Fatalf("manifestPath = %q, want %q", got, want)
		}
	})
}

func TestParsePhases(t *testing.T) {
	t.Parallel()

	// The phase names are the config file's spelling of
	// [pipeline.Phase]. Bound once here so the mapping under test is
	// stated in one place.
	const (
		nameFrontend  = "frontend"
		nameAnnotator = "annotator"
		nameGenerator = "generator"
	)

	t.Run("translates every known phase name", func(t *testing.T) {
		t.Parallel()
		got := parsePhases([]string{nameFrontend, nameAnnotator, nameGenerator})
		want := []pipeline.Phase{
			pipeline.PhaseFrontend,
			pipeline.PhaseAnnotator,
			pipeline.PhaseGenerator,
		}
		if len(got) != len(want) {
			t.Fatalf("parsePhases = %v, want %v", got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("parsePhases[%d] = %v, want %v", i, got[i], want[i])
			}
		}
	})

	t.Run("preserves the declared order", func(t *testing.T) {
		t.Parallel()
		got := parsePhases([]string{nameGenerator, nameFrontend})
		if len(got) != 2 || got[0] != pipeline.PhaseGenerator {
			t.Fatalf("parsePhases = %v, want generator first", got)
		}
	})

	t.Run("silently drops an unknown phase name", func(t *testing.T) {
		t.Parallel()
		// LoadConfig validates the names, so an unknown one here
		// means a hand-built Config; dropping it keeps the known
		// phases parallel rather than failing the whole run.
		got := parsePhases([]string{nameFrontend, "backend", "sink"})
		if len(got) != 1 || got[0] != pipeline.PhaseFrontend {
			t.Fatalf("parsePhases = %v, want only the frontend phase", got)
		}
	})

	t.Run("no names yields an empty slice", func(t *testing.T) {
		t.Parallel()
		if got := parsePhases(nil); len(got) != 0 {
			t.Fatalf("parsePhases(nil) = %v, want empty", got)
		}
	})
}

func TestPluginOptionsFromConfig(t *testing.T) {
	t.Parallel()

	t.Run("carries options for an enabled plugin", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{Plugins: []ConfigPlugin{
			{Name: "repogen", Options: map[string]any{"pkg": "repo"}},
		}}
		got := pluginOptionsFromConfig(cfg)
		if len(got) != 1 || got[0].name != "repogen" || got[0].options["pkg"] != "repo" {
			t.Fatalf("pluginOptionsFromConfig = %+v, want repogen pkg=repo", got)
		}
	})

	t.Run("stringifies a non-string option value", func(t *testing.T) {
		t.Parallel()
		// YAML yields ints and bools as typed values, but the plugin
		// options contract is map[string]string — an un-stringified
		// value would reach the plugin's schema as the wrong type.
		cfg := &Config{Plugins: []ConfigPlugin{
			{Name: "enumgen", Options: map[string]any{"depth": 3, "strict": true}},
		}}
		got := pluginOptionsFromConfig(cfg)
		if len(got) != 1 {
			t.Fatalf("pluginOptionsFromConfig = %+v, want one entry", got)
		}
		if got[0].options["depth"] != "3" || got[0].options["strict"] != "true" {
			t.Fatalf("options = %+v, want stringified values", got[0].options)
		}
	})

	t.Run("skips a disabled plugin", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{Plugins: []ConfigPlugin{
			{Name: "repogen", Enabled: new(false), Options: map[string]any{"pkg": "repo"}},
		}}
		if got := pluginOptionsFromConfig(cfg); len(got) != 0 {
			t.Fatalf("a disabled plugin must contribute no options; got %+v", got)
		}
	})

	t.Run("skips an enabled plugin carrying no options", func(t *testing.T) {
		t.Parallel()
		cfg := &Config{Plugins: []ConfigPlugin{{Name: "repogen"}}}
		if got := pluginOptionsFromConfig(cfg); len(got) != 0 {
			t.Fatalf("an option-less plugin must contribute no entry; got %+v", got)
		}
	})
}
