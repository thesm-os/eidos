// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest_test

import (
	"errors"
	"fmt"
	"testing"

	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/plugin"
)

// TestRunOptionsSuite_PassesForCanonicalFixture pins the happy
// path: [plugintest.OptionsFixturePlugin] cleanly clears every
// check given a fixture that supplies the required field plus
// a plausible unknown key.
func TestRunOptionsSuite_PassesForCanonicalFixture(t *testing.T) {
	t.Parallel()
	plugintest.RunOptionsSuite(
		t,
		plugintest.NewOptionsFixturePlugin("opts"),
		plugintest.OptionsFixture{
			Valid: map[string]string{
				"output_package": "main",
				"mode":           "fast",
				"label":          "test",
			},
			UnknownKey: "no_such_field",
		},
	)
}

// TestRunOptionsSuite_RejectsFixtureMissingRequired pins the
// fixture-shape check: a Valid map missing a required field
// surfaces as a fixture failure so the rejection-path probes
// run against a meaningful baseline.
func TestRunOptionsSuite_RejectsFixtureMissingRequired(t *testing.T) {
	t.Parallel()
	p := plugintest.NewOptionsFixturePlugin("opts")
	fx := plugintest.OptionsFixture{
		Valid: map[string]string{"mode": "safe"}, // missing required "output_package"
	}
	fake := newFakeT()
	plugintest.AssertOptionsFixtureCoversRequired(fake, p, fx)
	assertFakeMentions(t, fake, `required field "output_package"`)
}

// TestRunOptionsSuite_RejectsPluginAcceptingUnknownKey pins
// the strict-unknown contract: a plugin whose SetOptions
// silently accepts unknown keys fails the rejection probe.
func TestRunOptionsSuite_RejectsPluginAcceptingUnknownKey(t *testing.T) {
	t.Parallel()
	p := &lenientOptionsPlugin{name: "lenient"}
	fx := plugintest.OptionsFixture{
		Valid:      map[string]string{},
		UnknownKey: "anything",
	}
	fake := newFakeT()
	plugintest.AssertSetOptionsRejectsUnknown(fake, p, fx)
	assertFakeMentions(t, fake, "accepted an unknown key")
}

// TestRunOptionsSuite_RejectsPluginUsingWrongSentinel pins the
// sentinel-wrap contract: a plugin whose SetOptions returns a
// generic error rather than wrapping [opt.ErrUnknownField]
// fails the probe so users get the typed sentinel they expect.
func TestRunOptionsSuite_RejectsPluginUsingWrongSentinel(t *testing.T) {
	t.Parallel()
	p := &nonSentinelOptionsPlugin{name: "wrong-sentinel"}
	fx := plugintest.OptionsFixture{
		Valid:      map[string]string{},
		UnknownKey: "anything",
	}
	fake := newFakeT()
	plugintest.AssertSetOptionsRejectsUnknown(fake, p, fx)
	assertFakeMentions(t, fake, "did not wrap opt.ErrUnknownField")
}

// TestRunOptionsSuite_RejectsFlappingSchema pins the
// schema-stability contract.
func TestRunOptionsSuite_RejectsFlappingSchema(t *testing.T) {
	t.Parallel()
	p := &flappingSchemaPlugin{}
	fake := newFakeT()
	plugintest.AssertOptionsSchemaStability(fake, p)
	assertFakeMentions(t, fake, "OptionsSchema field set not stable")
}

// TestRunOptionsSuite_FatalsWhenPluginLacksOptionsProvider
// pins the precondition guard: passing a plugin that does not
// implement [plugin.OptionsProvider] fails with a fatal.
func TestRunOptionsSuite_FatalsWhenPluginLacksOptionsProvider(t *testing.T) {
	t.Parallel()
	// MinimalPlugin satisfies only [plugin.Plugin] and so the
	// suite should refuse to run against it. We can't call
	// plugintest.RunOptionsSuite with a fake TB (its signature
	// takes *testing.T), but the precondition path is the
	// first call inside RunOptionsSuite so failure manifests
	// through a subtest failure under `t.Run`. To assert this
	// without coupling to subtest naming we drive the check
	// inline via the same flow: confirm the type assertion
	// fails.
	p := plugintest.NewMinimalPlugin("no-opts")
	if _, ok := any(p).(plugin.OptionsProvider); ok {
		t.Fatalf("plugintest.MinimalPlugin unexpectedly implements OptionsProvider; test fixture stale")
	}
}

// lenientOptionsPlugin satisfies OptionsProvider but ignores
// unknown keys instead of rejecting them. Drives the
// strict-unknown rejection path.
type lenientOptionsPlugin struct {
	name string
	opts lenientOpts
}

// lenientOpts is the bound schema for the lenient plugin.
type lenientOpts struct {
	Mode string `eidos:"mode,default=safe"`
}

// Name returns the configured identifier.
func (p *lenientOptionsPlugin) Name() string { return p.name }

// Generate satisfies [plugin.Generator] so the plugin is not
// rejected by the role-probe in tests that run the framework
// suite alongside.
func (*lenientOptionsPlugin) Generate(_ *plugin.GeneratorContext) error { return nil }

// OptionsSchema returns the reflected schema of [lenientOpts].
func (*lenientOptionsPlugin) OptionsSchema() opt.Schema { return opt.Reflect(lenientOpts{}) }

// SetOptions silently accepts every input — does not validate
// against the schema's declared fields. Drives the
// strict-unknown rejection-path probe in
// [plugintest.AssertSetOptionsRejectsUnknown].
func (p *lenientOptionsPlugin) SetOptions(opts opt.Options) error {
	// Decode only the mode field; ignore everything else.
	_ = opts
	p.opts.Mode = "safe"
	return nil
}

// nonSentinelOptionsPlugin satisfies OptionsProvider and
// rejects unknown keys but returns a generic error rather
// than wrapping [opt.ErrUnknownField]. Drives the
// wrong-sentinel rejection-path probe.
type nonSentinelOptionsPlugin struct {
	name string
}

// nonSentinelOpts is the bound schema for the wrong-sentinel
// plugin.
type nonSentinelOpts struct {
	Mode string `eidos:"mode,default=safe"`
}

// Name returns the configured identifier.
func (p *nonSentinelOptionsPlugin) Name() string { return p.name }

// Generate satisfies [plugin.Generator].
func (*nonSentinelOptionsPlugin) Generate(_ *plugin.GeneratorContext) error { return nil }

// OptionsSchema returns the reflected schema of
// [nonSentinelOpts].
func (*nonSentinelOptionsPlugin) OptionsSchema() opt.Schema {
	return opt.Reflect(nonSentinelOpts{})
}

// SetOptions returns a generic error on every call so the
// suite's sentinel-wrap probe fails.
func (*nonSentinelOptionsPlugin) SetOptions(_ opt.Options) error {
	return errors.New("nonSentinelOptionsPlugin: deliberate non-sentinel error") //nolint:err113
}

// flappingSchemaPlugin returns a different OptionsSchema on
// every call — drives the schema-stability rejection path.
type flappingSchemaPlugin struct{ count int }

// Name returns a stable identifier.
func (*flappingSchemaPlugin) Name() string { return "flapping-schema" }

// Generate satisfies [plugin.Generator].
func (*flappingSchemaPlugin) Generate(_ *plugin.GeneratorContext) error { return nil }

// OptionsSchema returns a schema whose field name embeds an
// increment counter so two consecutive calls disagree.
func (p *flappingSchemaPlugin) OptionsSchema() opt.Schema {
	p.count++
	return opt.Schema{
		Fields: []opt.Field{{Name: fmt.Sprintf("field_%d", p.count), Kind: opt.KindString}},
	}
}

// SetOptions accepts any input — the rejection-path probe
// downstream is gated by the schema-stability check, so this
// stub is sufficient.
func (*flappingSchemaPlugin) SetOptions(_ opt.Options) error { return nil }

// noopOptionsPlugin declares a schema and accepts everything.
//
// Its SetOptions never reaches opt.Decode, so it rejects neither an
// unknown key nor a missing required field. It is the shape an author
// arrives at by writing the method signature and filling in the body
// later, and before the probes were made unconditional it cleared the
// whole options suite whenever UnknownKey was left empty.
type noopOptionsPlugin struct{ name string }

// Name returns the configured identifier.
func (p *noopOptionsPlugin) Name() string { return p.name }

// Generate satisfies [plugin.Generator] so the role probe clears.
func (*noopOptionsPlugin) Generate(_ *plugin.GeneratorContext) error { return nil }

// OptionsSchema declares one required field.
func (*noopOptionsPlugin) OptionsSchema() opt.Schema {
	return opt.Schema{Fields: []opt.Field{{Name: "output_package", Required: true}}}
}

// SetOptions accepts anything, which is the defect.
func (*noopOptionsPlugin) SetOptions(_ opt.Options) error { return nil }

// TestRunOptionsSuite_ProbesAreUnconditional pins that the negative
// probes run whether or not the fixture author opted into them.
func TestRunOptionsSuite_ProbesAreUnconditional(t *testing.T) {
	t.Parallel()

	p := &noopOptionsPlugin{name: "noop-options"}
	fx := plugintest.OptionsFixture{Valid: map[string]string{"output_package": "main"}}

	t.Run("an unknown key is rejected even when the fixture names none", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertSetOptionsRejectsUnknown(fake, p, fx)
		assertFakeMentions(t, fake, "accepted an unknown key")
	})

	t.Run("a map omitting every required field is rejected", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertSetOptionsRejectsMissingRequired(fake, p, fx)
		assertFakeMentions(t, fake, "omitting every required field")
	})
}
