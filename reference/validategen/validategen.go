// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package validategen

import (
	"embed"
	"fmt"
	"io/fs"
	"text/template"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/reference/handlergen"
	"go.thesmos.sh/eidos/sdk"
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

const langGo = "golang"

//go:embed templates/golang/*.tmpl
var goTemplates embed.FS

// Validator is the function this plugin emits into its own file.
type Validator struct {
	sdk.BaseEmit

	// FuncName is the generated validator's identifier.
	FuncName string

	// SubjectRef is the type being validated.
	SubjectRef emit.Ref
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
	ValidatorRef *emit.Expr

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

var _ emit.OutputPackageSetter = (*Entry)(nil)

// subjectRef names the struct being validated, qualified when its
// package is known and bare when it is not. emit.External rejects an
// empty path, so the two cases cannot share a construction.
func subjectRef(origin node.Node, name string) emit.Ref {
	if pkg := pkgPathOf(origin); pkg != "" {
		return emit.External(pkg, name)
	}
	return emit.Builtin(name)
}

// Plugin emits a validator per handler and calls it from the handler's
// prebody.
//
// It is the only plugin in the ensemble that both owns a file and
// contributes to another plugin's. Those are independent capabilities:
// Outputs says where a file lands, templates say how a value renders,
// and a plugin needs an Output only for decls it owns. The prebody
// entry needs none — it renders inside handlergen's file.
type Plugin struct{}

// New returns a plugin instance.
func New() *Plugin { return &Plugin{} }

// Name satisfies [sdk.Plugin].
func (*Plugin) Name() string { return Name }

// Version satisfies [sdk.Versioned].
func (*Plugin) Version() string { return Version }

// Priority places the plugin in the composition bucket, after
// handlergen's foundation bucket.
func (*Plugin) Priority() sdk.Priority { return sdk.GeneratorComposition }

// Provides publishes this plugin's label.
func (*Plugin) Provides() []string { return []string{Capability} }

// Requires reports no dependencies within its bucket.
func (*Plugin) Requires() []string { return nil }

// Outputs declares the file holding the generated validators.
func (*Plugin) Outputs(lang string) []sdk.Output {
	if lang == langGo {
		return []sdk.Output{{Suffix: GoSuffix}}
	}
	return nil
}

// Templates ships both templates.
func (*Plugin) Templates(lang string) (fs.FS, bool) {
	if lang != langGo {
		return nil, false
	}
	sub, err := fs.Sub(goTemplates, "templates/golang")
	if err != nil {
		return nil, false
	}
	return sub, true
}

// TemplateFuncs contributes nothing; the shared Go helpers are already
// in the backend's overrideable funcmap.
func (*Plugin) TemplateFuncs(string) template.FuncMap { return nil }

// TemplateOverrides replaces nothing.
func (*Plugin) TemplateOverrides(string) template.FuncMap { return nil }

// Generate emits one validator per handler and a prebody call to it.
func (*Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name, sdk.EmitTarget{})
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
			SubjectRef: emit.Ptr(subjectRef(origin, host.Source)),
		}
		if err := ctx.Store.Emit().AppendOriginSlot(
			origin, "top", v, c.Provenance(Name+".validator."+host.Source),
		); err != nil {
			return fmt.Errorf("%s: append validator: %w", Name, err)
		}

		entry := &Entry{
			BaseEmit: sdk.BaseEmit{SetByName: c.SetBy(), SourcePos: host.Pos()},
			// Provisional, and deliberately a bare identifier: the
			// validator lands beside its source by default, where a
			// qualified reference would be wrong. SetOutputPackages
			// upgrades it to a qualified one if Layout routes the
			// validator into a different package.
			ValidatorRef: emit.NewIdent(fn),
			FuncName:     fn,
			Handler:      host.Source,
		}
		if err := host.Slot(handlergen.PrebodySlot).Append(entry, c.Provenance(EntryID)); err != nil {
			return fmt.Errorf("%s: append prebody call: %w", Name, err)
		}
	}
	return nil
}

// pkgPathOf returns the import path of the package owning n, or "" when
// it cannot be determined — in which case the backend's same-package
// elision leaves the reference unqualified, which is correct for a decl
// landing beside its source.
func pkgPathOf(n node.Node) string {
	type packaged interface{ PkgPath() string }
	if p, ok := n.(packaged); ok {
		return p.PkgPath()
	}
	return ""
}
