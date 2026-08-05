// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"sort"
	"strings"

	"go.thesmos.sh/eidos/core/contract"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
	"go.thesmos.sh/eidos/store"
)

// formatMetaValue renders v in a human-readable form for the
// Metadata: section of `eidos explain`. A meta.Bag that has gone
// through JSON round-trip (via the frontend cache, manifest, etc.)
// holds its values as [json.RawMessage] rather than the typed Go
// value the Setter originally supplied — the Bag is type-erased on
// the wire and the typed [meta.Key.Get] path is the one that re-
// hydrates each value. The explain command works against an arbitrary
// key set without static type information, so this helper decodes a
// json.RawMessage payload back into a Go any value before formatting.
// Non-RawMessage values format directly via `%v`.
func formatMetaValue(v any) string {
	raw, ok := v.(json.RawMessage)
	if !ok {
		return fmt.Sprintf("%v", v)
	}
	var decoded any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return string(raw)
	}
	return fmt.Sprintf("%v", decoded)
}

// ExplainConfig holds the inputs for [ExplainCommand]. The
// selector identifies the entity, slot, or meta key the command
// inspects.
type ExplainConfig struct {
	File     *Config
	Plugins  []plugin.Plugin
	Selector string
	Format   DiagFormat
	Verbose  bool
	Quiet    bool

	// Routing carries the run's routing-layer flag overrides;
	// see [RoutingFlags] for the per-field semantics.
	Routing RoutingFlags
}

// ExplainCommand prints a provenance trace for a source or emit
// entity, member (method / field / variant), slot, or meta key.
// Runs the pipeline against an in-memory sink so the inspection
// is non-destructive.
//
// Selector grammar — both source-side and emit-side anchors are
// accepted; the command tries source-side first (the primary use
// case: "what will the pipeline do with this source decl?") then
// falls back to emit-side ("where did this synthetic decl come
// from?"):
//
//   - `pkg.Name` — source decl by short-package-name + identifier,
//     e.g. `blog.Article`. Disambiguates across packages by trying
//     the fully-qualified form too: `example.com/blog.Article`.
//   - `pkg.Name.Member` — method, field, or enum variant on the
//     named decl, e.g. `blog.Searcher.Find` (method on interface)
//     or `blog.Article.ID` (field on struct). Resolution greedily
//     splits the suffix off the rightmost dot, so packages with
//     dotted paths still parse correctly.
//   - `pkg.Name:slot` — slot contributions on that decl's rendered
//     output (debug-side, emit-anchored). Combines with the member
//     form: `pkg.Name.Method:prebody` lists Prebody contributions
//     on the named method's emit-side counterpart.
//   - `pkg.Name#key` — meta-key trail on that source or emit decl.
//     Combines with the member form.
type ExplainCommand struct{ Config ExplainConfig }

// RegisterFlags binds [ExplainCommand]'s flags into fs. The
// selector is a positional argument, not a flag.
func (c *ExplainCommand) RegisterFlags(fs *flag.FlagSet) {
	fs.Var(&c.Config.Format, FlagDiagFormat, UsageDiagFormat)
	fs.BoolVar(&c.Config.Verbose, FlagVerbose, false, UsageVerbose)
	fs.BoolVar(&c.Config.Quiet, FlagQuiet, false, UsageQuiet)
	c.Config.Routing.Register(fs)
}

// Execute resolves the selector against the emit graph and prints
// the corresponding trace.
func (c *ExplainCommand) Execute(ctx context.Context, env *Env) (exit int) {
	defer recoverInto(env, &exit)

	if c.Config.Selector == "" {
		writeErr(env, "explain: selector is required "+
			"(e.g. \"users.User\", \"users.User:methods\", \"users.User#shape.writer\")")
		return ExitUserError
	}
	sel, err := parseSelector(c.Config.Selector)
	if err != nil {
		writeErr(env, "explain: %v", err)
		return ExitUserError
	}
	cfg := c.Config.File
	if cfg == nil {
		cfg = DefaultConfig()
	}
	// Per spec: explain ignores -target so users can inspect any
	// entity regardless of an outer scope filter; the routing
	// flags still validate and the rest of the resolved policy
	// (output / package / layout / output-dir) threads through.
	routingForResolve := c.Config.Routing
	routingForResolve.Target = ""
	resolved, err := routingForResolve.Resolve(env, cfg, c.Config.Verbose)
	if err != nil {
		writeErr(env, "%v", err)
		return ExitUserError
	}
	p, err := buildPipeline(env, cfg, c.Config.Plugins, pipelineOverride{
		Verbose:      c.Config.Verbose,
		SinkOverride: sink.NewMemory(),
		Routing:      resolved,
	})
	if err != nil {
		writeErr(env, "%v", err)
		return ExitUserError
	}
	runErr := p.Run(ctx, patternsOrDefault(cfg)...)
	rerr := RenderDiagnostics(
		env.Stderr,
		p.Diag(),
		c.Config.Format,
		c.Config.Verbose,
		c.Config.Quiet,
	)
	if rerr != nil {
		writeErr(env, "%v", rerr)
	}
	if runErr != nil && !errors.Is(runErr, pipeline.ErrRunHadErrors) {
		writeErr(env, "%v", runErr)
		return ExitPipelineError
	}
	return c.report(env, p.Store(), sel)
}

// unattributedPlugin is the placeholder set-by attribution
// reported when an emit node or slot contribution carries no
// SetBy field — typically synthetic / framework-internal output.
// The constant unifies the four occurrences across the per-kind
// reporters.
const unattributedPlugin = "(unattributed)"

// selectorKind discriminates the three selector forms.
type selectorKind int

const (
	selectorEntity selectorKind = iota
	selectorSlot
	selectorMeta
)

// selector is a parsed explain target. Kind selects which Anchor
// + Modifier interpretation applies:
//
//   - selectorEntity: Anchor is "<pkg.QName>", Modifier empty.
//   - selectorSlot:   Anchor is "<pkg.QName>", Modifier is the
//     slot name.
//   - selectorMeta:   Anchor is "<pkg.QName>", Modifier is the
//     meta-key name.
type selector struct {
	Kind     selectorKind
	Anchor   string
	Modifier string
}

// parseSelector accepts one of the three documented forms and
// produces a [selector]. The pkg-and-qname portion is the anchor;
// the optional `:slot` or `#key` suffix is the modifier.
//
// The first colon or hash decides the modifier kind. An empty
// anchor or empty modifier returns a user error so the cli
// surfaces a positioned diagnostic rather than silently producing
// an unhelpful empty trace.
func parseSelector(raw string) (selector, error) {
	if raw == "" {
		return selector{}, errors.New("cli: empty selector")
	}
	if anchor, mod, ok := strings.Cut(raw, "#"); ok {
		if anchor == "" || mod == "" {
			return selector{}, fmt.Errorf(
				"meta-key selector %q: anchor and key are both required",
				raw,
			)
		}
		return selector{Kind: selectorMeta, Anchor: anchor, Modifier: mod}, nil
	}
	if anchor, mod, ok := strings.Cut(raw, ":"); ok {
		if anchor == "" || mod == "" {
			return selector{}, fmt.Errorf(
				"slot selector %q: anchor and slot name are both required",
				raw,
			)
		}
		return selector{Kind: selectorSlot, Anchor: anchor, Modifier: mod}, nil
	}
	return selector{Kind: selectorEntity, Anchor: raw}, nil
}

// report dispatches on the selector kind and prints the matching
// trace. An unresolvable selector exits ExitUserError so CI gates
// distinguish "I can't find this" from "the pipeline failed".
//
// Resolution order: source-side first (the headline case — what
// will the pipeline do with this source decl?) then emit-side
// (debug-side fallback for synthetic decls without a source
// anchor). The source-side report enumerates every plugin's
// output for the resolved decl plus directives and metadata; the
// emit-side report keeps the legacy provenance-trace shape.
func (c *ExplainCommand) report(env *Env, s *store.Store, sel selector) int {
	if source := findSourceEntity(s, sel.Anchor); source != nil {
		switch sel.Kind {
		case selectorEntity:
			return c.reportSourceEntity(env, s, source)
		case selectorSlot:
			return c.reportSourceSlot(env, s, source, sel.Modifier)
		case selectorMeta:
			return c.reportSourceMeta(env, source, sel.Modifier)
		default:
			writeErr(env, "explain: unknown selector kind %d", sel.Kind)
			return ExitUserError
		}
	}
	if parent, member, ok := resolveSourceMember(s, sel.Anchor); ok {
		switch sel.Kind {
		case selectorEntity:
			return c.reportSourceMember(env, s, parent, member)
		case selectorSlot:
			return c.reportSourceMemberSlot(env, s, member, sel.Modifier)
		case selectorMeta:
			return c.reportSourceMeta(env, member, sel.Modifier)
		default:
			writeErr(env, "explain: unknown selector kind %d", sel.Kind)
			return ExitUserError
		}
	}
	entity := findEntity(s, sel.Anchor)
	if entity == nil {
		writeErr(env, "explain: %q resolves to no source or emit entity", sel.Anchor)
		return ExitUserError
	}
	switch sel.Kind {
	case selectorEntity:
		return c.reportEntity(env, entity)
	case selectorSlot:
		return c.reportSlot(env, entity, sel.Modifier)
	case selectorMeta:
		return c.reportMeta(env, entity, sel.Modifier)
	default:
		writeErr(env, "explain: unknown selector kind %d", sel.Kind)
		return ExitUserError
	}
}

// resolveSourceMember walks anchor right-to-left looking for the
// longest source-entity prefix that owns a member named by the
// remaining suffix. Returns (parent, member, true) when both
// resolve; (nil, nil, false) when no split produces a valid pair.
// Used after [findSourceEntity] fails so direct entity anchors
// take precedence over member-suffix interpretations.
func resolveSourceMember(s *store.Store, anchor string) (node.Node, node.Node, bool) {
	for i := strings.LastIndexByte(anchor, '.'); i > 0; i = strings.LastIndexByte(anchor[:i], '.') {
		prefix := anchor[:i]
		memberName := anchor[i+1:]
		if memberName == "" {
			continue
		}
		parent := findSourceEntity(s, prefix)
		if parent == nil {
			continue
		}
		if m := lookupMember(parent, memberName); m != nil {
			return parent, m, true
		}
	}
	return nil, nil, false
}

// lookupMember returns the method, field, or enum-variant on
// parent matching name, or nil when parent owns no member by that
// name (or doesn't own members of any kind).
func lookupMember(parent node.Node, name string) node.Node {
	switch p := parent.(type) {
	case *node.Struct:
		if m := p.MethodByName(name); m != nil {
			return m
		}
		if f := p.FieldByName(name); f != nil {
			return f
		}
	case *node.Interface:
		if m := p.MethodByName(name); m != nil {
			return m
		}
	case *node.Enum:
		for _, v := range p.Variants {
			if v.Name == name {
				return v
			}
		}
	}
	return nil
}

// reportSourceMember prints the per-member view: subject + kind,
// owner reference with position, member-specific shape (signature
// for methods, type + tag for fields, value for variants),
// directives, metadata, slot contributions (sourced from the
// emit-side counterparts), and the emit-side outputs anchored
// to this member.
func (*ExplainCommand) reportSourceMember(env *Env, s *store.Store, parent, member node.Node) int {
	w := env.Stdout
	memberLabel := sourceQName(parent) + "." + memberDisplayName(member)
	fmt.Fprintf(w, "Subject:    %s (%s)\n", memberLabel, member.Kind())
	ownerLine := sourceQName(parent) + " (" + string(parent.Kind()) + ")"
	if pos := parent.Pos(); pos.File != "" {
		ownerLine += " at " + pos.String()
	}
	fmt.Fprintf(w, "Owner:      %s\n", ownerLine)
	if pos := member.Pos(); pos.File != "" {
		fmt.Fprintf(w, "Position:   %s\n", pos.String())
	}
	if sig := memberShape(member); sig != "" {
		fmt.Fprintf(w, "%s\n", sig)
	}
	if dirs := member.Directives(); len(dirs) > 0 {
		fmt.Fprintln(w, "Directives:")
		for _, d := range dirs {
			fmt.Fprintf(w, "  %s\n", formatDirective(d))
		}
	}
	if meta := member.Meta(); meta != nil {
		if names := meta.Names(); len(names) > 0 {
			sort.Strings(names)
			fmt.Fprintln(w, "Metadata:")
			for _, name := range names {
				if v, ok := meta.RawValue(name); ok {
					fmt.Fprintf(w, "  %s = %s\n", name, formatMetaValue(v))
				}
			}
		}
	}
	emitMembers := findEmitMembersForOrigin(s, member)
	if slots := summariseMemberSlots(emitMembers); len(slots) > 0 {
		fmt.Fprintln(w, "Slots:")
		for _, line := range slots {
			fmt.Fprintf(w, "  %s\n", line)
		}
	}
	outputs := summariseMemberOutputs(emitMembers)
	if len(outputs) == 0 {
		fmt.Fprintln(w, "Outputs:    (no plugin emitted output for this member)")
		return ExitOK
	}
	fmt.Fprintln(w, "Outputs:")
	for _, group := range outputs {
		fmt.Fprintf(w, "  %s\n", group.label())
		for _, item := range group.items {
			fmt.Fprintf(w, "    %s\n", item)
		}
	}
	return ExitOK
}

// reportSourceMemberSlot inspects the named slot on every emit-side
// counterpart of the source member and prints the union of
// contributions. Cross-cutting weavers attach to emit methods'
// Prebody / Postbody / ParamsSlot, so the inspection walks one
// hop forward from the source method.
func (c *ExplainCommand) reportSourceMemberSlot(
	env *Env, s *store.Store, member node.Node, name string,
) int {
	hosts := findEmitMembersForOrigin(s, member)
	if len(hosts) == 0 {
		writeErr(
			env,
			"explain: %s has no emit-side counterpart to inspect slot %q on",
			memberDisplayName(member),
			name,
		)
		return ExitUserError
	}
	for _, host := range hosts {
		owner, ok := host.(emit.SlotHost)
		if !ok {
			continue
		}
		slot := owner.Slot(name)
		if slot != nil && slot.Len() > 0 {
			return c.reportSlot(env, host, name)
		}
	}
	writeErr(
		env,
		"explain: slot %q has no contributions on any emit-side counterpart of %s",
		name,
		memberDisplayName(member),
	)
	return ExitUserError
}

// memberDisplayName returns the human-readable identifier for a
// source member node — the Name field of methods / fields /
// variants. Empty for nodes without a stable Name field.
func memberDisplayName(n node.Node) string {
	switch m := n.(type) {
	case *node.Method:
		return m.Name
	case *node.Field:
		return m.Name
	case *node.EnumVariant:
		return m.Name
	default:
		return ""
	}
}

// memberShape renders the kind-specific signature line for a
// member. Methods print as `Signature: Name(<params>) <returns>`,
// fields print as `Type:       <type>` (plus `Tag:` when present),
// variants print as `Value:      <value>`. Empty return suppresses
// the line entirely.
func memberShape(n node.Node) string {
	switch m := n.(type) {
	case *node.Method:
		return "Signature:  " + renderMethodSig(m)
	case *node.Field:
		typeStr := renderTypeRef(m.Type)
		out := "Type:       " + typeStr
		if m.Tag != "" {
			out += "\nTag:        `" + m.Tag + "`"
		}
		return out
	case *node.EnumVariant:
		return "Value:      " + m.Value
	default:
		return ""
	}
}

// renderMethodSig prints a method's signature in
// `Name(p1 T1, p2 T2) (r1, r2)` form. Anonymous params render
// with their type only; multi-return methods get parentheses
// around the return list to mirror Go's syntax.
func renderMethodSig(m *node.Method) string {
	params := make([]string, 0, len(m.Params))
	for i, p := range m.Params {
		typ := renderTypeRef(p.Type)
		name := p.Name
		if name == "" {
			name = fmt.Sprintf("arg%d", i)
		}
		params = append(params, name+" "+typ)
	}
	returns := make([]string, 0, len(m.Returns))
	for _, r := range m.Returns {
		// A declared return name is documentation the signature
		// carries; `explain` exists to show what the model holds, so
		// render it when the source declared one. Blank returns
		// normalise to unnamed upstream, so a mixed rendering here
		// reflects a genuinely mixed source signature.
		if r.Name != "" {
			returns = append(returns, r.Name+" "+renderTypeRef(r.Type))
			continue
		}
		returns = append(returns, renderTypeRef(r.Type))
	}
	sig := m.Name + "(" + strings.Join(params, ", ") + ")"
	switch len(returns) {
	case 0:
		return sig
	case 1:
		return sig + " " + returns[0]
	default:
		return sig + " (" + strings.Join(returns, ", ") + ")"
	}
}

// renderTypeRef projects a [node.TypeRef] back into a
// source-shaped string. Recursive on Elem / MapKey / MapValue and
// covers the kinds explain reports against; type kinds without a
// natural one-line rendering fall back to the literal Name.
func renderTypeRef(t *node.TypeRef) string {
	if t == nil {
		return ""
	}
	switch t.TypeKind {
	case node.TypeRefPointer:
		return "*" + renderTypeRef(t.Elem)
	case node.TypeRefSlice:
		return "[]" + renderTypeRef(t.Elem)
	case node.TypeRefMap:
		return "map[" + renderTypeRef(t.MapKey) + "]" + renderTypeRef(t.MapValue)
	case node.TypeRefArray:
		return fmt.Sprintf("[%d]%s", t.ArrayLen, renderTypeRef(t.Elem))
	case node.TypeRefNamed, node.TypeRefTypeParam:
		if t.Package != "" {
			return t.Package + "." + t.Name
		}
		return t.Name
	default:
		return t.Name
	}
}

// formatDirective renders one directive back into its source-form
// line: `+gen:NAME args…` for positive, `-gen:NAME args…` for
// negated. Pulled out of [reportSourceEntity] so the per-member
// branch shares the same rendering.
func formatDirective(d *directive.Directive) string {
	prefix := "+"
	if d.Negated {
		prefix = "-"
	}
	line := prefix + "gen:" + string(d.Name)
	if d.Raw != "" {
		rest := strings.TrimPrefix(d.Raw, string(d.Name))
		line += rest
	}
	return line
}

// findEmitMembersForOrigin walks every emit-side method / field
// related to member and returns them. The relation tests in
// order:
//
//  1. Direct: emit.Method.Origin == member (the plugin set
//     Origin on the emitted member directly).
//  2. Indirect by name + parent-origin: the emit member's host
//     struct / interface has Origin == member's parent, AND the
//     emit member's Name matches member's Name. This catches
//     plugin patterns (mockgen, weavers) that set Origin on the
//     host but not on the per-method node.
//
// Direct matches win when both kinds resolve; the indirect form
// is a fallback so users see "what plugin produced something
// named after this method?".
func findEmitMembersForOrigin(s *store.Store, member node.Node) []emit.Node {
	if s == nil {
		return nil
	}
	parent := parentOf(member)
	memberName := memberDisplayName(member)
	var direct, indirect []emit.Node
	s.Emit().Methods().Range(func(m *emit.Method) bool {
		switch {
		case m.Origin() == member:
			direct = append(direct, m)
		case parent != nil && memberName != "" && m.Name == memberName && emitHostOrigin(m.Owner) == parent:
			indirect = append(indirect, m)
		}
		return true
	})
	s.Emit().Fields().Range(func(f *emit.Field) bool {
		switch {
		case f.Origin() == member:
			direct = append(direct, f)
		case parent != nil && memberName != "" && f.Name == memberName && emitHostOrigin(f.Owner) == parent:
			indirect = append(indirect, f)
		}
		return true
	})
	if len(direct) > 0 {
		return direct
	}
	return indirect
}

// parentOf returns the declaring host of a member node. Each
// member kind carries its owner via the Owner field set by the
// frontend; explain uses the back-pointer to correlate emit-side
// outputs by their host's origin attribution.
func parentOf(n node.Node) node.Node {
	switch m := n.(type) {
	case *node.Method:
		return m.Owner
	case *node.Field:
		return m.Owner
	case *node.EnumVariant:
		return m.Owner
	default:
		return nil
	}
}

// emitHostOrigin returns the Origin attribution of host — the
// source-side node the emit struct / interface was generated
// from. Returns nil for hosts without a BaseEmit origin or for
// non-routable host kinds (including source-side hosts on top-
// level methods anchored at a [node.Enum] etc.).
//
// host is typed [contract.Node] so callers passing either an
// emit-side or source-side owner all flow through the same
// origin-resolution path; the type assertion picks out the
// emit-only Origin() method without restricting the input
// surface upfront.
func emitHostOrigin(host contract.Node) node.Node {
	if host == nil {
		return nil
	}
	type originAccessor interface{ Origin() node.Node }
	if h, ok := host.(originAccessor); ok {
		return h.Origin()
	}
	return nil
}

// summariseMemberSlots collects slot contributions on every emit
// counterpart in hosts and returns one summary line per slot
// (slot name + contribution count + comma-separated set-by
// attributions). Slot lookup is via the slotMap surface every
// emit method exposes.
func summariseMemberSlots(hosts []emit.Node) []string {
	var lines []string
	for _, host := range hosts {
		owner, ok := host.(emit.SlotHost)
		if !ok {
			continue
		}
		for slotName, slot := range slotsOf(owner) {
			if slot.Len() == 0 {
				continue
			}
			authors := make([]string, 0, slot.Len())
			for i := range slot.Len() {
				setBy := slot.ProvenanceAt(i).SetBy
				if setBy == "" {
					setBy = unattributedPlugin
				}
				authors = append(authors, setBy)
			}
			lines = append(
				lines,
				fmt.Sprintf(
					"%s: %d contribution(s) — %s",
					slotName,
					slot.Len(),
					strings.Join(authors, ", "),
				),
			)
		}
	}
	sort.Strings(lines)
	return lines
}

// slotsOf returns the slot map on an emit.SlotHost when the host
// exposes one via the slotMap mixin (the embedded surface on
// every routable kind). An empty map otherwise — the caller
// iterates safely either way.
func slotsOf(host emit.SlotHost) map[string]*emit.Slot {
	type slotEnumerator interface {
		SlotsByName() map[string]*emit.Slot
	}
	if se, ok := host.(slotEnumerator); ok {
		return se.SlotsByName()
	}
	return nil
}

// summariseMemberOutputs enumerates the emit-side counterparts of
// a source member, grouped by producing plugin. Each entry's host
// is also surfaced so the user sees "Find on SearcherMock" rather
// than just "Find".
func summariseMemberOutputs(hosts []emit.Node) []outputGroup {
	groups := map[string]*outputGroup{}
	for _, host := range hosts {
		setBy := emitSetBy(host)
		if setBy == "" {
			setBy = unattributedPlugin
		}
		g, ok := groups[setBy]
		if !ok {
			g = &outputGroup{plugin: setBy}
			groups[setBy] = g
		}
		g.items = append(g.items, emitMemberLine(host))
	}
	out := make([]outputGroup, 0, len(groups))
	for _, g := range groups {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].plugin < out[j].plugin })
	return out
}

// emitSetBy returns the SetBy attribution stamped on host's
// BaseEmit. Method and Field nodes typically don't carry their
// own SetBy (they're inline-rendered by the host struct /
// interface template), so the helper falls back to the owner's
// attribution when the direct field is empty — the user sees
// "mockgen" rather than unattributedPlugin for a method inside a
// mockgen-emitted struct.
func emitSetBy(host contract.Node) string {
	type setByAccessor interface{ SetBy() string }
	if h, ok := host.(setByAccessor); ok {
		if id := h.SetBy(); id != "" {
			return id
		}
	}
	switch h := host.(type) {
	case *emit.Method:
		if h.Owner != nil {
			return emitSetBy(h.Owner)
		}
	case *emit.Field:
		if h.Owner != nil {
			return emitSetBy(h.Owner)
		}
	}
	return ""
}

// emitMemberLine formats one emit-side member for the Outputs
// section: kind, name, owner reference.
func emitMemberLine(host emit.Node) string {
	switch h := host.(type) {
	case *emit.Method:
		owner := "(detached)"
		if h.Owner != nil {
			owner = emitOwnerLabel(h.Owner)
		}
		return fmt.Sprintf("%s on %s (method)", h.Name, owner)
	case *emit.Field:
		owner := "(detached)"
		if h.Owner != nil {
			owner = emitOwnerLabel(h.Owner)
		}
		return fmt.Sprintf("%s on %s (field)", h.Name, owner)
	default:
		return fmt.Sprintf("%v (%s)", host, host.Kind())
	}
}

// emitOwnerLabel formats an emit owner reference — struct or
// interface name, falling back to the kind tag when the owner is
// neither.
func emitOwnerLabel(owner contract.Node) string {
	switch o := owner.(type) {
	case *emit.Struct:
		return o.Name
	case *emit.Interface:
		return o.Name
	default:
		return string(owner.Kind())
	}
}

// findSourceEntity walks every source-side node bucket for an
// entity matching anchor. Matching tries two forms in sequence:
// the fully-qualified `<path>.<Name>` (which the store indexes
// by) and the short-package-form `<pkg.Name>.<Name>` (the
// human-friendly form go: directives and CLI invocations use).
// Returns nil when neither form matches anywhere.
func findSourceEntity(s *store.Store, anchor string) node.Node {
	if s == nil {
		return nil
	}
	v := s.Nodes()
	if found := lookupByQName(v, anchor); found != nil {
		return found
	}
	pkgName, base, ok := strings.Cut(anchor, ".")
	if !ok || pkgName == "" || base == "" {
		return nil
	}
	var found node.Node
	v.Packages().Range(func(p *node.Package) bool {
		if p.Name != pkgName {
			return true
		}
		qualified := p.Path + "." + base
		if hit := lookupByQName(v, qualified); hit != nil {
			found = hit
			return false
		}
		return true
	})
	return found
}

// lookupByQName probes every source bucket for the supplied
// fully-qualified name and returns the first match. Each bucket
// indexes its entries by QName so the lookup is O(1) per bucket.
func lookupByQName(v *store.NodeView, qname string) node.Node {
	if s, ok := v.Structs().ByQName(qname); ok {
		return s
	}
	if i, ok := v.Interfaces().ByQName(qname); ok {
		return i
	}
	if f, ok := v.Functions().ByQName(qname); ok {
		return f
	}
	if a, ok := v.Aliases().ByQName(qname); ok {
		return a
	}
	if vd, ok := v.Variables().ByQName(qname); ok {
		return vd
	}
	if cn, ok := v.Constants().ByQName(qname); ok {
		return cn
	}
	if e, ok := v.Enums().ByQName(qname); ok {
		return e
	}
	return nil
}

// reportSourceEntity prints the source-anchored view of n: a
// subject summary line, every directive attached to n, every
// meta-key stamp on n's bag, and one block per plugin whose emit
// graph contributed something for n — listing each contribution's
// Target path and the routed decl identifiers.
func (*ExplainCommand) reportSourceEntity(env *Env, s *store.Store, n node.Node) int {
	w := env.Stdout
	fmt.Fprintf(w, "Subject:    %s (%s)\n", sourceQName(n), n.Kind())
	if pos := n.Pos(); pos.File != "" {
		fmt.Fprintf(w, "Position:   %s\n", pos.String())
	}
	if dirs := n.Directives(); len(dirs) > 0 {
		fmt.Fprintln(w, "Directives:")
		for _, d := range dirs {
			fmt.Fprintf(w, "  %s\n", formatDirective(d))
		}
	}
	if meta := n.Meta(); meta != nil {
		if names := meta.Names(); len(names) > 0 {
			sort.Strings(names)
			fmt.Fprintln(w, "Metadata:")
			for _, name := range names {
				if v, ok := meta.RawValue(name); ok {
					fmt.Fprintf(w, "  %s = %s\n", name, formatMetaValue(v))
				}
			}
		}
	}
	outputs := collectSourceOutputs(s, n)
	if len(outputs) == 0 {
		fmt.Fprintln(w, "Outputs:    (no plugin emitted output for this source)")
		return ExitOK
	}
	fmt.Fprintln(w, "Outputs:")
	for _, group := range outputs {
		fmt.Fprintf(w, "  %s\n", group.label())
		for _, item := range group.items {
			fmt.Fprintf(w, "    %s\n", item)
		}
	}
	return ExitOK
}

// reportSourceSlot resolves the source entity's emit-side
// counterpart and dispatches to the existing slot-trace reporter.
// Slots live on emit decls; the source side carries the directives
// that triggered emission but not the slot bag itself, so the
// command walks one hop forward through the emit graph.
func (c *ExplainCommand) reportSourceSlot(
	env *Env,
	s *store.Store,
	n node.Node,
	name string,
) int {
	hosts := findEmitDeclsForOrigin(s, n)
	if len(hosts) == 0 {
		writeErr(
			env,
			"explain: %s has no emit-side counterpart to inspect slot %q on",
			sourceQName(n),
			name,
		)
		return ExitUserError
	}
	for _, host := range hosts {
		if owner, ok := host.(emit.SlotHost); ok {
			if slot := owner.Slot(name); slot != nil && slot.Len() > 0 {
				return c.reportSlot(env, host, name)
			}
		}
	}
	writeErr(
		env,
		"explain: slot %q has no contributions on any emit decl for %s",
		name,
		sourceQName(n),
	)
	return ExitUserError
}

// reportSourceMeta prints the source-side meta-key value. Source
// decls carry meta bags just like emit decls; the reporter is the
// same shape as the emit-side reportMeta to keep the user
// experience uniform across selector forms.
func (*ExplainCommand) reportSourceMeta(env *Env, n node.Node, key string) int {
	meta := n.Meta()
	if meta == nil {
		writeErr(env, "explain: source entity has no meta bag")
		return ExitUserError
	}
	v, ok := meta.RawValue(key)
	if !ok {
		writeErr(env, "explain: meta key %q is unset on %s", key, sourceQName(n))
		return ExitUserError
	}
	fmt.Fprintf(env.Stdout, "Meta %q = %s\n", key, formatMetaValue(v))
	return ExitOK
}

// sourceQName returns a print-friendly identifier for n —
// `<path>.<Name>` for structs / interfaces / functions / aliases /
// variables / constants / enums; bare name for nodes without a
// package field.
func sourceQName(n node.Node) string {
	type qnamer interface{ QName() string }
	if q, ok := n.(qnamer); ok {
		return q.QName()
	}
	return string(n.Kind())
}

// outputGroup buckets emit-side contributions by their producing
// plugin so the reportSourceEntity output reads as one block per
// plugin: "plugin → file (decl-names)".
type outputGroup struct {
	plugin string
	items  []string
}

// label formats the group's header line — the plugin name on the
// left, a colon, and a hint at the count when more than one
// contribution lands.
func (g outputGroup) label() string {
	switch len(g.items) {
	case 0:
		return g.plugin + ":"
	case 1:
		return g.plugin + ":"
	default:
		return fmt.Sprintf("%s: (%d contributions)", g.plugin, len(g.items))
	}
}

// collectSourceOutputs walks every emit-decl bucket and groups the
// entries whose Origin == n by their producing plugin. Each item
// is rendered as `<target-path>  <Name> (<kind>)`. Output order
// is alphabetical by plugin name so the report stays deterministic
// across runs.
func collectSourceOutputs(s *store.Store, n node.Node) []outputGroup {
	if s == nil {
		return nil
	}
	groups := map[string]*outputGroup{}
	add := func(setBy, line string) {
		if setBy == "" {
			setBy = unattributedPlugin
		}
		g, ok := groups[setBy]
		if !ok {
			g = &outputGroup{plugin: setBy}
			groups[setBy] = g
		}
		g.items = append(g.items, line)
	}
	v := s.Emit()
	format := func(target emit.Target, name, kind string) string {
		path := target.JoinPath()
		if path == "" {
			path = "(unrouted)"
		}
		return fmt.Sprintf("%-40s %s (%s)", path, name, kind)
	}
	v.Structs().Range(func(e *emit.Struct) bool {
		if e.Origin() == n {
			add(e.SetBy(), format(e.Target, e.Name, "struct"))
		}
		return true
	})
	v.Interfaces().Range(func(e *emit.Interface) bool {
		if e.Origin() == n {
			add(e.SetBy(), format(e.Target, e.Name, "interface"))
		}
		return true
	})
	v.Functions().Range(func(e *emit.Function) bool {
		if e.Origin() == n {
			add(e.SetBy(), format(e.Target, e.Name, "function"))
		}
		return true
	})
	v.Variables().Range(func(e *emit.Variable) bool {
		if e.Origin() == n {
			add(e.SetBy(), format(e.Target, e.Name, "variable"))
		}
		return true
	})
	v.Constants().Range(func(e *emit.Constant) bool {
		if e.Origin() == n {
			add(e.SetBy(), format(e.Target, e.Name, "constant"))
		}
		return true
	})
	v.Enums().Range(func(e *emit.Enum) bool {
		if e.Origin() == n {
			add(e.SetBy(), format(e.Target, e.Name, "enum"))
		}
		return true
	})
	v.Aliases().Range(func(e *emit.Alias) bool {
		if e.Origin() == n {
			add(e.SetBy(), format(e.File, e.Name, "alias"))
		}
		return true
	})
	v.Files().Range(func(f *emit.File) bool {
		fileTarget := emit.Target{Dir: f.Dir, Filename: f.Name, Package: f.Package}
		for slotName, slot := range f.SlotsByName() {
			for i := range slot.Len() {
				item := slot.At(i)
				if item == nil || item.Origin() != n {
					continue
				}
				prov := slot.ProvenanceAt(i)
				line := fmt.Sprintf(
					"%-40s slot:%s (%s)",
					fileTarget.JoinPath(), slotName, item.Kind(),
				)
				add(prov.SetBy, line)
			}
		}
		return true
	})
	out := make([]outputGroup, 0, len(groups))
	for _, g := range groups {
		out = append(out, *g)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].plugin < out[j].plugin })
	return out
}

// findEmitDeclsForOrigin returns every routable emit decl whose
// Origin equals n. Used by the source-side slot reporter to find
// candidate emit decls that might host the requested slot.
func findEmitDeclsForOrigin(s *store.Store, n node.Node) []emit.Node {
	var out []emit.Node
	v := s.Emit()
	v.Structs().Range(func(e *emit.Struct) bool {
		if e.Origin() == n {
			out = append(out, e)
		}
		return true
	})
	v.Interfaces().Range(func(e *emit.Interface) bool {
		if e.Origin() == n {
			out = append(out, e)
		}
		return true
	})
	return out
}

// findEntity walks every emit-store bucket for an entity whose
// QName equals anchor. Returns the first match or nil.
//
// Multiple kinds may carry the same QName (a struct and an alias
// with identical names, for example); the function picks the
// first match in bucket-iteration order, which is deterministic
// across runs but doesn't disambiguate. Callers needing
// disambiguation should pass a kind-qualified selector — out of
// scope for this iteration.
func findEntity(s *store.Store, anchor string) emit.Node {
	if s == nil {
		return nil
	}
	v := s.Emit()
	var found emit.Node
	scan := func(n emit.Node, qname string) bool {
		if qname == anchor {
			found = n
			return false
		}
		return true
	}
	v.Structs().Range(func(e *emit.Struct) bool { return scan(e, e.QName()) })
	if found != nil {
		return found
	}
	v.Interfaces().Range(func(e *emit.Interface) bool { return scan(e, e.QName()) })
	if found != nil {
		return found
	}
	v.Functions().Range(func(e *emit.Function) bool { return scan(e, e.QName()) })
	if found != nil {
		return found
	}
	v.Variables().Range(func(e *emit.Variable) bool { return scan(e, e.QName()) })
	if found != nil {
		return found
	}
	v.Constants().Range(func(e *emit.Constant) bool { return scan(e, e.QName()) })
	if found != nil {
		return found
	}
	v.Enums().Range(func(e *emit.Enum) bool { return scan(e, e.QName()) })
	if found != nil {
		return found
	}
	v.Aliases().Range(func(e *emit.Alias) bool { return scan(e, e.QName()) })
	return found
}

// reportEntity prints kind / origin / directive count / meta-key
// count for the entity. Mirrors the spec's sketched output shape.
func (*ExplainCommand) reportEntity(env *Env, n emit.Node) int {
	w := env.Stdout
	fmt.Fprintf(w, "Kind:       %s\n", n.Kind())
	if origin := n.Origin(); origin != nil {
		pos := origin.Pos()
		if pos.File != "" {
			fmt.Fprintf(w, "Origin:     %s\n", pos.String())
		}
	}
	if dirs := n.Directives(); len(dirs) > 0 {
		fmt.Fprintln(w, "Directives:")
		for _, d := range dirs {
			fmt.Fprintf(w, "  - %s\n", d.Name)
		}
	}
	if meta := n.Meta(); meta != nil {
		names := meta.Names()
		if len(names) > 0 {
			sort.Strings(names)
			fmt.Fprintln(w, "Meta keys:")
			for _, name := range names {
				fmt.Fprintf(w, "  - %s\n", name)
			}
		}
	}
	return ExitOK
}

// reportSlot prints the contributions of a named slot on the
// entity, each line prefixed with its index and the
// Provenance.SetBy / Provenance.ID values when present.
func (*ExplainCommand) reportSlot(env *Env, host emit.Node, name string) int {
	owner, ok := host.(emit.SlotHost)
	if !ok {
		writeErr(env, "explain: %s does not own slots", host.Kind())
		return ExitUserError
	}
	slot := owner.Slot(name)
	if slot == nil || slot.Len() == 0 {
		writeErr(env, "explain: slot %q has no contributions", name)
		return ExitUserError
	}
	fmt.Fprintf(env.Stdout, "Slot %q (%d contributions):\n", name, slot.Len())
	for i := range slot.Len() {
		prov := slot.ProvenanceAt(i)
		setBy := prov.SetBy
		if setBy == "" {
			setBy = unattributedPlugin
		}
		idSuffix := ""
		if prov.ID != "" {
			idSuffix = " id=" + prov.ID
		}
		fmt.Fprintf(env.Stdout, "  [%d] set by %s%s\n", i, setBy, idSuffix)
	}
	return ExitOK
}

// reportMeta prints the meta-key's value and authority on the
// entity. Unset keys exit ExitUserError so CI gates can detect
// "I asked about a key that doesn't exist".
func (*ExplainCommand) reportMeta(env *Env, n emit.Node, key string) int {
	meta := n.Meta()
	if meta == nil {
		writeErr(env, "explain: entity has no meta bag")
		return ExitUserError
	}
	v, ok := meta.RawValue(key)
	if !ok {
		writeErr(env, "explain: meta key %q is unset on %s", key, n.Kind())
		return ExitUserError
	}
	fmt.Fprintf(env.Stdout, "Meta %q = %s\n", key, formatMetaValue(v))
	return ExitOK
}
