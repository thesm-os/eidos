// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package gofixture

import (
	"fmt"
	"go/format"
	"maps"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// goSourceBasename is the file the projection writes inside the
// directory [goPrinter.sourcePath] resolves.
//
// One file rather than one per declaration: a [Builder] holds exactly
// one package by contract, and nothing about a support package's
// compilation depends on which of its files a declaration sits in.
// The name matches the `source.go` several fixtures already
// hand-write, so adopting the projection is a substitution rather
// than a rename.
const goSourceBasename = "source.go"

// projectedBody is the body every projected function and method
// carries.
//
// The projection reproduces shape, not behaviour, because shape is
// all the generated code binds to: a double names the interface it
// doubles and a repository names the struct it stores, and neither
// calls into the support package at run time. A panic is what makes
// that boundary loud — a generated test that did call one fails
// naming this string rather than silently observing a zero value.
const projectedBody = `panic("gofixture: GoSource projects shape, not behaviour")`

// sourceHeader identifies the projected file wherever it surfaces —
// a compiler error inside a throwaway module, a dumped listing.
//
// Deliberately not the canonical `Code generated … DO NOT EDIT.`
// marker: this file stands in for code a consumer hand-wrote, and
// stamping it as generated would make it lie to every assertion that
// keys off that marker.
const sourceHeader = "// Projected from a gofixture builder by Builder.GoSource.\n" +
	"// It stands in for the package a consumer hand-writes; change the\n" +
	"// fixture, not this.\n\n"

// UnspellableError is the panic value [Builder.GoSource] raises for a
// node graph carrying something it cannot render as Go.
//
// A returned error would be wrong twice over. GoSource is called in
// the expression position a [golangtest.File] wants, so an error
// return would be dropped or checked by exactly the discipline this
// projection exists to remove; and the condition is fixture misuse,
// which [Builder.Build] already answers with a panic. What matters is
// that the failure names the construct — a projection that quietly
// omitted a field would reintroduce the drift, one file further down.
type UnspellableError struct {
	// Where identifies the declaration being rendered.
	Where string

	// Construct names the shape that has no Go spelling here.
	Construct string
}

// Error implements the error interface.
func (e *UnspellableError) Error() string {
	return fmt.Sprintf("gofixture: GoSource cannot spell %s in %s; "+
		"hand-write that package instead of projecting it, rather than "+
		"shipping a support file the fixture no longer describes",
		e.Construct, e.Where)
}

// GoSource projects the accumulating package to the Go source a
// consumer would have typed, returning the file's module-relative
// path and its contents.
//
// # The drift this kills
//
// Every render fixture needs the hand-written package its generated
// output references — the `type User struct{…}` a repository stores,
// the interface a double doubles. Written by hand, that package and
// the node graph that drove the run are bound only by review: rename
// a field in the fixture and the support source goes stale, and the
// failure arrives as a compile error inside a throwaway module,
// naming code the author never wrote, for the wrong reason. Projected
// from the same builder, the pair cannot disagree.
//
// # Using it
//
// The two return values are exactly [golangtest.GoFile]'s parameters,
// so no adapter stands between them:
//
//	gen := golangtest.Rendered(t, run).
//	    WithSource(golangtest.GoFile(fixture().GoSource()))
//
// The projection deliberately returns plain strings rather than a
// golangtest type. This package builds the input to a run and
// golangtest asserts on its output; coupling either to the other
// would make a plain store fixture drag in the toolchain harness, or
// the output assertions drag in the fixture builder. Both stay usable
// alone, and [golangtest.GoFile] already exists for the adaptation.
//
// # What it emits, and what it refuses
//
// Every declaration the builder accepts, with the imports its type
// expressions need: structs with fields, tags and embeds, interfaces
// with method sets, named types and aliases, enums, constants,
// variables and functions, generic and not. Function and method
// bodies panic; see [projectedBody].
//
// Two things are excluded by design rather than by oversight, because
// neither is part of the surface generated code binds to. Directives
// are dropped: a support file is never re-parsed by a pipeline, and a
// fixture directive's keyword arguments live in a map whose iteration
// order would make the projection non-deterministic. Imports recorded
// via [Builder.Import] are dropped too — an import nothing references
// is a compile error, and those entries exist for tests inspecting a
// frontend's import view.
//
// Anything else the graph carries that has no Go spelling stops the
// test with an [UnspellableError] naming it. That is the whole
// bargain: a support package missing a field is precisely the drift
// this projection exists to kill, so it refuses rather than emits.
func (b *Builder) GoSource() (path, src string) {
	p := &goPrinter{
		pkg:     b.pkg,
		imports: map[string]string{},
		taken:   map[string]bool{},
		where:   "the fixture package",
	}
	return p.file()
}

// goPrinter renders one [node.Package] as Go, accumulating the
// imports its type expressions turn out to need.
//
// Imports are collected during the walk rather than in a pass before
// it, so the two can never disagree about what the file references —
// the recurring failure mode of a generator that computes its import
// set separately from its body.
type goPrinter struct {
	pkg *node.Package

	// imports maps an import path to the identifier this file refers
	// to it by, and taken is that identifier set — two paths whose
	// last segment collides would otherwise both render as the same
	// qualifier and silently name each other's types.
	imports map[string]string
	taken   map[string]bool

	// where names the declaration being rendered, for a failure that
	// has to be actionable from the message alone.
	where string

	// declared indexes the imports the fixture recorded via
	// [Builder.Import] / [Builder.ImportAs], keyed by the identifier a
	// qualifier would spell them as. Built on first use; nil until a
	// declaration carries verbatim text worth scanning.
	declared map[string]string
}

// file assembles the complete source file.
//
// The body is rendered before the import block is written because
// rendering is what discovers the imports; the result then goes
// through [format.Source], which doubles as the projection's own
// syntax check — source this printer cannot parse never reaches a
// caller as a mysterious compile error two layers down.
func (p *goPrinter) file() (path, src string) {
	if p.pkg.Name == "" {
		p.fail("a package with no name")
	}
	body := p.decls()

	var b strings.Builder
	b.WriteString(sourceHeader)
	b.WriteString("package " + p.pkg.Name + "\n")
	b.WriteString(p.importBlock())
	b.WriteString(body)

	out, err := format.Source([]byte(b.String()))
	if err != nil {
		p.fail(fmt.Sprintf("source it could not parse back (%v):\n%s", err, b.String()))
	}
	return p.sourcePath(), string(out)
}

// sourcePath places the projected file in the directory the fixture's
// own declarations report living in.
//
// Derived rather than assumed, and for the same reason the projection
// exists: the support file only compiles alongside the generated
// output if the two share a directory, and where the output lands is
// decided by the synthetic file each declaration carries — which a
// fixture is free to repoint with a sub-builder's Pos. Hard-coding
// `<pkg>/` would put the support file beside output that had been
// routed to the module root, and the failure would arrive as
// undefined identifiers rather than as a misplaced file.
//
// An empty package falls back to the package name, which is what
// [declFile] would have stamped had anything been declared.
func (p *goPrinter) sourcePath() string {
	dirs := map[string]struct{}{}
	collectDirs(dirs, p.pkg.Aliases)
	collectDirs(dirs, p.pkg.Enums)
	collectDirs(dirs, p.pkg.Structs)
	collectDirs(dirs, p.pkg.Interfaces)
	collectDirs(dirs, p.pkg.Constants)
	collectDirs(dirs, p.pkg.Variables)
	collectDirs(dirs, p.pkg.Functions)

	switch len(dirs) {
	case 0:
		return p.pkg.Name + "/" + goSourceBasename
	case 1:
		for dir := range dirs {
			if dir == "" {
				return goSourceBasename
			}
			return dir + "/" + goSourceBasename
		}
	}
	return p.fail("declarations spread across " + strconv.Itoa(len(dirs)) +
		" directories, which is more than one package and more than one file")
}

// collectDirs records the directory each declaration's synthetic
// source file names, treating the module root as the empty string.
func collectDirs[T node.Node](dirs map[string]struct{}, decls []T) {
	for _, decl := range decls {
		dir := path.Dir(decl.Pos().File)
		if dir == "." {
			dir = ""
		}
		dirs[dir] = struct{}{}
	}
}

// decls renders every declaration, types first.
//
// Order is fixed and does not follow the package's field order: a
// reader checking a support file against generated output looks for
// the type the output names, and Go's package scope makes the
// ordering free of any compilation consequence. Methods follow their
// owner immediately, as an author would write them.
func (p *goPrinter) decls() string {
	var b strings.Builder
	for _, a := range p.pkg.Aliases {
		p.printAlias(&b, a)
	}
	for _, e := range p.pkg.Enums {
		p.printEnum(&b, e)
	}
	for _, s := range p.pkg.Structs {
		p.printStruct(&b, s)
	}
	for _, i := range p.pkg.Interfaces {
		p.printInterface(&b, i)
	}
	for _, c := range p.pkg.Constants {
		p.printConstant(&b, c)
	}
	for _, v := range p.pkg.Variables {
		p.printVariable(&b, v)
	}
	for _, f := range p.pkg.Functions {
		p.printFunction(&b, f)
	}
	return b.String()
}

// importBlock renders the imports the walk registered, sorted by
// path so two runs of the same fixture produce identical bytes.
//
// A single import takes the unparenthesised form an author writes,
// because the file this stands in for is one an author wrote.
// literalPattern matches the spans of Go source that carry text
// rather than code — string, raw-string and rune literals, and both
// comment forms — so markTextRefs can blank them before looking for
// qualifiers.
//
// A path inside a message (`errors.New("cfg: a.b failed")`) or a
// package named in a note (`// see errors.New`) is prose, not a
// selector. Matching either activates an import on the strength of
// text, and when that import is one the fixture declared but does not
// otherwise use, the projection emits an unused import — the compile
// error the pruning exists to prevent, reintroduced by the fix for it.
var literalPattern = regexp.MustCompile(
	"`[^`]*`" + `|"(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|//[^\n]*|/\*(?s:.*?)\*/`,
)

// qualifierPattern matches an identifier that begins a selector chain:
// one not preceded by a dot or a word character. The leading-dot
// exclusion is what keeps `cfg.Default.Timeout` from treating
// `Default` as a package — only `cfg` starts the chain, and only it
// can name an import.
var qualifierPattern = regexp.MustCompile(`(^|[^\w.])([A-Za-z_]\w*)\s*\.`)

// markTextRefs records every declared import whose qualifier appears
// in expr, so an initialiser keeps the import it needs.
//
// [Builder.Import] entries are otherwise dropped — see [Builder.GoSource]
// — and the import set is accumulated from *type* expressions as they
// render. An initialiser is opaque Go text, so it is a reference the
// type walk structurally cannot see: a fixture declaring
// `errors` and initialising a sentinel with `errors.New(...)`
// projected to source that named `errors` and imported nothing.
//
// Only imports the fixture already declared can be activated. A
// qualifier matching none is left alone rather than refused, because
// at package level `var A = B.Field` legitimately names another
// declaration in the same package, and text alone cannot separate
// that from a genuinely missing import. The unmatched case therefore
// behaves exactly as it does today, and a truly absent import stays
// the compile error it already was.
func (p *goPrinter) markTextRefs(expr string) {
	if expr == "" || len(p.pkg.Imports) == 0 {
		return
	}
	if p.declared == nil {
		p.declared = make(map[string]string, len(p.pkg.Imports))
		for _, imp := range p.pkg.Imports {
			if imp == nil || imp.Path == "" {
				continue
			}
			name := imp.Alias
			if name == "" {
				name = golang.PackageName(imp.Path)
			}
			if name != "" {
				p.declared[name] = imp.Path
			}
		}
	}
	scanned := literalPattern.ReplaceAllString(expr, `""`)
	for _, m := range qualifierPattern.FindAllStringSubmatch(scanned, -1) {
		if path, ok := p.declared[m[2]]; ok {
			p.qualifier(path)
		}
	}
}

func (p *goPrinter) importBlock() string {
	paths := slices.Sorted(maps.Keys(p.imports))
	switch len(paths) {
	case 0:
		return ""
	case 1:
		return "\nimport " + p.importSpec(paths[0]) + "\n"
	}
	var b strings.Builder
	b.WriteString("\nimport (\n")
	for _, path := range paths {
		b.WriteString("\t" + p.importSpec(path) + "\n")
	}
	b.WriteString(")\n")
	return b.String()
}

// importSpec renders one import, naming it only when the qualifier
// differs from what the path already implies.
func (p *goPrinter) importSpec(path string) string {
	if qualifier := p.imports[path]; qualifier != golang.PackageName(path) {
		return qualifier + " " + strconv.Quote(path)
	}
	return strconv.Quote(path)
}

// qualifier returns the identifier this file names path by,
// registering the import on first use and aliasing it when the
// obvious name is already spoken for.
//
// The alias matters more than its rarity suggests: two paths ending
// in the same segment — a `v1` and a `v2` of one API, two `models`
// packages — would otherwise both render as one qualifier, and the
// support file would compile while naming the wrong types.
func (p *goPrinter) qualifier(path string) string {
	if q, ok := p.imports[path]; ok {
		return q
	}
	base := golang.PackageName(path)
	if base == "" {
		return p.fail("an import path with no package name (" + strconv.Quote(path) + ")")
	}
	q := base
	for n := 2; p.taken[q]; n++ {
		q = base + strconv.Itoa(n)
	}
	p.imports[path], p.taken[q] = q, true
	return q
}

// printAlias renders a type alias or a defined type.
func (p *goPrinter) printAlias(b *strings.Builder, a *node.Alias) {
	defer p.enter("type " + a.Name)()
	if a.Name == "" {
		p.fail("a type declaration with no name")
	}
	if a.Target == nil {
		p.fail("a type declaration with no target type")
	}
	if a.IsAlias && len(a.Methods) > 0 {
		p.fail("methods on a true alias (`type X = Y`), which Go declares on Y")
	}
	assign := " "
	if a.IsAlias {
		assign = " = "
	}
	openDecl(b, a.DocLines)
	fmt.Fprintf(b, "type %s%s%s%s\n", a.Name, p.typeParamDecl(a.TypeParams),
		assign, p.typeExpr(a.Target, maxTypeDepth))
	for _, m := range a.Methods {
		p.printMethod(b, a.Name, a.TypeParams, m)
	}
}

// printEnum renders an enum as the named type and const block a Go
// author writes it as.
func (p *goPrinter) printEnum(b *strings.Builder, e *node.Enum) {
	defer p.enter("enum " + e.Name)()
	if e.Name == "" {
		p.fail("an enum with no name")
	}
	if e.Underlying == nil {
		p.fail("an enum with no underlying type, which Go has no declaration for")
	}
	openDecl(b, e.DocLines)
	fmt.Fprintf(b, "type %s %s\n", e.Name, p.typeExpr(e.Underlying, maxTypeDepth))
	p.printVariants(b, e)
	for _, m := range e.Methods {
		p.printMethod(b, e.Name, nil, m)
	}
}

// printVariants renders an enum's const block.
//
// A variant with no value is only spellable as part of an unbroken
// iota run from the first: a bare name after an explicitly valued
// sibling repeats that sibling's expression, which is legal Go and
// almost never what the fixture meant. Refused rather than guessed.
func (p *goPrinter) printVariants(b *strings.Builder, e *node.Enum) {
	if len(e.Variants) == 0 {
		return
	}
	b.WriteString("\nconst (\n")
	counting := false
	for i, v := range e.Variants {
		switch {
		case v.Name == "":
			p.fail("an enum variant with no name")
		case v.Value != "":
			fmt.Fprintf(b, "\t%s %s = %s\n", v.Name, e.Name, v.Value)
			counting = false
		case i == 0:
			fmt.Fprintf(b, "\t%s %s = iota\n", v.Name, e.Name)
			counting = true
		case counting:
			fmt.Fprintf(b, "\t%s\n", v.Name)
		default:
			p.fail("variant " + v.Name + " with no value after a valued sibling, " +
				"which Go can only spell by repeating that sibling's expression")
		}
	}
	b.WriteString(")\n")
}

// printStruct renders a struct and the methods declared on it.
func (p *goPrinter) printStruct(b *strings.Builder, s *node.Struct) {
	defer p.enter("struct " + s.Name)()
	if s.Name == "" {
		p.fail("a struct with no name")
	}
	openDecl(b, s.DocLines)
	fmt.Fprintf(b, "type %s%s struct {", s.Name, p.typeParamDecl(s.TypeParams))
	if len(s.Embeds) == 0 && len(s.Fields) == 0 {
		// `struct{}` rather than an empty pair of braces on two lines:
		// gofmt keeps whichever it is handed, and the collapsed form is
		// the one an author writes.
		b.WriteString("}\n")
	} else {
		b.WriteString("\n")
		for _, e := range s.Embeds {
			fmt.Fprintf(b, "\t%s\n", p.typeExpr(e.Type, maxTypeDepth))
		}
		for _, f := range s.Fields {
			p.printField(b, f)
		}
		b.WriteString("}\n")
	}
	for _, m := range s.Methods {
		p.printMethod(b, s.Name, s.TypeParams, m)
	}
}

// printField renders one named struct field with its tag.
//
// Unlike a declaration it takes no separating blank line, documented
// or not: a struct body reads as one unit, and the blank line every
// other opener writes would split the field list wherever a fixture
// happened to attach a comment.
func (p *goPrinter) printField(b *strings.Builder, f *node.Field) {
	defer p.enter("field " + f.Name + " of " + p.where)()
	if f.Name == "" {
		p.fail("a struct field with no name — an embed belongs in Embeds")
	}
	writeDocs(b, f.DocLines, "\t")
	fmt.Fprintf(b, "\t%s %s", f.Name, p.typeExpr(f.Type, maxTypeDepth))
	if tag := p.fieldTag(f); tag != "" {
		fmt.Fprintf(b, " %s", tag)
	}
	b.WriteString("\n")
}

// printInterface renders an interface's embeds and method set.
func (p *goPrinter) printInterface(b *strings.Builder, i *node.Interface) {
	defer p.enter("interface " + i.Name)()
	if i.Name == "" {
		p.fail("an interface with no name")
	}
	openDecl(b, i.DocLines)
	fmt.Fprintf(b, "type %s%s interface {", i.Name, p.typeParamDecl(i.TypeParams))
	if len(i.Embeds) == 0 && len(i.Methods) == 0 {
		b.WriteString("}\n")
		return
	}
	b.WriteString("\n")
	for _, e := range i.Embeds {
		fmt.Fprintf(b, "\t%s\n", p.typeExpr(e.Type, maxTypeDepth))
	}
	for _, m := range i.Methods {
		fmt.Fprintf(b, "\t%s\n", p.methodSpec(m))
	}
	b.WriteString("}\n")
}

// printConstant renders a standalone constant.
func (p *goPrinter) printConstant(b *strings.Builder, c *node.Constant) {
	defer p.enter("constant " + c.Name)()
	if c.Name == "" {
		p.fail("a constant with no name")
	}
	if c.Value == "" {
		p.fail("a constant with no value, which Go has no declaration for")
	}
	openDecl(b, c.DocLines)
	// Value is verbatim Go text on the same footing as a variable's
	// initialiser: `const Timeout = time.Second` references an import
	// no type expression mentions.
	p.markTextRefs(c.Value)
	if c.Type == nil {
		fmt.Fprintf(b, "const %s = %s\n", c.Name, c.Value)
		return
	}
	fmt.Fprintf(b, "const %s %s = %s\n", c.Name,
		p.typeExpr(c.Type, maxTypeDepth), c.Value)
}

// printVariable renders a package-level variable in whichever of
// Go's three forms the node populates.
func (p *goPrinter) printVariable(b *strings.Builder, v *node.Variable) {
	defer p.enter("variable " + v.Name)()
	if v.Name == "" {
		p.fail("a variable with no name")
	}
	if v.Type == nil && v.InitExpr == "" {
		p.fail("a variable with neither a type nor an initialiser, which Go " +
			"has nothing to infer from")
	}
	openDecl(b, v.DocLines)
	b.WriteString("var " + v.Name)
	if v.Type != nil {
		b.WriteString(" " + p.typeExpr(v.Type, maxTypeDepth))
	}
	if v.InitExpr != "" {
		p.markTextRefs(v.InitExpr)
		b.WriteString(" = " + v.InitExpr)
	}
	b.WriteString("\n")
}

// printFunction renders a standalone function.
func (p *goPrinter) printFunction(b *strings.Builder, f *node.Function) {
	defer p.enter("function " + f.Name)()
	if f.Name == "" {
		p.fail("a function with no name")
	}
	openDecl(b, f.DocLines)
	fmt.Fprintf(b, "func %s%s%s {\n\t%s\n}\n", f.Name,
		p.typeParamDecl(f.TypeParams), p.signature(f.Params, f.Returns), projectedBody)
}

// printMethod renders a method on a named type.
func (p *goPrinter) printMethod(b *strings.Builder, owner string, params []*node.TypeParam, m *node.Method) {
	defer p.enter("method " + m.Name + " on " + owner)()
	if m.Name == "" {
		p.fail("a method with no name")
	}
	if len(m.TypeParams) > 0 {
		p.fail("a type parameter on a method, which Go allows only on the receiver type")
	}
	openDecl(b, m.DocLines)
	fmt.Fprintf(b, "func (%s) %s%s {\n\t%s\n}\n", p.receiver(owner, params, m),
		m.Name, p.signature(m.Params, m.Returns), projectedBody)
}

// receiver renders a method's receiver from its owner rather than
// from the reference the node carries.
//
// The node's receiver is a bare `*Pkg.Name` — the shape the fixture
// seeds and a frontend records — which on a generic type is invalid
// Go: `func (l *List) Get()` does not compile where `List` takes a
// parameter. Composing from the owner is the only way the type
// arguments arrive. A reference naming some other type is refused,
// so the composition can never quietly discard what a test pinned.
func (p *goPrinter) receiver(owner string, params []*node.TypeParam, m *node.Method) string {
	base, pointer := m.Receiver, true
	if base != nil {
		if base.IsPointer() {
			base = base.Elem
		} else {
			pointer = false
		}
		switch {
		case base == nil || base.Name == "":
			p.fail("a method receiver with no named type")
		case base.Name != owner:
			p.fail("a method receiver naming " + base.Name + " rather than " + owner)
		}
	}
	star := ""
	if pointer {
		star = "*"
	}
	return strings.TrimSpace(m.ReceiverName + " " + star + owner + p.typeParamUse(params))
}

// methodSpec renders an interface method — a name and a signature,
// with no receiver and no body.
func (p *goPrinter) methodSpec(m *node.Method) string {
	defer p.enter("method " + m.Name + " of " + p.where)()
	if m.Name == "" {
		return p.fail("an interface method with no name")
	}
	if len(m.TypeParams) > 0 {
		return p.fail("a type parameter on an interface method, which Go does not allow")
	}
	return m.Name + p.signature(m.Params, m.Returns)
}

// signature renders a parameter list and its returns.
func (p *goPrinter) signature(params []*node.Param, returns []*node.Return) string {
	return "(" + p.paramList(params) + ")" + p.returnList(returns)
}

// paramList renders a signature's parameters.
//
// Go rejects a list mixing named and unnamed parameters, and a
// fixture is free to build one — [MethodBuilder.Param] takes the
// empty name as the anonymous form. Any name at all therefore
// promotes the whole list, with the anonymous slots spelled `_`:
// that preserves what the fixture said (this slot has no name) in
// the only syntax Go has for saying it.
func (p *goPrinter) paramList(params []*node.Param) string {
	named := slices.ContainsFunc(params, func(prm *node.Param) bool { return prm.Name != "" })
	parts := make([]string, 0, len(params))
	for i, prm := range params {
		expr := p.typeExpr(prm.Type, maxTypeDepth)
		if prm.Variadic {
			if i != len(params)-1 {
				return p.fail("a variadic parameter that is not the last, which Go rejects")
			}
			expr = "..." + expr
		}
		if named {
			parts = append(parts, blankIfEmpty(prm.Name)+" "+expr)
			continue
		}
		parts = append(parts, expr)
	}
	return strings.Join(parts, ", ")
}

// returnList renders a signature's returns, parenthesised only when
// Go requires it — a lone unnamed return takes no brackets, and a
// named one takes them however alone it is.
func (p *goPrinter) returnList(returns []*node.Return) string {
	if len(returns) == 0 {
		return ""
	}
	named := slices.ContainsFunc(returns, func(r *node.Return) bool { return r.Name != "" })
	parts := make([]string, 0, len(returns))
	for _, r := range returns {
		expr := p.typeExpr(r.Type, maxTypeDepth)
		if named {
			expr = blankIfEmpty(r.Name) + " " + expr
		}
		parts = append(parts, expr)
	}
	if len(parts) == 1 && !named {
		return " " + parts[0]
	}
	return " (" + strings.Join(parts, ", ") + ")"
}

// blankIfEmpty spells an anonymous slot in a list Go has already
// forced into its named form.
func blankIfEmpty(name string) string {
	if name == "" {
		return "_"
	}
	return name
}

// openDecl starts a declaration: a separating blank line, then
// whatever doc comment the node carried.
func openDecl(b *strings.Builder, docs []string) {
	b.WriteString("\n")
	writeDocs(b, docs, "")
}

// writeDocs writes a doc comment at the given indent.
//
// Lines are split on newlines before they are prefixed. A single
// DocLines entry holding an embedded newline would otherwise emit a
// bare line into the file, and the projection would fail parsing its
// own output rather than naming what the fixture carried.
func writeDocs(b *strings.Builder, docs []string, indent string) {
	for _, doc := range docs {
		for line := range strings.SplitSeq(doc, "\n") {
			b.WriteString(strings.TrimRight(indent+"// "+line, " ") + "\n")
		}
	}
}

// enter narrows the location a failure reports to the node being
// rendered and returns the restore, so callers spell the pairing as
// a single deferred call.
func (p *goPrinter) enter(where string) func() {
	outer := p.where
	p.where = where
	return func() { p.where = outer }
}

// fail stops the projection, naming the construct and where it was
// found.
//
// Declared as returning a string so it composes into the expression
// positions the renderers use; it never returns.
func (p *goPrinter) fail(construct string) string {
	// Test-only fixture; an unspellable graph is misuse-on-construction
	// and surfaces the way [Builder.Build] surfaces its own.
	panic(&UnspellableError{Where: p.where, Construct: construct}) //nolint:forbidigo
}
