// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package sentinel generates the checks that hold a package's error
// contract to what its declarations promise.
//
// Callers match on declared error values, read them in logs and branch
// on them, so their messages and identities are as much an API as any
// exported signature — and nothing in a compiler holds them to it. Two
// values that match each other collapse a caller's branches; a wrapper
// that drops its cause truncates every chain it takes part in; a
// message that omits a field the type carries hides the one detail the
// struct exists to record. Each is invisible from the declarations.
//
// Nothing here writes production code. The author declares the errors;
// this plugin asserts their invariants.
//
// # Language-neutral core
//
// This file names no language. Which identifiers count as declared
// errors, which declarations take part in the error protocol, and what
// each of those owes are all asked through [sdk.ErrorRules] — see
// [sdk.ErrorInfo] for the vocabulary the answers arrive in. The
// signatures around them are spelled in the templates, which are
// per-language by construction.
//
// See the package README for what is asserted, what is deliberately
// not, and the limits.
package sentinel

import (
	"fmt"
	"slices"

	"go.thesmos.sh/eidos/sdk"
)

// Name is the plugin's stable identifier.
const Name = "sentinel"

// Version composes into the pipeline's plugin fingerprint, which
// frontends fold into their cache keys — so bumping it invalidates a
// warm cache populated when this plugin emitted something else.
const Version = "2.0.0"

// Capability is the label the plugin advertises so a downstream
// consumer can declare a documentary dependency on these checks.
const Capability = "sentinel"

// DirectiveName is the bare directive name, without the `+gen:`
// prefix, that opts a package in.
const DirectiveName sdk.DirectiveName = "sentinel"

// NoOverlapName names another package this one's declared errors must
// stay distinct from.
//
// A separate directive rather than a key on [DirectiveName] because it
// repeats: a package may name several neighbours, and each line unions
// into one set. A key would have to encode the list into one value.
const NoOverlapName sdk.DirectiveName = "sentinel-no-overlap-with"

// PrefixKey overrides the prefix every message must begin with, and
// [PrefixOff] suppresses that check.
const (
	PrefixKey = "prefix"
	PrefixOff = "off"
)

// PrefixSeparator joins a package's name to the rest of its messages.
//
// This plugin's own convention rather than a language's: it is what
// the check asserts and what an author writes, and neither is decided
// by how the language spells anything.
const PrefixSeparator = ": "

// FileSlot is the [sdk.EmitFile] slot the checks land in. `top`
// renders between the package clause and the first core declaration,
// which is where a block of whole declarations belongs.
const FileSlot = "top"

// SlotChecks is the check file's function block, after the checks this
// plugin derives.
//
// For an assertion this plugin cannot see: an error that has to keep
// matching one in a package it does not name, or a message a wire
// format pins the wording of.
const SlotChecks = "checks"

// KindTests is the plugin-defined emit kind. The backend resolves a
// template by the kind's string value, so the constant doubles as the
// name its template defines.
const KindTests sdk.Kind = "sentinel.tests"

// Options carries the plugin's user-tunable settings.
type Options struct {
	// Prefix overrides the message prefix every declared error is
	// asserted to carry, for a repository whose convention is not the
	// package's own name.
	//
	// A per-package `prefix=` on the directive still wins: the option
	// is the repository stating its default, and the directive is one
	// package stating an exception.
	Prefix string `eidos:"prefix"`
}

// Plugin is the error-contract check generator. Zero value is
// unusable; go through [New] so the embedded [sdk.Holder] binds to the
// options field.
type Plugin struct {
	*sdk.Base
	*sdk.Holder[Options]
	opts Options
}

// New returns a fresh plugin instance.
//
// Nothing is required and the bucket is not load-bearing. The scan
// reads source declarations, so it sees the same graph whichever
// bucket it runs in, and the cross-cutting one is where a plugin that
// only asserts belongs rather than where this one has to be.
//
// It does not reach what other generators emitted, and should not. An
// error a run generates carries its message and its identity by
// construction — the emitting generator composes the prefix and names
// the type it refuses — so every check here would assert what that
// generator already guarantees, which is the vacuous check this plugin
// exists to avoid writing.
//
// [Capability] is published so a consumer can declare a documentary
// dependency on these checks.
func New() *Plugin {
	p := &Plugin{Base: sdk.NewPlugin(Name).
		Version(Version).
		Priority(sdk.GeneratorCrossCutting).
		Provides(Capability).
		Directives(directives()...).
		For(goSupport()).
		Build()}
	p.Holder = sdk.BindOptions(&p.opts)
	return p
}

// directives declares both schemas.
func directives() []sdk.DirectiveSchema {
	return []sdk.DirectiveSchema{
		sdk.NewDirective(DirectiveName).
			Describe(
				"Generates checks over the host package's error contract: every " +
					"declared error's message prefix, its distinctness from its " +
					"neighbours, and one check per exported type taking part in the " +
					"error protocol. `prefix=<value>` overrides the expected message " +
					"prefix; `prefix=off` suppresses that check. The negated form is " +
					"rejected — removing the directive is the suppression.",
			).
			AllowedKeys(PrefixKey).
			On(sdk.NodeKindPackage).
			DenyNegation().
			Build(),
		sdk.NewDirective(NoOverlapName).
			Describe(
				"Names another package whose declared errors must not match this " +
					"package's. Takes one import path and repeats; each line adds to " +
					"the set.",
			).
			Positional("package").
			On(sdk.NodeKindPackage).
			DenyNegation().
			Build(),
	}
}

// Sentinel is one declared package-level error value.
type Sentinel struct {
	// Name is the identifier, which the checks report by so a failure
	// names the declaration rather than its message.
	Name string

	// Ref qualifies it. The checks live in the package's external test
	// package, so nothing is reachable unqualified.
	Ref *sdk.Expr
}

// Field is one member of an error type, with what a check writes into
// it.
type Field struct {
	Name string

	// Sample is the value a check writes. Ask [sdk.Sample.OK] rather
	// than comparing against the zero value.
	Sample sdk.Sample

	// Verbatim reports that the message carries this value's text
	// unchanged, so a check may assert the text appears in it.
	Verbatim bool
}

// Peer is another error type declared in the same package, named so a
// check can assert this one does not match it.
type Peer struct {
	Name string
	Ref  *sdk.Expr

	// Addressed mirrors [ErrType.Addressed] for the peer, because a
	// check builds a value of it and the two need not agree.
	Addressed bool
}

// ErrType is one exported declaration taking part in the error
// protocol.
type ErrType struct {
	Name string
	Ref  *sdk.Expr

	// Addressed reports that a value has to be addressed before it
	// carries the contract, which decides whether a check builds
	// `&T{}` or `T{}`.
	Addressed bool

	// Cause is the member holding the wrapped error, empty when the
	// type carries none. Without one there is nothing to hand the
	// type, so its unwrap check is withheld rather than written
	// against an absent value.
	Cause string

	// Compares and Unwraps record the two optional halves. Each earns
	// a check, and a type declaring neither gets neither rather than a
	// vacuous one.
	Compares, Unwraps bool

	// Fields are the members a message is expected to mention.
	Fields []Field

	// Prefix, Peers and Seeds are the package's facts this type's own
	// checks need, carried here rather than reached for through the
	// enclosing value.
	//
	// Two of the assertions are about the type's place among its
	// neighbours rather than about the type alone: it must not match
	// another declared here, and where it compares itself, the
	// standard comparison has to reach that method — which needs a
	// declared error to compare against. Carried so the rendered block
	// stands on its own, which is what a slot contributor extending it
	// also gets.
	Prefix string
	Peers  []Peer
	Seeds  []Sentinel

	// decl is the declaration this was projected from, carried so the
	// anchor comes from what the scan already found rather than from a
	// second one that could disagree with it.
	decl *sdk.Struct
}

// Seed returns the declared error a comparison check compares against,
// nil when the package declares none.
func (e ErrType) Seed() *Sentinel {
	if len(e.Seeds) == 0 {
		return nil
	}
	return &e.Seeds[0]
}

// Written returns the members a check can put a value in.
func (e ErrType) Written() []Field {
	out := make([]Field, 0, len(e.Fields))
	for _, f := range e.Fields {
		if f.Sample.OK() {
			out = append(out, f)
		}
	}
	return out
}

// Checked returns the members whose value a message is expected to
// carry unchanged, which is what decides whether the message check is
// emitted.
func (e ErrType) Checked() []Field {
	out := make([]Field, 0, len(e.Fields))
	for _, f := range e.Fields {
		if f.Verbatim && f.Sample.OK() {
			out = append(out, f)
		}
	}
	return out
}

// Neighbour is another package this one's declared errors must stay
// distinct from.
type Neighbour struct {
	// Path is the import path as written in the directive, and Name
	// the identifier the loaded package declares — which is what a
	// check name reads better as.
	Path, Name string

	Sentinels []Sentinel
}

// Tests is the emit value rendered into the output.
type Tests struct {
	sdk.BaseEmit

	// PackageName is the declaring package's identifier, which names
	// the check function.
	PackageName string

	// Prefix is what every message must begin with, empty when the
	// check is suppressed.
	Prefix string

	Sentinels  []Sentinel
	ErrTypes   []ErrType
	Neighbours []Neighbour

	checks *sdk.Slot
}

// Checks returns the slot rendered after this plugin's own checks.
func (t *Tests) Checks() *sdk.Slot {
	if t.checks == nil {
		t.checks = sdk.NewSlot(SlotChecks, "")
		t.checks.Owner = t
	}
	return t.checks
}

// Slot satisfies [sdk.SlotHost] so the backend's `slot` helper reaches
// the region by name. An unknown name yields an empty slot rather than
// nil, so a template asking for one this kind does not have renders
// nothing instead of failing.
func (t *Tests) Slot(name string) *sdk.Slot {
	if name == SlotChecks {
		return t.Checks()
	}
	return sdk.NewSlot(name, "")
}

// Kind returns [KindTests].
func (*Tests) Kind() sdk.Kind { return KindTests }

var (
	_ sdk.EmitNode = (*Tests)(nil)
	_ sdk.SlotHost = (*Tests)(nil)
)

// Generate queues one set of checks against every annotated package.
//
// A package with neither a declared error nor an error type is
// reported: the directive says its errors are a contract, and a file
// asserting nothing about an empty set would read as though they had
// been checked.
func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
	c := sdk.NewProvenance(Name)
	unread := map[string]bool{}
	scanned := map[string][]Sentinel{}
	for _, pkg := range ctx.Reader.Packages().Slice() {
		rules, lang, ok := p.SourceOf(pkg)
		if !ok {
			p.report(ctx, pkg, sdk.LanguageOf(pkg), unread, "are not read")
			continue
		}
		er, ok := rules.(sdk.ErrorRules)
		if !ok {
			p.report(ctx, pkg, lang, unread, "describe no error protocol")
			continue
		}
		scanned[pkg.Path] = sentinelsOf(er, pkg)
	}
	// Indexed for the whole run before anything is queued, because the
	// cross-package check needs a neighbour's set and the neighbour may
	// be annotated, unannotated, or not annotated yet — reading it from
	// one index keeps one answer to "what does that package declare".
	for _, pkg := range ctx.Reader.Packages().Slice() {
		if !pkg.HasPositiveDirective(DirectiveName) {
			continue
		}
		rules, _, ok := p.SourceOf(pkg)
		if !ok {
			continue
		}
		er, ok := rules.(sdk.ErrorRules)
		if !ok {
			continue
		}
		if err := p.generatePackage(ctx, c, pkg, er, scanned); err != nil {
			return err
		}
	}
	return nil
}

// generatePackage queues the checks for one annotated package.
func (p *Plugin) generatePackage(
	ctx *sdk.GeneratorContext, c *sdk.Provenance, pkg *sdk.Package,
	er sdk.ErrorRules, scanned map[string][]Sentinel,
) error {
	found := scanned[pkg.Path]
	types := errTypesOf(ctx, er, pkg)
	if len(found) == 0 && len(types) == 0 {
		ctx.Diag.Errorf(pkg.Pos(),
			"%s: package %q carries +gen:%s but declares no error value and no "+
				"type taking part in the error protocol",
			Name, pkg.Path, DirectiveName)
		return nil
	}
	anchor := anchorOf(pkg, found, types)
	prefix := p.prefixOf(pkg)
	relate(types, prefix, found)
	value := &Tests{
		BaseEmit:    sdk.EmitBase(c, anchor),
		PackageName: pkg.Name,
		Prefix:      prefix,
		Sentinels:   found,
		ErrTypes:    types,
		Neighbours:  neighboursOf(ctx, pkg, scanned),
	}
	// Identified by the package rather than by the anchor: the anchor
	// is whichever declaration the package happened to offer, and
	// naming it would move this value's identifier when an unrelated
	// type is renamed.
	if err := sdk.QueueEmitAs(
		ctx.Store.Emit(), c, FileSlot, anchor, pkg.Name, value,
	); err != nil {
		// Wrapped even though the queue names the plugin and the slot:
		// what it cannot name is which package the run was on, which is
		// the only part a reader needs to find the source line.
		return fmt.Errorf("%s: queue package %q: %w", Name, pkg.Path, err)
	}
	return nil
}

// sentinelsOf returns the package's declared error values, in
// declaration order.
//
// Found by the language's own naming convention rather than by type,
// because a value's declared type says nothing here: every one of them
// is the same interface, and what marks one as part of the contract is
// how it was named.
func sentinelsOf(er sdk.ErrorRules, pkg *sdk.Package) []Sentinel {
	var out []Sentinel
	for _, v := range pkg.Variables {
		if !er.IsSentinelName(v.Name) {
			continue
		}
		out = append(out, Sentinel{
			Name: v.Name,
			Ref:  sdk.NewExternal(v.Package, v.Name),
		})
	}
	return out
}

// errTypesOf lifts every exported declaration in the package that
// takes part in the error protocol, sorted by name.
//
// Sorted because the rendered file lists them and a run has to be
// byte-identical to its predecessor; the reader's order is stable but
// says nothing a reader of the output would predict.
func errTypesOf(
	ctx *sdk.GeneratorContext, er sdk.ErrorRules, pkg *sdk.Package,
) []ErrType {
	var out []ErrType
	for _, s := range pkg.Structs {
		info, ok := er.ErrorOf(s, ctx.Reader)
		if !ok {
			continue
		}
		reportUnresolved(ctx, s, info.Unresolved)
		out = append(out, ErrType{
			Name:      s.Name,
			Ref:       sdk.NewExternal(s.Package, s.Name),
			Addressed: info.Addressed,
			Cause:     info.Cause,
			Compares:  info.Compares,
			Unwraps:   info.Unwraps,
			Fields:    fieldsOf(info.Members),
			decl:      s,
		})
	}
	slices.SortFunc(out, func(a, b ErrType) int {
		switch {
		case a.Name < b.Name:
			return -1
		case a.Name > b.Name:
			return 1
		default:
			return 0
		}
	})
	return out
}

// relate gives each error type the package facts its own checks need.
//
// After the whole set is known rather than during the scan, because
// one of them is every *other* type in it — a type cannot be told
// about its neighbours before they have all been found.
func relate(types []ErrType, prefix string, found []Sentinel) {
	for i := range types {
		types[i].Prefix = prefix
		types[i].Seeds = found
		types[i].Peers = nil
		for _, other := range types {
			if other.Name == types[i].Name {
				continue
			}
			types[i].Peers = append(types[i].Peers, Peer{
				Name:      other.Name,
				Ref:       other.Ref,
				Addressed: other.Addressed,
			})
		}
	}
}

// fieldsOf lifts the projected members.
func fieldsOf(members []sdk.ErrorMember) []Field {
	out := make([]Field, 0, len(members))
	for _, m := range members {
		out = append(out, Field{Name: m.Name, Sample: m.Sample, Verbatim: m.Verbatim})
	}
	return out
}

// reportUnresolved raises a part of a declaration the run could not
// reach.
//
// The contract is smaller than the truth when this fires, so
// generating against it quietly asserts something the type may not
// promise — or omits something it does. The usual cause is a run
// narrower than the declaration, which the author can fix and the
// generator cannot.
func reportUnresolved(
	ctx *sdk.GeneratorContext, s *sdk.Struct, unresolved []string,
) {
	for _, written := range unresolved {
		ctx.Diag.Warnf(s.Pos(),
			"%s: %q folds in %q, which this run did not resolve, so its error "+
				"contract is checked against less than the type carries",
			Name, s.Name, written)
	}
}

// anchorOf returns the declaration the output file is composed from.
//
// Layout builds the filename from the origin's source basename, so the
// anchor decides where the checks land. The first declared error in
// source order, or failing that the first error type, puts them beside
// the declarations they are about.
//
// The error type comes from what the scan already resolved rather than
// from a second one. The two asked different questions once — one
// walked the folded-in contract, the other only the declarations — so
// a package whose only error type inherits its contract was found to
// have one and then anchored nowhere, and its checks were dropped
// without a diagnostic.
//
// One of the two is always available: the caller refuses a package
// declaring neither before it gets here.
func anchorOf(pkg *sdk.Package, found []Sentinel, types []ErrType) sdk.Node {
	if len(found) > 0 {
		for _, v := range pkg.Variables {
			if v.Name == found[0].Name {
				return v
			}
		}
	}
	return types[0].decl
}

// prefixOf resolves what every message must begin with, or empty when
// the check is suppressed.
//
// The last declaration wins, matching every other per-declaration key
// in this repository. [sdk.Node.Directive] is first-wins and answers a
// different question — whether the directive is there at all.
func (p *Plugin) prefixOf(pkg *sdk.Package) string {
	base := pkg.Name
	if p.opts.Prefix != "" {
		base = p.opts.Prefix
	}
	dir := sdk.Last(pkg.Directives(), DirectiveName)
	if dir == nil {
		return base + PrefixSeparator
	}
	raw, declared := dir.KV[PrefixKey]
	switch {
	case !declared:
		return base + PrefixSeparator
	case raw == PrefixOff || raw == "":
		// Suppressed rather than empty. Every string begins with the
		// empty string, so a check written against one passes for any
		// input and reads as though the contract had been examined.
		return ""
	default:
		return raw + PrefixSeparator
	}
}

// neighboursOf resolves every package named by a no-overlap directive.
//
// A neighbour declaring no errors is kept with an empty set rather
// than dropped: the rendered file lists it, so a directive pointing at
// a package that has none is visible as an empty check instead of as a
// missing one.
func neighboursOf(
	ctx *sdk.GeneratorContext, pkg *sdk.Package, scanned map[string][]Sentinel,
) []Neighbour {
	var out []Neighbour
	for _, dir := range pkg.Directives() {
		if dir.Name != sdk.DirectiveName(NoOverlapName) || len(dir.Args) == 0 {
			continue
		}
		path := dir.Args[0]
		if path == pkg.Path {
			ctx.Diag.Errorf(pkg.Pos(),
				"%s: package %q declares +gen:%s against itself",
				Name, pkg.Path, NoOverlapName)
			continue
		}
		out = append(out, Neighbour{
			Path:      path,
			Name:      neighbourName(ctx, pkg, path),
			Sentinels: scanned[path],
		})
	}
	return out
}

// neighbourName returns the identifier the named package declares.
//
// Read from the run rather than derived from the path. A package's
// identifier and its directory usually agree and occasionally do not,
// and deriving one is a rule about how a language writes import paths
// — which a core naming no language has no business applying. A path
// the run did not load is reported: the check against it would be
// vacuous, since a package this run cannot see declares nothing it can
// compare against.
func neighbourName(
	ctx *sdk.GeneratorContext, pkg *sdk.Package, path string,
) string {
	if found, ok := ctx.Reader.PackageAt(path); ok && found.Name != "" {
		return found.Name
	}
	ctx.Diag.Warnf(pkg.Pos(),
		"%s: %q names %q, which this run did not load, so the check against it "+
			"compares against nothing",
		Name, NoOverlapName, path)
	return path
}

// report warns once per language this plugin cannot read.
//
// An unmarked package is passed over quietly: the marker names the
// language a package was written in, so its absence means nothing
// claimed it — a fixture, a bridge, a synthesised graph. Warning about
// those would put a diagnostic on every unit test that builds a store
// by hand, which is where the real warning would then go unread.
func (p *Plugin) report(
	ctx *sdk.GeneratorContext, pkg *sdk.Package, lang string,
	seen map[string]bool, because string,
) {
	if lang == "" || seen[lang] {
		return
	}
	seen[lang] = true
	ctx.Diag.Warnf(pkg.Pos(),
		"%s: declarations written in %q %s, so no error-contract checks are "+
			"generated for them; this plugin reads: %v",
		Name, lang, because, p.Languages())
}
