// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest

import (
	"errors"
	"maps"
	"slices"
	"testing"

	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/plugin"
)

// OptionsFixture describes the inputs the [RunOptionsSuite]
// drives a [plugin.OptionsProvider] against. The Valid map is
// the canonical "everything supplied" set the plugin should
// accept; UnknownKey is a key absent from the schema that the
// strict-decode path should reject with [opt.ErrUnknownField].
//
// Plugins whose schemas have no required fields can leave Valid
// empty — the suite still exercises the empty-input
// (defaults-only) path. Plugins whose schemas have required
// fields must populate Valid with at least every required name
// so the happy-path probe doesn't trip on the missing-required
// check before it reaches the unknown-key probe.
type OptionsFixture struct {
	// Valid is the "all valid values" map the suite calls
	// SetOptions with on the happy-path probe. The plugin
	// should accept this without error.
	Valid map[string]string

	// UnknownKey is a key absent from the plugin's schema. The
	// suite drives SetOptions with a map containing this key
	// (plus the Valid entries, to keep required fields
	// satisfied) and asserts the call returns
	// [opt.ErrUnknownField].
	UnknownKey string
}

// RunOptionsSuite runs the conformance checks every
// [plugin.OptionsProvider] must satisfy: OptionsSchema returns
// the same schema across calls (the pipeline reads it once at
// Build time); SetOptions accepts the empty input map (all
// optional defaults / required-field rejection) cleanly;
// SetOptions accepts the supplied Valid map; SetOptions rejects
// the UnknownKey-augmented map with [opt.ErrUnknownField].
//
// Plugins that do not implement [plugin.OptionsProvider] are
// not the target of this suite; passing one to RunOptionsSuite
// fails the build with a positioned diagnostic via [t.Fatalf].
//
// The Valid map should satisfy every required field the
// schema declares; the suite checks the schema's required-field
// set against Valid and reports a fixture-shape failure when
// they don't agree, so the rejection-path checks aren't masked
// by a missing-required error.
func RunOptionsSuite(t *testing.T, p plugin.Plugin, fixture OptionsFixture) {
	t.Helper()
	provider, ok := any(p).(plugin.OptionsProvider)
	if !ok {
		t.Fatalf("RunOptionsSuite: plugin %T does not implement plugin.OptionsProvider", p)
	}
	t.Run("OptionsSchema returns a stable schema across calls", func(t *testing.T) {
		assertOptionsSchemaStability(t, provider)
	})
	t.Run("fixture covers every required schema field", func(t *testing.T) {
		assertOptionsFixtureCoversRequired(t, provider, fixture)
	})
	t.Run("SetOptions accepts the supplied Valid values", func(t *testing.T) {
		assertSetOptionsAcceptsValid(t, provider, fixture)
	})
	t.Run("SetOptions rejects a map omitting every required field", func(t *testing.T) {
		assertSetOptionsRejectsMissingRequired(t, provider, fixture)
	})
	t.Run("SetOptions rejects an UnknownKey with ErrUnknownField", func(t *testing.T) {
		assertSetOptionsRejectsUnknown(t, provider, fixture)
	})
}

// assertOptionsSchemaStability calls OptionsSchema twice and
// fails when the field set differs across calls. The schema is
// derived at Build time and the pipeline assumes it stays
// constant across runs; a schema that changed between calls
// would surface as inconsistent validation behaviour.
func assertOptionsSchemaStability(tb testing.TB, p plugin.OptionsProvider) {
	tb.Helper()
	first := p.OptionsSchema().Names()
	second := p.OptionsSchema().Names()
	if !slices.Equal(first, second) {
		tb.Errorf("OptionsSchema field set not stable across calls: first=%v second=%v", first, second)
	}
}

// assertOptionsFixtureCoversRequired fails when the fixture's
// Valid map omits a required field. The rejection-path checks
// downstream rely on the happy-path SetOptions call clearing
// the schema; a missing-required error would mask whatever the
// rejection-path probe is actually trying to surface.
func assertOptionsFixtureCoversRequired(tb testing.TB, p plugin.OptionsProvider, fx OptionsFixture) {
	tb.Helper()
	for _, f := range p.OptionsSchema().Fields {
		if !f.Required {
			continue
		}
		if _, ok := fx.Valid[f.Name]; !ok {
			tb.Errorf(
				"OptionsFixture.Valid is missing required field %q; "+
					"populate it so downstream rejection probes aren't masked by ErrMissingRequired",
				f.Name,
			)
		}
	}
}

// assertSetOptionsAcceptsValid drives SetOptions with the
// fixture's Valid map and fails when it returns a non-nil
// error. The Valid map represents the canonical success case;
// a rejection here points at a fixture mismatch or a schema
// bug.
func assertSetOptionsAcceptsValid(tb testing.TB, p plugin.OptionsProvider, fx OptionsFixture) {
	tb.Helper()
	if err := p.SetOptions(opt.New(p.OptionsSchema(), fx.Valid)); err != nil {
		tb.Errorf("SetOptions rejected the Valid fixture values: %v (values=%v)", err, fx.Valid)
	}
}

// assertSetOptionsRejectsUnknown drives SetOptions with the
// fixture's Valid map plus a key not declared in the schema
// and fails when the call returns nil or returns an error not
// wrapping [opt.ErrUnknownField]. The strict-unknown contract
// catches config-file typos at decode time rather than
// silently dropping the offending entry.
//
// The probe key is the fixture's UnknownKey when it declares one and
// a synthesised absent name otherwise.
//
// It used to return silently when UnknownKey was empty — printing PASS
// under a subtest name asserting rejection, having asserted nothing.
// The stated rationale was that a plugin's schema might cover "every
// plausible name", which cannot be true: the schema enumerates its own
// fields, so a name outside that set is always constructible. A
// SetOptions that is `return nil` — never reaching opt.Decode, and so
// never rejecting anything — cleared this check whenever the author
// omitted one optional fixture field.
func assertSetOptionsRejectsUnknown(tb testing.TB, p plugin.OptionsProvider, fx OptionsFixture) {
	tb.Helper()
	probeKey := fx.UnknownKey
	if probeKey == "" {
		probeKey = synthesiseUnknownKey(p.OptionsSchema())
	}
	values := make(map[string]string, len(fx.Valid)+1)
	maps.Copy(values, fx.Valid)
	values[probeKey] = "any"
	err := p.SetOptions(opt.New(p.OptionsSchema(), values))
	if err == nil {
		tb.Errorf(
			"SetOptions accepted an unknown key %q; the strict-decode contract "+
				"requires every input key to match a declared field",
			probeKey,
		)
		return
	}
	if !errors.Is(err, opt.ErrUnknownField) {
		tb.Errorf(
			"SetOptions rejected the unknown key %q but the error did not wrap opt.ErrUnknownField: %v",
			probeKey, err,
		)
	}
}

// synthesiseUnknownKey returns a field name the schema does not
// declare, so the unknown-key probe runs whether or not the fixture
// author supplied one.
//
// The schema enumerates its own fields, which makes an absent name
// trivially constructible: take a name no author would choose and
// extend it until it collides with nothing.
func synthesiseUnknownKey(schema opt.Schema) string {
	declared := make(map[string]struct{}, len(schema.Fields))
	for _, name := range schema.Names() {
		declared[name] = struct{}{}
	}
	candidate := "plugintest_no_such_field"
	for {
		if _, taken := declared[candidate]; !taken {
			return candidate
		}
		candidate += "_x"
	}
}

// assertSetOptionsRejectsMissingRequired drives SetOptions with every
// required field omitted and fails when the call succeeds.
//
// The suite held both halves of this contract and checked neither: it
// forbids the fixture from omitting a required field
// ([assertOptionsFixtureCoversRequired]), which is right for the
// probes downstream of it, but that left opt.ErrMissingRequired
// asserted nowhere. A plugin that never reaches opt.Decode satisfies
// every other check in this suite.
//
// Schemas declaring no required field have nothing to omit, so the
// check reports that rather than passing over it.
func assertSetOptionsRejectsMissingRequired(tb testing.TB, p plugin.OptionsProvider, fx OptionsFixture) {
	tb.Helper()
	schema := p.OptionsSchema()

	var required []string
	for _, f := range schema.Fields {
		if f.Required {
			required = append(required, f.Name)
		}
	}
	if len(required) == 0 {
		tb.Skipf("schema declares no required field; there is nothing to omit")
		return
	}

	values := make(map[string]string, len(fx.Valid))
	maps.Copy(values, fx.Valid)
	for _, name := range required {
		delete(values, name)
	}

	err := p.SetOptions(opt.New(schema, values))
	if err == nil {
		tb.Errorf(
			"SetOptions accepted a map omitting every required field (%v); a plugin that never "+
				"reaches opt.Decode silently runs on defaults the author never chose",
			required,
		)
		return
	}
	if !errors.Is(err, opt.ErrMissingRequired) {
		tb.Errorf(
			"SetOptions rejected the missing-required map but the error did not wrap "+
				"opt.ErrMissingRequired: %v",
			err,
		)
	}
}
