// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest

import (
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"
	"text/template"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/priority"
)

// RunSuite runs every framework-conformance check applicable to
// the plugin's declared capabilities and roles. The checks pin
// the invariants the pipeline relies on at registration / build
// time: stable [plugin.Plugin.Name], role-interface compliance,
// deterministic [plugin.CapabilityProvider] ordering, unique
// [plugin.DirectiveProvider] schema names, well-formed
// [plugin.EmitVersioned] entries, stable [plugin.NodesOnly]
// declaration, stable [plugin.FilenameProvider.Outputs] per
// language, and a well-formed Outputs slice (non-empty suffixes,
// unique tags, at-most-one empty-tag output at index 0).
//
// Per-role contract checks (idempotency, determinism, diagnostic
// discipline) belong on the role-specific suites: see
// [RunAnnotatorSuite], [RunGeneratorSuite], [RunBackendSuite],
// [RunFrontendSuite], and [RunOptionsSuite]. Plugin authors
// invoke whichever apply.
//
// Failures surface through `t.Errorf` rather than `t.Fatalf` so
// a single failing contract surfaces every downstream failure
// too — the author sees the full report in one run instead of
// chasing one cascade at a time.
func RunSuite(t *testing.T, p plugin.Plugin) {
	t.Helper()
	for _, c := range frameworkChecks() {
		t.Run(c.name, func(t *testing.T) {
			c.fn(t, p)
		})
	}
}

// probeLanguages are the language identifiers the framework suite
// drives capability lookups with. "golang" is the only backend
// language in tree; the second entry is deliberately a language no
// backend claims, so a plugin's negative path ("I contribute
// nothing here") is exercised rather than assumed.
var probeLanguages = []string{ConformanceLanguage, "no-such-language"}

// check pairs one framework contract with the assertion that enforces
// it and the [Violation] whose fixture must defeat that assertion.
//
// RunSuite walks this table rather than listing t.Run calls inline so
// the set of checks is enumerable. That is what lets the package's
// meta-tests assert their own completeness: a check added without a
// fixture that defeats it, or a fixture that no longer defeats the
// check it names, fails the build on the commit that introduces the
// gap rather than sitting green.
//
// Consequence worth stating: the names below are a contract, not
// cosmetics. They appear in adopters' test output and the meta-tests
// key on the pairing, so renaming one is a visible change.
type check struct {
	name      string
	fn        func(testing.TB, plugin.Plugin)
	violation Violation
}

// frameworkChecks returns every universal contract [RunSuite] runs,
// in execution order.
func frameworkChecks() []check {
	return []check{
		{"Name returns a non-empty, stable identifier", assertStableName, ViolationUnstableName},
		{"implements at least one of the documented role interfaces", assertImplementsARole, ViolationNoRole},
		{
			"CapabilityProvider is implemented in full or not at all",
			assertCapabilityProviderIsComplete,
			ViolationPartialCapability,
		},
		{
			"CapabilityProvider returns deterministic Provides + Requires",
			assertCapabilityProviderStability,
			ViolationUnstableProvides,
		},
		{
			"DirectiveProvider schemas declare unique non-empty names",
			assertDirectiveSchemaUniqueness,
			ViolationDuplicateDirective,
		},
		{"Versioned reports a stable version string", assertVersionedStability, ViolationUnstableVersion},
		{
			"EmitVersioned declares stable, non-empty majors",
			assertEmitVersionedStability,
			ViolationUnstableEmitVersions,
		},
		{"NodesOnly returns a stable declaration", assertNodesOnlyStability, ViolationUnstableNodesOnly},
		{
			"FilenameProvider returns stable Outputs per language",
			assertFilenameProviderStability,
			ViolationUnstableOutputs,
		},
		{"FilenameProvider returns a well-formed Outputs slice", assertOutputsShape, ViolationEmptySuffix},
		{
			"TemplateProvider returns stable, well-formed template contributions",
			assertTemplateProviderStability,
			ViolationFuncInBothMaps,
		},
		{
			"TemplateProvider ships templates that parse and claim no reserved name",
			assertTemplatesParse,
			ViolationUnparsableTemplate,
		},
		{
			"declaration accessors return slices the caller may keep",
			assertAccessorsReturnFreshSlices,
			ViolationSharedSlice,
		},
	}
}

// assertTemplateProviderStability pins the [plugin.TemplateProvider]
// contract the backend relies on when it merges plugin templates
// into its own funcmap.
//
// Two properties matter at merge time. Repeated lookups must agree,
// because the backend queries a plugin once per render pass and a
// shifting answer would make output depend on render order. And a
// name must not appear in both TemplateFuncs and TemplateOverrides
// for one language: the backend treats the first as "must not
// collide" and the second as "intentionally replaces", so a name in
// both is a contradiction it cannot resolve deterministically.
//
// Plugins that do not implement the interface return early; this is
// an optional capability.
func assertTemplateProviderStability(tb testing.TB, p plugin.Plugin) {
	tb.Helper()
	provider, ok := any(p).(plugin.TemplateProvider)
	if !ok {
		return
	}
	for _, lang := range probeLanguages {
		firstFS, firstOK := provider.Templates(lang)
		secondFS, secondOK := provider.Templates(lang)
		if firstOK != secondOK {
			tb.Errorf("TemplateProvider.Templates(%q) not stable: first ok=%v second ok=%v", lang, firstOK, secondOK)
		}
		if firstOK && firstFS == nil {
			tb.Errorf("TemplateProvider.Templates(%q) reported ok but returned a nil fs.FS", lang)
		}
		if !firstOK && secondFS != nil && firstFS != nil {
			tb.Errorf("TemplateProvider.Templates(%q) returned a filesystem while reporting ok=false", lang)
		}

		funcs := assertStableFuncMap(tb, lang, "TemplateFuncs", provider.TemplateFuncs)
		overrides := assertStableFuncMap(tb, lang, "TemplateOverrides", provider.TemplateOverrides)
		for name := range overrides {
			if _, dup := funcs[name]; dup {
				tb.Errorf("TemplateProvider declares %q in both TemplateFuncs and TemplateOverrides for %q; "+
					"a name is either a new registration or an intentional override, not both", name, lang)
			}
		}
	}
}

// assertStableFuncMap checks that two consecutive lookups agree on
// the registered names and that no name is empty, returning the
// first result so callers can cross-check it against another map.
// Only key sets are compared: func values are not comparable in Go,
// and the backend keys its merge on name alone.
func assertStableFuncMap(
	tb testing.TB, lang, label string, lookup func(string) template.FuncMap,
) template.FuncMap {
	tb.Helper()
	first, second := lookup(lang), lookup(lang)

	firstNames := slices.Sorted(maps.Keys(first))
	secondNames := slices.Sorted(maps.Keys(second))
	if !slices.Equal(firstNames, secondNames) {
		tb.Errorf("TemplateProvider.%s(%q) not stable: first=%v second=%v", label, lang, firstNames, secondNames)
	}
	for _, name := range firstNames {
		if name == "" {
			tb.Errorf("TemplateProvider.%s(%q) registers the empty name; funcmap keys must be non-empty", label, lang)
		}
	}
	return first
}

// assertStableName pins Name's empty-string and stability
// contracts: every framework caller treats the result as the
// plugin's identifier — diagnostic attribution, cache-key
// composition, manifest entries all use it.
func assertStableName(tb testing.TB, p plugin.Plugin) {
	tb.Helper()
	first := p.Name()
	if first == "" {
		tb.Errorf("Plugin.Name returned the empty string; framework callers require a stable identifier")
	}
	second := p.Name()
	if first != second {
		tb.Errorf("Plugin.Name not stable across calls: first=%q second=%q", first, second)
	}
}

// assertImplementsARole verifies p satisfies at least one of the
// four role interfaces. A plugin that satisfies none is
// effectively dead code — the pipeline never invokes anything
// on it. The check surfaces this as a contract failure rather
// than a silent no-op at pipeline-Build time.
func assertImplementsARole(tb testing.TB, p plugin.Plugin) {
	tb.Helper()
	if _, ok := any(p).(plugin.Frontend); ok {
		return
	}
	if _, ok := any(p).(plugin.Annotator); ok {
		return
	}
	if _, ok := any(p).(plugin.Generator); ok {
		return
	}
	if _, ok := any(p).(plugin.Backend); ok {
		return
	}
	tb.Errorf(
		"plugin %T implements no role interface "+
			"(Frontend / Annotator / Generator / Backend); pipeline would never invoke it",
		p,
	)
}

// assertCapabilityProviderIsComplete fails a plugin that declares
// [plugin.CapabilityProvider.Priority] without the rest of the
// interface.
//
// CapabilityProvider is all-or-nothing: Priority, Provides and
// Requires together. A plugin declaring only some of them does not
// satisfy the interface, so the pipeline's type assertion fails and
// the plugin collapses into the default priority bucket — executing
// in registration order, with the ordering its author wrote down
// silently discarded. Nothing else reports it: the pipeline sees a
// plugin that opted out, which is legal.
//
// This check exists because [assertCapabilityProviderStability]
// structurally cannot catch it. That check opens with a
// CapabilityProvider assertion and returns when it fails, so it is
// unreachable in exactly the case that is broken. The partial
// implementation has now shipped twice — once in the protobuf-to-Go
// bridge, once across all three shape plugins — which is what a
// silent failure mode looks like from the outside.
//
// Declaring none of the three is fine and common: ordering is then
// the caller's registration order by design.
func assertCapabilityProviderIsComplete(tb testing.TB, p plugin.Plugin) {
	tb.Helper()
	if _, ok := any(p).(plugin.CapabilityProvider); ok {
		return
	}
	// Probing for Priority alone is what separates "opted out" from
	// "tried to opt in and missed". Its presence is an author
	// declaring an ordering intent the pipeline is not reading.
	if _, declared := any(p).(interface {
		Priority() priority.Priority
	}); !declared {
		return
	}
	_, hasProvides := any(p).(interface{ Provides() []string })
	_, hasRequires := any(p).(interface{ Requires() []string })
	tb.Errorf("plugin %q declares Priority() but not %s, so it does not satisfy "+
		"plugin.CapabilityProvider: the pipeline ignores the declared priority "+
		"and runs the plugin in the default bucket, in registration order",
		p.Name(), missingCapabilityMethods(hasProvides, hasRequires))
}

// missingCapabilityMethods renders the absent half of the
// [plugin.CapabilityProvider] method set for the diagnostic above.
func missingCapabilityMethods(hasProvides, hasRequires bool) string {
	switch {
	case !hasProvides && !hasRequires:
		return "Provides() or Requires()"
	case !hasProvides:
		return "Provides()"
	default:
		return "Requires()"
	}
}

// assertCapabilityProviderStability pins the deterministic-
// ordering contract: the resolver depends on Provides / Requires
// returning the same sequence across calls so capability-topo
// ordering stays reproducible.
func assertCapabilityProviderStability(tb testing.TB, p plugin.Plugin) {
	tb.Helper()
	provider, ok := any(p).(plugin.CapabilityProvider)
	if !ok {
		return
	}
	// Priority participates in bucket ordering on every pipeline
	// build, so an unstable value reorders plugins between runs and
	// breaks the determinism guarantee. The value itself is not
	// constrained — [priority.Priority] is a plain int and custom
	// buckets are explicitly supported — so stability is the whole
	// contract here.
	if firstPri, secondPri := provider.Priority(), provider.Priority(); firstPri != secondPri {
		tb.Errorf("CapabilityProvider.Priority not stable: first=%d second=%d", firstPri, secondPri)
	}

	first := provider.Provides()
	second := provider.Provides()
	if !slices.Equal(first, second) {
		tb.Errorf("CapabilityProvider.Provides not stable: first=%v second=%v", first, second)
	}
	firstReq := provider.Requires()
	secondReq := provider.Requires()
	if !slices.Equal(firstReq, secondReq) {
		tb.Errorf("CapabilityProvider.Requires not stable: first=%v second=%v", firstReq, secondReq)
	}
	for _, cap := range first {
		if cap == "" {
			tb.Errorf("CapabilityProvider.Provides contains the empty string; capability labels must be non-empty")
		}
	}
	for _, cap := range firstReq {
		if cap == "" {
			tb.Errorf("CapabilityProvider.Requires contains the empty string; capability labels must be non-empty")
		}
	}
}

// assertDirectiveSchemaUniqueness pins the directive-schema
// contract: every declared schema has a non-empty Name, and no
// two schemas in the same plugin share a Name. Duplicates would
// shadow each other at registration time; the framework's
// directive registry rejects them.
func assertDirectiveSchemaUniqueness(tb testing.TB, p plugin.Plugin) {
	tb.Helper()
	provider, ok := any(p).(plugin.DirectiveProvider)
	if !ok {
		return
	}
	seen := map[directive.Name]struct{}{}
	for _, schema := range provider.Directives() {
		if schema.Name == "" {
			tb.Errorf("DirectiveProvider.Directives contains a schema with empty Name")
			continue
		}
		if _, dup := seen[schema.Name]; dup {
			tb.Errorf("DirectiveProvider.Directives declares duplicate schema name %q", schema.Name)
			continue
		}
		seen[schema.Name] = struct{}{}
	}
}

// assertVersionedStability pins the Versioned contract: when a
// plugin opts into cache invalidation via [plugin.Versioned],
// the returned string must be stable across calls so cache
// keys compose deterministically. The empty string is
// permitted and signals "do not contribute to the cache key";
// the framework's cache machinery treats it as opt-out and
// callers reading the value for header rendering already
// branch on emptiness.
func assertVersionedStability(tb testing.TB, p plugin.Plugin) {
	tb.Helper()
	versioned, ok := any(p).(plugin.Versioned)
	if !ok {
		return
	}
	first := versioned.Version()
	second := versioned.Version()
	if first != second {
		tb.Errorf("Versioned.Version not stable across calls: first=%q second=%q", first, second)
	}
}

// assertEmitVersionedStability pins the EmitVersioned contract:
// the declared major-version list is stable across calls and
// every entry is non-empty so the pipeline's compatibility
// check at Build time produces deterministic results. A plugin
// that declared a list and then mutated it between calls would
// admit or reject the run depending on call ordering — the
// stability check forecloses that surprise.
func assertEmitVersionedStability(tb testing.TB, p plugin.Plugin) {
	tb.Helper()
	ev, ok := any(p).(plugin.EmitVersioned)
	if !ok {
		return
	}
	first := ev.EmitVersions()
	second := ev.EmitVersions()
	if !slices.Equal(first, second) {
		tb.Errorf("EmitVersioned.EmitVersions not stable: first=%v second=%v", first, second)
	}
	for _, v := range first {
		if v == "" {
			tb.Errorf("EmitVersioned.EmitVersions contains the empty string; majors must be non-empty")
		}
	}
}

// assertNodesOnlyStability pins the [plugin.NodesOnly] contract:
// the declaration is static (the docblock calls it a "static
// contract, not a runtime switch") so the pipeline can plan the
// generator phase's parallelisation at Build time. A plugin
// whose NodesOnly toggles between calls would invalidate the
// pipeline's scheduling decision.
func assertNodesOnlyStability(tb testing.TB, p plugin.Plugin) {
	tb.Helper()
	no, ok := any(p).(plugin.NodesOnly)
	if !ok {
		return
	}
	first := no.NodesOnly()
	second := no.NodesOnly()
	if first != second {
		tb.Errorf("NodesOnly not stable across calls: first=%v second=%v", first, second)
	}
}

// assertFilenameProviderStability pins the
// [plugin.FilenameProvider] contract: Outputs returns the same
// slice for the same language across calls. A plugin whose
// Outputs flipped between calls would produce different
// filenames on consecutive runs — a layout-determinism
// violation the pipeline cannot recover from.
//
// The check exercises the languages every framework backend
// the suite anticipates encountering uses, plus an empty
// language to verify the plugin doesn't panic on the empty
// string. Plugins that target a language not in this list are
// covered by the per-language stability invariant: the second
// call with the same argument equals the first.
//
// The well-formed-Outputs shape rules (non-empty suffixes,
// unique tags, at-most-one empty-tag output at index 0) are
// enforced by [assertOutputsShape] in addition to this
// stability check — both run as part of [RunSuite].
func assertFilenameProviderStability(tb testing.TB, p plugin.Plugin) {
	tb.Helper()
	fp, ok := any(p).(plugin.FilenameProvider)
	if !ok {
		return
	}
	for _, lang := range probeLanguages {
		first := fp.Outputs(lang)
		second := fp.Outputs(lang)
		if !slices.Equal(first, second) {
			tb.Errorf(
				"FilenameProvider.Outputs(%q) not stable: first=%+v second=%+v",
				lang, first, second,
			)
		}
	}
}

// assertOutputsShape pins the well-formedness rules on a
// [plugin.FilenameProvider]'s returned Outputs slice — the same
// rules the pipeline's Build-time validation enforces, run from
// the conformance suite so plugin authors see violations during
// development. Rules: every Suffix is non-empty; tags within
// the slice are unique; at most one Output declares an empty
// Tag; when present, the empty-Tag Output is at index 0.
//
// The check runs against the same languages
// [assertFilenameProviderStability] exercises so a plugin
// shipping outputs for multiple backends gets the shape check
// on each set.
func assertOutputsShape(tb testing.TB, p plugin.Plugin) {
	tb.Helper()
	fp, ok := any(p).(plugin.FilenameProvider)
	if !ok {
		return
	}
	for _, lang := range probeLanguages {
		outputs := fp.Outputs(lang)
		seenTags := make(map[string]int, len(outputs))
		emptyTagCount := 0
		for i, o := range outputs {
			if o.Suffix == "" {
				tb.Errorf(
					"FilenameProvider.Outputs(%q)[%d]: Suffix is required",
					lang, i,
				)
			}
			if prev, dup := seenTags[o.Tag]; dup {
				tb.Errorf(
					"FilenameProvider.Outputs(%q): tag %q appears at index %d and %d",
					lang, o.Tag, prev, i,
				)
			}
			seenTags[o.Tag] = i
			if o.Tag == "" {
				emptyTagCount++
				if i != 0 {
					tb.Errorf(
						"FilenameProvider.Outputs(%q)[%d]: empty-tag output must be declared at index 0",
						lang, i,
					)
				}
			}
		}
		if emptyTagCount > 1 {
			tb.Errorf(
				"FilenameProvider.Outputs(%q): %d outputs declare an empty Tag; "+
					"at most one is permitted (the plugin's primary output)",
				lang, emptyTagCount,
			)
		}
	}
}

// reservedTemplatePrefix mirrors the backend's reserved template-name
// prefix. It is duplicated rather than imported because plugintest is
// the language-neutral conformance suite: importing backend/golang to
// read one constant would couple every plugin author's test run to a
// specific backend. The cost of duplication is that a change to the
// backend's reserved set must be mirrored here; the check below
// exists precisely so that mismatch surfaces as a conformance failure
// rather than a render-time surprise.
const reservedTemplatePrefix = "fragment."

// assertTemplatesParse pins that a [plugin.TemplateProvider] ships
// templates the backend can actually parse, and that none of them
// claims a reserved name.
//
// Without this, a malformed template surfaces from
// mergePluginContributions midway through Render — after the frontend
// has parsed the workspace and every generator has run. The author
// learns about a typo at the most expensive possible moment, and the
// diagnostic names the merged tree rather than their file. Parsing at
// conformance time moves that to the commit that introduced it.
//
// Templates are parsed into a throwaway tree with no funcmap, so
// unresolved function names are tolerated: the plugin's own
// TemplateFuncs and the backend's reserved set are merged later, and
// rejecting a template here for calling `imp` would be wrong.
func assertTemplatesParse(tb testing.TB, p plugin.Plugin) {
	tb.Helper()

	tp, ok := any(p).(plugin.TemplateProvider)
	if !ok {
		return
	}

	for _, lang := range probeLanguages {
		fsys, declared := tp.Templates(lang)
		if !declared || fsys == nil {
			continue
		}

		entries, err := fs.Glob(fsys, "*.tmpl")
		if err != nil {
			tb.Errorf("TemplateProvider.Templates(%q): walking the filesystem failed: %v", lang, err)
			continue
		}

		for _, name := range entries {
			body, err := fs.ReadFile(fsys, name)
			if err != nil {
				tb.Errorf("TemplateProvider.Templates(%q): reading %s failed: %v", lang, name, err)
				continue
			}

			// Missing functions are resolved at render time against
			// the merged funcmap, so they must not fail the parse.
			// template.Option("missingkey=zero") is deliberately not
			// set: it governs execution, not parsing.
			parsed, err := parsePermissively(name, body)
			if err != nil {
				tb.Errorf("TemplateProvider.Templates(%q): %s does not parse: %v; "+
					"the backend would surface this midway through Render instead",
					lang, name, err)
				continue
			}

			for _, defined := range parsed.Templates() {
				if strings.HasPrefix(defined.Name(), reservedTemplatePrefix) {
					tb.Errorf("TemplateProvider.Templates(%q): %s defines %q, which claims the "+
						"reserved %q prefix the backend uses for its own fragments",
						lang, name, defined.Name(), reservedTemplatePrefix)
				}
			}
		}
	}
}

// parsePermissively parses body with every function it references
// stubbed out, so the check judges syntax rather than resolution.
//
// text/template rejects an unknown function at parse time, and a
// plugin template legitimately calls the backend's reserved funcmap
// (render, imp, slot, …) plus its own extensions and those of any
// other plugin in the run — none of which exist here. Rather than
// guessing which identifiers are calls, this asks the parser: each
// failure names the function it wanted, that name is stubbed, and the
// parse retries. A pattern-matching approach missed `typeArgs` in
// `{{ $x := typeArgs . }}` position, which is the general problem
// with deciding call position by regex.
//
// The iteration bound stops a pathological template from looping;
// reaching it is reported as a parse failure rather than silently
// accepted.
func parsePermissively(name string, body []byte) (*template.Template, error) {
	fm := template.FuncMap{}
	stub := func(_ ...any) any { return nil }

	for range maxStubbedTemplateFuncs {
		parsed, err := template.New(name).Funcs(fm).Parse(string(body))
		if err == nil {
			return parsed, nil
		}
		m := undefinedFuncPattern.FindStringSubmatch(err.Error())
		if m == nil {
			// A genuine syntax error rather than an unresolved name.
			// The stub count is worth carrying: it distinguishes "this
			// template was malformed on sight" from "it parsed until a
			// function was stubbed in", which is the difference between
			// an author's typo and a bad interaction with the funcmap.
			return nil, fmt.Errorf("after stubbing %d undefined function(s): %w", len(fm), err)
		}
		fm[m[1]] = stub
	}
	return nil, fmt.Errorf("template references more than %d undefined functions",
		maxStubbedTemplateFuncs)
}

// maxStubbedTemplateFuncs bounds the stub-and-retry loop in
// [parsePermissively].
const maxStubbedTemplateFuncs = 128

// undefinedFuncPattern extracts the function name from
// text/template's parse error, which is the oracle for what the
// template expects to be in scope at render time.
var undefinedFuncPattern = regexp.MustCompile(`function "([^"]+)" not defined`)

// assertAccessorsReturnFreshSlices pins that a plugin's declaration
// accessors hand back a slice the caller may keep, not the plugin's
// own state.
//
// The pipeline calls these repeatedly and stores what it gets:
// Provides / Requires feed the capability topo-sort, Directives feed
// the shared registry, Outputs feeds routing. A returned slice that
// aliases the plugin's field means any consumer that sorts, filters
// or appends in place silently rewrites the plugin's declaration for
// every later caller — and the corruption surfaces as a routing or
// ordering bug far from its cause.
//
// `docs/plugin/recipes.md` lists this among its anti-patterns.
// Nothing enforced it until now.
//
// Aliasing is detected by backing-array identity rather than by
// mutating and observing, so the check cannot itself corrupt a plugin
// that does share state.
func assertAccessorsReturnFreshSlices(tb testing.TB, p plugin.Plugin) {
	tb.Helper()

	if cp, ok := any(p).(plugin.CapabilityProvider); ok {
		assertNotAliased(tb, "CapabilityProvider.Provides", cp.Provides(), cp.Provides())
		assertNotAliased(tb, "CapabilityProvider.Requires", cp.Requires(), cp.Requires())
	}
	if dp, ok := any(p).(plugin.DirectiveProvider); ok {
		assertNotAliased(tb, "DirectiveProvider.Directives", dp.Directives(), dp.Directives())
	}
	if fp, ok := any(p).(plugin.FilenameProvider); ok {
		for _, lang := range probeLanguages {
			assertNotAliased(tb,
				fmt.Sprintf("FilenameProvider.Outputs(%q)", lang),
				fp.Outputs(lang), fp.Outputs(lang))
		}
	}
}

// assertNotAliased fails when two independent calls to the same
// accessor return slices over one backing array.
//
// Two calls returning the same array means the value is stored rather
// than constructed, so a caller holding the first result observes
// mutations made through the second.
func assertNotAliased[T any](tb testing.TB, label string, first, second []T) {
	tb.Helper()
	if len(first) == 0 || len(second) == 0 {
		return
	}
	if &first[0] != &second[0] {
		return
	}
	tb.Errorf("%s returns the plugin's own slice: two calls share one backing array, so a "+
		"caller that sorts or filters the result in place rewrites the plugin's declaration "+
		"for every later caller; return a copy", label)
}
