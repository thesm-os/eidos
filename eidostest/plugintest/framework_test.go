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
		// Keyed on ConformanceLanguage: under the old "go" key the
		// suite probed a language this map has no entry for, so the
		// every-output-tagged shape this subtest exists to bless was
		// never actually presented to a single check.
		p := plugintest.NewMultiOutputFixturePlugin()
		p.OutputsByLang = map[string][]plugin.Output{
			plugintest.ConformanceLanguage: {
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

	t.Run("a plugin that implements no template surface reports the check as skipped", func(t *testing.T) {
		t.Parallel()
		// This subtest used to assert the check "passes silently",
		// which was the defect rather than the contract: a silent pass
		// is indistinguishable from a plugin whose templates were all
		// validated, so an author could not tell which of the suite's
		// checks had examined anything. A non-implementer must still
		// not fail — it is not doing anything wrong — but it must say
		// so.
		fake := newFakeT()
		captureFatal(func() {
			plugintest.AssertTemplateProviderStability(fake, plugintest.NewFixturePlugin())
		})
		if fake.failed {
			t.Fatalf("non-implementer reported a failure: errs=%v fatals=%v", fake.errs, fake.fatals)
		}
		if len(fake.skips) == 0 {
			t.Fatalf("non-implementer must report the check as skipped; recorded nothing")
		}
		if !strings.Contains(fake.skips[0], "plugin.TemplateProvider") {
			t.Errorf("the skip must name the absent capability; got %q", fake.skips[0])
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

// nestedTemplateFS returns a Templates hook shipping body at a path
// one directory below the filesystem root — the shape produced by
// `//go:embed templates/golang/*.tmpl` without the matching
// [fs.Sub]. It is the documented idiom's failure mode, and the only
// reason no in-tree plugin exposes it is that every one of them
// remembers the fs.Sub.
func nestedTemplateFS(body string) func(string) (fs.FS, bool) {
	return func(lang string) (fs.FS, bool) {
		if lang != plugintest.ConformanceLanguage {
			return nil, false
		}
		return fstest.MapFS{
			"templates/golang/nested.tmpl": &fstest.MapFile{Data: []byte(body)},
		}, true
	}
}

// TestAssertTemplatesParse_WalksNestedDirectories pins that the
// template check enumerates the filesystem the way the backend does.
// The backend walks recursively; a root-only glob silently validated
// nothing for any plugin that forgot an fs.Sub, and reported the same
// green as one whose templates were all checked.
func TestAssertTemplatesParse_WalksNestedDirectories(t *testing.T) {
	t.Parallel()

	t.Run("an unparsable template below the filesystem root is reported", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertTemplatesParse(fake, &templateProviderPlugin{
			name:      "nested-unparsable",
			templates: nestedTemplateFS(`{{ define "fixture.bad" }}{{ .Unclosed `),
		})
		assertFakeMentions(t, fake, "does not parse")
	})

	t.Run("a reserved-prefix definition below the filesystem root is reported", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertTemplatesParse(fake, &templateProviderPlugin{
			name:      "nested-reserved",
			templates: nestedTemplateFS(`{{ define "fragment.hijack" }}x{{ end }}`),
		})
		assertFakeMentions(t, fake, "reserved")
	})

	t.Run("a well-formed template below the filesystem root is accepted", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertTemplatesParse(fake, &templateProviderPlugin{
			name:      "nested-ok",
			templates: nestedTemplateFS(`{{ define "fixture.ok" }}ok{{ end }}`),
		})
		if fake.failed {
			t.Errorf("a parsable nested template must clear the check; got: %v", fake.errs)
		}
	})
}

// TestAssertTemplateFuncsAvoidReservedNames pins the funcmap half of
// the same divergence. The backend rejects a plugin funcmap entry
// colliding with its own reserved set, and does so from
// mergePluginContributions — which returns before renderTargets, so
// the whole run writes zero files for every plugin in the
// composition. Catching it at conformance time is the difference
// between one author's failing test and a broken build for everyone
// composing with them.
func TestAssertTemplateFuncsAvoidReservedNames(t *testing.T) {
	t.Parallel()

	fixed := func(fm template.FuncMap) func(string) template.FuncMap {
		return func(lang string) template.FuncMap {
			if lang != plugintest.ConformanceLanguage {
				return nil
			}
			return fm
		}
	}
	noop := func(_ ...any) any { return nil }

	t.Run("a TemplateFuncs entry claiming a reserved core name is reported", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertTemplateFuncsAvoidReservedNames(fake, &templateProviderPlugin{
			name:  "reserved-extension",
			funcs: fixed(template.FuncMap{"imp": noop}),
		})
		assertFakeMentions(t, fake, "imp")
	})

	t.Run("a TemplateOverrides entry targeting a reserved core name is reported", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertTemplateFuncsAvoidReservedNames(fake, &templateProviderPlugin{
			name:      "reserved-override",
			overrides: fixed(template.FuncMap{"render": noop}),
		})
		assertFakeMentions(t, fake, "render")
	})

	t.Run("a name outside the reserved set is accepted", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertTemplateFuncsAvoidReservedNames(fake, &templateProviderPlugin{
			name:      "own-names",
			funcs:     fixed(template.FuncMap{"myPluginHelper": noop}),
			overrides: fixed(template.FuncMap{"lowerCamel": noop}),
		})
		if fake.failed {
			t.Errorf("names outside the reserved set must clear the check; got: %v", fake.errs)
		}
	})
}

// rustOnlyPlugin declares a malformed Outputs slice for a non-Go
// backend and nothing for the Go one — the shape any plugin targeting
// a language this repository does not ship takes.
type rustOnlyPlugin struct{ name string }

// Name returns the configured identifier.
func (p *rustOnlyPlugin) Name() string { return p.name }

// Generate satisfies [plugin.Generator] so the role probe clears.
func (*rustOnlyPlugin) Generate(_ *plugin.GeneratorContext) error { return nil }

// Outputs declares a slice violating three shape rules at once — an
// empty Suffix, a duplicate Tag, and the primary entry away from
// index 0 — under a language the default probe set never asks about.
func (*rustOnlyPlugin) Outputs(lang string) []plugin.Output {
	if lang != "rust" {
		return nil
	}
	return []plugin.Output{
		{Tag: "dup", Suffix: "_a.rs"},
		{Tag: "dup", Suffix: ""},
		{Suffix: "_primary.rs"},
	}
}

// TestRunSuiteFor_ProbesTheCallersLanguage pins that a plugin
// targeting a non-Go backend gets its per-language checks actually
// run.
//
// probeLanguages was a package-private constant pair, so every
// per-language lookup asked a plugin about "golang" and nothing else.
// A plugin keyed on any other spelling answered no probe: its Outputs
// were validated as an empty slice and the checks reported green
// having examined nothing, right up until Build rejected the same
// slice in the author's own project.
func TestRunSuiteFor_ProbesTheCallersLanguage(t *testing.T) {
	t.Parallel()

	p := &rustOnlyPlugin{name: "rust-only"}

	t.Run("the default probe set validates an empty slice and reports nothing", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertOutputsShape(fake, p)
		if fake.failed {
			t.Fatalf("the Go-only probe set cannot see a rust plugin's outputs: %v", fake.errs)
		}
	})

	t.Run("naming the plugin's own language surfaces the malformed slice", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertOutputsShape(fake, p, "rust")
		if !fake.failed {
			t.Fatalf("a malformed rust Outputs slice must fail once rust is probed")
		}
	})
}

// tmplPlugin is a template provider whose template body and funcmap
// the caller supplies, so a case varies exactly one of the two.
type tmplPlugin struct {
	plugintest.FixturePlugin
	body      string
	funcs     template.FuncMap
	overrides template.FuncMap
}

func (p *tmplPlugin) Templates(lang string) (fs.FS, bool) {
	if lang != plugintest.ConformanceLanguage {
		return nil, false
	}
	return fstest.MapFS{"t.tmpl": &fstest.MapFile{Data: []byte(p.body)}}, true
}

func (p *tmplPlugin) TemplateFuncs(string) template.FuncMap     { return p.funcs }
func (p *tmplPlugin) TemplateOverrides(string) template.FuncMap { return p.overrides }

// newTmplPlugin returns a provider shipping body and registering
// funcs, named so a report names it.
func newTmplPlugin(body string, funcs template.FuncMap) *tmplPlugin {
	p := &tmplPlugin{FixturePlugin: *plugintest.NewFixturePlugin(), body: body, funcs: funcs}
	p.PluginName = "tmpl"
	return p
}

// TestAssertTemplateFuncsResolve pins the check that closes the gap
// between "this name is bindable" and "this name is bound".
//
// A template calling a function nobody registers parses, ships, and
// fails midway through Render in the consumer's build — naming the
// merged template tree rather than the file that called it.
func TestAssertTemplateFuncsResolve(t *testing.T) {
	t.Parallel()

	reserved := template.FuncMap{"render": func(any) string { return "" }}

	t.Run("accepts a call the plugin registers", func(t *testing.T) {
		t.Parallel()
		f := newFakeT()
		plugintest.AssertTemplateFuncsResolve(f,
			newTmplPlugin(`{{ mine . }}`, template.FuncMap{"mine": func(any) string { return "" }}),
			reserved, plugintest.ConformanceLanguage)
		if f.failed {
			t.Fatalf("a registered call was reported:\n%s", joinFake(f))
		}
	})

	t.Run("accepts a call the backend reserves", func(t *testing.T) {
		t.Parallel()
		// The reason the parse check stubs everything: a plugin
		// legitimately calls the backend's own funcmap, which it does
		// not register itself.
		f := newFakeT()
		plugintest.AssertTemplateFuncsResolve(f,
			newTmplPlugin(`{{ render . }}`, nil), reserved, plugintest.ConformanceLanguage)
		if f.failed {
			t.Fatalf("a reserved call was reported:\n%s", joinFake(f))
		}
	})

	t.Run("accepts a call an override provides", func(t *testing.T) {
		t.Parallel()
		p := newTmplPlugin(`{{ swapped . }}`, nil)
		p.overrides = template.FuncMap{"swapped": func(any) string { return "" }}
		f := newFakeT()
		plugintest.AssertTemplateFuncsResolve(f, p, reserved, plugintest.ConformanceLanguage)
		if f.failed {
			t.Fatalf("an overridden call was reported:\n%s", joinFake(f))
		}
	})

	t.Run("reports a call nobody provides", func(t *testing.T) {
		t.Parallel()
		f := newFakeT()
		plugintest.AssertTemplateFuncsResolve(f,
			newTmplPlugin(`{{ absent . }}`, nil), reserved, plugintest.ConformanceLanguage)
		if !f.failed {
			t.Fatal("an unresolvable call was accepted")
		}
		if got := joinFake(f); !strings.Contains(got, `"absent"`) {
			t.Fatalf("report does not name the function:\n%s", got)
		}
	})

	t.Run("names every unresolvable call, not just the first", func(t *testing.T) {
		t.Parallel()
		// A hand-maintained list drifts one name at a time; a report
		// that stopped at the first would too.
		f := newFakeT()
		plugintest.AssertTemplateFuncsResolve(f,
			newTmplPlugin(`{{ alpha . }}{{ beta . }}`, nil), reserved, plugintest.ConformanceLanguage)
		got := joinFake(f)
		for _, want := range []string{`"alpha"`, `"beta"`} {
			if !strings.Contains(got, want) {
				t.Fatalf("report does not name %s:\n%s", want, got)
			}
		}
	})

	t.Run("finds a call in a pipeline and an assignment position", func(t *testing.T) {
		t.Parallel()
		// Read from the parser rather than by pattern, which is what
		// makes these positions cost nothing to support.
		f := newFakeT()
		plugintest.AssertTemplateFuncsResolve(f,
			newTmplPlugin(`{{ $x := piped . | chained }}{{ $x }}`, nil),
			reserved, plugintest.ConformanceLanguage)
		got := joinFake(f)
		for _, want := range []string{`"piped"`, `"chained"`} {
			if !strings.Contains(got, want) {
				t.Fatalf("report does not name %s:\n%s", want, got)
			}
		}
	})

	t.Run("says nothing about a language the plugin does not target", func(t *testing.T) {
		t.Parallel()
		// A Go generator asked about Rust brings nothing, which is not
		// a failure.
		f := newFakeT()
		plugintest.AssertTemplateFuncsResolve(f,
			newTmplPlugin(`{{ absent . }}`, nil), reserved, "rust")
		if f.failed {
			t.Fatalf("an untargeted language was reported:\n%s", joinFake(f))
		}
	})
}

// backendExtraPlugin ships one template calling whatever body it is
// given, and registers no funcmap of its own — so every name the
// template calls has to come from the backend.
type backendExtraPlugin struct{ body string }

func (backendExtraPlugin) Name() string                            { return "extragen" }
func (backendExtraPlugin) Generate(*plugin.GeneratorContext) error { return nil }

func (p backendExtraPlugin) Templates(lang string) (fs.FS, bool) {
	if lang != plugintest.ConformanceLanguage {
		return nil, false
	}
	return fstest.MapFS{
		"templates/golang/extragen.x.tmpl": &fstest.MapFile{Data: []byte(p.body)},
	}, true
}

func (backendExtraPlugin) TemplateFuncs(string) template.FuncMap     { return nil }
func (backendExtraPlugin) TemplateOverrides(string) template.FuncMap { return nil }

// TestReservedTemplateFuncs_CoversWhatTemplatesCall pins the
// relationship between the assertion's argument and the names a
// template may legally call.
//
// The seed carried the reserved dispatch helpers and the shared
// lang/golang bundle and not the backend's overrideable extras, so a
// plugin calling `camel` failed its own suite against a template that
// renders correctly — and the diagnostic said the backend does not
// provide the name, which sends the author to register it themselves
// and collide with the backend's entry.
//
// The drift tests in backend/golang guard the mirrored name lists.
// This pins the other half: that the assembled map is what the
// assertion actually needs, which is the join those tests cannot see.
func TestReservedTemplateFuncs_CoversWhatTemplatesCall(t *testing.T) {
	t.Parallel()

	t.Run("a template calling backend extras resolves", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertTemplateFuncsResolve(fake,
			backendExtraPlugin{body: `{{ define "extragen.x" }}` +
				`{{ camel .Name }}{{ pascal .Name }}{{ metaStr . "k" }}{{ join .Parts "," }}` +
				`{{ end }}`},
			plugintest.ReservedTemplateFuncs(plugintest.ConformanceLanguage),
			plugintest.ConformanceLanguage)
		if fake.failed {
			t.Fatalf("a template calling backend-provided helpers was reported unresolved: %v",
				fake.errs)
		}
	})

	t.Run("a template calling the shared conventions resolves", func(t *testing.T) {
		t.Parallel()
		fake := newFakeT()
		plugintest.AssertTemplateFuncsResolve(fake,
			backendExtraPlugin{body: `{{ define "extragen.x" }}` +
				`{{ if isSlice .Type }}{{ selfType . }}{{ end }}{{ end }}`},
			plugintest.ReservedTemplateFuncs(plugintest.ConformanceLanguage),
			plugintest.ConformanceLanguage)
		if fake.failed {
			t.Fatalf("a template calling lang/golang helpers was reported unresolved: %v",
				fake.errs)
		}
	})

	t.Run("a misspelled call is still reported", func(t *testing.T) {
		t.Parallel()
		// The check has to stay able to fail: a seed wide enough to
		// resolve everything would pass every plugin and assert
		// nothing.
		fake := newFakeT()
		plugintest.AssertTemplateFuncsResolve(fake,
			backendExtraPlugin{body: `{{ define "extragen.x" }}{{ camle .Name }}{{ end }}`},
			plugintest.ReservedTemplateFuncs(plugintest.ConformanceLanguage),
			plugintest.ConformanceLanguage)
		if !fake.failed {
			t.Fatal("a call to a name nobody provides was not reported")
		}
	})

	t.Run("the names accessors are each a proper subset of the map", func(t *testing.T) {
		t.Parallel()
		// The defect this closes was reading the two as a matched
		// pair. They are halves, and passing either alone is what
		// produced the false failure.
		full := plugintest.ReservedTemplateFuncs(plugintest.ConformanceLanguage)
		for _, name := range plugintest.ReservedTemplateFuncNames() {
			if _, ok := full[name]; !ok {
				t.Errorf("reserved name %q missing from ReservedTemplateFuncs", name)
			}
		}
		for _, name := range plugintest.OverrideableTemplateFuncNames() {
			if _, ok := full[name]; !ok {
				t.Errorf("overrideable name %q missing from ReservedTemplateFuncs", name)
			}
		}
		reserved := len(plugintest.ReservedTemplateFuncNames())
		if len(full) <= reserved {
			t.Fatalf("ReservedTemplateFuncs has %d entries and the reserved half alone has %d; "+
				"the map is not wider than its parts", len(full), reserved)
		}
	})
}
