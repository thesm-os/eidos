// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package debugweaver appends a debug-trace call to the
// [emit.Method.Prebody] slot of every method in the emit store —
// the canonical "entry trace" cross-cutting concern. The plugin
// runs in [sdk.GeneratorCrossCutting] and advertises the
// `trace` capability so other cross-cutting plugins (audit,
// metric, …) can declare a Requires dependency on a known
// trace-entry contribution.
//
// `-gen:debug` on an emit method skips that method; methods
// without the directive get the contribution. Each appended
// statement carries [Provenance.ID] `trace.entry` so later
// cross-cutting plugins can position themselves relative to the
// debug entry trace through `builder.Before` / `builder.After`.
//
// # Configurability
//
// [Options.Package] selects the import path of the package the
// rendered call references; [Options.Func] selects the function on
// that package. The renderer registers the import on the host
// file's import set via [emit.NewExternal] — the same flow
// [emit.External] type references use — so the rendered output is
// structurally correct without any plugin-side import-management
// scaffolding.
package debugweaver

import (
	"embed"
	"io/fs"
	"text/template"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier.
const Name = "debug-weaver"

// Version is the plugin's declared version. It composes into the
// pipeline's plugin fingerprint, which frontends fold into their cache
// keys — so bumping it invalidates a warm cache populated when this
// plugin behaved differently. A plugin that declares no version
// contributes an empty string and can never invalidate anything, which
// is a silent staleness bug waiting for its first behavioural change.
const Version = "1.0.0"

// Kind is the emit kind this plugin declares. It must equal the
// `define` name in templates/golang/debugweaver.trace.tmpl.
const Kind sdk.Kind = "debugweaver.trace"

const langGo = "golang"

//go:embed templates/golang/*.tmpl
var goTemplates embed.FS

// Capability is the capability label this plugin advertises so
// downstream cross-cutting contributors (audit, metric, …) can
// declare ordering through `Requires`.
const Capability = "trace"

// DirectiveName is the bare directive name the plugin reads from
// emit methods to suppress its contribution on a per-method basis.
const DirectiveName sdk.DirectiveName = "debug"

// EntryID is the [emit.Provenance.ID] stamped on every debug-weaver
// prebody contribution. Cross-cutting plugins that want to position
// their own statement relative to the entry trace pass this id to
// [builder.Before] / [builder.After].
const EntryID = "trace.entry"

// DefaultPackage is the import path the rendered call resolves to
// when [Options.Package] is unset. Stdlib `log` keeps the demo
// self-contained — projects override Package + Func to point at
// their real trace surface.
const DefaultPackage = "log"

// DefaultFunc is the function name the rendered call resolves to
// when [Options.Func] is unset.
const DefaultFunc = "Printf"

// DefaultFormat is the printf-style first argument to the trace
// call when [Options.Format] is unset. `%s` interpolates the
// fully-qualified method name (`<Type>.<Method>`).
const DefaultFormat = "debug: %s entered"

// Options carries the plugin's user-tunable settings.
type Options struct {
	// Package is the import path of the package the rendered trace
	// call references. Defaults to [DefaultPackage].
	Package string `eidos:"package,default=log"`

	// Func is the function name called on the trace package.
	// Defaults to [DefaultFunc].
	Func string `eidos:"func,default=Printf"`

	// Format is the printf-style first argument to the trace call.
	// Defaults to [DefaultFormat].
	Format string `eidos:"format,default=debug: %s entered"`
}

// Trace is the trace call this plugin contributes, as a
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
type Trace struct {
	sdk.BaseEmit

	// FuncRef is the trace function the entry calls, as an external
	// reference. Rendering it registers the import on the host file.
	FuncRef *emit.Expr

	// Format is the printf-style first argument.
	Format *emit.Expr

	// Subject is the fully-qualified "<Type>.<Method>" the trace
	// names.
	Subject *emit.Expr
}

// Kind binds this value to its template.
func (*Trace) Kind() sdk.Kind { return Kind }

// Plugin is the cross-cutting debug-weaver.
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

// Priority places the plugin in the cross-cutting bucket so it
// runs after foundation and composition generators.
func (*Plugin) Priority() sdk.Priority { return sdk.GeneratorCrossCutting }

// Provides advertises the trace capability.
func (*Plugin) Provides() []string { return []string{Capability} }

// Requires returns nil — debug-weaver has no upstream dependency.
func (*Plugin) Requires() []string { return nil }

// Directives declares the `-gen:debug` schema. The positive form
// is allowed by the framework default but carries no plugin
// semantics — debug-weaver applies to every method unconditionally
// unless suppressed.
func (*Plugin) Directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			On(node.KindMethod).
			Describe("Suppresses (-) the debug-entry trace on the host method.").
			Build(),
	}
}

// Generate walks every emit-store method and appends a trace call
// to its Prebody slot, except for methods that carry `-gen:debug`.
// The call resolves to `<Options.Package>.<Options.Func>(<format>,
// "<Type>.<Method>")` — the renderer registers the import for
// Options.Package on the host file's import set via the
// [emit.NewExternal] expression.
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name, sdk.EmitTarget{})
	for m := range ctx.Reader.EmitMethods().All() {
		if m.HasNegatedDirective(DirectiveName) {
			continue
		}
		trace := &Trace{
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
		_ = c.AppendPrebody(m, emit.NewRenderStmt(trace), EntryID)
	}
	return nil
}

// Templates ships the trace template.
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
// the documented default when the option is empty.
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
// the rendered log message reads `<Type>.<Method>` — the common
// "trace this method on this struct" mental model. Methods on
// interfaces, methods on structs, and methods on source-side
// types are all handled uniformly via [contract.Owner.OwnerName],
// so the function never type-switches the underlying Owner kind.
func ownerName(m *emit.Method) string {
	return m.OwnerName()
}
