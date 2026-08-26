// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescripttest

import (
	"slices"
	"strings"
	"testing"

	ts "github.com/tree-sitter/go-tree-sitter"
)

// The declaration assertions exist for their failure messages more
// than their subject matter.
//
// A substring check that fails says a substring is missing, which is
// true and useless: the interface is nearly always there, declaring
// something slightly different from what the test asked for. These
// say that instead — the property exists with another type, the
// method exists with another signature, the declaration exists as
// another kind.

// Decl is one top-level declaration, returned so an assertion can
// chain into what it declares.
type Decl struct {
	src  *Source
	node *ts.Node
	name string
}

// Name returns the declaration's identifier.
func (d *Decl) Name() string { return d.name }

// Text returns the declaration's source text, without the `export`
// or `declare` that wraps it.
//
// The declaration proper rather than the statement, because that is
// what the assertions above address: [Source.AssertInterface] finds
// `interface User` whether or not it was exported, and Text returning
// something the finder does not match on would read as a different
// node. Reach for [Source.AssertContains] to check the wrapper.
func (d *Decl) Text() string { return d.src.text(d.node) }

// AssertInterface fails when the file declares no interface of that
// name.
func (s *Source) AssertInterface(tb testing.TB, name string) *Decl {
	tb.Helper()
	return s.assertDecl(tb, "interface", name, "interface_declaration")
}

// AssertNoInterface fails when the file declares an interface of
// that name.
//
// Kind-specific, unlike [Source.AssertNoDecl]: what a generator
// emitting a class where it used to emit an interface broke is the
// consumer who wrote `implements`, and a name-only check would pass.
func (s *Source) AssertNoInterface(tb testing.TB, name string) *Source {
	tb.Helper()
	return s.assertNoDeclOfKind(tb, "interface", name, "interface_declaration")
}

// AssertClass fails when the file declares no class of that name.
func (s *Source) AssertClass(tb testing.TB, name string) *Decl {
	tb.Helper()
	return s.assertDecl(tb, "class", name, "class_declaration")
}

// AssertNoClass fails when the file declares a class of that name.
func (s *Source) AssertNoClass(tb testing.TB, name string) *Source {
	tb.Helper()
	return s.assertNoDeclOfKind(tb, "class", name, "class_declaration")
}

// AssertAlias fails when the file declares no type alias of that
// name.
func (s *Source) AssertAlias(tb testing.TB, name string) *Decl {
	tb.Helper()
	return s.assertDecl(tb, "type alias", name, "type_alias_declaration")
}

// AssertNoAlias fails when the file declares a type alias of that
// name.
func (s *Source) AssertNoAlias(tb testing.TB, name string) *Source {
	tb.Helper()
	return s.assertNoDeclOfKind(tb, "type alias", name, "type_alias_declaration")
}

// AssertEnum fails when the file declares no enum of that name.
func (s *Source) AssertEnum(tb testing.TB, name string) *Decl {
	tb.Helper()
	return s.assertDecl(tb, "enum", name, "enum_declaration")
}

// AssertNoEnum fails when the file declares an enum of that name.
func (s *Source) AssertNoEnum(tb testing.TB, name string) *Source {
	tb.Helper()
	return s.assertNoDeclOfKind(tb, "enum", name, "enum_declaration")
}

// AssertFunction fails when the file declares no function of that
// name.
func (s *Source) AssertFunction(tb testing.TB, name string) *Decl {
	tb.Helper()
	return s.assertDecl(tb, "function", name, "function_declaration", "function_signature")
}

// AssertNoFunction fails when the file declares a function of that
// name.
func (s *Source) AssertNoFunction(tb testing.TB, name string) *Source {
	tb.Helper()
	return s.assertNoDeclOfKind(tb, "function", name,
		"function_declaration", "function_signature")
}

// AssertBinding fails when the file declares no `const`, `let` or
// `var` of that name.
//
// One assertion for the three because the keyword is not what a test
// is usually about, and a generator switching `const` to `let` is a
// change [Source.AssertContains] catches with the spelling a reader
// would look for.
func (s *Source) AssertBinding(tb testing.TB, name string) *Decl {
	tb.Helper()
	return s.assertDecl(tb, "binding", name, "lexical_declaration", "variable_declaration")
}

// AssertNoBinding fails when the file declares a binding of that
// name.
func (s *Source) AssertNoBinding(tb testing.TB, name string) *Source {
	tb.Helper()
	return s.assertNoDeclOfKind(tb, "binding", name,
		"lexical_declaration", "variable_declaration")
}

// AssertNoDecl fails when the file declares anything of that name.
//
// Kind-agnostic on purpose: what a test means by "must not declare X"
// is that the identifier is not taken, and a generator that started
// emitting a class where it used to emit an interface has still
// broken whatever the assertion was protecting.
func (s *Source) AssertNoDecl(tb testing.TB, name string) *Source {
	tb.Helper()
	for _, n := range s.topLevel() {
		if declName(n, s.src) == name {
			tb.Errorf("typescripttest: %s declares %s as a %s, which it must not",
				s.path, name, n.Kind())
			return s
		}
	}
	return s
}

// AssertProperty fails when the named declaration has no property of
// that name and type.
//
// The type is compared as written, with whitespace collapsed —
// `string | null` matches however the renderer spaced it, and
// `string|null` does not match `string | number`. Pass an empty type
// to assert the property exists without pinning what it holds.
func (s *Source) AssertProperty(tb testing.TB, decl, property, typeExpr string) *Source {
	tb.Helper()
	body := s.bodyOf(tb, decl)
	if body == nil {
		return s
	}

	var found []string
	for i := range body.NamedChildCount() {
		member := body.NamedChild(i)
		name, typ, ok := propertyOf(member, s.src)
		if !ok {
			continue
		}
		found = append(found, name+": "+typ)
		if name != property {
			continue
		}
		if typeExpr == "" || collapse(typ) == collapse(typeExpr) {
			return s
		}
		tb.Errorf("typescripttest: %s declares %s.%s as %q, want %q",
			s.path, decl, property, typ, typeExpr)
		return s
	}
	tb.Errorf("typescripttest: %s declares no %s.%s; %s has %v",
		s.path, decl, property, decl, found)
	return s
}

// AssertNoProperty fails when the named declaration has a property of
// that name.
func (s *Source) AssertNoProperty(tb testing.TB, decl, property string) *Source {
	tb.Helper()
	body := s.bodyOf(tb, decl)
	if body == nil {
		return s
	}
	for i := range body.NamedChildCount() {
		name, _, ok := propertyOf(body.NamedChild(i), s.src)
		if ok && name == property {
			tb.Errorf("typescripttest: %s declares %s.%s, which it must not",
				s.path, decl, property)
			return s
		}
	}
	return s
}

// AssertMethod fails when the named declaration has no method of that
// name, and reports its actual signature when the name is there.
//
// Which is what is true most of the time: a generator that dropped a
// parameter or changed a return type still declares the method, and a
// substring check reports only that its own spelling is absent.
func (s *Source) AssertMethod(tb testing.TB, decl, method string) *Source {
	tb.Helper()
	body := s.bodyOf(tb, decl)
	if body == nil {
		return s
	}
	var found []string
	for i := range body.NamedChildCount() {
		member := body.NamedChild(i)
		name, ok := methodOf(member, s.src)
		if !ok {
			continue
		}
		found = append(found, name)
		if name == method {
			return s
		}
	}
	tb.Errorf("typescripttest: %s declares no %s.%s; %s declares %v",
		s.path, decl, method, decl, found)
	return s
}

// AssertNoMethod fails when the named declaration has a method of
// that name.
func (s *Source) AssertNoMethod(tb testing.TB, decl, method string) *Source {
	tb.Helper()
	body := s.bodyOf(tb, decl)
	if body == nil {
		return s
	}
	for i := range body.NamedChildCount() {
		name, ok := methodOf(body.NamedChild(i), s.src)
		if ok && name == method {
			tb.Errorf("typescripttest: %s declares %s.%s, which it must not",
				s.path, decl, method)
			return s
		}
	}
	return s
}

// AssertSignature fails when the named method's whole signature does
// not read as want, whitespace collapsed.
//
// The assertion a dropped rest marker or a swapped parameter fails: a
// double taking one value where the contract takes many still
// declares the method, and only the signature says so.
func (s *Source) AssertSignature(tb testing.TB, decl, method, want string) *Source {
	tb.Helper()
	body := s.bodyOf(tb, decl)
	if body == nil {
		return s
	}
	for i := range body.NamedChildCount() {
		member := body.NamedChild(i)
		name, ok := methodOf(member, s.src)
		if !ok || name != method {
			continue
		}
		got := strings.TrimSuffix(strings.TrimSpace(s.text(member)), ";")
		if collapse(got) == collapse(want) {
			return s
		}
		tb.Errorf("typescripttest: %s declares %s.%s as %q, want %q",
			s.path, decl, method, got, want)
		return s
	}
	tb.Errorf("typescripttest: %s declares no %s.%s", s.path, decl, method)
	return s
}

// AssertExtends fails when the named declaration does not extend the
// given type.
func (s *Source) AssertExtends(tb testing.TB, decl, typeExpr string) *Source {
	tb.Helper()
	return s.assertHeritage(tb, decl, typeExpr, "extends")
}

// AssertImplements fails when the named class does not implement the
// given type.
//
// Distinct from [Source.AssertExtends] because the two do different
// things: extending inherits members, implementing only asserts they
// are present. A generator that emitted one where the plugin meant
// the other produces a class whose members come from nowhere.
func (s *Source) AssertImplements(tb testing.TB, decl, typeExpr string) *Source {
	tb.Helper()
	return s.assertHeritage(tb, decl, typeExpr, "implements")
}

// AssertMembers fails when the named enum's members are not exactly
// those given, in order.
func (s *Source) AssertMembers(tb testing.TB, decl string, want ...string) *Source {
	tb.Helper()
	body := s.bodyOf(tb, decl)
	if body == nil {
		return s
	}
	var got []string
	for i := range body.NamedChildCount() {
		member := body.NamedChild(i)
		name := member.ChildByFieldName("name")
		if name == nil {
			// A bare member is the identifier itself rather than an
			// assignment with a name field.
			got = append(got, s.text(member))
			continue
		}
		got = append(got, s.text(name))
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		tb.Errorf("typescripttest: %s declares %s with members %v, want %v",
			s.path, decl, got, want)
	}
	return s
}

// Source returns the file the declaration was found in, so a chain
// that narrowed to one declaration can widen again without the caller
// holding both.
func (d *Decl) Source() *Source { return d.src }

// AssertDoc fails when the declaration's JSDoc block does not contain
// substr.
//
// A generated declaration's documentation is what a consumer's editor
// shows, and a generator that carries a source doc comment forward is
// making a claim about it that nothing else checks.
func (d *Decl) AssertDoc(tb testing.TB, substr string) *Decl {
	tb.Helper()
	if d.node == nil {
		return d
	}
	if doc := d.docText(); !strings.Contains(doc, substr) {
		tb.Errorf("typescripttest: %s's doc comment on %s does not contain %q; it reads:\n%s",
			d.src.path, d.name, substr, doc)
	}
	return d
}

// AssertDocLacks fails when the declaration's JSDoc block contains
// substr.
//
// The direction a generator projecting a source comment needs: a doc
// comment carried forward verbatim brings the source's `@internal`,
// its `TODO`, and its reference to a symbol the generated module does
// not export — each of which reads as a statement about the generated
// declaration and is false.
func (d *Decl) AssertDocLacks(tb testing.TB, substr string) *Decl {
	tb.Helper()
	if d.node == nil {
		return d
	}
	if doc := d.docText(); strings.Contains(doc, substr) {
		tb.Errorf("typescripttest: %s's doc comment on %s contains %q, which it must "+
			"not; it reads:\n%s", d.src.path, d.name, substr, doc)
	}
	return d
}

// Signature fails when the declaration's own signature does not read
// as want, whitespace collapsed.
//
// For a function or a generic type, where what a consumer binds to is
// the whole head rather than any one member — and where a substring
// check passes against a parameter list that gained one.
func (d *Decl) Signature(tb testing.TB, want string) *Decl {
	tb.Helper()
	if d.node == nil {
		tb.Errorf("typescripttest: %s declares no %s, so it has no signature to check",
			d.src.path, d.name)
		return d
	}
	got := headOf(d.src, d.node)
	if collapse(got) != collapse(want) {
		tb.Errorf("typescripttest: %s declares %s as %q, want %q",
			d.src.path, d.name, got, want)
	}
	return d
}

// docText returns the JSDoc block above the declaration, stripped of
// its markers.
func (d *Decl) docText() string {
	lines := strings.Split(string(d.src.src), "\n")
	row := int(outermost(d.node).StartPosition().Row)

	var block []string
	for i := row - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			break
		}
		block = append([]string{line}, block...)
		if strings.HasPrefix(line, "/**") {
			break
		}
		if !strings.HasPrefix(line, "*") && !strings.HasSuffix(line, "*/") {
			return ""
		}
	}
	if len(block) == 0 || !strings.HasPrefix(block[0], "/**") {
		return ""
	}

	var out []string
	for _, line := range block {
		line = strings.TrimPrefix(line, "/**")
		line = strings.TrimSuffix(line, "*/")
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "*"))
		out = append(out, line)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

// headOf renders a declaration's head — everything before its body.
func headOf(s *Source, n *ts.Node) string {
	text := s.text(n)
	if body := n.ChildByFieldName("body"); body != nil {
		if idx := int(body.StartByte() - n.StartByte()); idx > 0 && idx <= len(text) {
			return strings.TrimSpace(text[:idx])
		}
	}
	return strings.TrimSuffix(strings.TrimSpace(text), ";")
}

// assertNoDeclOfKind fails when the file declares something of that
// name *and* kind.
func (s *Source) assertNoDeclOfKind(
	tb testing.TB, what, name string, kinds ...string,
) *Source {
	tb.Helper()
	for _, n := range s.topLevel() {
		if declName(n, s.src) == name && slices.Contains(kinds, n.Kind()) {
			tb.Errorf("typescripttest: %s declares the %s %s, which it must not",
				s.path, what, name)
			return s
		}
	}
	return s
}

// assertDecl finds a top-level declaration of one of the given kinds.
func (s *Source) assertDecl(tb testing.TB, what, name string, kinds ...string) *Decl {
	tb.Helper()
	var wrongKind string
	for _, n := range s.topLevel() {
		if declName(n, s.src) != name {
			continue
		}
		if slices.Contains(kinds, n.Kind()) {
			return &Decl{src: s, node: n, name: name}
		}
		wrongKind = n.Kind()
	}
	if wrongKind != "" {
		tb.Errorf("typescripttest: %s declares %s as a %s, not as a(n) %s",
			s.path, name, wrongKind, what)
		return &Decl{src: s, name: name}
	}
	tb.Errorf("typescripttest: %s declares no %s %s; it declares %v",
		s.path, what, name, s.DeclNames())
	return &Decl{src: s, name: name}
}

// assertHeritage checks one heritage clause on a declaration.
func (s *Source) assertHeritage(tb testing.TB, decl, typeExpr, clause string) *Source {
	tb.Helper()
	n := s.declNode(decl)
	if n == nil {
		tb.Errorf("typescripttest: %s declares no %s; it declares %v",
			s.path, decl, s.DeclNames())
		return s
	}
	got := heritageOf(n, s.src, clause)
	for _, entry := range got {
		// Matched on the whole spelling or on the bare name, so a test
		// asserting `extends Base` need not know whether the generator
		// applied type arguments — and one that cares still pins
		// `Base<string>`.
		if collapse(entry) == collapse(typeExpr) || bareName(entry) == collapse(typeExpr) {
			return s
		}
	}
	tb.Errorf("typescripttest: %s declares %s %s %v, want %s among them",
		s.path, decl, clause, got, typeExpr)
	return s
}

// bodyOf returns a declaration's member list, reporting when the
// declaration itself is missing.
func (s *Source) bodyOf(tb testing.TB, decl string) *ts.Node {
	tb.Helper()
	n := s.declNode(decl)
	if n == nil {
		tb.Errorf("typescripttest: %s declares no %s; it declares %v",
			s.path, decl, s.DeclNames())
		return nil
	}
	if body := n.ChildByFieldName("body"); body != nil {
		return body
	}
	tb.Errorf("typescripttest: %s declares %s with no body", s.path, decl)
	return nil
}

// declNode returns the top-level declaration of that name, or nil.
func (s *Source) declNode(name string) *ts.Node {
	for _, n := range s.topLevel() {
		if declName(n, s.src) == name {
			return n
		}
	}
	return nil
}

// declName returns the identifier a top-level statement declares,
// empty for one that declares nothing named.
//
// A binding is the odd one: `const MAX = 1` wraps its name in a
// declarator, so the name field sits one level down.
func declName(n *ts.Node, src []byte) string {
	if n == nil {
		return ""
	}
	switch n.Kind() {
	case "lexical_declaration", "variable_declaration":
		d := n.NamedChild(0)
		if d == nil {
			return ""
		}
		name := d.ChildByFieldName("name")
		// A destructuring pattern binds several names at once, and the
		// node holding it is the whole pattern. Reporting `{ a, b }` as
		// one declaration would put a spelling no import can name into
		// every failure message that lists what the file declares, so
		// only a plain identifier counts.
		if name == nil || name.Kind() != "identifier" {
			return ""
		}
		return name.Utf8Text(src)
	case kindExport, "import_statement":
		// The child carries the name; topLevel already unwrapped it.
		return ""
	}
	if name := n.ChildByFieldName("name"); name != nil {
		return name.Utf8Text(src)
	}
	return ""
}

// propertyOf returns a member's name and rendered type, reporting
// false for a member that is not a property.
func propertyOf(n *ts.Node, src []byte) (name, typeExpr string, ok bool) {
	if n == nil {
		return "", "", false
	}
	switch n.Kind() {
	case "property_signature", "public_field_definition", "field_definition":
	default:
		return "", "", false
	}
	nameNode := n.ChildByFieldName("name")
	if nameNode == nil {
		return "", "", false
	}
	// The annotation node carries its own colon, which a caller
	// comparing against `string` does not want.
	annotation := n.ChildByFieldName("type")
	if annotation == nil {
		return nameNode.Utf8Text(src), "", true
	}
	return nameNode.Utf8Text(src),
		strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(annotation.Utf8Text(src)), ":")),
		true
}

// methodOf returns a member's name, reporting false for a member that
// is not a method.
func methodOf(n *ts.Node, src []byte) (string, bool) {
	if n == nil {
		return "", false
	}
	switch n.Kind() {
	case "method_signature", "method_definition", "abstract_method_signature":
	default:
		return "", false
	}
	name := n.ChildByFieldName("name")
	if name == nil {
		return "", false
	}
	return name.Utf8Text(src), true
}

// heritageOf returns the types a declaration names in one heritage
// clause.
//
// An interface writes `extends_type_clause` directly; a class wraps
// both clauses in `class_heritage`. Walking whichever is present
// answers for both without the caller knowing which kind it holds.
func heritageOf(n *ts.Node, src []byte, clause string) []string {
	var out []string
	var walk func(*ts.Node)
	walk = func(cur *ts.Node) {
		if cur == nil {
			return
		}
		kind := cur.Kind()
		if kind == "extends_type_clause" || kind == "extends_clause" || kind == "implements_clause" {
			if !strings.HasPrefix(kind, clause) {
				return
			}
			for i := range cur.NamedChildCount() {
				child := cur.NamedChild(i)
				if child.Kind() == "type_arguments" {
					continue
				}
				out = append(out, child.Utf8Text(src))
			}
			return
		}
		if kind != "class_heritage" && cur.ChildByFieldName("body") != nil {
			// Stop at the declaration's body; heritage precedes it and
			// descending further would read a nested type's clauses.
			for i := range cur.NamedChildCount() {
				if child := cur.NamedChild(i); child != cur.ChildByFieldName("body") {
					walk(child)
				}
			}
			return
		}
		for i := range cur.NamedChildCount() {
			walk(cur.NamedChild(i))
		}
	}
	walk(n)
	return out
}

// collapse reduces runs of whitespace to one space, so a comparison
// against a written-out type survives however the renderer spaced it.
func collapse(s string) string { return strings.Join(strings.Fields(s), " ") }

// bareName strips a heritage entry's type arguments.
func bareName(entry string) string {
	name, _, _ := strings.Cut(entry, "<")
	return collapse(name)
}
