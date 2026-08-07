// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package auditweaver appends an audit-record call to the
// [emit.Method.Prebody] slot of every method in the emit store —
// a cross-cutting concern paired with the debug-weaver entry
// trace. The plugin runs in [sdk.GeneratorCrossCutting] and
// declares `Requires: ["trace"]` so plan resolution orders it after
// debug-weaver; the rendered prebody therefore lists the debug
// trace first and the audit record second.
//
// `-gen:audit` on an emit method skips the contribution.
//
// # Configurability
//
// [Options.Package] selects the import path of the audit package
// the rendered call references, and [Options.Func] selects the
// function on that package. The renderer registers the import on
// the host file's import set via [emit.NewExternal] — the same
// flow [emit.External] type references use — so the rendered output
// is structurally correct without any plugin-side import-management
// scaffolding.
//
// The defaults target stdlib `log.Printf` so projects without a
// dedicated audit package generate compilable output out of the
// box; production deployments override Package + Func to point at
// their real audit surface.
package auditweaver

import (
	"embed"
	"io/fs"
	"text/template"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier.
const Name = "audit-weaver"

// Version is the plugin's declared version. It composes into the
// pipeline's plugin fingerprint, which frontends fold into their cache
// keys — so bumping it invalidates a warm cache populated when this
// plugin behaved differently. A plugin that declares no version
// contributes an empty string and can never invalidate anything, which
// is a silent staleness bug waiting for its first behavioural change.
const Version = "1.0.0"

// Kind is the emit kind this plugin declares. It must equal the
// `define` name in templates/golang/auditweaver.record.tmpl.
const Kind sdk.Kind = "auditweaver.record"

const langGo = "golang"

//go:embed templates/golang/*.tmpl
var goTemplates embed.FS

// Capability is the capability label this plugin advertises.
const Capability = "audit"

// RequiresTrace names the upstream capability this plugin depends
// on so the plan orders the trace contributor (typically
// debug-weaver) first.
const RequiresTrace = "trace"

// DirectiveName is the bare directive name read from emit methods
// to suppress the audit contribution on a per-method basis.
const DirectiveName sdk.DirectiveName = "audit"

// EntryID is the [emit.Provenance.ID] stamped on every audit-weaver
// prebody contribution. Other cross-cutting plugins may position
// themselves relative to the audit record through
// `builder.Before` / `builder.After`.
const EntryID = "audit.record"

// DefaultPackage is the import path the rendered call resolves to
// when [Options.Package] is unset. Stdlib `log` is the default so
// out-of-the-box generation lands compilable code; the message
// rendered through `log.Printf` is the same fully-qualified
// "<Type>.<Method>" string a dedicated audit package would receive.
const DefaultPackage = "log"

// DefaultFunc is the function name the rendered call resolves to
// when [Options.Func] is unset. Stdlib `log.Printf` accepts the
// `"audit: %s"` format with the method-name argument.
const DefaultFunc = "Printf"

// DefaultFormat is the format string passed as the first call
// argument when [Options.Format] is unset — the printf-style
// template the configured Func receives. `%s` interpolates the
// fully-qualified method name. Set explicitly when targeting an
// audit Func with a different signature.
const DefaultFormat = "audit: %s"

// Options carries the plugin's user-tunable settings.
type Options struct {
	// Package is the import path of the audit package the rendered
	// call references. The renderer registers the import on the
	// host file's import set automatically. Defaults to
	// [DefaultPackage].
	Package string `eidos:"package,default=log"`

	// Func is the function name called on the audit package.
	// Defaults to [DefaultFunc].
	Func string `eidos:"func,default=Printf"`

	// Format is the printf-style first argument to the audit call.
	// `%s` interpolates the fully-qualified method name
	// (`<Type>.<Method>`). Defaults to [DefaultFormat].
	Format string `eidos:"format,default=audit: %s"`
}

// Record is the audit call this plugin contributes, as a
// plugin-defined emit kind rather than a hand-assembled [emit.Stmt].
//
// The prebody slot it lands in is constrained to [emit.KindStmt], so
// the contribution is wrapped by [emit.NewRenderStmt]: the wrapper
// satisfies the slot, and the backend renders it by dispatching to the
// template registered under this Kind. That is what lets the plugin own
// its own spelling — the alternative is encoding the call shape in Go
// against the [emit.Stmt] union, which no other reference contributor
// does.
//
// The three fields are [emit.Expr] rather than plain strings so the
// backend performs the literal escaping and the import registration.
// Holding raw text here and interpolating it in the template would
// re-implement both, badly.
type Record struct {
	sdk.BaseEmit

	// FuncRef is the audit function the record calls, as an external
	// reference. Rendering it registers the import on the host file.
	FuncRef *emit.Expr

	// Format is the printf-style first argument.
	Format *emit.Expr

	// Subject is the fully-qualified "<Type>.<Method>" the record
	// names.
	Subject *emit.Expr
}

// Kind binds this value to its template.
func (*Record) Kind() sdk.Kind { return Kind }

// Plugin is the cross-cutting audit-weaver.
type Plugin struct {
	*sdk.Holder[Options]
	opts Options
}

// New returns a fresh plugin instance with the options holder
// bound.
func New() *Plugin {
	p := &Plugin{}
	p.Holder = sdk.BindOptions(&p.opts)
	return p
}

// Name returns [Name].
func (*Plugin) Name() string { return Name }

// Version satisfies [sdk.Versioned].
func (*Plugin) Version() string { return Version }

// Priority places the plugin in the cross-cutting bucket.
func (*Plugin) Priority() sdk.Priority { return sdk.GeneratorCrossCutting }

// Provides advertises the audit capability.
func (*Plugin) Provides() []string { return []string{Capability} }

// Requires declares the trace capability dependency so the plan
// orders the trace contributor first.
func (*Plugin) Requires() []string { return []string{RequiresTrace} }

// Directives declares the `-gen:audit` schema. The positive form
// is allowed by the framework default but carries no plugin
// semantics — audit-weaver applies to every method unconditionally
// unless suppressed.
func (*Plugin) Directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			On(node.KindMethod).
			Describe("Suppresses (-) the audit record on the host method.").
			Build(),
	}
}

// Generate walks every emit-store method and appends an audit call
// to its Prebody slot, skipping methods that carry `-gen:audit`.
// The call resolves to `<Options.Package>.<Options.Func>(<format>,
// "<Type>.<Method>")` — the renderer registers the import for
// Options.Package on the host file's import set via the
// [emit.NewExternal] expression.
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name)
	for m := range ctx.Reader.EmitMethods().All() {
		if m.HasNegatedDirective(DirectiveName) {
			continue
		}
		record := &Record{
			BaseEmit: sdk.BaseEmit{SetByName: c.SetBy(), SourcePos: m.Pos()},
			FuncRef:  sdk.NewExternal(p.pkg(), p.funcName()),
			Format:   emit.NewLiteralString(p.format()),
			Subject:  emit.NewLiteralString(ownerName(m) + "." + m.Name),
		}
		// AppendPrebody can only fail when host is nil or carries
		// an unsupported kind — neither possible for the *emit.Method
		// values EmitMethods yields, and the render wrapper reports
		// emit.KindStmt by construction. The Append is therefore
		// infallible at this call site.
		_ = c.AppendPrebody(m, emit.NewRenderStmt(record), EntryID)
	}
	return nil
}

// Templates ships the record template.
//
// A contributor shipping a template needs no [sdk.FilenameProvider]:
// templates say how a value renders, outputs say where a file lands,
// and this plugin renders inside a file it does not own.
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

// TemplateFuncs contributes nothing.
//
// The shared Go helpers are already merged into the backend's
// overrideable funcmap, so returning them here re-registers names that
// exist — a Build-time collision that would stop this plugin and any
// other plugin doing the same from appearing in one pipeline.
func (*Plugin) TemplateFuncs(string) template.FuncMap { return nil }

// TemplateOverrides replaces nothing.
func (*Plugin) TemplateOverrides(string) template.FuncMap { return nil }

// pkg / funcName / format return the configured option value or
// the documented default when the option is empty. Centralised so
// the rendered behaviour is consistent across every Generate path
// even if a caller bypasses SetOptions.
func (p *Plugin) pkg() string {
	if p.opts.Package != "" {
		return p.opts.Package
	}
	return DefaultPackage
}

func (p *Plugin) funcName() string {
	if p.opts.Func != "" {
		return p.opts.Func
	}
	return DefaultFunc
}

func (p *Plugin) format() string {
	if p.opts.Format != "" {
		return p.opts.Format
	}
	return DefaultFormat
}

// ownerName returns the simple receiver-type name of m's owner so
// the rendered audit record reads `<Type>.<Method>`. Owner kinds
// (Struct, Interface, source-side enum / alias for free-standing
// methods) are uniformly handled by [contract.Owner.OwnerName] —
// the helper delegates rather than type-switching.
func ownerName(m *emit.Method) string {
	return m.OwnerName()
}
