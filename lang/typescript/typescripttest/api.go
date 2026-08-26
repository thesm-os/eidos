// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescripttest

import (
	"slices"
	"strings"
	"testing"

	ts "github.com/tree-sitter/go-tree-sitter"

	"go.thesmos.sh/eidos/eidostest/pipelinetest"
)

// API renders the file's exported surface: every exported
// declaration and its shape, sorted, without bodies, comments or
// formatting.
//
// # Why this exists beside a byte golden
//
// A byte golden answers "what does this file read like" and churns on
// every comment reflow, so a template change produces two hundred
// changed lines and the review becomes a formality. A substring
// assertion answers "is this one construct present" and says nothing
// about the two hundred lines around it.
//
// What a consumer actually depends on is the exported surface, and a
// golden over that changes only when their code would have to. Adding
// a property is one added line; renaming a parameter is nothing at
// all; changing a signature is one line replaced. That is a diff a
// reviewer reads rather than scrolls.
//
// Deliberately excluded: unexported declarations, doc comments,
// bodies, and member order within a declaration — none of which a
// consumer importing the module can observe. Members are sorted so a
// generator that reorders them without changing what it offers
// produces no diff.
//
// Imports are included, unlike Go's rendering of the same idea: a
// TypeScript module's imports *are* part of what a consumer installs,
// because an added specifier is a package their project must resolve.
func (s *Source) API() string {
	var lines []string
	for _, spec := range s.Imports() {
		lines = append(lines, "import "+spec)
	}
	for _, n := range s.topLevel() {
		lines = append(lines, apiLines(s, n)...)
	}
	slices.Sort(lines)
	lines = slices.Compact(lines)
	return strings.Join(lines, "\n") + "\n"
}

// AssertAPIGolden compares the file's exported surface against a
// golden file, rewriting it when the test binary is run with
// `-update-golden`.
//
// The assertion to reach for where a byte golden would churn. It
// fails when a consumer's code would have had to change and stays
// quiet otherwise, which is what makes it worth reading in review.
func AssertAPIGolden(tb testing.TB, s *Source, goldenPath string) *Source {
	tb.Helper()
	pipelinetest.MatchesGoldenBytes(tb, []byte(s.API()), goldenPath)
	return s
}

// apiLines renders one declaration's exported surface, empty for one
// a consumer cannot see.
func apiLines(s *Source, n *ts.Node) []string {
	if !exported(n) {
		return nil
	}
	name := declName(n, s.src)
	if name == "" {
		return nil
	}
	switch n.Kind() {
	case "interface_declaration", "class_declaration":
		return hostAPI(s, n, name)
	case "type_alias_declaration":
		return []string{"type " + name + s.text(n.ChildByFieldName("type_parameters"))}
	case "enum_declaration":
		return enumAPI(s, n, name)
	case "function_declaration", "function_signature":
		return []string{"function " + signatureOf(s, n, name)}
	case "lexical_declaration", "variable_declaration":
		return []string{"const " + name + annotationOf(s, bindingDeclarator(n))}
	default:
		return nil
	}
}

// hostAPI renders an interface or class and its members.
//
// One line per member rather than one per declaration, so a diff
// names the member that changed rather than reprinting the whole
// type. The kind is dropped from the member lines because a consumer
// reads `User.id` the same way whether the declaration was an
// interface or a class.
func hostAPI(s *Source, n *ts.Node, name string) []string {
	head := "interface"
	if n.Kind() == "class_declaration" {
		head = "class"
	}
	out := []string{head + " " + name + s.text(n.ChildByFieldName("type_parameters"))}
	for _, clause := range []string{"extends", "implements"} {
		for _, entry := range heritageOf(n, s.src, clause) {
			out = append(out, name+" "+clause+" "+collapse(entry))
		}
	}

	body := n.ChildByFieldName("body")
	if body == nil {
		return out
	}
	for i := range body.NamedChildCount() {
		member := body.NamedChild(i)
		if !visibleMember(s, member) {
			continue
		}
		if prop, typ, ok := propertyOf(member, s.src); ok {
			out = append(out, name+"."+prop+optionalMark(member)+": "+collapse(typ))
			continue
		}
		if method, ok := methodOf(member, s.src); ok {
			out = append(out, name+"."+collapse(signatureOf(s, member, method)))
		}
	}
	return out
}

// enumAPI renders an enum and its members.
//
// The declared values as well as the names: an enum's members are the
// values a consumer compares against and serialises, so changing
// `Admin = 'admin'` to `Admin = 'ADMIN'` breaks them exactly as
// removing the member would.
func enumAPI(s *Source, n *ts.Node, name string) []string {
	out := []string{"enum " + name}
	body := n.ChildByFieldName("body")
	if body == nil {
		return out
	}
	for i := range body.NamedChildCount() {
		out = append(out, name+"."+collapse(s.text(body.NamedChild(i))))
	}
	return out
}

// visibleMember reports whether a consumer holding a value of the
// declaration can reach the member.
//
// A `private` or `protected` member is not part of what the module
// offers — the whole point of the modifier — so including it would
// make the golden churn on a change no consumer can observe, which is
// the churn this rendering exists to remove. A `#name` field is
// private at runtime and reads as one to the grammar rather than
// through a modifier, so both spellings are checked.
func visibleMember(s *Source, member *ts.Node) bool {
	if name := member.ChildByFieldName("name"); name != nil {
		if name.Kind() == "private_property_identifier" {
			return false
		}
	}
	for i := range member.NamedChildCount() {
		if member.NamedChild(i).Kind() != "accessibility_modifier" {
			continue
		}
		if level := s.text(member.NamedChild(i)); level != "public" {
			return false
		}
	}
	return true
}

// signatureOf renders a callable's name, generics, parameters and
// return.
func signatureOf(s *Source, n *ts.Node, name string) string {
	var b strings.Builder
	b.WriteString(name)
	b.WriteString(s.text(n.ChildByFieldName("type_parameters")))
	b.WriteString(s.text(n.ChildByFieldName("parameters")))
	b.WriteString(annotationOf(s, n))
	return collapse(b.String())
}

// annotationOf renders a node's `: T`, empty for one with no declared
// type.
//
// A callable records its annotation under `return_type` and a binding
// under `type`, so both are asked for — the arm a harness omits
// silently reports every signature as returning nothing.
func annotationOf(s *Source, n *ts.Node) string {
	if n == nil {
		return ""
	}
	for _, field := range []string{"return_type", "type"} {
		annotation := n.ChildByFieldName(field)
		if annotation == nil {
			continue
		}
		// The node carries its own colon, and the surface spells it
		// the way the source does — `f(): T`, not `f() : T`.
		text := collapse(s.text(annotation))
		return ": " + strings.TrimSpace(strings.TrimPrefix(text, ":"))
	}
	return ""
}

// optionalMark renders the `?` on a member that carries one.
func optionalMark(n *ts.Node) string {
	for i := range n.ChildCount() {
		if n.Child(i).Kind() == "?" {
			return "?"
		}
	}
	return ""
}

// bindingDeclarator returns the declarator inside a `const` or `let`,
// which is where its name and type sit.
func bindingDeclarator(n *ts.Node) *ts.Node {
	if n == nil {
		return nil
	}
	return n.NamedChild(0)
}

// exported reports whether a consumer importing the module can see
// the declaration.
//
// True for anything the walk reached through an `export`, which is
// what [Source.topLevel] preserves the wrapper for: a declaration and
// its export statement both appear, so a member of the second is
// exported and a bare one at the top level is not.
func exported(n *ts.Node) bool {
	for cur := n.Parent(); cur != nil; cur = cur.Parent() {
		if _, wrapper := wrappers[cur.Kind()]; !wrapper {
			continue
		}
		if cur.Kind() == kindExport {
			return true
		}
	}
	return false
}
