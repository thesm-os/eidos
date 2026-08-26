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
