// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"errors"
	"fmt"
	"path"
	"strings"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/writer"
)

// Spelling a type or a symbol: as a display string for a human, and
// as an [emit.Ref] for the backend.
//
// The two are not interchangeable and conflating them is the
// mistake this file exists to prevent. A display string names a
// type in a diagnostic; a reference names one in generated source
// and carries the import the rendered file has to register. Text
// cannot ask for an import, so a generator that builds a qualified
// name by concatenation emits a file naming a package it never
// imports.

// ErrBadSymbol is returned by [RefForQualified] for a value that is
// neither a bare identifier nor `<import/path>.<Symbol>`, and by
// [ResolveQualified] for one that is neither a bare identifier nor
// `<qualifier>.<Symbol>`.
var ErrBadSymbol = errors.New("golang: malformed symbol reference")

// ErrUnresolvedQualifier is returned by [ResolveQualified] for a
// qualifier the file's import block does not bind.
//
// Distinct from [ErrBadSymbol] because the two are different
// author-facing problems: a malformed value was typed wrong, while an
// unresolved qualifier was typed right against imports that are not
// there — usually a directive naming a package the file references
// only through the directive, which is an unused import and therefore
// unwritable.
var ErrUnresolvedQualifier = errors.New("golang: unresolved package qualifier")

// QualifierOf splits a source-level qualified identifier into its
// package qualifier and the symbol it selects. The qualifier is empty
// for a bare identifier.
//
// Split on the *first* dot, which is the opposite of what
// [RefForQualified] does and is the difference between the two
// notations. A qualifier in source is a single identifier and cannot
// contain a dot, so `gopkg.in/yaml.v3.Marshal` is not source Go and
// splitting it from the right would manufacture a qualifier that is
// not an identifier. A directive value, which is what RefForQualified
// reads, carries an import path instead — and a path may contain
// dots, so that one splits from the right.
//
// Purely a split: `.Foo` and `Foo.` return what they hold rather than
// an error, because a caller inspecting a name should not have to
// handle a failure to find a dot. [ResolveQualified] does the
// validating.
func QualifierOf(raw string) (qualifier, symbol string) {
	before, after, found := strings.Cut(raw, ".")
	if !found {
		return "", raw
	}
	return before, after
}

// ImportForQualifier returns the import f binds qualifier to, or nil
// when the file's import block binds no such name.
//
// Explicit aliases are matched before derived ones. Two imports can
// present the same local name — one aliased `pb`, another whose path
// ends in `pb` — which real Go rejects but a fixture or a partially
// loaded graph can hold; preferring the alias resolves it the way an
// author reading the file would, rather than by slice order.
//
// Blank and dot imports bind no qualifier and are skipped. A dot
// import does merge its namespace into the file's, so a bare
// identifier in a file carrying one may name a symbol from the dotted
// package rather than an in-package declaration — [ResolveQualified]
// resolves the bare form against the source package regardless, which
// is wrong for that case and right for every other. Dot imports are
// rare enough, and discouraged enough, that guessing between the two
// would cost more than it saves.
func ImportForQualifier(f *node.File, qualifier string) *node.Import {
	if f == nil || qualifier == "" {
		return nil
	}
	for _, imp := range f.Imports {
		if imp != nil && imp.Alias == qualifier && bindsAName(imp.Alias) {
			return imp
		}
	}
	for _, imp := range f.Imports {
		if imp != nil && imp.Alias == "" && imp.LocalName() == qualifier {
			return imp
		}
	}
	return nil
}

// bindsAName reports whether an import alias introduces a qualifier.
// Go's two special forms do not: `_` imports for the side effect and
// `.` merges the namespace.
//
// The two spellings come from [writer], whose own docblock says the
// rule outlives that package. Restating them here would be two
// statements of one rule with no test relating them — and this side
// reads them off a source graph while that side writes them into an
// import block, so a disagreement would surface as a qualifier that
// resolves on the way in and vanishes on the way out.
func bindsAName(alias string) bool {
	return alias != writer.BlankAlias && alias != writer.DotAlias
}

// ResolveQualified lifts a source-level type or symbol reference into
// the reference a rendered file can use, resolving its qualifier
// against f's import block.
//
// The step [RefForQualified] cannot take. That one reads everything
// before the last dot as an import path, which is right for a
// directive value an author wrote — an import written only to feed a
// directive is an unused import and does not compile, so the notation
// carries its own path. It is wrong for text read out of source,
// where `pb.Event` means whatever `pb` was bound to *in that file*
// and resolving it as the import path `pb` produces an ExternalRef
// the backend rejects at render, naming neither the value nor the
// function that mangled it.
//
//	pb.Event         -> whatever the file aliased pb to
//	context.Context  -> the imported path, qualifier derived
//	Tier             -> resolved against srcPkg
//	string           -> a builtin, needing no import
//
// A bare identifier never consults f: a predeclared name renders bare
// and anything else is taken to be declared in srcPkg, which is
// [RefFor]'s rule and stays consistent here.
func ResolveQualified(f *node.File, raw, srcPkg string) (emit.Ref, error) {
	if raw == "" {
		return nil, fmt.Errorf("%w: empty", ErrBadSymbol)
	}
	if strings.HasPrefix(raw, ".") || strings.HasSuffix(raw, ".") {
		return nil, fmt.Errorf("%w: %q", ErrBadSymbol, raw)
	}
	qualifier, symbol := QualifierOf(raw)
	if strings.Contains(symbol, ".") {
		// A selector chain, not a qualified identifier. Accepting it
		// would render `pkg.a.B` as a type name, which parses as a
		// field access and fails in the consumer's build.
		return nil, fmt.Errorf("%w: %q selects through more than one qualifier", ErrBadSymbol, raw)
	}
	if qualifier == "" {
		return RefFor(symbol, srcPkg), nil
	}
	imp := ImportForQualifier(f, qualifier)
	if imp == nil {
		return nil, fmt.Errorf("%w: %q in %s", ErrUnresolvedQualifier, qualifier, fileLabel(f))
	}
	return emit.External(imp.Path, symbol), nil
}

// FileOf returns the [node.File] within pkg that declared n, or nil
// when pkg records no file of that name.
//
// The step between what a caller holds and what [ResolveQualified]
// takes. A generator has a declaration; imports are scoped to the file
// that wrote them, and the only link between the two is the
// declaration's [position.Pos] — whose File is a path while
// [node.Package.FileByName] keys on a basename. Composing that at each
// call site is one `path.Base` away from a lookup that always misses,
// silently: every qualifier then fails to resolve and the generator
// reports the source as importing nothing.
//
// Nil for a positionless node, which is the honest answer rather than
// a guess: a synthetic declaration has no file and therefore no
// imports in scope.
func FileOf(pkg *node.Package, n node.Node) *node.File {
	if pkg == nil || n == nil {
		return nil
	}
	name := n.Pos().File
	if name == "" {
		return nil
	}
	return pkg.FileByName(path.Base(name))
}

// fileLabel names the file a resolution failed against, for the
// message. A nil file is reported as such rather than as an empty
// name: resolving against no file at all is a different caller
// mistake from resolving against one whose imports fall short.
func fileLabel(f *node.File) string {
	if f == nil {
		return "no file"
	}
	if f.Name != "" {
		return f.Name
	}
	return "an unnamed file"
}

// QName returns the fully-qualified spelling of a type reference —
// `example.com/store.User`, or the bare name for a builtin or an
// in-package type.
//
// The form the shape vocabulary records and consumers read back, so
// a stamp written by one plugin and read by another agrees. Empty
// for nil, so a caller need not guard before interpolating.
func QName(t *node.TypeRef) string {
	if t == nil {
		return ""
	}
	if t.Package == "" {
		return t.Name
	}
	return t.Package + "." + t.Name
}

// Display returns the spelling a source author would recognise —
// `store.User` rather than `example.com/store.User`.
//
// For diagnostics, never for generated source. The last path
// segment is what appears in the author's own file, so a message
// using it names something they can search for; the full path names
// something they wrote once in an import block.
func Display(t *node.TypeRef) string {
	if t == nil {
		return ""
	}
	if t.Package == "" {
		return t.Name
	}
	return path.Base(t.Package) + "." + t.Name
}

// MethodQName composes the store's canonical method key,
// `<ownerQName>.<method>`.
//
// The form the shape resolver rewrites sibling references into, and
// the key a consumer looks a method up by. Composed here so the
// spelling lives once rather than in each package that has to match
// it.
func MethodQName(ownerQName, method string) string {
	if ownerQName == "" {
		return method
	}
	return ownerQName + "." + method
}

// LocalName returns the trailing identifier of a possibly-qualified
// name.
//
// What a generator needs to compose a call on a subject it already
// holds: the resolver rewrites a directive's sibling reference into
// the qualified form that makes it unambiguous across packages, and
// that form is exactly what a call expression cannot use.
//
// A name carrying no qualifier is returned unchanged rather than
// treated as an error — an unresolved reference has already been
// diagnosed by the run that produced it, and failing twice for one
// cause helps nobody.
func LocalName(qualified string) string {
	if i := strings.LastIndex(qualified, "."); i >= 0 {
		return qualified[i+1:]
	}
	return qualified
}

// RefFor lifts a type name written by a source author into the
// reference a rendered file can use.
//
// A predeclared name renders bare; anything else is taken to be
// declared in srcPkg and qualified against it. That rule is what
// makes a directive argument usable from a generated file routed
// somewhere else — the backend elides the qualifier where the two
// share a package and registers the import where they do not.
//
// A name that carries its own qualifier is not resolvable here: the
// generator would have to invent the import path. Use
// [RefForQualified] for the notation that supplies one.
func RefFor(name, srcPkg string) emit.Ref {
	if IsPredeclared(name) || srcPkg == "" {
		return emit.Builtin(name)
	}
	return emit.External(srcPkg, name)
}

// RefForQualified lifts a directive value naming a symbol into a
// reference, accepting both notations authors write.
//
//	Validate                     -> resolved against srcPkg
//	example.com/x.Validate       -> a full import path, needing no import
//	gopkg.in/yaml.v3.Marshal     -> also a full path; the dots before
//	                                the last one belong to the path
//
// Split on the last dot, because a path may contain them. The
// second notation exists because an import written only to feed a
// directive is an unused import, which does not compile — without
// it a value can only name a package the file already uses for real
// code.
//
// A leading or trailing dot returns [ErrBadSymbol] rather than a
// reference to the empty string, which would render as a bare `.`
// the consumer's compiler reports against generated code.
func RefForQualified(raw, srcPkg string) (emit.Ref, error) {
	if raw == "" {
		return nil, fmt.Errorf("%w: empty", ErrBadSymbol)
	}
	i := strings.LastIndex(raw, ".")
	if i < 0 {
		return RefFor(raw, srcPkg), nil
	}
	if i == 0 || i == len(raw)-1 {
		return nil, fmt.Errorf("%w: %q", ErrBadSymbol, raw)
	}
	return emit.External(raw[:i], raw[i+1:]), nil
}

// RefsOf lifts a list of source types into their emit form.
//
// Returns nil for an empty list so a caller appending the result to
// a signature emits nothing rather than an empty bracket list.
func RefsOf(types []*node.TypeRef) []emit.Ref {
	if len(types) == 0 {
		return nil
	}
	out := make([]emit.Ref, len(types))
	for i, t := range types {
		out[i] = FromNode(t)
	}
	return out
}

// ParamRefs lifts a parameter list's declared types into emit form,
// dropping the names.
//
// What a function type takes: `func(context.Context, string) error`
// names no parameters, so the identifiers a body would bind are not
// part of the type.
func ParamRefs(params []*node.Param) []emit.Ref {
	if len(params) == 0 {
		return nil
	}
	out := make([]emit.Ref, len(params))
	for i, p := range params {
		if p != nil {
			out[i] = FromNode(p.Type)
		}
	}
	return out
}

// ReturnRefs lifts a return list's declared types into emit form.
func ReturnRefs(returns []*node.Return) []emit.Ref {
	if len(returns) == 0 {
		return nil
	}
	out := make([]emit.Ref, len(returns))
	for i, r := range returns {
		if r != nil {
			out[i] = FromNode(r.Type)
		}
	}
	return out
}

// PkgPathOf returns the import path of the package owning n, or
// empty when the node kind carries none.
//
// [node.Node] declares no package accessor — a field and a package
// both satisfy the interface and only one has a path — so a caller
// wanting this has to reach for the concrete kind. The type switch
// is the whole implementation, and it is here rather than in each
// caller because getting it wrong is silent: an assertion against
// an accessor no kind implements compiles, always misses, and
// leaves every reference unqualified.
//
// Empty is the honest answer for a kind that has no package. The
// backend's same-package elision renders an unqualified reference,
// which is correct for a declaration landing beside its source.
func PkgPathOf(n node.Node) string {
	switch v := n.(type) {
	case *node.Package:
		return v.Path
	case *node.Struct:
		return v.Package
	case *node.Interface:
		return v.Package
	case *node.Function:
		return v.Package
	case *node.Alias:
		return v.Package
	case *node.Enum:
		return v.Package
	case *node.Variable:
		return v.Package
	case *node.Constant:
		return v.Package
	case *node.TypeRef:
		return v.Package
	case *node.Method:
		// A method carries no package of its own; its owner does, and
		// walking up is what makes a method usable as an origin.
		return PkgPathOf(v.Owner)
	case *node.Field:
		return PkgPathOf(v.Owner)
	default:
		return ""
	}
}

// SubjectRef names a declaration from wherever generated output
// lands.
//
// Qualified against the origin's own package when it has one, bare
// otherwise: [emit.External] rejects an empty path, so the two
// cases cannot share a construction and every caller that wanted
// this wrote the branch itself.
func SubjectRef(origin node.Node, name string) emit.Ref {
	return RefFor(name, PkgPathOf(origin))
}

// ResolveValue splits a value written in a directive into the package
// it names and the symbol within it, reporting an empty package for a
// plain literal.
//
// The step above [RefForQualified] and [ResolveQualified], which
// answer for one notation each and cannot tell which they were
// handed. Both notations are things authors write, for reasons that
// do not overlap:
//
//	time.Second                     -> resolved against f's import block
//	example.com/seed.DefaultRegion  -> a full import path, needing no import
//	gopkg.in/yaml.v3.Marshal        -> also a path; the earlier dots are its
//	"localhost"                     -> a literal, passed through untouched
//
// They split on opposite dots and that is the whole distinction. An
// import path may hold dots, so the full-path form splits from the
// right; a Go qualifier is one identifier and cannot hold one, so the
// source form splits from the left. Reading source text with the
// right-hand rule manufactures a qualifier that is not an identifier.
//
// Which rule applies is decided by the file rather than by the text. A
// slash before the last dot settles it early — no qualifier holds one
// — but its absence settles nothing: a single-segment import path is
// its own qualifier spelling, so `time.Duration` is both notations at
// once. The import block is what tells them apart, because a qualifier
// the file never bound is not a qualifier. So the source form is tried
// first and the path form is what an unbound qualifier falls back to.
//
// The cost is that a name which is neither bound in the file nor a real
// package resolves to an import the consumer's compiler then rejects,
// rather than to a diagnostic here. Nothing available at this point
// distinguishes a typo from a package this run never loaded, and the
// alternative was refusing every stdlib package outright: `time` has no
// longer spelling, and an import added to bind the qualifier would be
// unused and would not compile.
//
// The second notation exists because an import written only to feed a
// directive is an unused import, which does not compile. Without it a
// value can only name a package the file already uses for real code.
//
// A nil file resolves no qualifier at all, which is what a
// positionless declaration gets: [ErrUnresolvedQualifier] rather than
// a guess against some other file's imports.
func ResolveValue(f *node.File, value string) (pkg, symbol string, err error) {
	if malformed := IsWellFormedLiteral(value); malformed != nil {
		return "", "", fmt.Errorf("%q: %w", value, malformed)
	}
	if !namesSymbol(value) {
		return "", value, nil
	}
	ref, err := resolveValueRef(f, value)
	if err != nil {
		return "", "", err
	}
	ext, ok := ref.(*emit.ExternalRef)
	if !ok {
		// A bare identifier — a constant the declaring package owns. It
		// renders as itself and registers no import, which is what an
		// empty source package asks [RefFor] for: whichever file later
		// renders the value is not known here.
		return "", value, nil
	}
	return ext.Package, ext.Name, nil
}

// resolveValueRef hands value to whichever rule its notation calls
// for, and improves the diagnostic for the case authors hit most.
func resolveValueRef(f *node.File, value string) (emit.Ref, error) {
	if dot := strings.LastIndex(value, "."); dot > 0 && strings.Contains(value[:dot], "/") {
		ref, err := RefForQualified(value, "")
		if err != nil {
			return nil, fmt.Errorf("%q: %w", value, err)
		}
		return ref, nil
	}
	ref, err := ResolveQualified(f, value, "")
	switch {
	case errors.Is(err, ErrUnresolvedQualifier):
		// Read as a path instead. The slash above is a fast path, not
		// the decision: a single-segment import path is its own
		// qualifier spelling, so `time.Duration` is both forms at once
		// and no amount of looking at the text tells them apart. The
		// import block does — a qualifier the file never bound is not a
		// qualifier — so the answer is taken from the evidence rather
		// than guessed ahead of it.
		//
		// Advising the full-path form here, as this once did, asked an
		// author to write what they had already written: `time` has no
		// longer spelling, and an import added to satisfy the qualifier
		// would be unused and would not compile.
		//
		// A name that is neither bound in the file nor a real package
		// reaches the consumer's compiler as a missing import rather
		// than a diagnostic here. Nothing available at this point can
		// tell a typo from a package this run never loaded.
		return RefForQualified(value, "")
	case err != nil:
		return nil, fmt.Errorf("%q: %w", value, err)
	}
	return ref, nil
}

// namesSymbol reports whether value reads as a symbol rather than as
// a literal. A quoted string or a number can hold a dot without
// naming anything.
//
// A leading dot is a number too: `.5` is a legal Go float literal and
// Go's own scanner reads it as one. Reading it as a qualifier splits
// it into an empty qualifier and the symbol `5`, which matches every
// un-aliased import — so the first import in the file wins and the
// generated value says `http.5`.
func namesSymbol(value string) bool {
	if value == "" || value[0] == '"' || value[0] == '`' {
		return false
	}
	c := value[0]
	return (c < '0' || c > '9') && c != '-' && c != '+' && c != '.'
}
