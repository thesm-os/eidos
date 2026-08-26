// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"strings"
	"text/template"

	"go.thesmos.sh/eidos/core/naming"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/typescript"
)

// indent is one level of TypeScript indentation.
//
// Two spaces, not a tab. Every formatter the ecosystem uses defaults
// to spaces for TypeScript, and this has to be one value because
// byte-identical output admits no second one.
const indent = "  "

// funcMap returns the template helpers bound to this render state.
//
// Bound per state rather than shared, because the type helpers
// register imports as a side effect: a shared funcmap would record
// one target's imports against another's file.
func (s *renderState) funcMap() template.FuncMap {
	return template.FuncMap{
		// Dispatch.
		"render":     s.render,
		"renderType": s.renderType,

		// Declaration parts.
		"renderDocs":       renderDocs,
		"renderMembers":    s.renderMembers,
		"renderMethods":    s.renderMethods,
		"renderParams":     s.renderParams,
		"renderReturn":     s.renderReturn,
		"renderTypeParams": s.renderTypeParams,
		"renderHeritage":   s.renderHeritage,
		"renderVariants":   s.renderVariants,

		// Spelling.
		"quote":    typescript.Quote,
		"ident":    typescript.Ident,
		"propKey":  typescript.PropertyKey,
		"exported": exportPrefix,
		"indent":   indentBlock,

		// Declaration-level metadata.
		"abstractKw":    abstractPrefix,
		"constKw":       constEnumPrefix,
		"signatures":    verbatimSignatures,
		"renderInit":    s.renderInit,
		"annotation":    s.annotation,
		"overloadLines": s.overloadLines,

		// Case conversion, for a template deriving a name.
		"camel":  naming.Camel,
		"pascal": naming.Pascal,
		"scream": naming.ScreamingSnake,

		// Metadata, string-keyed because templates are text.
		"meta":     metaString,
		"metaBool": metaBool,
	}
}

// renderDocs renders a doc comment as a JSDoc block.
//
// JSDoc rather than `//` lines: an editor surfaces a `/** */` block
// on hover and ignores a line comment, so a generated type documented
// with `//` is documented only for someone reading the file.
//
// A single line stays on one line, which is what a human writes for a
// one-sentence doc and what every formatter leaves alone.
func renderDocs(lines []string) string {
	trimmed := trimBlank(lines)
	switch len(trimmed) {
	case 0:
		return ""
	case 1:
		return "/** " + trimmed[0] + " */\n"
	}

	var b strings.Builder
	b.WriteString("/**\n")
	for _, l := range trimmed {
		if l == "" {
			b.WriteString(" *\n")
			continue
		}
		b.WriteString(" * " + l + "\n")
	}
	b.WriteString(" */\n")
	return b.String()
}

// trimBlank drops leading and trailing blank lines, which carry no
// content and would render as empty JSDoc rows.
func trimBlank(lines []string) []string {
	start, end := 0, len(lines)
	for start < end && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	for end > start && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return lines[start:end]
}

// exportPrefix returns the `export ` keyword for a declaration that
// leaves its module, or empty for one that does not.
//
// Every declaration a generator emits is exported unless it says
// otherwise: a generated type nothing can import is a type nothing
// can use, so absence of the marker means exported rather than not.
func exportPrefix(n emit.Node) string {
	if n == nil {
		return "export "
	}
	if exported, ok := typescript.MetaExported.Get(n.Meta()); ok && !exported {
		return ""
	}
	return "export "
}

// indentBlock indents every non-empty line of body by one level.
//
// Blank lines are left empty rather than filled with spaces, because
// trailing whitespace is what the normaliser would strip anyway and
// every formatter treats as an error.
func indentBlock(body string) string {
	if body == "" {
		return ""
	}
	lines := strings.Split(strings.TrimRight(body, "\n"), "\n")
	for i, l := range lines {
		if strings.TrimSpace(l) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = indent + l
	}
	return strings.Join(lines, "\n")
}

// metaString reads a string-valued metadata key by name.
func metaString(n emit.Node, key string) string {
	if n == nil {
		return ""
	}
	raw, ok := n.Meta().RawValue(key)
	if !ok {
		return ""
	}
	s, _ := raw.(string)
	return s
}

// metaBool reads a bool-valued metadata key by name.
func metaBool(n emit.Node, key string) bool {
	if n == nil {
		return false
	}
	raw, ok := n.Meta().RawValue(key)
	if !ok {
		return false
	}
	b, _ := raw.(bool)
	return b
}

// abstractPrefix returns the `abstract ` keyword for a class stamped
// so, empty otherwise.
func abstractPrefix(n emit.Node) string {
	if abstract, _ := typescript.MetaAbstract.Get(n.Meta()); abstract {
		return "abstract "
	}
	return ""
}

// constEnumPrefix returns the `const ` keyword for an enum stamped
// [typescript.MetaConstEnum].
//
// A const enum is inlined at every use site and emits no runtime
// object, which is a different contract from the same members in an
// ordinary enum — a consumer iterating the object at run time gets
// nothing.
func constEnumPrefix(n emit.Node) string {
	if c, _ := typescript.MetaConstEnum.Get(n.Meta()); c {
		return "const "
	}
	return ""
}

// verbatimSignatures renders an interface's index and construct
// signatures, one line each.
//
// Both are carried in verbatim source form because the model has no
// variant for either: an index signature declares the shape of any
// key rather than a named member, and a construct signature is a
// callable with no name. Writing the text back out is the only
// faithful treatment.
func verbatimSignatures(n emit.Node) string {
	var b strings.Builder
	if sig, ok := typescript.MetaIndexSignature.Get(n.Meta()); ok && sig != "" {
		b.WriteString(sig + ";\n")
	}
	if sig, ok := typescript.MetaConstructSignature.Get(n.Meta()); ok && sig != "" {
		b.WriteString(sig + ";\n")
	}
	return b.String()
}

// renderInit spells a variable or constant's initialiser as ` = expr`,
// empty for a declaration that carries none.
//
// The structured expression wins over the verbatim stamp: a generator
// that built an [emit.Expr] said exactly what it meant, while
// [typescript.MetaInitialiser] is what a bridge copies from source.
// Either way the template drops `declare` for the initialised form —
// an ambient declaration admits no initialiser, and a value this
// backend can spell (a literal, an identifier, a reference) is not
// the runtime code the ambient spelling exists to avoid.
func (s *renderState) renderInit(n emit.Node) (string, error) {
	var expr *emit.Expr
	switch decl := n.(type) {
	case *emit.Variable:
		expr = decl.Init
	case *emit.Constant:
		expr = decl.Value
	}
	if expr != nil {
		got, err := s.renderExpr(expr)
		if err != nil {
			return "", err
		}
		if got != "" {
			return " = " + got, nil
		}
	}
	if text, ok := typescript.MetaInitialiser.Get(n.Meta()); ok && text != "" {
		return " = " + text, nil
	}
	return "", nil
}

// annotation spells `: T`, empty for a declaration whose type is
// inferred.
//
// A helper rather than the template writing the colon, because a
// binding with an initialiser may legitimately carry no declared type
// — `export const MAX = 100;` — and a template spelling `: ` around
// an empty rendering emits an annotation with no type in it.
func (s *renderState) annotation(ref emit.Ref) (string, error) {
	got, err := s.renderType(ref)
	if err != nil || got == "" {
		return "", err
	}
	return ": " + got, nil
}

// overloadLines renders a function's overload signatures as whole
// lines, nil for a function that declares none — which is what lets
// the template's range/else fall through to the derived signature.
//
// Each line carries its own export and declare keywords, because
// TypeScript's function overloads are sibling declarations rather
// than members of one: `declare function f(a: string): void;` twice
// is the form, and a caller resolves against them top-down in source
// order.
func (*renderState) overloadLines(n emit.Node) []string {
	overloads, ok := typescript.MetaOverloads.Get(n.Meta())
	if !ok || len(overloads) == 0 {
		return nil
	}
	prefix := exportPrefix(n) + "declare function "
	out := make([]string, 0, len(overloads))
	for _, o := range overloads {
		out = append(out, prefix+o.Text+";")
	}
	return out
}
