// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package debugweaver appends a debug-trace call to the
// [sdk.EmitMethod.Prebody] slot of every method in the emit store —
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
// file's import set via [sdk.NewExternal] — the same flow
// [sdk.External] type references use — so the rendered output is
// structurally correct without any plugin-side import-management
// scaffolding.
package debugweaver

import (
	"embed"

	"go.thesmos.sh/eidos/sdk"
	sdkgo "go.thesmos.sh/eidos/sdk/golang"
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

//go:embed templates/golang/*.tmpl
var goTemplates embed.FS

// Capability is the capability label this plugin advertises so
// downstream cross-cutting contributors (audit, metric, …) can
// declare ordering through `Requires`.
const Capability = "trace"

// DirectiveName is the bare directive name the plugin reads from
// emit methods to suppress its contribution on a per-method basis.
const DirectiveName sdk.DirectiveName = "debug"

// EntryID is the [sdk.EmitProvenance.ID] stamped on every debug-weaver
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
// plugin-defined emit kind rather than a hand-assembled [sdk.Stmt].
//
// The prebody slot it lands in is constrained to [sdk.EmitKindStmt], so
// the contribution is wrapped by [sdk.NewRenderStmt]: the wrapper
// satisfies the slot, and the backend renders it by dispatching to the
// template registered under this Kind. That is what lets the plugin own
// its own spelling — the alternative is encoding the call shape in Go
// against the [sdk.Stmt] union, which no other reference contributor
// does.
//
// The three fields are [sdk.Expr] rather than plain strings so the
// backend performs the literal escaping and the import registration.
// Holding raw text here and interpolating it in the template would
// re-implement both, badly.
type Trace struct {
	sdk.BaseEmit

	// FuncRef is the trace function the entry calls, as an external
	// reference. Rendering it registers the import on the host file.
	FuncRef *sdk.Expr

	// Format is the printf-style first argument.
	Format *sdk.Expr

	// Subject is the fully-qualified "<Type>.<Method>" the trace
	// names.
	Subject *sdk.Expr
}

// Kind binds this value to its template.
func (*Trace) Kind() sdk.Kind { return Kind }

// Plugin is the cross-cutting debug-weaver.
type Plugin struct {
	*sdkgo.Base
	*sdk.Holder[Options]
	opts Options
}

// New returns a fresh plugin instance with the options holder
// bound.
//
// It ships a template but declares no [sdk.Output], and so needs no
// [sdk.FilenameProvider]: templates say how a value renders, outputs
// say where a file lands, and this plugin renders inside methods it
// does not own.
//
// The cross-cutting bucket runs it after the foundation and
// composition generators that produce those methods. [Capability] is
// published so downstream cross-cutting contributors (audit, metric,
// …) can order against a known trace entry; nothing upstream is
// required in turn.
func New() *Plugin {
	p := &Plugin{Base: sdkgo.NewPlugin(Name).
		Templates(goTemplates).
		Version(Version).
		Priority(sdk.GeneratorCrossCutting).
		Provides(Capability).
		Directives(directives()...).
		Build()}
	p.Holder = sdk.BindOptions(&p.opts)
	return p
}

// directives declares the `-gen:debug` schema. The positive form is
// allowed by the framework default but carries no plugin semantics —
// debug-weaver applies to every method unconditionally unless
// suppressed.
func directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			On(sdk.NodeKindMethod).
			Describe("Suppresses (-) the debug-entry trace on the host method.").
			Build(),
	}
}

// Generate walks every emit-store method and appends a trace call
// to its Prebody slot, except for methods that carry `-gen:debug`.
// The call resolves to `<Options.Package>.<Options.Func>(<format>,
// "<Type>.<Method>")` — the renderer registers the import for
// Options.Package on the host file's import set via the
// [sdk.NewExternal] expression.
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name)
	for m := range ctx.Reader.EmitMethods().All() {
		if m.HasNegatedDirective(DirectiveName) {
			continue
		}
		trace := &Trace{
			BaseEmit: sdk.BaseEmit{SetByName: c.SetBy(), SourcePos: m.Pos()},
			FuncRef:  sdk.NewExternal(p.opts.Package, p.opts.Func),
			Format:   sdk.NewLiteralString(p.opts.Format),
			Subject:  sdk.NewLiteralString(m.OwnerName() + "." + m.Name),
		}
		// AppendPrebody can only fail when host is nil or carries
		// an unsupported kind — neither possible for the *sdk.EmitMethod
		// values EmitMethods yields, and the render wrapper reports
		// sdk.EmitKindStmt by construction. The Append is therefore
		// infallible at this call site.
		_ = c.AppendPrebody(m, sdk.NewRenderStmt(trace), EntryID)
	}
	return nil
}
