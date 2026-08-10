// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package validategen

import (
	"embed"
	"fmt"

	refconv "go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/reference/handlergen"
	"go.thesmos.sh/eidos/sdk"
	sdkgo "go.thesmos.sh/eidos/sdk/golang"
)

// Name is the plugin's stable identifier.
const Name = "validategen"

// Capability is this plugin's published label.
const Capability = "http.validate"

// Version is the plugin's declared version. It composes into the
// pipeline fingerprint frontends fold into their cache keys.
const Version = "1.0.0"

// Kind and EntryKind are the two emit kinds this plugin declares: the
// validator function it owns, and the call it contributes into
// handlergen's prebody. Each must equal a `define` name under
// templates/golang.
const (
	Kind      sdk.Kind = "validategen.validator"
	EntryKind sdk.Kind = "validategen.entry"
)

// EntryID is the [sdk.Provenance] ID stamped on the prebody call.
const EntryID = "validategen.call"

// GoSuffix is the per-source trailer Layout appends for the file this
// plugin owns.
const GoSuffix = "_validate.go"

//go:embed templates/golang/*.tmpl
var goTemplates embed.FS

// Validator is the function this plugin emits into its own file.
type Validator struct {
	sdk.BaseEmit

	// FuncName is the generated validator's identifier.
	FuncName string

	// SubjectRef is the type being validated.
	SubjectRef sdk.Ref
}

// Kind binds this value to its template.
func (*Validator) Kind() sdk.Kind { return Kind }

// Entry is the call this plugin contributes into handlergen's prebody.
//
// It carries a ref to the validator rather than a rendered name,
// because the validator's package is not known during Generate: Layout
// decides it, and a `+gen:out pkg=` directive can move it. See
// [Entry.SetOutputPackages].
type Entry struct {
	sdk.BaseEmit

	// ValidatorRef names the generated validator.
	ValidatorRef *sdk.Expr

	// FuncName is retained so SetOutputPackages can rebuild the ref
	// once Layout has decided where the validator landed.
	FuncName string

	// Handler names the host type, for the rendered comment.
	Handler string
}

// Kind binds this value to its template.
func (*Entry) Kind() sdk.Kind { return EntryKind }

// SetOutputPackages repoints the entry at the validator's real package
// once Layout has resolved it.
//
// This is the only plugin in the ensemble that needs the hook, because
// it is the only one referencing its own other output. Layout calls it
// at most once, after every Target resolves, and may pass a partial
// map — a run that recorded routing errors reaches dispatch with tags
// missing, and byTag[""] can be present but empty. An empty path means
// "not derivable", not "same package", so the provisional ref set
// during Generate is deliberately left in place: a wrong package is a
// compile error naming the symbol, while a bare name silently binds to
// whatever else is in scope.
func (e *Entry) SetOutputPackages(byTag map[string]string) {
	if path := byTag[""]; path != "" {
		e.ValidatorRef = sdk.NewExternal(path, e.FuncName)
	}
}

var _ sdk.OutputPackageSetter = (*Entry)(nil)

// Plugin emits a validator per handler and calls it from the handler's
// prebody.
//
// It is the only plugin in the ensemble that both owns a file and
// contributes to another plugin's. Those are independent capabilities:
// Outputs says where a file lands, templates say how a value renders,
// and a plugin needs an Output only for decls it owns. The prebody
// entry needs none — it renders inside handlergen's file.
type Plugin struct{ *sdkgo.Base }

// New returns a plugin instance.
//
// The single [sdk.Output] is the file holding the generated
// validators; the embedded tree ships both templates, one per declared
// emit kind. A generator needs both — an output without a template
// tree renders nothing, a tree without an output gives Layout no
// filename to compose — which is why [sdkgo.NewGenerator] takes them
// together.
//
// The composition bucket places it one after handlergen's foundation
// bucket, so the handler exists to contribute to, and before the
// cross-cutting and finalize contributors, so validation precedes them
// in the rendered prebody. Nothing is required: the dependency is on a
// plugin in another bucket, and Requires resolves only within one.
func New() *Plugin {
	return &Plugin{Base: sdkgo.NewGenerator(Name, goTemplates, sdk.Output{Suffix: GoSuffix}).
		Version(Version).
		Priority(sdk.GeneratorComposition).
		Provides(Capability).
		Build()}
}

// Generate emits one validator per handler and a prebody call to it.
func (*Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name)
	for _, pending := range ctx.Store.Emit().PendingOriginSlots() {
		host, ok := pending.Item.(*handlergen.Handler)
		if !ok {
			continue
		}
		fn := "Validate" + host.Source
		origin := host.Origin()

		v := &Validator{
			BaseEmit:   sdk.BaseEmit{OriginNode: origin, SetByName: c.SetBy(), SourcePos: host.Pos()},
			FuncName:   fn,
			SubjectRef: sdk.Ptr(refconv.SubjectRef(origin, host.Source)),
		}
		if err := ctx.Store.Emit().AppendOriginSlot(
			origin, "top", v, c.Provenance(Name+".validator."+host.Source),
		); err != nil {
			return fmt.Errorf("%s: append validator: %w", Name, err)
		}

		entry := &Entry{
			// The origin is carried even though the entry is appended to
			// the host's slot rather than routed by origin: Layout keys
			// its output-package dispatch on (origin, plugin), and
			// recordOutputPath drops a nil origin outright, so an entry
			// without one could never be handed the paths its own
			// validator resolved to. Necessary but not yet sufficient —
			// dispatch reaches values through sdk.EmitWalk, which descends
			// only into the built-in emit kinds, so an entry sitting in
			// a plugin-defined host's slot is still unreachable. See the
			// skipped subtest in validategen_test.go.
			BaseEmit: sdk.BaseEmit{OriginNode: origin, SetByName: c.SetBy(), SourcePos: host.Pos()},
			// Provisional, and deliberately a bare identifier: the
			// validator lands beside its source by default, where a
			// qualified reference would be wrong. SetOutputPackages
			// upgrades it to a qualified one if Layout routes the
			// validator into a different package.
			ValidatorRef: sdk.NewIdent(fn),
			FuncName:     fn,
			Handler:      host.Source,
		}
		if err := host.Slot(handlergen.PrebodySlot).Append(entry, c.Provenance(EntryID)); err != nil {
			return fmt.Errorf("%s: append prebody call: %w", Name, err)
		}
	}
	return nil
}
