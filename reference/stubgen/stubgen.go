// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package stubgen

import (
	"fmt"

	refconv "go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/sdk"
	sdkgo "go.thesmos.sh/eidos/sdk/golang"
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

// SlotName is the [sdk.EmitFile] slot both emit values append into.
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
	*sdkgo.Base
	*sdk.Holder[Options]
	opts Options
}

// New returns a fresh plugin instance with the options holder bound.
//
// The foundation bucket is where a stub belongs: it is a base type
// other generators may decorate, so it has to exist before the
// composition and cross-cutting buckets run. Requires stays empty —
// the plugin reads source interfaces and waits on no other plugin's
// contribution — which is why the bucket, not a capability, is what
// orders it.
//
// [GoOutputs] carries both files this plugin owns, primary first.
// Neither the templates nor the emit values need a helper of their
// own: the two `*.tmpl` call only the backend's canonical renderers,
// so the plugin registers no template function and overrides no
// builtin.
func New() *Plugin {
	p := &Plugin{Base: sdkgo.NewGenerator(Name, goTemplates, GoOutputs()...).
		Version(Version).
		Priority(sdk.GeneratorFoundation).
		Provides(Capability).
		Directives(directives()...).
		Build()}
	p.Holder = sdk.BindOptions(&p.opts)
	return p
}

// directives declares the `+gen:stub` schema.
//
// The directive takes no positional argument and denies negation: a
// stub exists exactly where one is declared, so deleting the line is
// the suppression and a negated form would have nothing to act on.
func directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			Describe(
				"Generates a recording test double for the annotated interface, " +
					"plus a companion test file proving the double satisfies it. " +
					"Takes no arguments. The negated form is rejected — a stub " +
					"exists only where declared, so removing the directive is the " +
					"suppression.",
			).
			On(sdk.NodeKindInterface).
			DenyNegation().
			Build(),
	}
}

// suffix returns the configured stub-type suffix, or the documented
// default when unset.
func (p *Plugin) suffix() string {
	if p.opts.Suffix != "" {
		return p.opts.Suffix
	}
	return DefaultSuffix
}

// Method is one interface method in the form the templates render.
//
// The signature half — parameters, returns, their identifiers, the
// recorded-call field each maps to, and whether the generated method
// may carry the source's return names — is [refconv.Sig], embedded
// rather than restated. Those are facts about a Go signature, not
// about stubs, and the copy this plugin used to carry had already
// disagreed with the framework's: it numbered unnamed returns across
// every slot, so `(User, error)` recorded `Result0, Result1` where
// every generator on the shared projection records `Result, Err`.
//
// Embedded rather than a named field so the templates keep reaching
// `.Params`, `.Returns` and `.NamedReturns` directly.
type Method struct {
	*refconv.Sig

	// CallType is the identifier of the per-method recorded-call
	// struct — `<Iface><Method>Call`.
	CallType string

	// FuncField is the identifier of the stub's func-valued field —
	// `<Method>Func`.
	FuncField string

	// CallsField is the identifier of the recorded-call slice —
	// `<Method>Calls`.
	CallsField string
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
// [sdk.OutputPackageSetter]:
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
	IfaceRef *sdk.Expr

	// StubRef qualifies the generated stub. Set during Generate
	// against the source package as a provisional value, then
	// corrected by [Tests.SetOutputPackages] once routing resolves.
	// The provisional value is what a run without a Layout phase —
	// a direct generator unit test — observes.
	StubRef *sdk.Expr

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
	c := sdk.NewProvenance(Name)
	for iface := range ctx.Reader.Interfaces().All() {
		if !iface.HasPositiveDirective(DirectiveName) {
			continue
		}
		// Resolved rather than declared: an interface composed
		// purely of embeds declares nothing of its own, and reading
		// Methods alone both rejects it here and — for a partly
		// embedding one — emits a stub short the embedded methods,
		// which does not satisfy the interface it doubles.
		set := ctx.Reader.MethodSet(iface)
		for _, issue := range set.Issues {
			name, _ := sdk.EmbedName(issue.Embed)
			ctx.Diag.Errorf(iface.Pos(),
				"%s: interface %q embeds %q, which %s; the generated stub would be missing its methods",
				Name, iface.QName(), name, issue.Reason)
		}
		if len(set.Methods) == 0 {
			ctx.Diag.Errorf(iface.Pos(),
				"%s: interface %q carries +gen:%s but has no methods; nothing to double",
				Name, iface.QName(), DirectiveName)
			continue
		}

		typeName := iface.Name + p.suffix()
		methods := methodsOf(iface.Name, typeName, set.Methods)

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

// methodsOf lifts every method in the resolved set into the
// rendered form both outputs share. Free function rather than a
// method: the lifting depends only on the source signature, not on
// plugin options.
//
// Takes the resolved set rather than the interface so an embedded
// method is doubled like a declared one.
//
// typeName is the stub struct's identifier, needed because the
// receiver identifier is derived from it and then disambiguated
// against the parameters — see [receiverIdentFor].
func methodsOf(ifaceName, typeName string, methods []*sdk.Method) []Method {
	out := make([]Method, 0, len(methods))
	for _, m := range methods {
		out = append(out, Method{
			Sig:        refconv.SigOf(m, refconv.WithReceiverFromType(typeName)),
			CallType:   ifaceName + m.Name + "Call",
			FuncField:  m.Name + "Func",
			CallsField: m.Name + "Calls",
		})
	}
	return out
}
