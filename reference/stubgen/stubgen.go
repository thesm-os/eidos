// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package stubgen

import (
	"fmt"
	"io/fs"
	"text/template"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier.
const Name = "stubgen"

// Version is the plugin's declared version. It composes into the
// pipeline's plugin fingerprint, which frontends fold into their cache
// keys — so bumping it invalidates a warm cache populated when this
// plugin behaved differently. A plugin that declares no version
// contributes an empty string and can never invalidate anything, which
// is a silent staleness bug waiting for its first behavioural change.
const Version = "1.0.0"

// Capability is the label the plugin advertises so a downstream
// consumer can declare a documentary dependency on stub generation.
const Capability = "stub"

// DirectiveName is the bare directive name — without the `+gen:`
// prefix — the plugin reads from source interfaces.
const DirectiveName sdk.DirectiveName = "stub"

// SlotName is the [emit.File] slot both emit values append into.
// `top` renders between the package clause and the first core decl,
// which is where a template-rendered block of whole declarations
// belongs.
const SlotName = "top"

// KindStub and KindStubTests are the plugin-defined emit kinds. The
// backend resolves a template by the kind's string value, so each
// constant doubles as the name the matching template defines.
const (
	KindStub      sdk.Kind = "stubgen.stub"
	KindStubTests sdk.Kind = "stubgen.tests"
)

// DefaultSuffix is the trailer appended to the source interface's
// name to form the stub type's identifier.
const DefaultSuffix = "Stub"

// langGo is the backend language identifier the per-language
// adapters key on. Every dispatcher below compares against it, so a
// second language arrives as one more arm rather than a scattered
// string literal.
const langGo = "golang"

// Options carries the plugin's user-tunable settings.
//
// Recording is deliberately absent. A stub that records nothing is
// mockgen, and a toggle would leave two reference plugins whose
// difference is a config value rather than a purpose.
type Options struct {
	// Suffix overrides the stub type's name suffix. Empty falls back
	// to [DefaultSuffix].
	Suffix string `eidos:"suffix,default=Stub"`
}

// Plugin is the stub generator. The zero value is unusable; go
// through [New] so the embedded holder binds to the options field.
type Plugin struct {
	*sdk.Holder[Options]
	opts Options
}

// New returns a fresh plugin instance with the options holder bound.
func New() *Plugin {
	p := &Plugin{}
	p.Holder = sdk.BindOptions(&p.opts)
	return p
}

// Name returns [Name].
func (*Plugin) Name() string { return Name }

// Version satisfies [sdk.Versioned].
func (*Plugin) Version() string { return Version }

// Priority places the plugin in the foundation bucket: the stub is a
// base type other generators may decorate, so it must exist before
// composition and cross-cutting plugins run.
func (*Plugin) Priority() sdk.Priority { return sdk.GeneratorFoundation }

// Provides advertises [Capability].
func (*Plugin) Provides() []string { return []string{Capability} }

// Requires returns nil — the plugin reads source interfaces and
// depends on no other plugin's contribution.
func (*Plugin) Requires() []string { return nil }

// Directives declares the `+gen:stub` schema.
//
// The directive takes no positional argument and denies negation: a
// stub exists exactly where one is declared, so deleting the line is
// the suppression and a negated form would have nothing to act on.
func (*Plugin) Directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			Describe(
				"Generates a recording test double for the annotated interface, " +
					"plus a companion test file proving the double satisfies it. " +
					"Takes no arguments. The negated form is rejected — a stub " +
					"exists only where declared, so removing the directive is the " +
					"suppression.",
			).
			On(node.KindInterface).
			DenyNegation().
			Build(),
	}
}

// Outputs dispatches to the per-language adapter. Adding a language
// adds an arm here; unknown languages return nil, which the
// framework reads as "no routable output for this backend".
func (*Plugin) Outputs(lang string) []sdk.Output {
	if lang == langGo {
		return GoOutputs()
	}
	return nil
}

// Templates dispatches to the per-language adapter's template tree.
func (*Plugin) Templates(lang string) (fs.FS, bool) {
	if lang == langGo {
		return GoTemplates()
	}
	return nil, false
}

// TemplateFuncs dispatches to the per-language adapter's funcmap.
func (*Plugin) TemplateFuncs(string) template.FuncMap { return nil }

// TemplateOverrides returns nil — the plugin replaces no canonical
// funcmap entry.
func (*Plugin) TemplateOverrides(string) template.FuncMap { return nil }

// suffix returns the configured stub-type suffix, or the documented
// default when unset.
func (p *Plugin) suffix() string {
	if p.opts.Suffix != "" {
		return p.opts.Suffix
	}
	return DefaultSuffix
}

// Param is one rendered parameter: the in-method identifier and its
// type, already lifted to an [emit.Ref] so `renderType` consumes it.
type Param struct {
	Name string
	Type emit.Ref

	// Field is the exported field name the recorded-call struct uses
	// for this parameter.
	Field string
}

// Return is one rendered return slot.
//
// Name is the source's declared return name, empty when the
// signature did not name it. Field is always populated — a
// recorded-call struct needs a field name whether or not the source
// supplied one, so unnamed returns fall back to positional.
type Return struct {
	Name  string
	Type  emit.Ref
	Field string

	// Local is the identifier the generated body binds this return
	// to when capturing the delegate's result. It equals Name when
	// the signature declares named results, and is positional
	// otherwise. Computed in Go rather than in the template so the
	// collision guard lives next to the rule it enforces.
	Local string
}

// Method is one rendered interface method.
type Method struct {
	Name string

	// CallType is the identifier of the per-method recorded-call
	// struct — `<Iface><Method>Call`.
	CallType string

	// FuncField is the identifier of the stub's func-valued field —
	// `<Method>Func`.
	FuncField string

	// CallsField is the identifier of the recorded-call slice —
	// `<Method>Calls`.
	CallsField string

	Params  []Param
	Returns []Return

	// NamedReturns reports whether the generated signature declares
	// its return names. See [namedReturnsUsable] for why this is
	// all-or-nothing rather than per-return.
	NamedReturns bool
}

// Stub is the emit value rendered into the primary output.
type Stub struct {
	sdk.BaseEmit

	// TypeName is the stub struct's identifier — `<Iface><Suffix>`.
	TypeName string

	// IfaceName is the source interface's identifier, used in the
	// generated doc comments.
	IfaceName string

	Methods []Method
}

// Kind returns [KindStub].
func (*Stub) Kind() sdk.Kind { return KindStub }

// Tests is the emit value rendered into the tagged test output.
//
// The companion always lands in an external test package — the
// framework appends `_test` to whatever package the primary output
// resolved to — so it can never reach either type it exercises
// unqualified. Both are carried as [sdk.NewExternal] expressions and
// the backend registers the qualifying imports.
//
// The two references resolve from different places, and the
// difference is the whole reason [Tests] implements
// [emit.OutputPackageSetter]:
//
//   - IfaceRef names the source interface, which is hand-written and
//     stays where the author put it. Its package is known during
//     Generate.
//   - StubRef names the stub this plugin generates, which follows
//     `out=` / `pkg=` routing. Its package is not decided until
//     Layout, so it is filled in by [Tests.SetOutputPackages].
type Tests struct {
	sdk.BaseEmit

	TypeName  string
	IfaceName string

	// IfaceRef qualifies the source interface. Set during Generate.
	IfaceRef *emit.Expr

	// StubRef qualifies the generated stub. Set during Generate
	// against the source package as a provisional value, then
	// corrected by [Tests.SetOutputPackages] once routing resolves.
	// The provisional value is what a run without a Layout phase —
	// a direct generator unit test — observes.
	StubRef *emit.Expr

	Methods []Method
}

// Kind returns [KindStubTests].
func (*Tests) Kind() sdk.Kind { return KindStubTests }

// SetOutputPackages repoints [Tests.StubRef] at wherever Layout
// routed the primary output.
//
// The companion is always the external test package of the primary
// (`<pkg>_test`), so the reference is always qualified — there is no
// routing under which the stub and its test share a package, and
// therefore no case where the correct rendering is a bare name.
//
// An empty path for the primary tag means the Target resolved
// without a derivable import path, which centralised routing does.
// The provisional source-package reference is left in place rather
// than replaced with an unqualified name: a wrong package is a
// compile error naming the symbol, while a bare name silently binds
// to whatever else is in scope.
func (t *Tests) SetOutputPackages(byTag map[string]string) {
	if path := byTag[""]; path != "" {
		t.StubRef = sdk.NewExternal(path, t.TypeName)
	}
}

// Generate walks every source interface carrying `+gen:stub` and
// queues one [Stub] against the primary output and one [Tests]
// against the tagged test output. The Layout phase resolves each
// contribution's target; both land beside the source interface by
// default and follow directive / config / CLI overrides otherwise.
//
// Interfaces without the directive are skipped silently. An
// annotated interface with no methods is skipped with a positioned
// diagnostic — a double with no behaviour to stand in for is
// certainly a mistake, and emitting an empty struct would hide it.
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name, sdk.EmitTarget{})
	for iface := range ctx.Reader.Interfaces().All() {
		if !iface.HasPositiveDirective(DirectiveName) {
			continue
		}
		if len(iface.Methods) == 0 {
			ctx.Diag.Errorf(iface.Pos(),
				"%s: interface %q carries +gen:%s but declares no methods; nothing to double",
				Name, iface.QName(), DirectiveName)
			continue
		}

		typeName := iface.Name + p.suffix()
		methods := methodsOf(iface)

		stub := &Stub{
			BaseEmit: sdk.BaseEmit{
				OriginNode: iface,
				SetByName:  c.SetBy(),
				SourcePos:  iface.Pos(),
			},
			TypeName:  typeName,
			IfaceName: iface.Name,
			Methods:   methods,
		}
		if err := ctx.Store.Emit().AppendOriginSlot(
			iface, SlotName, stub, c.Provenance("stubgen.stub."+iface.Name),
		); err != nil {
			return fmt.Errorf("%s: append stub slot: %w", Name, err)
		}

		tests := &Tests{
			BaseEmit: sdk.BaseEmit{
				OriginNode:    iface,
				SetByName:     c.SetBy(),
				SourcePos:     iface.Pos(),
				OutputTagName: GoTestOutputTag,
			},
			TypeName:  typeName,
			IfaceName: iface.Name,
			StubRef:   sdk.NewExternal(iface.Package, typeName),
			IfaceRef:  sdk.NewExternal(iface.Package, iface.Name),
			Methods:   methods,
		}
		if err := ctx.Store.Emit().AppendOriginSlot(
			iface, SlotName, tests, c.Provenance("stubgen.tests."+iface.Name),
		); err != nil {
			return fmt.Errorf("%s: append tests slot: %w", Name, err)
		}
	}
	return nil
}

// methodsOf lifts every method on iface into the rendered form both
// outputs share. Free function rather than a method: the lifting
// depends only on the source signature, not on plugin options.
func methodsOf(iface *node.Interface) []Method {
	out := make([]Method, 0, len(iface.Methods))
	for _, m := range iface.Methods {
		params := paramsOf(m)
		named := namedReturnsUsable(m)
		out = append(out, Method{
			Name:         m.Name,
			CallType:     iface.Name + m.Name + "Call",
			FuncField:    m.Name + "Func",
			CallsField:   m.Name + "Calls",
			Params:       params,
			Returns:      withLocals(returnsOf(m), params, named),
			NamedReturns: named,
		})
	}
	return out
}
