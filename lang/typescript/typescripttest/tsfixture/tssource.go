// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tsfixture

import (
	"fmt"
	"path"
	"slices"
	"strconv"
	"strings"

	"go.thesmos.sh/eidos/core/contract"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// tsSourceBasename is the file the projection writes into.
//
// `index.ts` because a bare directory specifier resolves to it: a
// generated file importing `./models` finds this without the fixture
// having to know what the support file was called.
const tsSourceBasename = "index.ts"

// sourceHeader marks the projected file as machine-made, so a reader
// finding one in a failure dump knows not to edit it.
const sourceHeader = "// Projected by tsfixture. DO NOT EDIT.\n\n"

// indent is one level of TypeScript indentation, matching what the
// backend emits — a support file and a generated one sitting side by
// side in a failure dump should not differ in shape for no reason.
const indent = "  "

// UnspellableError reports a construct the graph carries that
// [Builder.TSSource] has no TypeScript spelling for. It is raised as a
// panic rather than returned.
//
// A returned error would be wrong twice over. TSSource is called in
// the expression position a [typescripttest.File] wants, so an error
// return would be dropped or checked by exactly the discipline this
// projection exists to remove; and the condition is fixture misuse,
// which [Builder.Build] already answers with a panic. What matters is
// that the failure names the construct — a projection that quietly
// omitted a property would reintroduce the drift, one file further
// down.
type UnspellableError struct {
	// Where identifies the declaration being rendered.
	Where string

	// Construct names the shape that has no TypeScript spelling here.
	Construct string
}

// Error implements the error interface.
func (e *UnspellableError) Error() string {
	return fmt.Sprintf("tsfixture: TSSource cannot spell %s in %s; "+
		"hand-write that module instead of projecting it, rather than "+
		"shipping a support file the fixture no longer describes",
		e.Construct, e.Where)
}

// TSSource projects the accumulating package to the TypeScript a
// consumer would have typed, returning the file's root-relative path
// and its contents.
//
// # The drift this kills
//
// Every render fixture needs the hand-written module its generated
// output imports from — the `interface User` a repository stores, the
// contract a double doubles. Written by hand, that module and the node
// graph that drove the run are bound only by review: rename a property
// in the fixture and the support source goes stale, and the failure
// arrives as a type error inside a throwaway project, naming code the
// author never wrote, for the wrong reason. Projected from the same
// builder, the pair cannot disagree.
//
// # Using it
//
// The two return values are exactly [typescripttest.TSFile]'s
// parameters, so no adapter stands between them:
//
//	gen := typescripttest.Rendered(t, run).
//	    WithSource(typescripttest.TSFile(fixture().TSSource()))
//
// The projection deliberately returns plain strings rather than a
// typescripttest type. This package builds the input to a run and
// typescripttest asserts on its output; coupling either to the other
// would make a plain store fixture drag in the toolchain harness, or
// the output assertions drag in the fixture builder.
//
// # What it emits, and what it refuses
//
// Every declaration the builder accepts, with the imports its type
// expressions need: interfaces with properties and method signatures,
// classes, type aliases, enums, functions, constants and bindings,
// generic and not. Modifiers the fixture stamped are spelled —
// `readonly`, `?`, `static`, the access keywords.
//
// Function and method bodies are not: the projection emits
// declarations, and a class whose methods have no bodies is
// `declare`d so the file still loads. `async` goes with them — it
// describes how a body produces its result, and a declaration
// carrying it is rejected outright. The return type is what a caller
// binds to, so nothing a consumer can see is lost.
//
// Two things are excluded by design rather than by oversight, because
// neither is part of the surface generated code binds to. Directives
// are dropped: a support file is never re-parsed by a pipeline, and a
// fixture directive's keyword arguments live in a map whose iteration
// order would make the projection non-deterministic. Imports recorded
// via [Builder.Import] are dropped too — an import nothing references
// is an error under noUnusedLocals, and those entries exist for tests
// inspecting a frontend's import view.
//
// Anything else the graph carries that has no TypeScript spelling
// stops the test with an [UnspellableError] naming it. That is the
// whole bargain: a support module missing a property is precisely the
// drift this projection exists to kill, so it refuses rather than
// emits.
func (b *Builder) TSSource() (path, src string) {
	p := &tsPrinter{
		pkg:     b.pkg,
		imports: map[string]map[string]struct{}{},
		where:   "the fixture package",
	}
	return p.file()
}

// tsPrinter renders one [node.Package] as TypeScript, accumulating
// the imports its type expressions turn out to need.
//
// Imports are collected during the walk rather than in a pass before
// it, so the two can never disagree about what the file references —
// the recurring failure mode of a generator that computes its import
// block from a model rather than from what it actually spelled.
type tsPrinter struct {
	pkg *node.Package

	// imports maps a module specifier to the names taken from it.
	// Every one is type-only: the projection emits declarations, so
	// nothing it writes constructs a value.
	imports map[string]map[string]struct{}

	// where names the declaration being rendered, for the failure
	// message. Set before each declaration and read only on the way
	// out.
	where string
}

// file renders the whole package.
func (p *tsPrinter) file() (path, src string) {
	if p.pkg.Name == "" {
		p.fail("a package with no name")
	}
	// Before the import block, because spelling a type is what
	// registers its import.
	body := p.decls()

	var b strings.Builder
	b.WriteString(sourceHeader)
	b.WriteString(p.importBlock())
	b.WriteString(body)
	return p.sourcePath(), b.String()
}

// sourcePath places the projected file in the directory the fixture's
// own declarations report living in.
//
// Derived rather than assumed, and for the same reason the projection
// exists: the support file is only importable by the generated output
// if the specifier the output writes resolves to it, and where the
// output lands is decided by the synthetic file each declaration
// carries — which a fixture is free to repoint with a sub-builder's
// Pos. Hard-coding `<pkg>/` would put the support file beside output
// routed to the project root, and the failure would arrive as an
// unresolved module rather than as a misplaced file.
//
// An empty package falls back to the package name, which is what
// [declFile] would have stamped had anything been declared.
func (p *tsPrinter) sourcePath() string {
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
		return p.pkg.Name + "/" + tsSourceBasename
	case 1:
		for dir := range dirs {
			if dir == "" {
				return tsSourceBasename
			}
			return dir + "/" + tsSourceBasename
		}
	}
	return p.fail("declarations spread across " + strconv.Itoa(len(dirs)) +
		" directories, which is more than one module and more than one file")
}

// collectDirs records the directory each declaration's synthetic
// source file names, treating the project root as the empty string.
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
// the type the output names, and TypeScript hoists type declarations
// so the ordering has no effect on what loads.
func (p *tsPrinter) decls() string {
	var b strings.Builder
	for _, a := range p.pkg.Aliases {
		write(&b, p.alias(a))
	}
	for _, e := range p.pkg.Enums {
		write(&b, p.enum(e))
	}
	for _, i := range p.pkg.Interfaces {
		write(&b, p.iface(i))
	}
	for _, s := range p.pkg.Structs {
		write(&b, p.class(s))
	}
	for _, c := range p.pkg.Constants {
		write(&b, p.constant(c))
	}
	for _, v := range p.pkg.Variables {
		write(&b, p.variable(v))
	}
	for _, f := range p.pkg.Functions {
		write(&b, p.function(f))
	}
	return b.String()
}

// write appends one declaration followed by a blank line.
func write(b *strings.Builder, decl string) {
	b.WriteString(decl)
	b.WriteString("\n")
}

// importBlock renders the imports the walk collected, sorted.
func (p *tsPrinter) importBlock() string {
	if len(p.imports) == 0 {
		return ""
	}
	specifiers := make([]string, 0, len(p.imports))
	for spec := range p.imports {
		specifiers = append(specifiers, spec)
	}
	slices.Sort(specifiers)

	var b strings.Builder
	for _, spec := range specifiers {
		names := make([]string, 0, len(p.imports[spec]))
		for n := range p.imports[spec] {
			names = append(names, n)
		}
		slices.Sort(names)
		b.WriteString("import type { " + strings.Join(names, ", ") + " } from " +
			typescript.Quote(spec) + ";\n")
	}
	b.WriteString("\n")
	return b.String()
}

// docs renders a JSDoc block, empty for a declaration carrying none.
//
// Projected rather than dropped because a support file turns up in
// every toolchain failure dump this harness prints, and a reader
// checking it against the fixture that produced it is reading two
// things that should say the same words.
func docs(lines []string, pad string) string {
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(pad + "/**\n")
	for _, line := range lines {
		b.WriteString(strings.TrimRight(pad+" * "+line, " ") + "\n")
	}
	b.WriteString(pad + " */\n")
	return b.String()
}

// alias renders `export type X = Y;`.
func (p *tsPrinter) alias(a *node.Alias) string {
	p.where = "alias " + a.Name
	if a.Target == nil {
		p.fail("an alias naming no type")
	}
	return docs(a.DocLines, "") + "export type " + a.Name + p.typeParams(a.TypeParams) +
		" = " + p.typeOf(a.Target) + ";\n"
}

// enum renders `export enum X { … }`.
func (p *tsPrinter) enum(e *node.Enum) string {
	p.where = "enum " + e.Name

	var b strings.Builder
	b.WriteString(docs(e.DocLines, ""))
	b.WriteString("export ")
	if c, _ := typescript.MetaConstEnum.Get(e.Meta()); c {
		b.WriteString("const ")
	}
	b.WriteString("enum " + e.Name + " {\n")
	for _, v := range e.Variants {
		if v == nil || v.Name == "" {
			p.fail("an enum member with no name")
		}
		b.WriteString(docs(v.DocLines, indent))
		b.WriteString(indent + v.Name)
		if v.Value != "" {
			b.WriteString(" = " + v.Value)
		}
		b.WriteString(",\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// iface renders `export interface X { … }`.
func (p *tsPrinter) iface(i *node.Interface) string {
	p.where = "interface " + i.Name

	var b strings.Builder
	b.WriteString(docs(i.DocLines, ""))
	b.WriteString("export interface " + i.Name + p.typeParams(i.TypeParams))
	b.WriteString(p.heritage(i.Embeds, typescript.HeritageExtends))
	b.WriteString(" {\n")
	if sig, ok := typescript.MetaIndexSignature.Get(i.Meta()); ok && sig != "" {
		b.WriteString(indent + sig + ";\n")
	}
	if sig, ok := typescript.MetaConstructSignature.Get(i.Meta()); ok && sig != "" {
		b.WriteString(indent + sig + ";\n")
	}
	for _, f := range i.Fields {
		b.WriteString(docs(f.DocLines, indent) + indent + p.property(f))
	}
	for _, m := range i.Methods {
		b.WriteString(docs(m.DocLines, indent) + indent + p.signature(m) + ";\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// class renders `export declare class X { … }`.
//
// Declared rather than defined: the projection emits declarations and
// a class with bodiless methods is not a class TypeScript will load.
// `declare` is the form that says "this exists, described here,
// implemented elsewhere", which is exactly what a support file is.
func (p *tsPrinter) class(s *node.Struct) string {
	p.where = "class " + s.Name

	var b strings.Builder
	b.WriteString(docs(s.DocLines, ""))
	b.WriteString("export declare ")
	if a, _ := typescript.MetaAbstract.Get(s.Meta()); a {
		b.WriteString("abstract ")
	}
	b.WriteString("class " + s.Name + p.typeParams(s.TypeParams))
	b.WriteString(p.heritage(s.Embeds, typescript.HeritageExtends))
	b.WriteString(p.heritage(s.Embeds, typescript.HeritageImplements))
	b.WriteString(" {\n")
	for _, f := range s.Fields {
		b.WriteString(docs(f.DocLines, indent) + indent + p.property(f))
	}
	for _, m := range s.Methods {
		b.WriteString(docs(m.DocLines, indent) + indent + p.member(m) + ";\n")
	}
	b.WriteString("}\n")
	return b.String()
}

// constant renders `export declare const x: T;`.
func (p *tsPrinter) constant(c *node.Constant) string {
	p.where = "constant " + c.Name
	return docs(c.DocLines, "") + "export declare const " + c.Name + p.annotation(c.Type) + ";\n"
}

// variable renders `export declare let x: T;`.
func (p *tsPrinter) variable(v *node.Variable) string {
	p.where = "variable " + v.Name
	return docs(v.DocLines, "") + "export declare let " + v.Name + p.annotation(v.Type) + ";\n"
}

// function renders `export declare function f(…): R;`.
func (p *tsPrinter) function(f *node.Function) string {
	p.where = "function " + f.Name
	return docs(f.DocLines, "") + "export declare function " + f.Name + p.typeParams(f.TypeParams) +
		"(" + p.params(f.Params) + "): " + p.returns(f.Returns) + ";\n"
}

// annotation renders `: T`, empty for a declaration whose type is
// inferred.
func (p *tsPrinter) annotation(t *node.TypeRef) string {
	if t == nil {
		return ""
	}
	return ": " + p.typeOf(t)
}

// property renders one property signature.
func (p *tsPrinter) property(f *node.Field) string {
	if f == nil || f.Name == "" {
		p.fail("a property with no name")
	}
	var b strings.Builder
	b.WriteString(p.visibility(f))
	if s, _ := typescript.MetaStatic.Get(f.Meta()); s {
		b.WriteString("static ")
	}
	if ro, _ := typescript.MetaReadonly.Get(f.Meta()); ro {
		b.WriteString("readonly ")
	}
	b.WriteString(typescript.PropertyKey(f.Name))
	if opt, _ := typescript.MetaOptional.Get(f.Meta()); opt {
		b.WriteString("?")
	} else if da, _ := typescript.MetaDefiniteAssignment.Get(f.Meta()); da {
		b.WriteString("!")
	}
	b.WriteString(p.annotation(f.Type) + ";\n")
	return b.String()
}

// member renders one class method signature, modifiers included.
func (p *tsPrinter) member(m *node.Method) string {
	var b strings.Builder
	b.WriteString(p.visibility(m))
	if s, _ := typescript.MetaStatic.Get(m.Meta()); s {
		b.WriteString("static ")
	}
	if a, _ := typescript.MetaAbstract.Get(m.Meta()); a {
		b.WriteString("abstract ")
	}
	b.WriteString(p.signature(m))
	return b.String()
}

// signature renders a method's name, parameters and return.
//
// An accessor is spelled with its keyword: a getter's use site is a
// property read, so projecting one as an ordinary method would give
// the support file a surface the source does not have.
func (p *tsPrinter) signature(m *node.Method) string {
	if m == nil || m.Name == "" {
		p.fail("a method with no name")
	}
	var b strings.Builder
	if kind, ok := typescript.MetaAccessor.Get(m.Meta()); ok && kind != "" {
		b.WriteString(kind + " ")
	}
	if g, _ := typescript.MetaGenerator.Get(m.Meta()); g {
		b.WriteString("*")
	}
	b.WriteString(typescript.PropertyKey(m.Name))
	if opt, _ := typescript.MetaOptional.Get(m.Meta()); opt {
		b.WriteString("?")
	}
	b.WriteString(p.typeParams(m.TypeParams))
	b.WriteString("(" + p.params(m.Params) + "): " + p.returns(m.Returns))
	return b.String()
}

// visibility renders an explicit access modifier, empty for a member
// that declared none.
//
// A `#name` hard-private field is refused rather than spelled: the
// name itself carries the `#`, so a projection emitting the keyword
// form would declare a differently-named member.
func (p *tsPrinter) visibility(n contract.Node) string {
	level, ok := typescript.MetaVisibility.Get(n.Meta())
	if !ok || level == "" || level == typescript.VisibilityPublic {
		return ""
	}
	if level == typescript.VisibilityHard {
		p.fail("a `#`-private member, whose name carries the marker")
	}
	return level + " "
}

// params renders a parameter list.
func (p *tsPrinter) params(params []*node.Param) string {
	parts := make([]string, 0, len(params))
	for i, param := range params {
		if param == nil {
			p.fail("a signature with a missing parameter")
		}
		name := param.Name
		if name == "" {
			name = "arg" + strconv.Itoa(i)
		}
		var b strings.Builder
		if param.Variadic {
			b.WriteString("...")
		}
		b.WriteString(typescript.Ident(name))
		if opt, _ := typescript.MetaOptional.Get(param.Meta()); opt && !param.Variadic {
			b.WriteString("?")
		}
		// A rest parameter's declared type is its element type, so the
		// spelled annotation is the array of it.
		t := p.typeOf(param.Type)
		if param.Variadic {
			t = arrayOf(t)
		}
		b.WriteString(": " + t)
		parts = append(parts, b.String())
	}
	return strings.Join(parts, ", ")
}

// returns renders a return clause — void for none, the type for one,
// the tuple for several.
func (p *tsPrinter) returns(returns []*node.Return) string {
	switch len(returns) {
	case 0:
		return typescript.TypeVoid
	case 1:
		if returns[0] == nil {
			p.fail("a signature with a missing return")
		}
		return p.typeOf(returns[0].Type)
	default:
		parts := make([]string, 0, len(returns))
		for _, r := range returns {
			if r == nil {
				p.fail("a signature with a missing return")
			}
			parts = append(parts, p.typeOf(r.Type))
		}
		return "[" + strings.Join(parts, ", ") + "]"
	}
}

// typeParams renders a generic parameter list, empty for none.
func (p *tsPrinter) typeParams(params []*node.TypeParam) string {
	if len(params) == 0 {
		return ""
	}
	parts := make([]string, 0, len(params))
	for _, tp := range params {
		if tp == nil || tp.Name == "" {
			p.fail("a generic parameter with no name")
		}
		part := tp.Name
		if bound := p.constraint(tp.Constraint); bound != "" {
			part += " extends " + bound
		}
		if def, ok := typescript.MetaTypeParamDefault.Get(tp.Meta()); ok && def != "" {
			part += " = " + def
		}
		parts = append(parts, part)
	}
	return "<" + strings.Join(parts, ", ") + ">"
}

// constraint renders a bound, several becoming the intersection
// TypeScript's single-type `extends` needs.
func (p *tsPrinter) constraint(c *node.Constraint) string {
	if c == nil || len(c.Embedded) == 0 {
		return ""
	}
	parts := make([]string, 0, len(c.Embedded))
	for _, e := range c.Embedded {
		parts = append(parts, p.typeOf(e))
	}
	return strings.Join(parts, " & ")
}

// heritage renders the clause of one kind, empty when the declaration
// has none of it.
//
// An unmarked embed reads as extends, which is the reading that holds
// for an interface — the only host that produces one.
func (p *tsPrinter) heritage(embeds []*node.Embed, want string) string {
	var parts []string
	for _, e := range embeds {
		if e == nil || e.Type == nil {
			continue
		}
		kind, ok := typescript.MetaHeritage.Get(e.Meta())
		if !ok || kind == "" {
			kind = typescript.HeritageExtends
		}
		if kind != want {
			continue
		}
		parts = append(parts, p.typeOf(e.Type))
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + want + " " + strings.Join(parts, ", ")
}

// fail panics with an [UnspellableError] naming the construct.
func (p *tsPrinter) fail(construct string) string {
	panic(&UnspellableError{Where: p.where, Construct: construct}) //nolint:forbidigo
}

// arrayOf spells the array form of an element type, parenthesising
// where a postfix `[]` would otherwise bind too tightly.
func arrayOf(elem string) string {
	if strings.ContainsAny(elem, "|&") || strings.Contains(elem, "=>") {
		return "(" + elem + ")[]"
	}
	return elem + "[]"
}
