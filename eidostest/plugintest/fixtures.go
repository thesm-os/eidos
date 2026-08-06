// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest

import (
	"fmt"
	"slices"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/priority"
)

// FixturePlugin is the canonical well-formed plugin the package
// ships as a reference implementation. The default value
// returned from [NewFixturePlugin] satisfies every optional role
// interface the framework conformance suite probes — stable
// [plugin.Plugin.Name], at-least-one role (Generator),
// deterministic [plugin.CapabilityProvider], unique
// [plugin.DirectiveProvider] schemas, non-empty
// [plugin.Versioned], well-formed [plugin.EmitVersioned], stable
// [plugin.NodesOnly], and stable [plugin.FilenameProvider].
//
// Plugin authors use [FixturePlugin] two ways:
//
//   - As a contract reference: the source illustrates which
//     methods the framework's suite exercises and what their
//     return-value shape looks like for a "passes every check"
//     plugin.
//   - As a meta-test fixture: passing it to [RunSuite]
//     should always pass — a useful smoke test that the
//     suite implementation itself remains coherent across
//     refactors.
//
// Mutate the exported fields between construction and use to
// configure non-default behaviour. The struct is intentionally
// data-only (no behaviour beyond the role-interface methods) so
// tweaks remain obvious; downstream tests needing real
// behaviour build their own plugin and run the suite against it
// instead.
type FixturePlugin struct {
	// PluginName is the identifier returned by [FixturePlugin.Name].
	PluginName string

	// PluginPriority is the bucket returned by
	// [FixturePlugin.Priority].
	PluginPriority priority.Priority

	// CapabilityProvides backs [FixturePlugin.Provides].
	CapabilityProvides []string

	// CapabilityRequires backs [FixturePlugin.Requires].
	CapabilityRequires []string

	// DirectiveSchemas backs [FixturePlugin.Directives].
	DirectiveSchemas []directive.Schema

	// VersionString backs [FixturePlugin.Version]. Empty is a
	// valid declaration ("opt out of cache integration") per the
	// [plugin.Versioned] docblock.
	VersionString string

	// EmitMajors backs [FixturePlugin.EmitVersions].
	EmitMajors []string

	// OutputsByLang maps target language to the Outputs slice
	// [FixturePlugin.Outputs] returns. Each entry contributes one
	// rendered file when the backend matches the language key.
	OutputsByLang map[string][]plugin.Output

	// NodesOnlyDecl backs [FixturePlugin.NodesOnly].
	NodesOnlyDecl bool
}

// NewFixturePlugin returns a [FixturePlugin] pre-populated with
// values that clear every framework-conformance check. Mutate
// the exported fields before passing to a suite when a specific
// scenario needs tweaking.
func NewFixturePlugin() *FixturePlugin {
	return &FixturePlugin{
		PluginName:         "fixture",
		PluginPriority:     priority.GeneratorFoundation,
		CapabilityProvides: []string{fixtureCapability},
		CapabilityRequires: []string{"cap.zero"},
		DirectiveSchemas: []directive.Schema{
			directive.NewSchema("foo").On("Struct").Build(),
			directive.NewSchema("bar").On("Interface").Build(),
		},
		VersionString: "v1.0.0",
		EmitMajors:    []string{"1"},
		OutputsByLang: map[string][]plugin.Output{
			// Keyed on the language the suite actually probes. It read
			// "go" while every real plugin answers to "golang", which
			// is why the Outputs-shape check passed against this
			// fixture and iterated an empty slice for everything else.
			ConformanceLanguage: {{Suffix: "_fixture.go"}},
		},
		NodesOnlyDecl: true,
	}
}

// NewMultiOutputFixturePlugin returns a [FixturePlugin] declaring
// the canonical multi-output shape — a primary (empty-tag) output
// plus one tagged "test" output for the "go" language. The
// fixture satisfies every framework-conformance check the same
// way [NewFixturePlugin] does; the extra Output entry exercises
// the multi-output validation path in the conformance suite
// (well-formed slice, deterministic per-language return, primary
// at index 0) without surfacing a different shape elsewhere.
//
// Plugin authors building a real multi-output plugin use this
// fixture as a contract reference for the [plugin.Output] slice
// the [plugin.FilenameProvider.Outputs] method should return.
func NewMultiOutputFixturePlugin() *FixturePlugin {
	p := NewFixturePlugin()
	p.PluginName = "multi-output-fixture"
	p.OutputsByLang = map[string][]plugin.Output{
		"go": {
			{Suffix: "_fixture.go"},
			{Tag: fixtureTag, Suffix: "_fixture_test.go"},
		},
	}
	return p
}

// Name returns the configured identifier.
func (p *FixturePlugin) Name() string { return p.PluginName }

// Generate satisfies [plugin.Generator]; the fixture exists
// only as a contract vehicle and performs no work.
func (*FixturePlugin) Generate(_ *plugin.GeneratorContext) error { return nil }

// Priority satisfies [plugin.CapabilityProvider].
func (p *FixturePlugin) Priority() priority.Priority { return p.PluginPriority }

// Provides satisfies [plugin.CapabilityProvider].
func (p *FixturePlugin) Provides() []string { return slices.Clone(p.CapabilityProvides) }

// Requires satisfies [plugin.CapabilityProvider].
func (p *FixturePlugin) Requires() []string { return slices.Clone(p.CapabilityRequires) }

// Directives satisfies [plugin.DirectiveProvider].
func (p *FixturePlugin) Directives() []directive.Schema { return slices.Clone(p.DirectiveSchemas) }

// Version satisfies [plugin.Versioned].
func (p *FixturePlugin) Version() string { return p.VersionString }

// EmitVersions satisfies [plugin.EmitVersioned].
func (p *FixturePlugin) EmitVersions() []string { return p.EmitMajors }

// Outputs satisfies [plugin.FilenameProvider].
func (p *FixturePlugin) Outputs(lang string) []plugin.Output {
	return slices.Clone(p.OutputsByLang[lang])
}

// NodesOnly satisfies [plugin.NodesOnly].
func (p *FixturePlugin) NodesOnly() bool { return p.NodesOnlyDecl }

// MinimalPlugin is the smallest possible plugin: satisfies
// [plugin.Plugin] alone, with no role interface. The framework
// conformance suite's role probe rejects this — useful both as
// a negative-path reference for the suite's behaviour and as a
// starting point for downstream tests that need a stub
// [plugin.Plugin] value without any role baggage.
type MinimalPlugin struct {
	// PluginName is the identifier returned by [MinimalPlugin.Name].
	PluginName string
}

// NewMinimalPlugin returns a [MinimalPlugin] with the supplied
// name.
func NewMinimalPlugin(name string) *MinimalPlugin {
	return &MinimalPlugin{PluginName: name}
}

// Name returns the configured identifier.
func (p *MinimalPlugin) Name() string { return p.PluginName }

// OptionsFixturePlugin is the reference plugin for exercising
// [plugin.OptionsProvider] flows. It binds a small options
// struct via [opt.Reflect] and surfaces the decoded values
// through [OptionsFixturePlugin.Opts] so [RunOptionsSuite] can
// assert defaults / required / one-of round-trip from the
// schema declaration directly.
type OptionsFixturePlugin struct {
	// PluginName is the identifier returned by Name.
	PluginName string

	// Opts is the decoded options struct, populated by
	// [OptionsFixturePlugin.SetOptions]. Tests inspect this
	// field to verify the round-trip.
	Opts OptionsFixtureOpts
}

// OptionsFixtureOpts is the typed options the
// [OptionsFixturePlugin] binds. The tags exercise the three
// schema features the options suite checks: required (no
// default), one-of with default, and a free-text option that
// falls back to the field's zero value when unset.
type OptionsFixtureOpts struct {
	// Output is required; absence triggers
	// [opt.ErrMissingRequired].
	Output string `eidos:"output_package,required"`

	// Mode is one of {fast, safe}; absence falls back to safe.
	Mode string `eidos:"mode,one_of=fast|safe,default=safe"`

	// Label is a free-text option with no default; absence
	// leaves the field at its Go zero value.
	Label string `eidos:"label"`
}

// NewOptionsFixturePlugin returns an [OptionsFixturePlugin]
// with the supplied name and a zero-valued options struct.
func NewOptionsFixturePlugin(name string) *OptionsFixturePlugin {
	return &OptionsFixturePlugin{PluginName: name}
}

// Name returns the configured identifier.
func (p *OptionsFixturePlugin) Name() string { return p.PluginName }

// Generate satisfies [plugin.Generator]; the fixture exists
// only as an options-round-trip vehicle.
func (*OptionsFixturePlugin) Generate(_ *plugin.GeneratorContext) error { return nil }

// OptionsSchema returns the reflected schema of
// [OptionsFixtureOpts].
func (*OptionsFixturePlugin) OptionsSchema() opt.Schema { return opt.Reflect(OptionsFixtureOpts{}) }

// SetOptions decodes opts into the plugin's options struct.
// Validation errors surface verbatim; the suite asserts the
// expected sentinel via [errors.Is].
func (p *OptionsFixturePlugin) SetOptions(opts opt.Options) error {
	if err := opts.Decode(&p.Opts); err != nil {
		return fmt.Errorf("OptionsFixturePlugin: SetOptions: %w", err)
	}
	return nil
}
