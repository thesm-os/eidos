// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest

import (
	"io/fs"
	"slices"
	"strconv"
	"testing/fstest"
	"text/template"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/priority"
)

// Violation names exactly one framework contract that the plugin
// returned by [BrokenPlugin] breaks.
//
// The type exists so the suite can prove itself. A conformance suite
// that has only ever run against correct plugins is an untested
// suite: every check passes, and nothing distinguishes "this contract
// holds" from "this check never fires". Each Violation is paired with
// the check it must defeat, and the pairing is asserted rather than
// asserted-to-be-asserted — see the package's own meta-tests.
//
// Plugin authors can use these too. Running [RunSuite] against a
// BrokenPlugin and watching it fail is the cheapest way to confirm a
// harness is wired up at all, which matters most in the setup where
// a suite is accidentally never invoked.
type Violation string

const (
	// ViolationUnstableName returns a different identifier on each
	// Name call. The pipeline keys registration, ordering tie-breaks,
	// diagnostics and cache identity off that string.
	ViolationUnstableName Violation = "unstable-name"

	// ViolationNoRole implements Plugin and no role interface, so the
	// pipeline would register it and never invoke it.
	ViolationNoRole Violation = "no-role"

	// ViolationPartialCapability declares Priority without Provides
	// and Requires. CapabilityProvider is all-or-nothing, so the
	// pipeline's type assertion fails and the declared ordering is
	// silently discarded. This shape has shipped twice in this repo.
	ViolationPartialCapability Violation = "partial-capability"

	// ViolationUnstableProvides returns a freshly-shuffled Provides
	// on each call, making the capability topo-sort non-deterministic.
	ViolationUnstableProvides Violation = "unstable-provides"

	// ViolationDuplicateDirective declares two schemas under one name.
	// The registry holds one owner per directive, so the second
	// silently loses.
	ViolationDuplicateDirective Violation = "duplicate-directive"

	// ViolationUnstableVersion derives its version from mutable state,
	// so a cache key composed from it changes on every run.
	ViolationUnstableVersion Violation = "unstable-version"

	// ViolationUnstableEmitVersions returns a different emit-major set
	// on each call, so Build-time compatibility checking is unsound.
	ViolationUnstableEmitVersions Violation = "unstable-emit-versions"

	// ViolationUnstableNodesOnly flips its NodesOnly declaration
	// between calls. The pipeline reads it once at Build to decide
	// whether a whole generator bucket may run concurrently.
	ViolationUnstableNodesOnly Violation = "unstable-nodes-only"

	// ViolationUnstableOutputs returns a different Outputs slice on
	// each call, so routing is not reproducible across phases.
	ViolationUnstableOutputs Violation = "unstable-outputs"

	// ViolationEmptySuffix declares an Output whose Suffix is empty.
	// Layout composes <src-basename><Suffix>, so an empty suffix
	// collides every source file's output onto one filename.
	ViolationEmptySuffix Violation = "empty-suffix"

	// ViolationDuplicateTag declares two Outputs under one Tag, so
	// `+gen:out tag=` and `-o <plugin>:<tag>` cannot address either.
	ViolationDuplicateTag Violation = "duplicate-tag"

	// ViolationTwoEmptyTags declares two primary Outputs. At most one
	// Output may carry the empty Tag.
	ViolationTwoEmptyTags Violation = "two-empty-tags"

	// ViolationEmptyTagNotFirst declares the primary Output somewhere
	// other than index 0, where the framework looks for it.
	ViolationEmptyTagNotFirst Violation = "empty-tag-not-first"

	// ViolationFuncInBothMaps registers one funcmap name in both
	// TemplateFuncs and TemplateOverrides. A name is either a new
	// registration or an intentional override, never both, and the
	// backend resolves the two through different collision rules.
	ViolationFuncInBothMaps Violation = "func-in-both-maps"

	// ViolationSharedSlice returns the plugin's own slice from a
	// declaration accessor, so a caller that sorts or filters it in
	// place rewrites the plugin's declaration for everyone after.
	ViolationSharedSlice Violation = "shared-slice"

	// ViolationUnparsableTemplate ships a template file that does not
	// parse. Today that surfaces mid-Render, after the run has
	// already done its work.
	//
	// The fixture ships it one directory below the filesystem root,
	// because that is where a real one lands: `//go:embed
	// templates/golang/*.tmpl` without the matching [fs.Sub] produces
	// exactly this shape, and a root-only enumeration would validate
	// nothing and report green.
	ViolationUnparsableTemplate Violation = "unparsable-template"

	// ViolationReservedFuncName registers a funcmap entry under a name
	// the backend reserves for its own dispatch helpers. The backend
	// rejects it while merging plugin contributions — before it renders
	// anything — so the run writes no files at all, for every plugin in
	// the composition rather than only this one.
	ViolationReservedFuncName Violation = "reserved-func-name"
)

// Violations returns every [Violation] this package ships, sorted.
//
// The suite's completeness meta-test walks this rather than a
// hand-maintained list, so a Violation added without a check to catch
// it — or a check added without a Violation to defeat it — fails the
// build on the commit that introduces the gap.
func Violations() []Violation {
	out := []Violation{
		ViolationUnstableName,
		ViolationNoRole,
		ViolationPartialCapability,
		ViolationUnstableProvides,
		ViolationDuplicateDirective,
		ViolationUnstableVersion,
		ViolationUnstableEmitVersions,
		ViolationUnstableNodesOnly,
		ViolationUnstableOutputs,
		ViolationEmptySuffix,
		ViolationDuplicateTag,
		ViolationTwoEmptyTags,
		ViolationEmptyTagNotFirst,
		ViolationFuncInBothMaps,
		ViolationUnparsableTemplate,
		ViolationReservedFuncName,
		ViolationSharedSlice,
	}
	slices.Sort(out)
	return out
}

// BrokenPlugin returns a plugin that breaks exactly v and satisfies
// every other framework contract.
//
// The "and nothing else" half is what makes the fixture useful. A
// plugin that is wrong in several ways makes a failing suite
// ambiguous about which check did the work, and a plugin that panics
// in its constructor makes the suite fail while proving nothing. Each
// fixture below is [NewFixturePlugin] with one behaviour overridden.
//
// An unrecognised Violation returns a well-formed plugin, so a
// caller's typo surfaces as "the suite unexpectedly passed" rather
// than a nil dereference.
func BrokenPlugin(v Violation) plugin.Plugin {
	base := NewFixturePlugin()
	switch v {
	case ViolationUnstableName:
		return &brokenUnstableName{FixturePlugin: base}
	case ViolationNoRole:
		return &brokenNoRole{}
	case ViolationPartialCapability:
		return &brokenPartialCapability{name: base.PluginName}
	case ViolationUnstableProvides:
		return &brokenUnstableProvides{FixturePlugin: base}
	case ViolationDuplicateDirective:
		base.DirectiveSchemas = []directive.Schema{
			directive.NewSchema("dupe").On("Struct").Build(),
			directive.NewSchema("dupe").On("Interface").Build(),
		}
		return base
	case ViolationUnstableVersion:
		return &brokenUnstableVersion{FixturePlugin: base}
	case ViolationUnstableEmitVersions:
		return &brokenUnstableEmitVersions{FixturePlugin: base}
	case ViolationUnstableNodesOnly:
		return &brokenUnstableNodesOnly{FixturePlugin: base}
	case ViolationUnstableOutputs:
		return &brokenUnstableOutputs{FixturePlugin: base}
	case ViolationEmptySuffix:
		base.OutputsByLang = map[string][]plugin.Output{
			ConformanceLanguage: {{Suffix: ""}},
		}
		return base
	case ViolationDuplicateTag:
		base.OutputsByLang = map[string][]plugin.Output{
			ConformanceLanguage: {
				{Tag: fixtureTag, Suffix: fixtureSuffixA},
				{Tag: fixtureTag, Suffix: fixtureSuffixB},
			},
		}
		return base
	case ViolationTwoEmptyTags:
		base.OutputsByLang = map[string][]plugin.Output{
			ConformanceLanguage: {
				{Suffix: fixtureSuffixA},
				{Suffix: fixtureSuffixB},
			},
		}
		return base
	case ViolationEmptyTagNotFirst:
		base.OutputsByLang = map[string][]plugin.Output{
			ConformanceLanguage: {
				{Tag: fixtureTag, Suffix: fixtureSuffixA},
				{Suffix: fixtureSuffixB},
			},
		}
		return base
	case ViolationSharedSlice:
		return &brokenSharedSlice{FixturePlugin: base}
	case ViolationFuncInBothMaps:
		return &brokenTemplateProvider{FixturePlugin: base, bothMaps: true}
	case ViolationUnparsableTemplate:
		return &brokenTemplateProvider{FixturePlugin: base, unparsable: true}
	case ViolationReservedFuncName:
		return &brokenTemplateProvider{FixturePlugin: base, reservedFunc: true}
	default:
		return base
	}
}

// brokenUnstableName violates the stable-identifier contract and
// nothing else.
type brokenUnstableName struct {
	*FixturePlugin
	calls int
}

func (p *brokenUnstableName) Name() string {
	p.calls++
	return "fixture-" + string(rune('a'+p.calls%26))
}

// brokenNoRole violates the at-least-one-role contract and nothing
// else. It deliberately does not embed [FixturePlugin], because
// embedding would inherit Generate and satisfy the very contract this
// fixture exists to break.
type brokenNoRole struct{}

func (*brokenNoRole) Name() string { return "no-role" }

// brokenPartialCapability declares Priority without Provides and
// Requires, violating CapabilityProvider's all-or-nothing contract
// and nothing else. It does not embed [FixturePlugin] for the same
// reason as [brokenNoRole]: the embedded methods would complete the
// interface.
type brokenPartialCapability struct{ name string }

func (p *brokenPartialCapability) Name() string { return p.name }

func (*brokenPartialCapability) Generate(_ *plugin.GeneratorContext) error { return nil }

func (*brokenPartialCapability) Priority() priority.Priority { return priority.GeneratorFoundation }

// brokenUnstableProvides violates the deterministic-capability
// contract and nothing else.
type brokenUnstableProvides struct {
	*FixturePlugin
	calls int
}

func (p *brokenUnstableProvides) Provides() []string {
	p.calls++
	if p.calls%2 == 0 {
		return []string{fixtureCapability, "cap.two"}
	}
	return []string{fixtureCapability}
}

// brokenUnstableVersion violates the stable-version contract and
// nothing else.
type brokenUnstableVersion struct {
	*FixturePlugin
	calls int
}

func (p *brokenUnstableVersion) Version() string {
	p.calls++
	return "v1.0." + string(rune('0'+p.calls%10))
}

// brokenUnstableEmitVersions violates the stable-emit-majors contract
// and nothing else.
type brokenUnstableEmitVersions struct {
	*FixturePlugin
	calls int
}

func (p *brokenUnstableEmitVersions) EmitVersions() []string {
	p.calls++
	if p.calls%2 == 0 {
		return []string{"1", "2"}
	}
	return []string{"1"}
}

// brokenUnstableNodesOnly violates the stable-NodesOnly contract and
// nothing else.
type brokenUnstableNodesOnly struct {
	*FixturePlugin
	calls int
}

func (p *brokenUnstableNodesOnly) NodesOnly() bool {
	p.calls++
	return p.calls%2 == 0
}

// brokenUnstableOutputs violates the stable-Outputs contract and
// nothing else. The two shapes are individually well-formed, so the
// shape check still passes and only the stability check fires.
type brokenUnstableOutputs struct {
	*FixturePlugin
	calls int
}

func (p *brokenUnstableOutputs) Outputs(lang string) []plugin.Output {
	if lang != ConformanceLanguage {
		return nil
	}
	p.calls++
	if p.calls%2 == 0 {
		return []plugin.Output{{Suffix: fixtureSuffixA}}
	}
	return []plugin.Output{{Suffix: fixtureSuffixB}}
}

// brokenTemplateProvider violates one TemplateProvider contract,
// selected by the flag set on it, and nothing else.
type brokenTemplateProvider struct {
	*FixturePlugin
	bothMaps     bool
	unparsable   bool
	reservedFunc bool
}

// fixtureTemplateDir is the directory the fixture nests its template
// under. It mirrors the documented embed idiom — `//go:embed
// templates/golang/*.tmpl` — so the shipped fixture exercises the
// recursive enumeration rather than only the filesystem root. A
// regression to a root-only glob makes this fixture stop defeating the
// check it names, which the completeness meta-test reports.
const fixtureTemplateDir = "templates/golang/"

func (p *brokenTemplateProvider) Templates(lang string) (fs.FS, bool) {
	if lang != ConformanceLanguage {
		return nil, false
	}
	body := `{{ define "fixture.ok" }}ok{{ end }}`
	if p.unparsable {
		// An unterminated action: parses as text/template only up to
		// the opening brace, which is exactly the failure that
		// currently waits until Render to surface.
		body = `{{ define "fixture.bad" }}{{ .Unclosed `
	}
	return fstest.MapFS{
		fixtureTemplateDir + "tmpl.tmpl": &fstest.MapFile{Data: []byte(body)},
	}, true
}

func (p *brokenTemplateProvider) TemplateFuncs(lang string) template.FuncMap {
	if lang != ConformanceLanguage {
		return nil
	}
	if p.reservedFunc {
		// `imp` is the backend's import-collection helper. A plugin
		// registering it shadows the one every core template calls.
		return template.FuncMap{"imp": func() string { return "" }}
	}
	return template.FuncMap{"fixtureHelper": func() string { return "" }}
}

func (p *brokenTemplateProvider) TemplateOverrides(lang string) template.FuncMap {
	if lang != ConformanceLanguage || !p.bothMaps {
		return nil
	}
	// The same name in both maps: the backend resolves registrations
	// and overrides through different collision rules, so a name in
	// both is unresolvable rather than merely redundant.
	return template.FuncMap{"fixtureHelper": func() string { return "" }}
}

// brokenSharedSlice hands out its own backing array from a
// declaration accessor, violating the caller-may-keep contract and
// nothing else.
type brokenSharedSlice struct {
	*FixturePlugin
	shared []string
}

func (p *brokenSharedSlice) Provides() []string {
	if p.shared == nil {
		p.shared = []string{"cap.one"}
	}
	return p.shared
}

// LyingNodesOnlyGenerator returns a generator that declares
// [plugin.NodesOnly] and reads the emit graph anyway, violating that
// contract and nothing else.
//
// It is exported separately from [BrokenPlugin] because the contract
// it breaks is a per-role one: proving it requires driving Generate
// against a store, which the framework suite does not do. Plugin
// authors can pass it to [RunGeneratorSuite] to confirm their harness
// actually runs the truthfulness check.
func LyingNodesOnlyGenerator() plugin.Generator {
	return &brokenLyingNodesOnly{FixturePlugin: NewFixturePlugin()}
}

// ErroringGenerator returns a generator that reports an Error-severity
// diagnostic on every input and returns nil, violating the diagnostic
// contract and nothing else.
//
// Exported separately from [BrokenPlugin] for the reason
// [LyingNodesOnlyGenerator] is: the contract it breaks is a per-role
// one, and proving it needs a store to drive Generate against, which
// the framework suite never builds.
func ErroringGenerator() plugin.Generator {
	return &brokenErroring{FixturePlugin: NewFixturePlugin()}
}

// ErroringAnnotator returns an annotator that reports an
// Error-severity diagnostic on every input and returns nil.
func ErroringAnnotator() plugin.Annotator {
	return &brokenErroring{FixturePlugin: NewFixturePlugin()}
}

// ErroringFrontend returns a frontend that reports an Error-severity
// diagnostic on every input, records no nodes, and returns nil.
//
// It fails the populate check as well as the diagnostic one, so drive
// it at the individual assertion rather than through
// [RunFrontendSuite]: a whole-suite run cannot attribute the failure.
func ErroringFrontend() plugin.Frontend {
	return &brokenErroring{FixturePlugin: NewFixturePlugin()}
}

// erroringDiagnosticMessage is the text every [brokenErroring] role
// method emits. Named so a meta-test can key on it without restating
// the literal, which is how a fixture and its assertion drift apart.
const erroringDiagnosticMessage = "plugintest fixture: unsupported input"

// brokenErroring emits one Error-severity diagnostic from each role
// method it implements and then returns nil, modelling the plugin the
// three role suites certified for as long as they discarded the sink:
// well-formed everywhere the framework checks look, and unusable,
// because [pipeline.Pipeline.Run] converts that diagnostic into
// ErrRunHadErrors on the author's first real invocation.
//
// The position is deliberately zero. One fixture then defeats both
// halves of the contract — no Error-severity diagnostics, and no
// diagnostic without a source position — and the second half is what
// [GeneratorFixture.AllowsPositionlessDiagnostics] waives.
type brokenErroring struct{ *FixturePlugin }

// Generate reports the sentinel diagnostic and returns cleanly, so
// only the diagnostic checks can catch it.
func (p *brokenErroring) Generate(ctx *plugin.GeneratorContext) error {
	ctx.Diag.For(p.Name()).Errorf(position.Pos{}, erroringDiagnosticMessage)
	return nil
}

// Annotate reports the sentinel diagnostic and returns cleanly.
func (p *brokenErroring) Annotate(ctx *plugin.AnnotatorContext) error {
	ctx.Diag.For(p.Name()).Errorf(position.Pos{}, erroringDiagnosticMessage)
	return nil
}

// Load reports the sentinel diagnostic, records nothing, and returns
// cleanly.
func (p *brokenErroring) Load(ctx *plugin.FrontendContext) error {
	ctx.Diag.For(p.Name()).Errorf(position.Pos{}, erroringDiagnosticMessage)
	return nil
}

// brokenLyingNodesOnly declares NodesOnly and then reads the emit
// graph, which the pipeline's concurrent bucket dispatch makes a data
// race.
type brokenLyingNodesOnly struct{ *FixturePlugin }

func (*brokenLyingNodesOnly) NodesOnly() bool { return true }

func (*brokenLyingNodesOnly) Generate(ctx *plugin.GeneratorContext) error {
	// The lie: a NodesOnly generator must not reach the emit graph.
	// Reading through the Reader means the ReadSet records it, so
	// both halves of the check have something to find.
	seen := ctx.Reader.EmitStructs().Count()
	pkg := &emit.Package{Name: "lying", Path: "example.com/lying"}
	pkg.Structs = []*emit.Struct{{
		Name:    "Saw" + strconv.Itoa(seen),
		Package: "example.com/lying",
	}}
	return ctx.Store.Emit().AddPackage(pkg)
}
