// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package sentinel is the production test-generator for
// typed error contracts. Annotating a source package with
// `+gen:sentinel` opts every Err* sentinel variable and
// every exported error-implementing struct in the package
// into a generated test file that pins the contracts
// callers rely on: sentinel uniqueness, cross-pair
// non-overlap under [errors.Is], wrap-chain preservation
// through [errors.Join] and [fmt.Errorf], and per-error-
// type [errors.As] round-trip / format completeness /
// non-overlap.
//
// The plugin generates ONLY tests — the user authors the
// sentinel variables and error types themselves, the plugin
// asserts their invariants. No production code is
// synthesised.
//
// # Language-agnostic core, language-keyed adapters
//
// This file holds the plugin's neutral core: the directive
// schema, the [Options] surface, the [Tests] emit value,
// and the [Plugin.Generate] pass that walks every annotated
// source package and queues one contribution per match.
// Per-language behaviour — the output suffix, the embedded
// template tree, the funcmap entries — lives in the sibling
// `sentinel_<lang>.go` adapter.
//
// # Source patterns scanned
//
//  1. Exported package-level variables whose identifier
//     begins with `Err` (e.g. `var ErrUserNotFound = errors.New(...)`)
//     — collected into the package's sentinel set.
//
//  2. Exported structs whose method set includes
//     `Error() string` — collected as custom error types.
//     The plugin separately notes whether each type
//     provides `Is(error) bool` and / or `Unwrap() error`
//     so the rendered tests cover those branches when
//     present.
//
// # Output
//
// One test file per annotated source package, rendered
// through the `sentinel.tests` template. The file's
// basename derives from the source file the anchor entity
// (the first Err* var or the first error type in source
// order) lives in, so the rendered tests colocate with the
// package's error declarations.
//
// # Runtime
//
// The rendered tests rely only on the standard library
// (`testing`, `errors`, `fmt`, `strings`). No external
// runtime dependency.
package sentinel

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"text/template"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier.
const Name = "sentinel"

// Version is the plugin's declared version. It composes into the
// pipeline's plugin fingerprint, which frontends fold into their cache
// keys — so bumping it invalidates a warm cache populated when this
// plugin behaved differently. A plugin that declares no version
// contributes an empty string and can never invalidate anything, which
// is a silent staleness bug waiting for its first behavioural change.
const Version = "1.0.0"

// Capability is the capability label the plugin advertises
// so downstream consumers can declare a documentary
// dependency through their own `Requires` list.
const Capability = "sentinel"

// DirectiveName is the bare directive name (without the
// `+gen:` or `-gen:` prefix) the plugin reads from source
// package declarations to opt the package in for test
// generation.
const DirectiveName sdk.DirectiveName = "sentinel"

// SlotName is the [emit.File] slot the plugin appends its
// rendered [Tests] emit values into. `top` renders the
// content between the package clause and the first core
// decl — the natural placement for a self-contained
// test-function block. Universal across target languages.
const SlotName = "top"

// KindTests is the plugin-defined [sdk.Kind] every [Tests]
// emit value reports. The matching template per language
// lives at `templates/<lang>/sentinel.tests.tmpl`.
const KindTests sdk.Kind = "sentinel.tests"

// ErrPrefix is the variable-name prefix that distinguishes
// a sentinel error from any other exported package-level
// var.
const ErrPrefix = "Err"

// ErrorMethodName is the method name on a struct that
// signals the type implements the error interface.
const ErrorMethodName = "Error"

// IsMethodName / UnwrapMethodName are the additional
// optional methods the rendered tests cover when present.
const (
	IsMethodName     = "Is"
	UnwrapMethodName = "Unwrap"
)

// errorTypeName is the rendered string for the builtin
// error interface — used to identify error-typed fields
// without importing go/types.
const errorTypeName = "error"

// nilLiteral is the zero spelling [buildFieldData] refuses.
// Named rather than inlined because the refusal is a rule
// about Go's untyped nil, not about the three characters.
const nilLiteral = "nil"

// PrefixKey is the directive keyword that pins (or kills)
// the message prefix every sentinel's `.Error()` is
// asserted to start with. Resolved per-package in
// [resolvePrefix].
const PrefixKey = "prefix"

// PrefixOff is the directive-arg sentinel that disables the
// prefix subtest entirely — `+gen:sentinel prefix=off`. An
// empty value (`+gen:sentinel prefix=`) carries the same
// meaning; the rendered test suite simply omits the prefix
// subtest for that package.
const PrefixOff = "off"

// Options carries the plugin's user-tunable settings. The
// sentinel plugin has no behaviour toggles today — the
// per-package `+gen:sentinel prefix=…` directive arg covers
// the only configurable knob the rendered tests surface.
// The struct exists so future settings can land without
// changing the plugin's options surface.
type Options struct{}

// Plugin is the sentinel test generator. Zero value is
// unusable; go through [New] so the embedded [sdk.Holder]
// binds to the options field.
type Plugin struct {
	*sdk.Holder[Options]
	opts Options
}

// New returns a fresh plugin instance with the options
// holder bound.
func New() *Plugin {
	p := &Plugin{}
	p.Holder = sdk.BindOptions(&p.opts)
	return p
}

// Name returns [Name].
func (*Plugin) Name() string { return Name }

// Version satisfies [sdk.Versioned].
func (*Plugin) Version() string { return Version }

// Priority places the plugin in the cross-cutting bucket so
// it runs after foundation / composition generators — its
// scan observes the post-generation package shape, which
// lets it assert invariants over Err* vars / error types
// that other plugins may have synthesised.
func (*Plugin) Priority() sdk.Priority { return sdk.GeneratorCrossCutting }

// Provides advertises [Capability].
func (*Plugin) Provides() []string { return []string{Capability} }

// Requires returns nil — the sentinel plugin reads source
// nodes only.
func (*Plugin) Requires() []string { return nil }

// Directives declares the `+gen:sentinel` schema on the
// source package node — annotating the package's doc
// comment opts every error in the package into the
// generated test suite. The optional `prefix=` keyword arg
// overrides the default `<pkg>: ` prefix the rendered
// prefix subtest asserts; an empty value or the literal
// `off` disables the subtest.
func (*Plugin) Directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			On(node.KindPackage).
			AllowedKeys(PrefixKey).
			Describe(
				"Opts the host source package in for sentinel-error / error-type " +
					"test generation. `prefix=<value>` overrides the default `<pkg>: ` " +
					"prefix asserted on every Err* var's .Error(); `prefix=off` (or " +
					"`prefix=`) disables the prefix subtest entirely.",
			).
			Build(),
	}
}

// Outputs dispatches to the per-language adapter for the
// requested language.
func (*Plugin) Outputs(lang string) []sdk.Output {
	if lang == golang.Language {
		return GoOutputs()
	}
	return nil
}

// Templates dispatches to the per-language adapter's
// embedded template tree.
func (*Plugin) Templates(lang string) (fs.FS, bool) {
	if lang == golang.Language {
		return GoTemplates()
	}
	return nil, false
}

// TemplateFuncs dispatches to the per-language adapter's
// funcmap.
func (*Plugin) TemplateFuncs(lang string) template.FuncMap {
	if lang == golang.Language {
		return GoFuncMap()
	}
	return nil
}

// TemplateOverrides returns nil — no per-language adapter
// currently overrides a canonical funcmap entry.
func (*Plugin) TemplateOverrides(string) template.FuncMap { return nil }

// Sentinel is one Err* package-level variable rolled into
// the rendered test suite.
type Sentinel struct {
	// Name is the variable identifier (e.g.
	// `ErrUserNotFound`).
	Name string

	// Ref is the [sdk.NewExternal] expression that renders
	// as the fully-qualified reference to the variable from
	// the external test package (e.g. `blog.ErrUserNotFound`).
	// The renderer registers the source package's import on
	// the rendered file's import set automatically.
	Ref *sdk.Expr
}

// Field is one exported field on a custom error type
// carrying the metadata the per-type tests need: type
// string for the sample-value selector, the sample value
// the test passes when constructing the error, and the
// substring the format-string assertion looks for. IsError
// is true when the field's type is the builtin `error`
// interface.
//
// Only fields the rendered tests can actually exercise
// reach this form — see [buildFieldData] for what is
// dropped and why.
type Field struct {
	Name             string
	TypeStr          string
	SampleValue      string
	FormatCheckValue string
	IsError          bool
}

// ErrorType is one custom error-implementing struct rolled
// into the rendered test suite.
type ErrorType struct {
	// Name is the struct identifier (e.g.
	// `ValidationError`).
	Name string

	// Ref is the [sdk.NewExternal] expression that renders
	// as the fully-qualified type reference from the
	// external test package (e.g. `blog.ValidationError`).
	Ref *sdk.Expr

	// OtherTypeRefs is the matching [sdk.NewExternal]
	// expression list for [OtherTypes] — the rendered
	// non-overlap subtest references peer types through
	// these so the import set stays correct under the
	// external-test-package shift.
	OtherTypeRefs []*sdk.Expr

	// Fields lists the exported fields the rendered tests
	// exercise — round-tripped through [errors.As] and
	// asserted to appear in the `Error()` format string.
	Fields []Field

	// OtherTypes is the names of every OTHER error type in
	// the same package — the rendered tests assert this
	// type does not [errors.Is]-match foreign types.
	OtherTypes []string

	// FormatCheckOrder is the ordered list of substrings the
	// rendered "Error format includes all fields" subtest
	// expects to find in source order. Pre-computed so the
	// template stays free of list-building primitives.
	FormatCheckOrder []string

	// HasIs / HasUnwrap report whether the struct provides
	// the matching method — the rendered tests gate the
	// corresponding subtests on these flags.
	HasIs     bool
	HasUnwrap bool

	// UnwrapField is the name of the field the Unwrap test
	// expects to be returned from `Unwrap()`. Populated when
	// the struct provides Unwrap AND carries at least one
	// error-typed field; empty otherwise.
	UnwrapField string
}

// Tests is the plugin-defined emit kind the rendered emit
// value reports. The matching `sentinel.tests` template
// renders it as the full test-function set for one source
// package.
//
// The struct carries only language-neutral data. The
// per-language template reaches stdlib symbols
// (`testing.T`, `errors.{Is,As,Join,New}`, `fmt.Errorf`,
// `strings.{HasPrefix,Contains}`) through the backend's
// `external` funcmap entry; the matching imports register
// on the host file via `renderExpr` automatically.
type Tests struct {
	sdk.BaseEmit

	// TestName is the suffix the rendered
	// `TestXxxSentinelErrors` function uses. Derived from
	// the package's short name in CamelCase (e.g.
	// `package auth` → `AuthSentinelErrors`).
	TestName string

	// Prefix is the message prefix every Err* sentinel's
	// `.Error()` is asserted to start with. Only meaningful
	// when [EmitPrefix] is true.
	Prefix string

	// EmitPrefix gates the rendered prefix subtest. False
	// when the source package declared `prefix=off` or
	// `prefix=` on `+gen:sentinel`, signalling the author
	// doesn't want a prefix assertion at all.
	EmitPrefix bool

	// Sentinels is the package's Err* variable set in
	// source order.
	Sentinels []Sentinel

	// ErrorTypes is the package's custom error-implementing
	// struct set in source order.
	ErrorTypes []ErrorType
}

// Kind returns [KindTests].
func (*Tests) Kind() sdk.Kind { return KindTests }

// Compile-time confirmation that *Tests satisfies
// [sdk.EmitNode].
var _ sdk.EmitNode = (*Tests)(nil)

// HasContent reports whether the package contributed any
// sentinels or error types worth generating tests for.
// Empty packages drop from the output set without surfacing
// a diagnostic.
func (t *Tests) HasContent() bool {
	return len(t.Sentinels) > 0 || len(t.ErrorTypes) > 0
}

// Generate walks every annotated source package, scans it
// for Err* sentinels and custom error types, and queues one
// [Tests] contribution against the first contributing
// entity's source file when the scan yields anything. The
// Layout phase resolves the contribution to a sibling
// `<basename>_sentinel_test.go` file via the standard
// routing precedence.
func (*Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name)
	for pkg := range ctx.Reader.Packages().All() {
		if !pkg.HasPositiveDirective(DirectiveName) {
			continue
		}
		sentinels := collectSentinels(pkg)
		errorTypes := collectErrorTypes(pkg)
		if len(sentinels) == 0 && len(errorTypes) == 0 {
			continue
		}
		applyExternalRefs(pkg.Path, sentinels, errorTypes)
		anchor := chooseAnchor(pkg, sentinels, errorTypes)
		if anchor == nil {
			continue
		}
		prefix, emitPrefix := resolvePrefix(pkg)
		tests := &Tests{
			BaseEmit: sdk.BaseEmit{
				OriginNode: anchor,
				SetByName:  c.SetBy(),
				SourcePos:  anchor.Pos(),
			},
			TestName:   testNameFor(pkg.Name),
			Prefix:     prefix,
			EmitPrefix: emitPrefix,
			Sentinels:  sentinels,
			ErrorTypes: errorTypes,
		}
		prov := c.Provenance("sentinel.tests." + pkg.Name)
		if err := ctx.Store.Emit().AppendOriginSlot(anchor, SlotName, tests, prov); err != nil {
			return fmt.Errorf("%s: append slot for package %q: %w", Name, pkg.Name, err)
		}
	}
	return nil
}

// collectSentinels returns the package's Err* variable set
// — every exported variable whose identifier starts with
// the canonical `Err` prefix.
func collectSentinels(pkg *node.Package) []Sentinel {
	var out []Sentinel
	for _, v := range pkg.Variables {
		if !isExported(v.Name) {
			continue
		}
		if !strings.HasPrefix(v.Name, ErrPrefix) {
			continue
		}
		out = append(out, Sentinel{Name: v.Name})
	}
	return out
}

// collectErrorTypes returns the package's custom error-type
// set — every exported struct whose method list includes
// `Error() string`. Per-type analysis (field samples,
// OtherTypes, HasIs / HasUnwrap) runs in a second pass once
// the full set is known so the cross-type non-overlap
// subtest data can be pre-computed.
func collectErrorTypes(pkg *node.Package) []ErrorType {
	var raw []*node.Struct
	for _, s := range pkg.Structs {
		if !isExported(s.Name) {
			continue
		}
		if !structImplementsError(s) {
			continue
		}
		raw = append(raw, s)
	}
	out := make([]ErrorType, 0, len(raw))
	for _, s := range raw {
		et := ErrorType{
			Name:      s.Name,
			HasIs:     hasMethod(s, IsMethodName, isErrorBoolSignature),
			HasUnwrap: hasMethod(s, UnwrapMethodName, returnsErrorOnly),
		}
		for _, f := range s.Fields {
			if !isExported(f.Name) {
				continue
			}
			fd, exercisable := buildFieldData(f)
			if !exercisable {
				continue
			}
			et.Fields = append(et.Fields, fd)
			if fd.IsError && et.HasUnwrap && et.UnwrapField == "" {
				et.UnwrapField = fd.Name
			}
			if fd.FormatCheckValue != "" {
				et.FormatCheckOrder = append(et.FormatCheckOrder, fd.FormatCheckValue)
			}
		}
		out = append(out, et)
	}
	for i := range out {
		for j := range out {
			if i == j {
				continue
			}
			out[i].OtherTypes = append(out[i].OtherTypes, out[j].Name)
		}
	}
	return out
}

// buildFieldData picks the rendered sample value and
// format-check substring for one struct field, and reports
// whether the rendered tests can exercise the field at all.
//
// string, int and error get a distinctive sample the format
// assertion can also look for. Everything else falls back to
// the type's zero literal, which the rendered test uses in
// two positions Go constrains further than a composite
// literal does: `want<F> := <sample>` needs a value with a
// default type, and `target.<F> != want<F>` needs a
// comparable one. The nil keyword satisfies neither — it has
// no default type — so a field whose only derivable zero is
// nil is dropped rather than rendered into a test that does
// not compile.
//
// A dropped field costs one assertion. Rendering it costs
// the whole file: the failure lands in the consumer's build
// of generated code, which is the last place an author looks.
func buildFieldData(f *node.Field) (Field, bool) {
	fd := Field{Name: f.Name}
	if f.Type != nil {
		fd.TypeStr = f.Type.Name
	}
	lower := strings.ToLower(f.Name)
	switch fd.TypeStr {
	case errorTypeName:
		fd.SampleValue = `errors.New("test-` + lower + `")`
		fd.FormatCheckValue = "test-" + lower
		fd.IsError = true
	case "string":
		fd.SampleValue = `"test-` + lower + `"`
		fd.FormatCheckValue = "test-" + lower
	case "int", "int32", "int64":
		fd.SampleValue = "42"
		fd.FormatCheckValue = "42"
	default:
		// Read through the shared vocabulary rather than a
		// private table: a partial copy answered "nil" for the
		// widths it omitted, which is how `Code: nil` reached an
		// int8 field.
		zero, derivable := golang.ZeroLiteral(f.Type)
		if !derivable || zero == nilLiteral {
			return Field{}, false
		}
		fd.SampleValue = zero
	}
	return fd, true
}

// applyExternalRefs stamps [sdk.NewExternal] expressions
// onto every sentinel and error-type so the template can
// render fully-qualified `<pkg>.<Name>` references that
// the backend's import scanner registers against the
// rendered file's import set automatically. Necessary
// because the framework's `_test.go → <pkg>_test` package
// shift moves the rendered file into an external test
// package — the source-package references must be
// qualified at that point.
func applyExternalRefs(packagePath string, sentinels []Sentinel, errorTypes []ErrorType) {
	for i := range sentinels {
		sentinels[i].Ref = sdk.NewExternal(packagePath, sentinels[i].Name)
	}
	for i := range errorTypes {
		errorTypes[i].Ref = sdk.NewExternal(packagePath, errorTypes[i].Name)
		for _, other := range errorTypes[i].OtherTypes {
			errorTypes[i].OtherTypeRefs = append(
				errorTypes[i].OtherTypeRefs,
				sdk.NewExternal(packagePath, other),
			)
		}
	}
}

// chooseAnchor returns the entity to anchor the [Tests]
// emit value to — the first Err* var in source order, or
// the first error type when the package declares no Err*
// vars. Both lists are guaranteed non-empty when this
// function is called.
func chooseAnchor(pkg *node.Package, sentinels []Sentinel, errorTypes []ErrorType) node.Node {
	if len(sentinels) > 0 {
		first := sentinels[0].Name
		for _, v := range pkg.Variables {
			if v.Name == first {
				return v
			}
		}
	}
	if len(errorTypes) > 0 {
		first := errorTypes[0].Name
		for _, s := range pkg.Structs {
			if s.Name == first {
				return s
			}
		}
	}
	return nil
}

// structImplementsError reports whether s declares an
// `Error() string` method.
func structImplementsError(s *node.Struct) bool {
	return hasMethod(s, ErrorMethodName, returnsStringOnly)
}

// hasMethod reports whether s declares a method named name
// whose signature satisfies the supplied signature
// predicate.
func hasMethod(s *node.Struct, name string, sigOK func(*node.Method) bool) bool {
	for _, m := range s.Methods {
		if m.Name == name && sigOK(m) {
			return true
		}
	}
	return false
}

// returnsOnly reports whether m's single return slot is the
// named builtin.
//
// The slot's type is what is asked, never the slot's binding
// name. [node.Return] carries both, and the binding name is
// empty for the anonymous form every `Error() string` in the
// wild is written in — so reading it classifies no method as
// anything.
func returnsOnly(m *node.Method, typeName string) bool {
	if len(m.Returns) != 1 || m.Returns[0] == nil {
		return false
	}
	t := m.Returns[0].Type
	return t != nil && t.Name == typeName && t.Package == ""
}

// returnsStringOnly reports whether m's signature is
// `() string`.
func returnsStringOnly(m *node.Method) bool {
	return len(m.Params) == 0 && returnsOnly(m, "string")
}

// returnsErrorOnly reports whether m's signature is
// `() error`.
func returnsErrorOnly(m *node.Method) bool {
	return len(m.Params) == 0 && returnsOnly(m, errorTypeName)
}

// isErrorBoolSignature reports whether m's signature is
// `(error) bool` — the standard `Is(error) bool` shape
// [errors.Is] consults.
func isErrorBoolSignature(m *node.Method) bool {
	if len(m.Params) != 1 {
		return false
	}
	if m.Params[0].Type == nil || m.Params[0].Type.Name != errorTypeName {
		return false
	}
	return returnsOnly(m, "bool")
}

// isExported reports whether name follows Go's
// exported-identifier rule (first rune upper-case ASCII for
// the simple cases the plugin targets).
func isExported(name string) bool {
	if name == "" {
		return false
	}
	c := name[0]
	return c >= 'A' && c <= 'Z'
}

// testNameFor returns the suffix the rendered
// `TestXxxSentinelErrors` function uses, derived from the
// package name in CamelCase. Idiomatic Go test naming keeps
// the rendered output predictable when callers grep for
// test functions.
func testNameFor(packageName string) string {
	if packageName == "" {
		return "SentinelErrors"
	}
	return strings.ToUpper(packageName[:1]) + packageName[1:] + "SentinelErrors"
}

// resolvePrefix reads the per-package `+gen:sentinel prefix=…`
// override and returns the effective prefix plus a flag for
// whether the prefix subtest should be emitted at all.
//
// Resolution table:
//
//	directive                             effective prefix    emit?
//	------------------------------------- ------------------- ----
//	+gen:sentinel                          "<pkg>: "          yes
//	+gen:sentinel prefix=<value>           "<value>"          yes
//	+gen:sentinel prefix=                  ""                 no
//	+gen:sentinel prefix=off               ""                 no
//
// Empty value and the literal `off` are synonyms — both
// express "I don't want the prefix assertion." The rendered
// tests omit the subtest in that case so the assertion
// isn't silently vacuous (every string starts with the
// empty string).
func resolvePrefix(pkg *node.Package) (prefix string, emit bool) {
	d := pkg.Directive(DirectiveName)
	if d == nil {
		return pkg.Name + ": ", true
	}
	value, hasKey := d.KV[PrefixKey]
	if !hasKey {
		return pkg.Name + ": ", true
	}
	if value == "" || value == PrefixOff {
		return "", false
	}
	return value, true
}

// ErrUnused is documented for symmetry with the other
// generator plugins' sentinel-error pattern; the sentinel
// plugin's diagnostic surface is currently empty, so the
// symbol exists only to leave room for future framework-
// level rejection paths (e.g. annotated package without
// any errors → warning).
var ErrUnused = errors.New("sentinel: reserved sentinel — currently unused")
