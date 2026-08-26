// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescripttest

import (
	"bytes"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"testing"

	ts "github.com/tree-sitter/go-tree-sitter"

	"go.thesmos.sh/eidos/eidostest/pipelinetest"
)

// Source is one parsed generated file.
//
// Parsed once and reused across assertions, because a generated file
// runs to hundreds of lines and a test asserting a dozen things about
// it should not pay for a dozen parses.
//
// The tree is C-allocated and never freed: a Source lives for one
// test and the process ends shortly after. Closing it would mean
// either a Close every caller must remember or a finaliser, and both
// buy back memory the test binary was about to release anyway.
type Source struct {
	tree *ts.Tree
	src  []byte
	path string
}

// Parse reads a file into a [Source], failing the test when
// tree-sitter cannot produce a tree at all.
//
// Syntax errors do not fail here — tree-sitter recovers from them and
// records them in the tree, so a test can still ask structural
// questions of a file it knows is broken. [Source.AssertParses] is
// the assertion that the file is clean.
func Parse(tb testing.TB, f File) *Source {
	tb.Helper()
	tree := parseTree(f.Path, f.Src)
	if tree == nil {
		tb.Fatalf("typescripttest: the parser returned no tree for %s\n%s",
			f.Path, numbered(f.Src))
		return nil
	}
	return &Source{tree: tree, src: f.Src, path: f.Path}
}

// Path returns the file's path, for a caller composing its own
// failure message.
func (s *Source) Path() string { return s.path }

// Bytes returns a copy of the file's content.
func (s *Source) Bytes() []byte { return slices.Clone(s.src) }

// Dump logs the file with line numbers, for a test being debugged.
func (s *Source) Dump(tb testing.TB) *Source {
	tb.Helper()
	tb.Logf("typescripttest: %s\n%s", s.path, numbered(s.src))
	return s
}

// AssertParses fails when the file contains a syntax error.
//
// The floor every other assertion stands on: a substring check passes
// against a template that renders an unclosed brace or a stray comma,
// and a structural query over a broken tree finds whatever
// tree-sitter recovered rather than what the generator meant.
//
// The failure prints the offending line with its neighbours, because
// the position it names is in generated code the author never wrote
// and cannot open.
func (s *Source) AssertParses(tb testing.TB) *Source {
	tb.Helper()
	bad := firstError(s.root())
	if bad == nil {
		return s
	}
	line := int(bad.StartPosition().Row) + 1
	tb.Errorf("typescripttest: %s is not valid TypeScript — %s at line %d:\n%s",
		s.path, bad.Kind(), line, lineContext(s.src, line))
	return s
}

// AssertGeneratedHeader fails when the file does not open with the
// machine-made marker.
//
// The line every tool that skips generated files looks for. A
// generator that stopped emitting it starts having its output
// reformatted by the consumer's own tooling, linted as hand-written,
// and shown in review diffs.
func (s *Source) AssertGeneratedHeader(tb testing.TB) *Source {
	tb.Helper()
	first := strings.TrimSpace(firstLine(s.src))
	if !strings.Contains(first, "Code generated") || !strings.Contains(first, "DO NOT EDIT") {
		tb.Errorf("typescripttest: %s opens with %q, which no generated-file check will "+
			"recognise", s.path, first)
	}
	return s
}

// Imports returns every module specifier the file imports from, in
// source order.
func (s *Source) Imports() []string {
	var out []string
	for _, n := range s.topLevel() {
		if spec := importSpecifier(n, s.src); spec != "" {
			out = append(out, spec)
		}
	}
	return out
}

// AssertImports fails when any of the named specifiers is absent.
func (s *Source) AssertImports(tb testing.TB, specifiers ...string) *Source {
	tb.Helper()
	got := s.Imports()
	for _, want := range specifiers {
		if !slices.Contains(got, want) {
			tb.Errorf("typescripttest: %s does not import from %q; it imports from %v",
				s.path, want, got)
		}
	}
	return s
}

// AssertNoImport fails when the named specifier is present.
//
// The assertion a self-import bug fails: a module importing itself is
// legal TypeScript that resolves to a cycle, and the only sign is
// that the binding is undefined at run time.
func (s *Source) AssertNoImport(tb testing.TB, specifier string) *Source {
	tb.Helper()
	if slices.Contains(s.Imports(), specifier) {
		tb.Errorf("typescripttest: %s imports from %q, which it must not", s.path, specifier)
	}
	return s
}

// AssertImportsOnly fails when the file imports from anything other
// than exactly the named specifiers.
//
// Stronger than [Source.AssertImports] and worth it where a generator
// composes its import block from what it spelled: an extra specifier
// means it registered an import for a type it did not emit, which
// fails under noUnusedLocals in the consumer's build and nowhere
// here.
func (s *Source) AssertImportsOnly(tb testing.TB, specifiers ...string) *Source {
	tb.Helper()
	got := slices.Clone(s.Imports())
	want := slices.Clone(specifiers)
	slices.Sort(got)
	slices.Sort(want)
	if !slices.Equal(got, want) {
		tb.Errorf("typescripttest: %s imports from %v, want exactly %v", s.path, got, want)
	}
	return s
}

// AssertContains fails when substr does not appear anywhere in the
// file.
//
// The blunt assertion, kept for the cases the structural ones cannot
// reach — a rendered expression, a comment's wording. Reach for
// [Source.AssertProperty] and its siblings first: they survive a
// change in spacing and say what is actually wrong when they fail.
func (s *Source) AssertContains(tb testing.TB, substr string) *Source {
	tb.Helper()
	if !strings.Contains(string(s.src), substr) {
		tb.Errorf("typescripttest: %s does not contain %q:\n%s",
			s.path, substr, numbered(s.src))
	}
	return s
}

// AssertNotContains fails when substr appears anywhere in the file.
func (s *Source) AssertNotContains(tb testing.TB, substr string) *Source {
	tb.Helper()
	if idx := bytes.Index(s.src, []byte(substr)); idx >= 0 {
		tb.Errorf("typescripttest: %s contains %q at line %d, which it must not:\n%s",
			s.path, substr, lineAt(s.src, idx), numbered(s.src))
	}
	return s
}

// AssertGolden compares the file against a golden file, rewriting it
// when the test binary is run with `-update-golden`.
//
// The assertion for output whose whole shape is the subject —
// formatting, ordering, blank lines. Everything else should be a
// structural assertion, because a golden file fails on every change
// and says only that something differs.
//
// Delegated to [pipelinetest.MatchesGoldenBytes] rather than
// reimplemented, so one flag drives every golden corpus in the
// workspace: a repository updating its fixtures runs one command
// rather than learning which harness owns which file.
func (s *Source) AssertGolden(tb testing.TB, goldenPath string) *Source {
	tb.Helper()
	pipelinetest.MatchesGoldenBytes(tb, s.src, goldenPath)
	return s
}

// DeclNames returns every top-level declaration's name, in source
// order.
func (s *Source) DeclNames() []string {
	var out []string
	for _, n := range s.topLevel() {
		if name := declName(n, s.src); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// AssertOrder fails when first does not appear before second.
//
// Declaration order carries no meaning to the compiler — TypeScript
// hoists types — so this is about the reader. A generator emitting a
// sorted surface that stopped sorting produces a diff on every run
// against an unchanged source.
func (s *Source) AssertOrder(tb testing.TB, first, second string) *Source {
	tb.Helper()
	names := s.DeclNames()
	a, b := slices.Index(names, first), slices.Index(names, second)
	switch {
	case a < 0:
		tb.Errorf("typescripttest: %s declares no %s; it declares %v", s.path, first, names)
	case b < 0:
		tb.Errorf("typescripttest: %s declares no %s; it declares %v", s.path, second, names)
	case a > b:
		tb.Errorf("typescripttest: %s declares %s after %s; order is %v",
			s.path, first, second, names)
	}
	return s
}

// AssertOrderAll fails when the named declarations do not appear in
// the order given. Declarations not named are ignored.
func (s *Source) AssertOrderAll(tb testing.TB, names ...string) *Source {
	tb.Helper()
	for i := 1; i < len(names); i++ {
		s.AssertOrder(tb, names[i-1], names[i])
	}
	return s
}

// root returns the tree's root node.
func (s *Source) root() *ts.Node { return s.tree.RootNode() }

// text returns n's source text.
func (s *Source) text(n *ts.Node) string {
	if n == nil {
		return ""
	}
	return n.Utf8Text(s.src)
}

// wrappers are the statements that carry a declaration rather than
// being one.
//
// `export declare class C` nests both, which is why unwrapping is a
// walk rather than one step: a harness that stripped only the export
// would find an ambient_declaration where an assertion asked for a
// class, and report the class absent from a file that declares it.
var wrappers = map[string]struct{}{ //nolint:gochecknoglobals // immutable lookup table
	kindExport:            {},
	"ambient_declaration": {},
}

// kindExport is the statement that wraps most generated declarations.
const kindExport = "export_statement"

// topLevel returns every statement at the file's top level, with the
// declarations inside each wrapper alongside it.
//
// The wrapper is kept as well as its contents, so an assertion about
// the export itself — a re-export naming its specifier — still has a
// node to read.
func (s *Source) topLevel() []*ts.Node {
	root := s.root()
	out := make([]*ts.Node, 0, root.NamedChildCount())
	var add func(*ts.Node)
	add = func(n *ts.Node) {
		out = append(out, n)
		if _, wrapper := wrappers[n.Kind()]; !wrapper {
			return
		}
		for i := range n.NamedChildCount() {
			add(n.NamedChild(i))
		}
	}
	for i := range root.NamedChildCount() {
		add(root.NamedChild(i))
	}
	return out
}

// numbered renders src with line numbers, so a failure naming a line
// in a file the author cannot open is still readable.
func numbered(src []byte) string {
	var b strings.Builder
	for i, line := range strings.Split(strings.TrimRight(string(src), "\n"), "\n") {
		fmt.Fprintf(&b, "%4d | %s\n", i+1, line)
	}
	return b.String()
}

// lineContext renders the lines around one, for a failure that
// already knows where it is.
func lineContext(src []byte, line int) string {
	lines := strings.Split(strings.TrimRight(string(src), "\n"), "\n")
	lo, hi := max(0, line-3), min(len(lines), line+2)

	var b strings.Builder
	for i := lo; i < hi; i++ {
		marker := " "
		if i+1 == line {
			marker = ">"
		}
		fmt.Fprintf(&b, "%s %4d | %s\n", marker, i+1, lines[i])
	}
	return b.String()
}

// firstLine returns src's first line.
func firstLine(src []byte) string {
	line, _, _ := strings.Cut(string(src), "\n")
	return line
}

// lineAt returns the 1-based line a byte offset falls on.
func lineAt(src []byte, offset int) int {
	return strings.Count(string(src[:offset]), "\n") + 1
}

// importSpecifier returns the module a statement imports from, empty
// for a statement that is not an import or re-export.
//
// Both forms carry the specifier as a `string` child, so one lookup
// answers `import { X } from './y'` and `export { X } from './y'`
// alike — which is what a test asking "does this file reach for that
// module" means either way.
func importSpecifier(n *ts.Node, src []byte) string {
	if n == nil {
		return ""
	}
	if kind := n.Kind(); kind != "import_statement" && kind != kindExport {
		return ""
	}
	source := n.ChildByFieldName("source")
	if source == nil {
		return ""
	}
	// The node includes its quotes; a caller comparing against `./y`
	// wants the value.
	text := source.Utf8Text(src)
	unquoted, err := strconv.Unquote(text)
	if err == nil {
		return unquoted
	}
	// Single-quoted, which strconv does not accept. The backend emits
	// exactly this form, so it is the common path rather than the
	// fallback it looks like.
	return strings.Trim(text, "'`\"")
}

// Normalised returns the file in the canonical form the backend
// emits, so a comparison is about what the file says rather than how
// it was spaced.
//
// The rules the TypeScript backend documents: two-space indent, no
// trailing whitespace, no run of blank lines longer than one, and a
// single trailing newline. Re-derived here rather than shared with
// the backend, because a harness importing the thing it checks would
// pass on output the backend normalised wrongly.
func (s *Source) Normalised() []byte {
	var out []string
	blank := 0
	for line := range strings.SplitSeq(string(s.src), "\n") {
		line = strings.TrimRight(line, " \t")
		if line == "" {
			blank++
			if blank > 1 {
				continue
			}
		} else {
			blank = 0
		}
		out = append(out, line)
	}
	// Leading and trailing blanks carry nothing and would otherwise
	// make an assertion about the body depend on them.
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return []byte(strings.Join(out, "\n") + "\n")
}

// AssertFormatted fails when the file is not already in canonical
// form.
//
// The assertion a template with a stray indent or a doubled blank
// line fails. Nothing downstream reports it — the output parses, it
// type-checks, and it lands in a consumer's repository looking
// hand-edited — so the only place it can be caught is here.
//
// What "canonical" means is the backend's decision and is stated in
// its package doc; see [Source.Normalised] for the rules this checks.
func (s *Source) AssertFormatted(tb testing.TB) *Source {
	tb.Helper()
	want := s.Normalised()
	if !bytes.Equal(want, s.src) {
		tb.Errorf("typescripttest: %s is not in canonical form:\n--- got ---\n%s"+
			"--- normalised ---\n%s", s.path, numbered(s.src), numbered(want))
	}
	return s
}

// AssertDocumented fails when an exported declaration carries no
// JSDoc block.
//
// A generated module is API, and an undocumented export is one a
// consumer's editor shows as a bare name. A generator that documents
// most of what it emits and forgets one is the common case, which is
// why this reports the whole set rather than the first.
//
// Only the top-level exports: a member's documentation is the
// declaration's business, and a generator projecting a source type
// carries whatever docs that type had.
func (s *Source) AssertDocumented(tb testing.TB) *Source {
	tb.Helper()
	var bare []string
	for _, n := range s.topLevel() {
		name := declName(n, s.src)
		if name == "" || !exported(n) {
			continue
		}
		if !s.documented(n) {
			bare = append(bare, name)
		}
	}
	if len(bare) > 0 {
		tb.Errorf("typescripttest: %s exports %v with no JSDoc, so a consumer's editor "+
			"shows them as bare names", s.path, bare)
	}
	return s
}

// documented reports whether a JSDoc block immediately precedes the
// declaration.
//
// Read from the bytes rather than the tree: tree-sitter attaches a
// comment as a sibling rather than to the node it documents, and the
// sibling to look at differs between a bare declaration and an
// exported one. The line above is what a reader calls the doc
// comment, and it is the same answer either way.
func (s *Source) documented(n *ts.Node) bool {
	start := outermost(n)
	above := strings.TrimSpace(lineAbove(s.src, int(start.StartPosition().Row)))
	return strings.HasSuffix(above, "*/")
}

// outermost returns the wrapper a declaration was reached through, so
// the line looked at is the one above `export` rather than the one
// above the declaration inside it.
func outermost(n *ts.Node) *ts.Node {
	for cur := n.Parent(); cur != nil; cur = cur.Parent() {
		if _, wrapper := wrappers[cur.Kind()]; !wrapper {
			break
		}
		n = cur
	}
	return n
}

// lineAbove returns the line before the 0-based row, empty at the top
// of the file.
func lineAbove(src []byte, row int) string {
	if row <= 0 {
		return ""
	}
	lines := strings.Split(string(src), "\n")
	if row > len(lines) {
		return ""
	}
	return lines[row-1]
}
