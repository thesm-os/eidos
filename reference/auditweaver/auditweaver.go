// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package auditweaver appends an audit-record call to the
// [sdk.EmitMethod.Prebody] slot of every method in the emit store —
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
// the host file's import set via [sdk.NewExternal] — the same
// flow [sdk.External] type references use — so the rendered output
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

	"go.thesmos.sh/eidos/reference/debugweaver"
	"go.thesmos.sh/eidos/sdk"
	sdkgo "go.thesmos.sh/eidos/sdk/golang"
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

//go:embed templates/golang/*.tmpl
var goTemplates embed.FS

// Capability is the capability label this plugin advertises.
const Capability = "audit"

// RequiresTrace names the upstream capability this plugin depends
// on so the plan orders the trace contributor (typically
// debug-weaver) first. It aliases the providing plugin's published
// const rather than repeating the label, so the two cannot drift
// into an ordering that silently stops holding.
const RequiresTrace = debugweaver.Capability

// DirectiveName is the bare directive name read from emit methods
// to suppress the audit contribution on a per-method basis.
const DirectiveName sdk.DirectiveName = "audit"

// EntryID is the [sdk.EmitProvenance.ID] stamped on every audit-weaver
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
type Record struct {
	sdk.BaseEmit

	// FuncRef is the audit function the record calls, as an external
	// reference. Rendering it registers the import on the host file.
	FuncRef *sdk.Expr

	// Format is the printf-style first argument.
	Format *sdk.Expr

	// Subject is the fully-qualified "<Type>.<Method>" the record
	// names.
	Subject *sdk.Expr
}

// Kind binds this value to its template.
func (*Record) Kind() sdk.Kind { return Kind }

// Plugin is the cross-cutting audit-weaver. The zero value is
// unusable; go through [New] so the embedded holder binds to the
// options field.
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
// say where a file lands, and this plugin renders inside a file it
// does not own.
//
// The cross-cutting bucket runs it after the foundation and
// composition generators that produce the methods it weaves into.
// Requires names [RequiresTrace] so plan resolution orders the trace
// contributor first, which is what puts the debug trace above the
// audit record in the rendered prebody.
//
// The shared Go helpers the builder registers are namespaced under
// [sdkgo.FuncPrefix] of this plugin's name, so two plugins carrying
// the same bundle never collide. The hyphen in `audit-weaver` is
// folded to an underscore there — text/template accepts only
// identifier-shaped function names, and the plugin name is a
// user-visible identity that provenance and directive scoping key
// on, so it is the prefix that bends, not the name.
func New() *Plugin {
	p := &Plugin{Base: sdkgo.NewPlugin(Name).
		Templates(goTemplates).
		Version(Version).
		Priority(sdk.GeneratorCrossCutting).
		Provides(Capability).
		Requires(RequiresTrace).
		Directives(directives()...).
		Build()}
	p.Holder = sdk.BindOptions(&p.opts)
	return p
}

// directives declares the `-gen:audit` schema. The positive form
// is allowed by the framework default but carries no plugin
// semantics — audit-weaver applies to every method unconditionally
// unless suppressed.
func directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			On(sdk.NodeKindMethod).
			Describe("Suppresses (-) the audit record on the host method.").
			Build(),
	}
}

// Generate walks every emit-store method and appends an audit call
// to its Prebody slot, skipping methods that carry `-gen:audit`.
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
		record := &Record{
			BaseEmit: sdk.BaseEmit{SetByName: c.SetBy(), SourcePos: m.Pos()},
			FuncRef:  sdk.NewExternal(p.pkg(), p.funcName()),
			Format:   sdk.NewLiteralString(p.format()),
			Subject:  sdk.NewLiteralString(ownerName(m) + "." + m.Name),
		}
		// AppendPrebody can only fail when host is nil or carries
		// an unsupported kind — neither possible for the *sdk.EmitMethod
		// values EmitMethods yields, and the render wrapper reports
		// sdk.EmitKindStmt by construction. The Append is therefore
		// infallible at this call site.
		_ = c.AppendPrebody(m, sdk.NewRenderStmt(record), EntryID)
	}
	return nil
}

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
func ownerName(m *sdk.EmitMethod) string {
	return m.OwnerName()
}
