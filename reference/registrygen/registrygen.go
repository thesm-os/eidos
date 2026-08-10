// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package registrygen showcases the plugin-defined-emit-kind path:
// it defines its own [Registration] emit kind outside the
// `emit.*` namespace, ships the matching `registration.tmpl`
// template through [plugin.TemplateProvider], and appends one
// origin-anchored Registration contribution for each source
// struct annotated with `+gen:register`. The Layout phase
// resolves each contribution's origin to a rendered file via
// the standard routing precedence (framework / project /
// per-plugin / CLI); every Registration whose resolved file
// shares a target lands in the same `func init() { ... }`
// block.
//
// # Output layout
//
// registrygen retains zero filename control of its own. Output
// routing is supplied by the framework's routing layer:
//
//   - Alongside-source (the framework default): one
//     `<src-basename>_registry.go` per source struct, dropped
//     next to its source file.
//   - Centralised (configured via project-level / per-plugin
//     output config or CLI overrides): each registration lands
//     in the configured Dir with the same `<basename>_registry.go`
//     filename per source struct.
//
// Aggregating multiple registrations into one rendered file is
// achieved by pinning a shared filename through `+gen:out` on
// each contributing source struct or via the CLI `-o` override
// — the routing-layer mechanism is uniform across plugins, not
// a plugin-specific switch.
package registrygen

import (
	"embed"

	"go.thesmos.sh/eidos/sdk"
	sdkgo "go.thesmos.sh/eidos/sdk/golang"
)

// Name is the plugin's stable identifier.
const Name = "registry-gen"

// Version is the plugin's declared version. It composes into the
// pipeline's plugin fingerprint, which frontends fold into their cache
// keys — so bumping it invalidates a warm cache populated when this
// plugin behaved differently. A plugin that declares no version
// contributes an empty string and can never invalidate anything, which
// is a silent staleness bug waiting for its first behavioural change.
const Version = "1.0.0"

// Capability is the capability label this plugin advertises.
const Capability = "registry"

// DirectiveName is the bare directive name read from source
// structs.
const DirectiveName sdk.DirectiveName = "register"

// Kind is the plugin-defined emit kind every [Registration] reports
// from [Registration.Kind]. The dotted spelling keeps it outside
// the core `emit.*` namespace.
const Kind sdk.Kind = "registrygen.registration"

// FilenameSuffix is the per-source filename suffix the routing
// layer appends to each contributing source struct's basename.
// `<src-basename>_registry.go` keeps the registration blocks
// recognisable wherever the routing layer places them.
const FilenameSuffix = "_registry.go"

// SlotName is the per-file slot the Layout phase materialises
// Registration contributions into. Matches [sdk.EmitFile]'s
// canonical `init` slot name so the rendered output collects
// every registration inside the file's `func init() { ... }`
// block.
const SlotName = "init"

// DefaultRegisterPackage is the import path the rendered
// register-call resolves to when [Options.RegisterPackage] is
// unset. Stdlib `log` keeps the demo self-contained so a fresh
// project produces compilable output without external setup;
// production deployments override RegisterPackage + RegisterFunc
// to point at the real registry surface (typically
// `example.com/registry` exposing `Register(name string, value
// any)` or similar).
const DefaultRegisterPackage = "log"

// DefaultRegisterFunc is the function name called on the registry
// package when [Options.RegisterFunc] is unset. `log.Print`
// accepts a variadic `...any` argument list so the
// `Func(name, value)` shape stays valid both for stdlib log and
// for a real `Register(name, value)` function — no shape
// translation needed when callers switch packages.
const DefaultRegisterFunc = "Print"

//go:embed templates/golang/*.tmpl
var goTemplates embed.FS

// Options carries the plugin's user-tunable settings. Routing
// is owned by the framework, so registrygen exposes no
// output-package / filename options — those land via project-
// level `output.*` config or CLI overrides applied through the
// routing layer.
type Options struct {
	// RegisterPackage is the import path of the registry package
	// the rendered call references. Defaults to
	// [DefaultRegisterPackage]. The renderer registers the import
	// on the host file's import set via the [sdk.NewExternal]
	// expression — no plugin-side import scaffolding needed.
	RegisterPackage string `eidos:"register_package,default=log"`

	// RegisterFunc is the function name called on the registry
	// package. The rendered call passes two positional arguments —
	// the struct's name (string) and a composite literal of the
	// struct — so the configured Func must accept that shape (or
	// a variadic `...any` like stdlib `log.Print`). Defaults to
	// [DefaultRegisterFunc].
	RegisterFunc string `eidos:"register_func,default=Print"`
}

// Plugin is the registry-gen generator. Go through [New] so the
// embedded [sdk.Holder] binds to the plugin's options field.
type Plugin struct {
	*sdkgo.Base
	*sdk.Holder[Options]
	opts Options
}

// New returns a fresh plugin instance with the options holder
// bound.
//
// The one declared [sdk.Output] carries [FilenameSuffix] and
// nothing else, because registrygen retains no filename control:
// the routing layer composes the name, and `+gen:out` on a source
// struct stays the way a user pins a specific one. Only Go is
// declared, so a run on any other backend gets nil and Layout
// reports a missing provider rather than composing Go-shaped
// names for it.
//
// The cross-cutting bucket runs it after the foundation and
// composition generators. It advertises [Capability] and requires
// nothing — no upstream plugin has to have produced anything
// before it walks the source structs.
//
// It registers no template helper of its own. `registration.tmpl`
// calls only the backend's dispatch helpers, and re-exporting the
// shared Go bundle from a plugin collides with the next plugin
// that does the same — a Build-time funcmap collision on a helper
// neither of them wrote.
func New() *Plugin {
	p := &Plugin{Base: sdkgo.NewGenerator(Name, goTemplates, sdk.Output{Suffix: FilenameSuffix}).
		Version(Version).
		Priority(sdk.GeneratorCrossCutting).
		Provides(Capability).
		Directives(sdk.NewDirective(DirectiveName).
			On(sdk.NodeKindStruct).
			Describe("Registers the host struct with the runtime registry on package init.").
			Build()).
		Build()}
	p.Holder = sdk.BindOptions(&p.opts)
	return p
}

// Registration is the plugin-defined emit kind every emitted
// registration carries. The matching `registration.tmpl` template
// renders it as a single-line `<RegisterFunc>(<NameLit>, <Init>)`
// call — slotted into the resolved file's `init` block so all
// registrations routed to the same file land inside one
// `func init() { ... }`.
type Registration struct {
	sdk.BaseEmit

	// Name is the source struct's identifier — also the key the
	// registry records the value under.
	Name string

	// NameLit is a pre-built [sdk.Expr] string literal carrying
	// the name in quoted form. Exposed so the template can render
	// it via `renderExpr` without needing to know how to escape.
	NameLit *sdk.Expr

	// Init is the expression evaluated at init time and passed as
	// the value argument to the register call. Generators produce
	// composite literals (`Article{}`), constructor calls, or any
	// other expression the registry accepts.
	Init *sdk.Expr

	// RegisterFunc is the register-call's callee — built via
	// [sdk.NewExternal] so the renderer registers the configured
	// import path with the rendered file's import set without
	// any plugin-side import scaffolding. Defaults to a reference
	// into stdlib `log.Print`; configurable through
	// [Options.RegisterPackage] / [Options.RegisterFunc].
	RegisterFunc *sdk.Expr
}

// Kind returns [Kind].
func (*Registration) Kind() sdk.Kind { return Kind }

// Compile-time confirmation that *Registration is a valid
// [sdk.EmitNode].
var _ sdk.EmitNode = (*Registration)(nil)

// Generate walks the source structs and appends one
// origin-anchored Registration to the `init` slot for each
// `+gen:register` annotated struct. The Layout phase resolves
// each contribution's origin to a rendered file downstream;
// the plugin itself sets no Target.
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name)
	for s := range ctx.Reader.Structs().All() {
		if !s.HasPositiveDirective(DirectiveName) {
			continue
		}
		reg := &Registration{
			BaseEmit: sdk.BaseEmit{
				OriginNode: s,
				SetByName:  c.SetBy(),
				SourcePos:  s.Pos(),
			},
			Name:         s.Name,
			NameLit:      sdk.NewLiteralString(s.Name),
			Init:         sdk.NewComposite(sdk.External(s.Package, s.Name), nil),
			RegisterFunc: sdk.NewExternal(p.opts.RegisterPackage, p.opts.RegisterFunc),
		}
		// Through AppendOrigin rather than AppendOriginSlot: the id this
		// spelled by hand had drifted, carrying `registry.<name>` where
		// every sibling carries `<kind>.<name>`. Letting the framework
		// compose it is what stops the next copy drifting too.
		if err := ctx.Store.Emit().AppendOrigin(c.SetBy(), SlotName, s, reg); err != nil {
			return err
		}
	}
	return nil
}
