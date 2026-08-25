// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golangtest

import (
	"go/ast"
	"go/token"
	"slices"
	"strings"
	"testing"
)

// Decl is a handle on one declaration the file makes, returned so an
// assertion can be narrowed without looking it up twice.
//
// A Decl for a declaration that is absent is not nil — the assertion
// that produced it already reported, and returning nil would turn one
// failure into a panic in the next chained call.
type Decl struct {
	src   *Source
	name  string
	recv  string
	fn    *ast.FuncDecl
	spec  *ast.TypeSpec
	value *ast.ValueSpec
	found bool
}

// Source returns the file the declaration was found in, so a chain
// that establishes a declaration exists can carry on narrowing into
// it without looking the file up again.
func (d *Decl) Source() *Source { return d.src }

// AssertType fails when the file declares no type of that name.
func (s *Source) AssertType(tb testing.TB, name string) *Decl {
	tb.Helper()
	if spec := s.typeSpec(name); spec != nil {
		return &Decl{src: s, name: name, spec: spec, found: true}
	}
	tb.Errorf("golangtest: %s declares no type %q; it declares %v", s.path, name, s.typeNames())
	return &Decl{src: s, name: name}
}

// AssertNoType fails when the file declares a type of that name.
func (s *Source) AssertNoType(tb testing.TB, name string) *Source {
	tb.Helper()
	if s.typeSpec(name) != nil {
		tb.Errorf("golangtest: %s declares type %q, which it must not", s.path, name)
	}
	return s
}

// AssertFunc fails when the file declares no plain function of that
// name. A method is not a match: [Source.AssertMethod] asks for one.
func (s *Source) AssertFunc(tb testing.TB, name string) *Decl {
	tb.Helper()
	if fn := s.funcDecl("", name); fn != nil {
		return &Decl{src: s, name: name, fn: fn, found: true}
	}
	if recv := s.receiverOf(name); recv != "" {
		tb.Errorf("golangtest: %s declares no function %q, but does declare the method %s.%s",
			s.path, name, recv, name)
		return &Decl{src: s, name: name}
	}
	tb.Errorf("golangtest: %s declares no function %q; it declares %v",
		s.path, name, s.funcNames())
	return &Decl{src: s, name: name}
}

// AssertNoFunc fails when the file declares a plain function of that
// name.
func (s *Source) AssertNoFunc(tb testing.TB, name string) *Source {
	tb.Helper()
	if s.funcDecl("", name) != nil {
		tb.Errorf("golangtest: %s declares function %q, which it must not", s.path, name)
	}
	return s
}

// AssertMethod fails when recv declares no method of that name.
//
// recv is the bare type name: a method on `*ItemBuilder` and one on
// `ItemBuilder` both match `ItemBuilder`, because a caller asking
// whether the setter exists does not mean to pin the receiver form.
// [Decl.AssertPointerReceiver] pins it where that matters.
func (s *Source) AssertMethod(tb testing.TB, recv, name string) *Decl {
	tb.Helper()
	if fn := s.funcDecl(recv, name); fn != nil {
		return &Decl{src: s, name: name, recv: recv, fn: fn, found: true}
	}
	tb.Errorf("golangtest: %s declares no method %s.%s; %s declares %v",
		s.path, recv, name, recv, s.methodNames(recv))
	return &Decl{src: s, name: name, recv: recv}
}

// AssertNoMethod fails when recv declares a method of that name.
func (s *Source) AssertNoMethod(tb testing.TB, recv, name string) *Source {
	tb.Helper()
	if s.funcDecl(recv, name) != nil {
		tb.Errorf("golangtest: %s declares method %s.%s, which it must not", s.path, recv, name)
	}
	return s
}

// AssertVar fails when the file declares no package-level var of that
// name.
//
// The declaration a registry, a sentinel block or a middleware chain
// generator emits as its whole output, and the one kind this package
// could not name: a test whose subject is `var Chain = […]` had to
// reach it obliquely, through an ordering assertion or a file-wide
// substring, neither of which distinguishes a var from a const or
// from a mention in a comment.
//
// A const of the same name is diagnosed rather than accepted: the two
// are not interchangeable to a consumer, who can take the address of
// one and not the other.
func (s *Source) AssertVar(tb testing.TB, name string) *Decl {
	tb.Helper()
	if spec, _ := s.valueSpec(name); spec != nil {
		return &Decl{src: s, name: name, value: spec, found: true}
	}
	if s.declaresConst(name) {
		tb.Errorf("golangtest: %s declares no var %q, but does declare a const of that name",
			s.path, name)
		return &Decl{src: s, name: name}
	}
	tb.Errorf("golangtest: %s declares no var %q; it declares %v", s.path, name, s.varNames())
	return &Decl{src: s, name: name}
}

// AssertNoVar fails when the file declares a package-level var of
// that name.
func (s *Source) AssertNoVar(tb testing.TB, name string) *Source {
	tb.Helper()
	if spec, _ := s.valueSpec(name); spec != nil {
		tb.Errorf("golangtest: %s declares var %q, which it must not", s.path, name)
	}
	return s
}

// AssertField fails when the named struct has no field of that name
// and type.
//
// The type is compared in canonical form, so the assertion is not
// disturbed by the column padding gofmt applies across a struct's
// fields — which a substring spelling the same field is.
func (s *Source) AssertField(tb testing.TB, typeName, field, typeExpr string) *Source {
	tb.Helper()
	st := s.structType(tb, typeName)
	if st == nil {
		return s
	}
	var declared []string
	for _, f := range st.Fields.List {
		rendered := exprString(f.Type)
		for _, n := range f.Names {
			if n.Name != field {
				declared = append(declared, n.Name+" "+rendered)
				continue
			}
			if normalise(rendered) != normalise(typeExpr) {
				tb.Errorf("golangtest: %s field %s.%s is %q, want %q",
					s.path, typeName, field, rendered, typeExpr)
			}
			return s
		}
	}
	tb.Errorf("golangtest: %s struct %s has no field %q; it declares %v",
		s.path, typeName, field, declared)
	return s
}

// AssertEmbeds fails when the named struct or interface does not
// embed the given type.
func (s *Source) AssertEmbeds(tb testing.TB, typeName, typeExpr string) *Source {
	tb.Helper()
	spec := s.typeSpec(typeName)
	if spec == nil {
		tb.Errorf("golangtest: %s declares no type %q; it declares %v",
			s.path, typeName, s.typeNames())
		return s
	}
	var fields *ast.FieldList
	switch decl := spec.Type.(type) {
	case *ast.StructType:
		fields = decl.Fields
	case *ast.InterfaceType:
		fields = decl.Methods
	default:
		tb.Errorf("golangtest: %s type %s is not a struct or interface, so it embeds nothing",
			s.path, typeName)
		return s
	}
	var embedded []string
	for _, f := range fields.List {
		if len(f.Names) > 0 {
			continue
		}
		rendered := exprString(f.Type)
		if normalise(rendered) == normalise(typeExpr) {
			return s
		}
		embedded = append(embedded, rendered)
	}
	tb.Errorf("golangtest: %s type %s does not embed %q; it embeds %v",
		s.path, typeName, typeExpr, embedded)
	return s
}

// Signature fails when the declaration's signature is not want.
//
// Spelled without the `func` keyword and without the name —
// `(ctx context.Context, id string) (string, error)` — because those
// were already established by the assertion that produced the [Decl],
// and repeating them is one more place for the two to disagree.
func (d *Decl) Signature(tb testing.TB, want string) *Decl {
	tb.Helper()
	if !d.found {
		return d
	}
	if d.fn == nil {
		tb.Errorf("golangtest: %s %q is %s, which has no signature", d.src.path, d.name, d.kind())
		return d
	}
	got := strings.TrimPrefix(render(d.fn.Type), "func")
	if normalise(got) != normalise(want) {
		tb.Errorf("golangtest: %s %s has signature %s, want %s",
			d.src.path, d.qualified(), normalise(got), normalise(want))
	}
	return d
}

// AssertPointerReceiver fails when the method is not declared on a
// pointer receiver.
//
// Load-bearing where the standard library consults a method set: an
// `Is` or an `Error` on the value form when the consumer holds a
// pointer — or the reverse — is never called, and the type silently
// behaves as though it declared nothing.
func (d *Decl) AssertPointerReceiver(tb testing.TB, want bool) *Decl {
	tb.Helper()
	if !d.found {
		return d
	}
	if d.fn == nil || d.fn.Recv == nil || len(d.fn.Recv.List) == 0 {
		tb.Errorf("golangtest: %s %q has no receiver", d.src.path, d.name)
		return d
	}
	_, isPtr := d.fn.Recv.List[0].Type.(*ast.StarExpr)
	if isPtr != want {
		tb.Errorf("golangtest: %s %s is declared on %s, want %s",
			d.src.path, d.qualified(), receiverForm(isPtr), receiverForm(want))
	}
	return d
}

// AssertDoc fails when the declaration's doc comment does not contain
// substr.
//
// Generated documentation is output too: a plugin that says why a
// check is absent is answering a reader who came looking for it, and
// a plugin that silently dropped the sentence would be worse than one
// that never had it.
func (d *Decl) AssertDoc(tb testing.TB, substr string) *Decl {
	tb.Helper()
	if !d.found {
		return d
	}
	doc := d.docText()
	if !strings.Contains(doc, substr) {
		tb.Errorf("golangtest: %s %s doc does not contain %q\n--- doc ---\n%s",
			d.src.path, d.qualified(), substr, doc)
	}
	return d
}

// AssertDocLacks fails when the declaration's doc comment contains
// substr.
//
// The half [Decl.AssertDoc] cannot state. A generator that withholds
// a check must not go on promising it: documentation that describes a
// guarantee the emitted code stopped making is worse than none, since
// a reader stops looking for the guarantee elsewhere. Nothing else
// sees it — the code is correct, the doc is a comment, and every
// structural assertion passes.
func (d *Decl) AssertDocLacks(tb testing.TB, substr string) *Decl {
	tb.Helper()
	if !d.found {
		return d
	}
	doc := d.docText()
	if strings.Contains(doc, substr) {
		tb.Errorf("golangtest: %s %s doc contains %q, which it must not\n--- doc ---\n%s",
			d.src.path, d.qualified(), substr, doc)
	}
	return d
}

// docText returns the declaration's doc comment.
func (d *Decl) docText() string {
	switch {
	case d.fn != nil && d.fn.Doc != nil:
		return d.fn.Doc.Text()
	case d.spec != nil && d.spec.Doc != nil:
		return d.spec.Doc.Text()
	case d.value != nil && d.value.Doc != nil:
		return d.value.Doc.Text()
	case d.spec != nil, d.value != nil:
		// A single-spec `type T struct{…}` or `var X = …` hangs its
		// comment on the GenDecl rather than the spec, which is the
		// shape every generated declaration takes.
		if doc := d.src.genDeclDoc(d.name); doc != "" {
			return doc
		}
	}
	return ""
}

// kind spells what a declaration is, for a message explaining why the
// question asked of it does not apply.
func (d *Decl) kind() string {
	if d.value != nil {
		return "a variable"
	}
	return "a type"
}

// qualified names the declaration the way a reader would write it.
func (d *Decl) qualified() string {
	if d.recv != "" {
		return d.recv + "." + d.name
	}
	return d.name
}

// receiverForm spells a receiver kind for a failure message.
func receiverForm(pointer bool) string {
	if pointer {
		return "a pointer receiver"
	}
	return "a value receiver"
}

// typeSpec returns the named type's declaration.
func (s *Source) typeSpec(name string) *ast.TypeSpec {
	for _, d := range s.file.Decls {
		gen, ok := d.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			if ts, isType := spec.(*ast.TypeSpec); isType && ts.Name.Name == name {
				return ts
			}
		}
	}
	return nil
}

// valueSpec returns the spec declaring the named package-level var,
// and the position of the name within it — which is what says how the
// spec's initialisers line up against its names.
func (s *Source) valueSpec(name string) (*ast.ValueSpec, int) {
	for _, d := range s.file.Decls {
		gen, ok := d.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vs, isValue := spec.(*ast.ValueSpec)
			if !isValue {
				continue
			}
			if i := slices.IndexFunc(vs.Names, func(n *ast.Ident) bool {
				return n.Name == name
			}); i >= 0 {
				return vs, i
			}
		}
	}
	return nil, 0
}

// declaresConst reports whether the file declares a constant of that
// name, so asking for a var can say which of the two it found.
func (s *Source) declaresConst(name string) bool {
	return slices.Contains(s.valueNames(token.CONST), name)
}

// varNames lists every package-level var the file declares.
func (s *Source) varNames() []string { return s.valueNames(token.VAR) }

// valueNames lists every name declared by one kind of value
// declaration.
func (s *Source) valueNames(tok token.Token) []string {
	var out []string
	for _, d := range s.file.Decls {
		gen, ok := d.(*ast.GenDecl)
		if !ok || gen.Tok != tok {
			continue
		}
		for _, spec := range gen.Specs {
			if vs, isValue := spec.(*ast.ValueSpec); isValue {
				for _, n := range vs.Names {
					out = append(out, n.Name)
				}
			}
		}
	}
	return out
}

// genDeclDoc returns the doc comment on the GenDecl wrapping a named
// spec.
func (s *Source) genDeclDoc(name string) string {
	for _, d := range s.file.Decls {
		gen, ok := d.(*ast.GenDecl)
		if !ok || gen.Doc == nil {
			continue
		}
		if slices.Contains(specNames(gen), name) {
			return gen.Doc.Text()
		}
	}
	return ""
}

// specNames lists the names a grouped declaration's specs introduce.
func specNames(gen *ast.GenDecl) []string {
	var out []string
	for _, spec := range gen.Specs {
		switch sp := spec.(type) {
		case *ast.TypeSpec:
			out = append(out, sp.Name.Name)
		case *ast.ValueSpec:
			for _, n := range sp.Names {
				out = append(out, n.Name)
			}
		}
	}
	return out
}

// structType returns the named type's struct body, reporting when the
// type is absent or is not a struct.
func (s *Source) structType(tb testing.TB, name string) *ast.StructType {
	tb.Helper()
	spec := s.typeSpec(name)
	if spec == nil {
		tb.Errorf("golangtest: %s declares no type %q; it declares %v", s.path, name, s.typeNames())
		return nil
	}
	st, ok := spec.Type.(*ast.StructType)
	if !ok {
		tb.Errorf("golangtest: %s type %s is %s, not a struct", s.path, name, exprString(spec.Type))
		return nil
	}
	return st
}

// funcDecl returns the function or method matching recv and name. An
// empty recv matches a plain function only.
func (s *Source) funcDecl(recv, name string) *ast.FuncDecl {
	for _, d := range s.file.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Name.Name != name {
			continue
		}
		if receiverName(fn) == recv {
			return fn
		}
	}
	return nil
}

// receiverOf returns the receiver a method of that name is declared
// on, so a failure can say the caller asked the wrong question.
func (s *Source) receiverOf(name string) string {
	for _, d := range s.file.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == name {
			return receiverName(fn)
		}
	}
	return ""
}

// typeNames lists every type the file declares.
func (s *Source) typeNames() []string {
	var out []string
	for _, d := range s.file.Decls {
		gen, ok := d.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			if ts, isType := spec.(*ast.TypeSpec); isType {
				out = append(out, ts.Name.Name)
			}
		}
	}
	return out
}

// funcNames lists every plain function the file declares.
func (s *Source) funcNames() []string {
	var out []string
	for _, d := range s.file.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil {
			out = append(out, fn.Name.Name)
		}
	}
	return out
}

// methodNames lists every method declared on recv.
func (s *Source) methodNames(recv string) []string {
	var out []string
	for _, d := range s.file.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && receiverName(fn) == recv {
			out = append(out, fn.Name.Name)
		}
	}
	slices.Sort(out)
	return out
}
