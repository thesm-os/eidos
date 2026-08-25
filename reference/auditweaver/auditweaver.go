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
	"go.thesmos.sh/eidos/reference/debugweaver"
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

// Plugin is the cross-cutting audit-weaver. The zero value is
// unusable; go through [New] so the embedded holder binds to the
// options field.
type Plugin struct {
	*sdk.Base
	*sdk.Holder[Options]
	opts Options
}

// New returns a fresh plugin instance with the options holder
// bound.
//
// It declares neither an [sdk.Output] nor a template tree. The first
// because a contributor renders inside a file it does not own; the
// second because its contribution is a plain call expression, which
// the backend already knows how to render. Its sibling [debugweaver]
// declares a kind and ships a template for the same shape — the two
// exist as a pair so the choice between the routes is legible, and
// this is the one to copy unless the contribution has a spelling the
// backend does not already have.
//
// The cross-cutting bucket runs it after the foundation and
// composition generators that produce the methods it weaves into.
// Requires names [RequiresTrace] so plan resolution orders the trace
// contributor first, which is what puts the debug trace above the
// audit record in the rendered prebody.
func New() *Plugin {
	p := &Plugin{Base: sdk.NewPlugin(Name).
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
		// Built from the emit constructors rather than through a
		// plugin-defined kind and a template of its own.
		//
		// This is the shorter of the two routes a contributor has, and
		// the one most of them want: the call shape is `pkg.Fn(a, b)`,
		// which the backend already renders — including registering the
		// import the external reference needs. Its sibling
		// [debugweaver] takes the other route and owns a kind, which
		// earns its keep only when the contribution has a spelling the
		// backend does not already know.
		//
		// AppendPrebody can only fail when host is nil or carries an
		// unsupported kind — neither possible for the *sdk.EmitMethod
		// values EmitMethods yields, and an expression statement is
		// sdk.EmitKindStmt by construction. Infallible at this site.
		call := sdk.NewCall(
			sdk.NewExternal(p.opts.Package, p.opts.Func),
			sdk.NewLiteralString(p.opts.Format),
			sdk.NewLiteralString(m.OwnerName()+"."+m.Name),
		)
		_ = c.AppendPrebody(m, sdk.NewExprStmt(call), EntryID)
	}
	return nil
}
