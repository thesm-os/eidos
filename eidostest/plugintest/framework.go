// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest

import (
	"fmt"
	"io/fs"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"
	"text/template"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/plugin"
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
	RunSuiteFor(t, p, ConformanceLanguage)
}

// RunSuiteFor is [RunSuite] driven against the caller's own backend
// languages rather than the default Go one.
//
// Five of the framework checks are per-language lookups, and the
// language they probe was a package-private constant. A plugin
// targeting any other backend answered none of those probes: its
// Outputs and Templates were validated as empty slices, and four
// checks reported green having examined nothing. Passing the
// languages the plugin actually claims makes those checks real.
//
// Each supplied language is probed alongside a name no backend claims,
// so the negative path — "I contribute nothing here" — stays
// exercised rather than assumed. Supplying none falls back to
// [ConformanceLanguage], so RunSuiteFor(t, p) and RunSuite(t, p) agree.
func RunSuiteFor(t *testing.T, p plugin.Plugin, languages ...string) {
	t.Helper()
	probes := probeLanguagesFor(languages...)
	for _, c := range frameworkChecks() {
		t.Run(c.name, func(t *testing.T) {
			c.fn(t, p, probes...)
		})
	}
}

// probeLanguagesFor returns the language set a run probes: the
// caller's own, plus one no backend claims so the negative path is
// covered. Falls back to [ConformanceLanguage] when the caller names
// none.
func probeLanguagesFor(languages ...string) []string {
	if len(languages) == 0 {
		languages = []string{ConformanceLanguage}
	}
	return append(slices.Clone(languages), unclaimedLanguage)
}

// unclaimedLanguage is a language no backend claims. Every probe set
// carries it so a plugin's negative path ("I contribute nothing here")
// is exercised rather than assumed.
const unclaimedLanguage = "no-such-language"

// probeLanguages is the default probe set — the Go conformance
// language plus the unclaimed one. Retained for the meta-tests that
// assert against the default without naming a language.
var probeLanguages = probeLanguagesFor()

// probeSet resolves a check's variadic language argument: the
// caller's set when RunSuiteFor supplied one, the default otherwise.
//
// Variadic rather than a required parameter so every check keeps its
// two-argument form for direct callers — the package's own
// rejection-path tests drive each assertion individually, and making
// the language set mandatory would have rewritten thirty call sites
// to say what the default already says.
func probeSet(languages []string) []string {
	if len(languages) == 0 {
		return probeLanguages
	}
	return languages
}

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
	fn        func(tb testing.TB, p plugin.Plugin, languages ...string)
	violation Violation
}

// frameworkChecks returns every universal contract [RunSuite] runs,
// in execution order.
func frameworkChecks() []check {
	return []check{
		{"Name returns a non-empty, stable identifier", assertStableName, ViolationUnstableName},
		{"implements at least one of the documented role interfaces", assertImplementsARole, ViolationNoRole},
		{
			"optional capabilities are implemented in full or not at all",
			assertNoPartialCapability,
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
			"TemplateProvider funcmap entries avoid the backend's reserved names",
			assertTemplateFuncsAvoidReservedNames,
			ViolationReservedFuncName,
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
func assertTemplateProviderStability(tb testing.TB, p plugin.Plugin, languages ...string) {
	tb.Helper()
	provider, ok := any(p).(plugin.TemplateProvider)
	if !ok {
		skipAbsentCapability(tb, "plugin.TemplateProvider")
		return
	}
	for _, lang := range probeSet(languages) {
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
func assertStableName(tb testing.TB, p plugin.Plugin, languages ...string) {
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
func assertImplementsARole(tb testing.TB, p plugin.Plugin, languages ...string) {
	tb.Helper()

	covered := detectedRoles(p)
	if len(covered) == 0 {
		tb.Errorf(
			"plugin %T implements no role interface "+
				"(Frontend / Annotator / Generator / Backend); pipeline would never invoke it",
			p,
		)
		return
	}

	// Name the per-role suite for each role found. RunSuite checks
	// declarations only — it cannot observe whether a sibling suite
	// ran, and a plugin that runs RunSuite alone prints a full column
	// of green indistinguishable from full conformance. Three shipped
	// in-tree annotators were in exactly that state. Logs rather than
	// errors: a partial or in-development plugin stays legal, but
	// "I ran conformance" stops being ambiguous in the output.
	for _, r := range covered {
		tb.Logf(
			"plugin implements %s; its per-role contracts (determinism, "+
				"diagnostic discipline, idempotency) are covered by %s, which this call does not run",
			r.iface, r.suite,
		)
	}
	if _, ok := any(p).(plugin.OptionsProvider); ok {
		tb.Logf(
			"plugin implements plugin.OptionsProvider; its options contracts are covered by " +
				"plugintest.RunOptionsSuite, which this call does not run",
		)
	}
}

// role pairs a role interface with the suite that checks its
// behavioural contracts.
type role struct{ iface, suite string }

// detectedRoles returns every role interface p satisfies, in the order
// the pipeline runs them.
//
// The type assertions were always here; only their answer was thrown
// away once it had proved non-empty. Keeping it is what lets RunSuite
// say which per-role suites a plugin still owes.
func detectedRoles(p plugin.Plugin) []role {
	var out []role
	if _, ok := any(p).(plugin.Frontend); ok {
		out = append(out, role{"plugin.Frontend", "plugintest.RunFrontendSuite"})
	}
	if _, ok := any(p).(plugin.Annotator); ok {
		out = append(out, role{"plugin.Annotator", "plugintest.RunAnnotatorSuite"})
	}
	if _, ok := any(p).(plugin.Generator); ok {
		out = append(out, role{"plugin.Generator", "plugintest.RunGeneratorSuite"})
	}
	if _, ok := any(p).(plugin.Backend); ok {
		out = append(out, role{"plugin.Backend", "plugintest.RunBackendSuite"})
	}
	return out
}

// assertNoPartialCapability reports a plugin that declares part of
// a multi-method optional capability.
//
// A Go interface assertion is all-or-nothing, so the declaration is
// discarded wholesale and nothing else reports it: the pipeline and
// the backend both see a plugin that opted out, which is legal. For
// CapabilityProvider that costs the plugin its declared ordering;
// for TemplateProvider it costs the entire template contribution,
// and the rendered output comes out short.
//
// This check exists because the per-capability checks structurally
// cannot catch it. Each opens with the composite assertion and
// skips when it fails, so every one of them is unreachable in
// exactly the case that is broken. The partial implementation has
// shipped twice in this repository.
//
// The detection is [plugin.Gaps] rather than a probe held here, so
// this check and Build cannot disagree about what "partial" means,
// and a method added to either capability is picked up by both.
//
// Declaring none of a capability's methods is fine and common:
// ordering is then the caller's registration order by design, and a
// generator rendering through the backend's builtin templates ships
// none of its own.
func assertNoPartialCapability(tb testing.TB, p plugin.Plugin, languages ...string) {
	tb.Helper()
	for _, gap := range plugin.Gaps(p) {
		tb.Errorf("plugin %q declares %s but not %s, so it does not satisfy %s: "+
			"the framework discards the declaration entirely and reports nothing",
			p.Name(),
			strings.Join(gap.Declared, " + "),
			strings.Join(gap.Missing, " + "),
			gap.Capability)
	}
}

// assertCapabilityProviderStability pins the deterministic-
// ordering contract: the resolver depends on Provides / Requires
// returning the same sequence across calls so capability-topo
// ordering stays reproducible.
func assertCapabilityProviderStability(tb testing.TB, p plugin.Plugin, languages ...string) {
	tb.Helper()
	provider, ok := any(p).(plugin.CapabilityProvider)
	if !ok {
		skipAbsentCapability(tb, "plugin.CapabilityProvider")
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
func assertDirectiveSchemaUniqueness(tb testing.TB, p plugin.Plugin, languages ...string) {
	tb.Helper()
	provider, ok := any(p).(plugin.DirectiveProvider)
	if !ok {
		skipAbsentCapability(tb, "plugin.DirectiveProvider")
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
func assertVersionedStability(tb testing.TB, p plugin.Plugin, languages ...string) {
	tb.Helper()
	versioned, ok := any(p).(plugin.Versioned)
	if !ok {
		skipAbsentCapability(tb, "plugin.Versioned")
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
func assertEmitVersionedStability(tb testing.TB, p plugin.Plugin, languages ...string) {
	tb.Helper()
	ev, ok := any(p).(plugin.EmitVersioned)
	if !ok {
		skipAbsentCapability(tb, "plugin.EmitVersioned")
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
func assertNodesOnlyStability(tb testing.TB, p plugin.Plugin, languages ...string) {
	tb.Helper()
	no, ok := any(p).(plugin.NodesOnly)
	if !ok {
		skipAbsentCapability(tb, "plugin.NodesOnly")
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
func assertFilenameProviderStability(tb testing.TB, p plugin.Plugin, languages ...string) {
	tb.Helper()
	fp, ok := any(p).(plugin.FilenameProvider)
	if !ok {
		skipAbsentCapability(tb, "plugin.FilenameProvider")
		return
	}
	for _, lang := range probeSet(languages) {
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
func assertOutputsShape(tb testing.TB, p plugin.Plugin, languages ...string) {
	tb.Helper()
	fp, ok := any(p).(plugin.FilenameProvider)
	if !ok {
		skipAbsentCapability(tb, "plugin.FilenameProvider")
		return
	}
	for _, lang := range probeSet(languages) {
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

// skipAbsentCapability marks the calling check skipped, naming the
// optional interface the plugin does not implement.
//
// The old shape was a bare `return`, which made "this plugin does not
// implement the capability" and "this plugin implements it correctly"
// print the identical green line. A plugin implementing one optional
// interface and a plugin implementing ten produced the same output,
// so an author had no way to tell which of the thirteen checks had
// examined anything at all — and the suite had no way to tell them.
//
// Skipf rather than Logf because a skipped subtest is what `go test`
// already has for "did not run", and it is greppable.
func skipAbsentCapability(tb testing.TB, iface string) {
	tb.Helper()
	tb.Skipf("plugin does not implement %s; this check examined nothing", iface)
}

// logSubjectCount records how many subjects a collection-driven check
// examined.
//
// A check whose body is a loop reports the same green over an empty
// collection as over a full one, and the suite had no vocabulary for
// the difference. This does not fail — a plugin legitimately declaring
// nothing for a probed language is conformant — but it puts the count
// in the output, so "validated 4 templates" and "validated 0" stop
// being the same line.
func logSubjectCount(tb testing.TB, label string, n int) {
	tb.Helper()
	tb.Logf("%s: examined %d subject(s)", label, n)
}

// reservedFuncNames mirrors the backend's reserved core funcmap
// entries — its dispatch, slot-composition and import-collection
// helpers. Duplicated rather than imported for the reason stated on
// [reservedTemplatePrefix]; `backend/golang` owns a drift test that
// fails when the two sets diverge, so the copy cannot rot in silence.
//
// The backend rejects a plugin that registers any of these through
// [plugin.TemplateProvider.TemplateFuncs] and one that targets any of
// them through [plugin.TemplateProvider.TemplateOverrides]. Both
// rejections happen while merging plugin contributions, which returns
// before a single file is rendered — so the run writes zero files for
// *every* plugin in the composition, not only the one at fault. That
// blast radius is why this is a conformance failure rather than a note.
var reservedFuncNames = []string{
	"external",
	"imp",
	"provenance",
	"render",
	"renderDocs",
	"renderEnumVariants",
	"renderExpr",
	"renderFunctionBody",
	"renderFunctionParams",
	"renderFunctionReturns",
	"renderInterfaceEmbeds",
	"renderInterfaceMethods",
	"renderMethodBody",
	"renderMethodParams",
	"renderMethodReturns",
	"renderParams",
	"renderReceiver",
	"renderReturns",
	"renderStmt",
	"renderStructEmbeds",
	"renderStructFields",
	"renderStructMethods",
	"renderType",
	"renderTypeParams",
	"slot",
}

// ReservedTemplateFuncNames returns the reserved funcmap names this
// suite checks plugin contributions against, sorted.
//
// Exported so a backend can assert in its own tests that the mirror
// above still matches the set it actually reserves. A backend whose
// reserved set grows without this one growing leaves plugin authors a
// name the suite calls legal and the backend rejects at merge time.
func ReservedTemplateFuncNames() []string { return slices.Clone(reservedFuncNames) }

// assertTemplateFuncsAvoidReservedNames pins that a
// [plugin.TemplateProvider] neither registers nor overrides a name the
// backend reserves for its own funcmap.
//
// Without this the collision surfaces from the backend's merge pass,
// which returns before rendering anything: one plugin's misnamed
// helper makes the entire run produce no output, and the plugin whose
// build breaks is as likely to be an innocent bystander in the same
// composition as the one that caused it.
func assertTemplateFuncsAvoidReservedNames(tb testing.TB, p plugin.Plugin, languages ...string) {
	tb.Helper()

	tp, ok := any(p).(plugin.TemplateProvider)
	if !ok {
		skipAbsentCapability(tb, "plugin.TemplateProvider")
		return
	}

	reserved := make(map[string]struct{}, len(reservedFuncNames))
	for _, name := range reservedFuncNames {
		reserved[name] = struct{}{}
	}

	for _, lang := range probeSet(languages) {
		reportReservedFuncNames(tb, lang, "TemplateFuncs", tp.TemplateFuncs(lang), reserved)
		reportReservedFuncNames(tb, lang, "TemplateOverrides", tp.TemplateOverrides(lang), reserved)
	}
}

// reportReservedFuncNames fails tb once per entry in fm whose name is
// reserved. Names are sorted before reporting so a plugin colliding on
// several produces the same failure text on every run — funcmaps are
// maps, and unsorted output would reorder per run.
func reportReservedFuncNames(
	tb testing.TB,
	lang, accessor string,
	fm template.FuncMap,
	reserved map[string]struct{},
) {
	tb.Helper()
	var bad []string
	for name := range fm {
		if _, clash := reserved[name]; clash {
			bad = append(bad, name)
		}
	}
	slices.Sort(bad)
	for _, name := range bad {
		tb.Errorf(
			"TemplateProvider.%s(%q) claims %q, which the backend reserves for its own funcmap; "+
				"the merge pass rejects it before rendering starts, so the run writes zero files for "+
				"every plugin in the composition. Rename it — a plugin-specific prefix is the "+
				"convention",
			accessor, lang, name,
		)
	}
}

// collectTemplateFiles returns every `.tmpl` path in fsys at any
// depth, mirroring the backend's own traversal.
//
// The depth is the point. A plugin embedding `templates/golang/*.tmpl`
// without the matching [fs.Sub] hands over a filesystem whose root
// holds only a directory — a root-only glob matches nothing there and
// validates nothing, reporting the same green as a plugin whose
// templates were all checked. The backend walks recursively and finds
// the files, so the divergence surfaced only at render time.
func collectTemplateFiles(fsys fs.FS) ([]string, error) {
	var out []string
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || path.Ext(p) != ".tmpl" {
			return nil
		}
		out = append(out, p)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("plugintest: walk template fs: %w", err)
	}
	return out, nil
}

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
func assertTemplatesParse(tb testing.TB, p plugin.Plugin, languages ...string) {
	tb.Helper()

	tp, ok := any(p).(plugin.TemplateProvider)
	if !ok {
		skipAbsentCapability(tb, "plugin.TemplateProvider")
		return
	}

	for _, lang := range probeSet(languages) {
		fsys, declared := tp.Templates(lang)
		if !declared || fsys == nil {
			continue
		}

		entries, err := collectTemplateFiles(fsys)
		if err != nil {
			tb.Errorf("TemplateProvider.Templates(%q): walking the filesystem failed: %v", lang, err)
			continue
		}
		logSubjectCount(tb, fmt.Sprintf("Templates(%q) parsed", lang), len(entries))

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
func assertAccessorsReturnFreshSlices(tb testing.TB, p plugin.Plugin, languages ...string) {
	tb.Helper()

	if cp, ok := any(p).(plugin.CapabilityProvider); ok {
		assertNotAliased(tb, "CapabilityProvider.Provides", cp.Provides(), cp.Provides())
		assertNotAliased(tb, "CapabilityProvider.Requires", cp.Requires(), cp.Requires())
	}
	if dp, ok := any(p).(plugin.DirectiveProvider); ok {
		assertNotAliased(tb, "DirectiveProvider.Directives", dp.Directives(), dp.Directives())
	}
	if fp, ok := any(p).(plugin.FilenameProvider); ok {
		for _, lang := range probeSet(languages) {
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
