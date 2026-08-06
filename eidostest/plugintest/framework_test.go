// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest_test

import (
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"text/template"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/plugin"
)

// TestRunSuite_PassesForWellFormedPlugin pins the happy path of
// the framework conformance suite: the canonical
// [plugintest.FixturePlugin] reference cleanly clears every
// check. The test doubles as a meta-test for the package: any
// suite refactor that breaks the contract surface trips this
// before downstream plugin authors notice.
func TestRunSuite_PassesForWellFormedPlugin(t *testing.T) {
	t.Parallel()

	t.Run("FixturePlugin clears every check", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, plugintest.NewFixturePlugin())
	})

	t.Run("FixturePlugin with empty Version clears the suite", func(t *testing.T) {
		t.Parallel()
		// Empty version is permitted per the [plugin.Versioned]
		// docblock — the plugin opts out of cache integration.
		// The suite must not reject this case.
		p := plugintest.NewFixturePlugin()
		p.VersionString = ""
		plugintest.RunSuite(t, p)
	})

	t.Run("multi-output FixturePlugin clears the suite", func(t *testing.T) {
		t.Parallel()
		// The conformance suite must accept a plugin declaring an
		// ordered set of outputs (primary + tagged secondary) so
		// long as the slice is well-formed: every Suffix
		// non-empty, tags unique, the empty-tag entry at index 0
		// when present.
		plugintest.RunSuite(t, plugintest.NewMultiOutputFixturePlugin())
	})

	t.Run("plugin with every-output-tagged Outputs clears the suite", func(t *testing.T) {
		t.Parallel()
		// A plugin can also declare every output with a non-empty
		// tag (the "no default output" mode). The shape rules
		// permit this — at most one empty-tag output, not
		// exactly one.
		p := plugintest.NewMultiOutputFixturePlugin()
		p.OutputsByLang = map[string][]plugin.Output{
			"go": {
				{Tag: "production", Suffix: "_fixture.go"},
				{Tag: "test", Suffix: "_fixture_test.go"},
			},
		}
		plugintest.RunSuite(t, p)
	})
}

// TestAssertStableName_RejectionPaths covers the two failure
// modes of [plugintest.AssertStableName]: empty identifier and
// mismatched values across calls. Both surface as recorded
// errors on the fake TB.
func TestAssertStableName_RejectionPaths(t *testing.T) {
	t.Parallel()

	t.Run("empty Name is rejected", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		p := plugintest.NewFixturePlugin()
		p.PluginName = ""
		plugintest.AssertStableName(fake, p)
		assertFakeMentions(t, fake, "Plugin.Name returned the empty string")
	})

	t.Run("flapping Name is rejected", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertStableName(fake, &flappingNamePlugin{})
		assertFakeMentions(t, fake, "not stable across calls")
	})
}

// TestAssertImplementsARole_RejectsRoleless covers the role
// probe's only failure mode: a plugin satisfying [plugin.Plugin]
// alone with no role interface. [plugintest.MinimalPlugin] is
// the canonical example.
func TestAssertImplementsARole_RejectsRoleless(t *testing.T) {
	t.Parallel()
	fake := newFakeT()
	plugintest.AssertImplementsARole(fake, plugintest.NewMinimalPlugin("no-role"))
	assertFakeMentions(t, fake, "implements no role interface")
}

// TestAssertCapabilityProviderStability_RejectionPaths pins the
// rejection paths of the capability stability check: empty
// entries in Provides, empty entries in Requires, and lists
// flapping across calls.
func TestAssertCapabilityProviderStability_RejectionPaths(t *testing.T) {
	t.Parallel()

	t.Run("empty Provides entry is rejected", func(t *testing.T) {
		t.Parallel()
		p := plugintest.NewFixturePlugin()
		p.CapabilityProvides = []string{"cap.one", ""}
		fake := newFakeT()
		plugintest.AssertCapabilityProviderStability(fake, p)
		assertFakeMentions(t, fake, "CapabilityProvider.Provides contains the empty string")
	})

	t.Run("empty Requires entry is rejected", func(t *testing.T) {
		t.Parallel()
		p := plugintest.NewFixturePlugin()
		p.CapabilityRequires = []string{""}
		fake := newFakeT()
		plugintest.AssertCapabilityProviderStability(fake, p)
		assertFakeMentions(t, fake, "CapabilityProvider.Requires contains the empty string")
	})

	t.Run("flapping Provides is rejected", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertCapabilityProviderStability(fake, &flappingProvidesPlugin{})
		assertFakeMentions(t, fake, "CapabilityProvider.Provides not stable")
	})
}

// TestAssertDirectiveSchemaUniqueness_RejectionPaths covers the
// empty-name and duplicate-name failure modes.
func TestAssertDirectiveSchemaUniqueness_RejectionPaths(t *testing.T) {
	t.Parallel()

	t.Run("duplicate schema name is rejected", func(t *testing.T) {
		t.Parallel()
		p := plugintest.NewFixturePlugin()
		p.DirectiveSchemas = []directive.Schema{
			directive.NewSchema("foo").On("Struct").Build(),
			directive.NewSchema("foo").On("Interface").Build(),
		}
		fake := newFakeT()
		plugintest.AssertDirectiveSchemaUniqueness(fake, p)
		assertFakeMentions(t, fake, "duplicate schema name")
	})

	t.Run("empty schema name is rejected", func(t *testing.T) {
		t.Parallel()
		p := plugintest.NewFixturePlugin()
		p.DirectiveSchemas = []directive.Schema{{Name: ""}}
		fake := newFakeT()
		plugintest.AssertDirectiveSchemaUniqueness(fake, p)
		assertFakeMentions(t, fake, "empty Name")
	})
}

// TestAssertVersionedStability_RejectsFlapping covers the
// stability rejection for [plugin.Versioned]. Empty is permitted
// and intentionally not a failure mode.
func TestAssertVersionedStability_RejectsFlapping(t *testing.T) {
	t.Parallel()
	fake := newFakeT()
	plugintest.AssertVersionedStability(fake, &flappingVersionPlugin{})
	assertFakeMentions(t, fake, "Versioned.Version not stable")
}

// TestAssertEmitVersionedStability_RejectionPaths covers the
// stability and non-empty-entry rejection paths.
func TestAssertEmitVersionedStability_RejectionPaths(t *testing.T) {
	t.Parallel()

	t.Run("empty entry is rejected", func(t *testing.T) {
		t.Parallel()
		p := plugintest.NewFixturePlugin()
		p.EmitMajors = []string{"1", ""}
		fake := newFakeT()
		plugintest.AssertEmitVersionedStability(fake, p)
		assertFakeMentions(t, fake, "EmitVersioned.EmitVersions contains the empty string")
	})

	t.Run("flapping list is rejected", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertEmitVersionedStability(fake, &flappingEmitVersionsPlugin{})
		assertFakeMentions(t, fake, "EmitVersioned.EmitVersions not stable")
	})
}

// TestAssertNodesOnlyStability_RejectsFlapping covers the
// only failure mode: the declaration flipping between calls.
func TestAssertNodesOnlyStability_RejectsFlapping(t *testing.T) {
	t.Parallel()
	fake := newFakeT()
	plugintest.AssertNodesOnlyStability(fake, &flappingNodesOnlyPlugin{})
	assertFakeMentions(t, fake, "NodesOnly not stable")
}

// TestAssertFilenameProviderStability_RejectsFlapping covers
// the only failure mode of the filename-suffix check.
func TestAssertFilenameProviderStability_RejectsFlapping(t *testing.T) {
	t.Parallel()
	fake := newFakeT()
	plugintest.AssertFilenameProviderStability(fake, &flappingSuffixPlugin{})
	assertFakeMentions(t, fake, "FilenameProvider.Outputs")
}

// TestAssertOutputsShape covers each Outputs-shape rule the
// conformance check enforces — every failure mode reports the
// matching diagnostic; valid slices report nothing.
func TestAssertOutputsShape(t *testing.T) {
	t.Parallel()

	t.Run("rejects an output with empty Suffix", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertOutputsShape(fake, &malformedOutputsPlugin{
			outputs: []plugin.Output{{Suffix: ""}},
		})
		assertFakeMentions(t, fake, "Suffix is required")
	})

	t.Run("rejects duplicate Tag values", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertOutputsShape(fake, &malformedOutputsPlugin{
			outputs: []plugin.Output{
				{Suffix: "_x.go"},
				{Tag: "test", Suffix: "_x_test.go"},
				{Tag: "test", Suffix: "_y_test.go"},
			},
		})
		assertFakeMentions(t, fake, `tag "test" appears at index`)
	})

	t.Run("rejects more than one output with empty Tag", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertOutputsShape(fake, &malformedOutputsPlugin{
			outputs: []plugin.Output{
				{Suffix: "_x.go"},
				{Suffix: "_y.go"},
			},
		})
		assertFakeMentions(t, fake, "outputs declare an empty Tag")
	})

	t.Run("rejects empty-Tag output not at index 0", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertOutputsShape(fake, &malformedOutputsPlugin{
			outputs: []plugin.Output{
				{Tag: "test", Suffix: "_x_test.go"},
				{Suffix: "_x.go"},
			},
		})
		assertFakeMentions(t, fake, "empty-tag output must be declared at index 0")
	})

	t.Run("accepts a well-formed multi-output slice silently", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertOutputsShape(fake, &malformedOutputsPlugin{
			outputs: []plugin.Output{
				{Suffix: "_x.go"},
				{Tag: "test", Suffix: "_x_test.go"},
			},
		})
		if fake.failed {
			t.Fatalf("well-formed Outputs reported a failure: errs=%v fatals=%v", fake.errs, fake.fatals)
		}
	})

	t.Run("non-implementer passes silently", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertOutputsShape(fake, plugintest.NewFixturePlugin())
		// FixturePlugin implements FilenameProvider with a valid
		// single-output slice for "go" only; every other language
		// returns nil and is therefore skipped.
		if fake.failed {
			t.Fatalf("default fixture reported a failure: errs=%v fatals=%v", fake.errs, fake.fatals)
		}
	})
}

// TestAssertTemplateProviderStability covers every rejection path of
// the [plugin.TemplateProvider] contract check plus the two shapes
// that must stay silent.
//
// The four failures are separate subtests rather than one fixture
// violating everything at once because the check reports through
// `Errorf` and keeps going: a fixture breaking two rules would pass
// a one-substring assertion even if only the other rule fired. One
// violation per subtest is what makes "this specific diagnostic
// fires" a claim the test can actually make.
func TestAssertTemplateProviderStability(t *testing.T) {
	t.Parallel()

	t.Run("a flapping ok flag is rejected", func(t *testing.T) {
		t.Parallel()
		var calls int
		fake := newFakeT()
		plugintest.AssertTemplateProviderStability(fake, &templateProviderPlugin{
			name: "flapping-templates",
			templates: func(string) (fs.FS, bool) {
				// Odd call reports "yes, I contribute templates";
				// even call reports "no". The backend queries once
				// per render pass, so a flip makes rendered output
				// depend on how many passes ran before it.
				calls++
				return fstest.MapFS{}, calls%2 == 1
			},
		})
		assertFakeMentions(t, fake, "not stable: first ok=true second ok=false")
	})

	t.Run("a nil filesystem behind a true flag is rejected", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertTemplateProviderStability(fake, &templateProviderPlugin{
			name:      "nil-fs-templates",
			templates: func(string) (fs.FS, bool) { return nil, true },
		})
		assertFakeMentions(t, fake, "reported ok but returned a nil fs.FS")
	})

	t.Run("a filesystem behind a false flag is rejected", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertTemplateProviderStability(fake, &templateProviderPlugin{
			name:      "disowned-fs-templates",
			templates: func(string) (fs.FS, bool) { return fstest.MapFS{}, false },
		})
		assertFakeMentions(t, fake, "returned a filesystem while reporting ok=false")
	})

	t.Run("a name declared as both an extension and an override is rejected", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertTemplateProviderStability(fake, &templateProviderPlugin{
			name:      "double-declared-func",
			funcs:     func(string) template.FuncMap { return template.FuncMap{"shout": strings.ToUpper} },
			overrides: func(string) template.FuncMap { return template.FuncMap{"shout": strings.ToLower} },
		})
		assertFakeMentions(t, fake, `declares "shout" in both TemplateFuncs and TemplateOverrides`)
	})

	t.Run("a well-formed provider passes silently", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertTemplateProviderStability(fake, &templateProviderPlugin{
			name:      "well-formed-templates",
			templates: func(string) (fs.FS, bool) { return fstest.MapFS{}, true },
			funcs:     func(string) template.FuncMap { return template.FuncMap{"shout": strings.ToUpper} },
			overrides: func(string) template.FuncMap { return template.FuncMap{"quiet": strings.ToLower} },
		})
		if fake.failed {
			t.Fatalf("well-formed TemplateProvider reported a failure: errs=%v fatals=%v", fake.errs, fake.fatals)
		}
	})

	t.Run("a plugin that implements no template surface passes silently", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertTemplateProviderStability(fake, plugintest.NewFixturePlugin())
		if fake.failed {
			t.Fatalf("non-implementer reported a failure: errs=%v fatals=%v", fake.errs, fake.fatals)
		}
	})
}

// TestAssertStableFuncMap covers the funcmap helper directly rather
// than only through [plugintest.AssertTemplateProviderStability].
//
// The helper carries two responsibilities the caller depends on and
// which the enclosing check cannot demonstrate on its own: it
// rejects a shifting name set, and it returns the first lookup so
// the caller can cross-check extensions against overrides. A helper
// that reported failures correctly but returned nil would leave the
// both-maps collision check silently inert — the failure mode this
// test exists to foreclose.
func TestAssertStableFuncMap(t *testing.T) {
	t.Parallel()

	t.Run("a shifting name set is rejected", func(t *testing.T) {
		t.Parallel()
		var calls int
		fake := newFakeT()
		plugintest.AssertStableFuncMap(fake, "golang", "TemplateFuncs", func(string) template.FuncMap {
			calls++
			return template.FuncMap{fmt.Sprintf("shout%d", calls): strings.ToUpper}
		})
		assertFakeMentions(t, fake, `TemplateProvider.TemplateFuncs("golang") not stable`)
	})

	t.Run("the empty name is rejected", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertStableFuncMap(fake, "golang", "TemplateOverrides", func(string) template.FuncMap {
			return template.FuncMap{"": strings.ToUpper}
		})
		assertFakeMentions(t, fake, `TemplateProvider.TemplateOverrides("golang") registers the empty name`)
	})

	t.Run("the first lookup is returned so callers can cross-check it", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		got := plugintest.AssertStableFuncMap(fake, "golang", "TemplateFuncs", func(string) template.FuncMap {
			return template.FuncMap{"shout": strings.ToUpper, "quiet": strings.ToLower}
		})
		if fake.failed {
			t.Fatalf("stable funcmap reported a failure: errs=%v fatals=%v", fake.errs, fake.fatals)
		}
		names := slices.Sorted(maps.Keys(got))
		if want := []string{"quiet", "shout"}; !slices.Equal(names, want) {
			t.Errorf("returned funcmap names = %v; want %v", names, want)
		}
	})
}

// assertFakeMentions fails t when none of the recorded error /
// fatal strings in fake contain substr.
func assertFakeMentions(t *testing.T, fake *fakeT, substr string) {
	t.Helper()
	joined := strings.Join(append(fake.errs, fake.fatals...), "\n")
	if !strings.Contains(joined, substr) {
		t.Errorf("fake TB did not record a message mentioning %q; got:\n%s", substr, joined)
	}
}
