// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golangtest

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
)

// Source is one parsed generated file.
//
// Parsed once and reused across assertions, because a generated file
// runs to hundreds of lines and a test asserting a dozen things about
// it should not pay for a dozen parses.
type Source struct {
	file *ast.File
	fset *token.FileSet
	src  []byte
	path string
}

// Parse reads a file into a [Source], failing the test when it is not
// syntactically valid Go.
//
// A parse failure is reported with the offending source numbered,
// because the position it names is in generated code the author never
// wrote and cannot open.
func Parse(tb testing.TB, f File) *Source {
	tb.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, f.Path, f.Src, parser.ParseComments)
	if err != nil {
		tb.Fatalf("golangtest: %s is not valid Go: %v\n%s", f.Path, err, numbered(f.Src))
	}
	return &Source{file: parsed, fset: fset, src: f.Src, path: f.Path}
}

// Path returns the file's path, for a caller composing its own
// failure message.
func (s *Source) Path() string { return s.path }

// Bytes returns a copy of the file's content.
func (s *Source) Bytes() []byte { return slices.Clone(s.src) }

// Dump logs the file with line numbers, for a test being debugged.
func (s *Source) Dump(tb testing.TB) *Source {
	tb.Helper()
	tb.Logf("golangtest: %s\n%s", s.path, numbered(s.src))
	return s
}

// AssertPackage fails when the file does not declare the named
// package.
//
// Worth pinning on a generator whose companion belongs in an external
// test package: the framework keys that shift off the filename, and a
// suffix that merely looked test-ish lands the suite in the source
// package where it can read private state and no longer proves the
// output works from outside.
func (s *Source) AssertPackage(tb testing.TB, name string) *Source {
	tb.Helper()
	if got := s.file.Name.Name; got != name {
		tb.Errorf("golangtest: %s declares package %q, want %q", s.path, got, name)
	}
	return s
}

// Imports returns every import path the file declares, in source
// order.
func (s *Source) Imports() []string {
	out := make([]string, 0, len(s.file.Imports))
	for _, spec := range s.file.Imports {
		p, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			// Unreachable from a parsed file: the parser rejects an
			// import whose path is not a valid string literal.
			continue
		}
		out = append(out, p)
	}
	return out
}

// AssertImports fails when any of the named paths is absent.
func (s *Source) AssertImports(tb testing.TB, paths ...string) *Source {
	tb.Helper()
	got := s.Imports()
	for _, want := range paths {
		if !slices.Contains(got, want) {
			tb.Errorf("golangtest: %s does not import %q; it imports %v", s.path, want, got)
		}
	}
	return s
}

// AssertNoImport fails when the named path is present.
func (s *Source) AssertNoImport(tb testing.TB, path string) *Source {
	tb.Helper()
	if slices.Contains(s.Imports(), path) {
		tb.Errorf("golangtest: %s imports %q, which it must not", s.path, path)
	}
	return s
}

// AssertImportsOnly fails when the file's import set is anything
// other than exactly the named paths.
//
// A generator's imports are part of its API. Emitting a new one is a
// breaking change for every consumer whose module does not already
// require it — and it is invisible in the generator's own tests,
// where the import always resolves because this module has it.
// Pinning the whole set is what turns that into a one-line diff.
func (s *Source) AssertImportsOnly(tb testing.TB, paths ...string) *Source {
	tb.Helper()
	got, want, extra, missing := diffSets(s.Imports(), paths)
	if extra == nil && missing == nil {
		return s
	}
	tb.Errorf("golangtest: %s imports %v, want %v (unexpected: %v; absent: %v)",
		s.path, got, want, extra, missing)
	return s
}

// diffSets sorts two name sets and reports what each holds that the
// other does not.
//
// Shared because an exact-set assertion is only worth having if its
// message says which way it went wrong: "want [A B C], got [A B D]"
// makes the reader diff two lists by eye, and they will get it wrong
// once the lists run past four entries.
func diffSets(have, wanted []string) (got, want, extra, missing []string) {
	got, want = slices.Clone(have), slices.Clone(wanted)
	slices.Sort(got)
	slices.Sort(want)
	for _, n := range got {
		if !slices.Contains(want, n) {
			extra = append(extra, n)
		}
	}
	for _, n := range want {
		if !slices.Contains(got, n) {
			missing = append(missing, n)
		}
	}
	return got, want, extra, missing
}

// AssertGeneratedHeader fails when the file carries no line matching
// Go's generated-code convention.
//
// The line is specified exactly — `^// Code generated .* DO NOT
// EDIT\.$`, on its own line, before the package clause — and tooling
// depends on it: gopls skips diagnostics, linters skip the file, and
// review tooling marks it machine-written. A generator whose header
// drifts by a character silently opts every consumer's output back
// into all of it, and no other test would see the difference.
func (s *Source) AssertGeneratedHeader(tb testing.TB) *Source {
	tb.Helper()
	head := s.headerLines()
	if slices.ContainsFunc(head, golang.IsGeneratedSource) {
		return s
	}
	tb.Errorf("golangtest: %s carries no `// Code generated ... DO NOT EDIT.` line before its "+
		"package clause, so downstream tooling will treat it as hand-written\n--- header ---\n%s",
		s.path, strings.Join(head, "\n"))
	return s
}

// headerLines returns every line before the package clause.
//
// The marker is only recognised there — a licence block may precede
// it, but a line after the package clause is an ordinary comment as
// far as every tool that looks for one is concerned.
func (s *Source) headerLines() []string {
	pkgLine := s.fset.Position(s.file.Package).Line
	lines := strings.Split(string(s.src), "\n")
	if pkgLine-1 < len(lines) {
		return lines[:pkgLine-1]
	}
	return lines
}

// AssertFormatted fails when the file is not gofmt-canonical.
//
// The backend formats what it renders, so this is a regression guard
// on that rather than on the templates — and an unformatted generated
// file shows up as a diff in every consumer's next `gofmt -l` run,
// blamed on them.
func (s *Source) AssertFormatted(tb testing.TB) *Source {
	tb.Helper()
	// The error is discarded rather than branched on: every Source
	// came through Parse, which fails first on anything the formatter
	// could reject, so the branch is unreachable from a test and a
	// line no reader could account for.
	want, _ := format.Source(s.src)
	if !bytes.Equal(want, s.src) {
		tb.Errorf("golangtest: %s is not gofmt-canonical\n--- got ---\n%s\n--- want ---\n%s",
			s.path, s.src, want)
	}
	return s
}

// CommandPlaceholder replaces the header's `Command:` value in a
// normalised file.
const CommandPlaceholder = "<command normalised by golangtest>"

// commandPrefix is the header line the backend stamps the invocation
// into.
const commandPrefix = "// Command:"

// Normalised returns the file with the header's volatile lines
// rewritten to a fixed value.
//
// The backend stamps the process invocation into the header when
// nothing overrides it, which under `go test` is the test binary's
// own flags — including a temp path that differs on every run. Left
// alone it makes a byte golden unusable; rewritten here it keeps the
// golden about the generated code rather than about how the suite was
// invoked.
//
// Every generator that keeps a golden had already written this,
// privately and identically.
func (s *Source) Normalised() []byte {
	lines := strings.Split(string(s.src), "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, commandPrefix) {
			lines[i] = commandPrefix + "   " + CommandPlaceholder
			break
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

// AssertGolden compares the normalised file against a golden, writing
// it when the golden is absent.
//
// The record of what the whole file reads like, where
// [AssertAPIGolden] is the record of what consumers depend on. Keep
// both if a reviewer wants to see the prose and the comments; keep
// only the API one if what they need is the change list.
//
// A missing golden is written rather than failed, and an existing one
// is never rewritten — regenerate by deleting it, which keeps the
// change visible in review.
func (s *Source) AssertGolden(tb testing.TB, goldenPath string) *Source {
	tb.Helper()
	got := s.Normalised()

	want, err := os.ReadFile(goldenPath) //nolint:gosec // a test-supplied fixture path.
	if os.IsNotExist(err) {
		writeGolden(tb, goldenPath, got)
		return s
	}
	if err != nil {
		tb.Fatalf("golangtest: read %s: %v", goldenPath, err)
	}
	if !bytes.Equal(want, got) {
		tb.Errorf("golangtest: %s does not match %s\n--- want ---\n%s\n--- got ---\n%s",
			s.path, goldenPath, want, got)
	}
	return s
}

// writeGolden records a golden that does not exist yet, so the first
// run after adding a fixture captures the output instead of demanding
// the author type it out.
func writeGolden(tb testing.TB, goldenPath string, body []byte) {
	tb.Helper()
	if err := os.MkdirAll(filepath.Dir(goldenPath), 0o750); err != nil {
		tb.Fatalf("golangtest: create %s: %v", filepath.Dir(goldenPath), err)
	}
	if err := os.WriteFile(goldenPath, body, 0o600); err != nil {
		tb.Fatalf("golangtest: write %s: %v", goldenPath, err)
	}
	tb.Logf("golangtest: wrote a new golden at %s; review it before committing", goldenPath)
}

// AssertDocumented fails when an exported declaration carries no doc
// comment.
//
// Generated code is read: by the consumer deciding whether to call
// it, and by their editor's hover. It is also linted — a project
// whose `revive` or `staticcheck` configuration requires comments on
// exported declarations gets those failures from generated files the
// moment they are not excluded, and blames itself for them.
func (s *Source) AssertDocumented(tb testing.TB) *Source {
	tb.Helper()
	bare := make([]string, 0, len(s.file.Decls))
	for _, d := range s.file.Decls {
		bare = append(bare, undocumented(d)...)
	}
	if len(bare) > 0 {
		slices.Sort(bare)
		tb.Errorf("golangtest: %s declares exported API with no doc comment: %v", s.path, bare)
	}
	return s
}

// undocumented lists one declaration's exported names carrying no doc
// comment.
func undocumented(d ast.Decl) []string {
	switch decl := d.(type) {
	case *ast.FuncDecl:
		if !ast.IsExported(decl.Name.Name) || decl.Doc != nil {
			return nil
		}
		if recv := receiverName(decl); recv != "" {
			if !ast.IsExported(recv) {
				return nil
			}
			return []string{recv + "." + decl.Name.Name}
		}
		return []string{decl.Name.Name}
	case *ast.GenDecl:
		return undocumentedSpecs(decl)
	default:
		return nil
	}
}

// undocumentedSpecs lists a grouped declaration's undocumented
// exported names.
//
// A spec inherits its group's comment: `// Errors.\nvar ( ErrA; ErrB
// )` documents both, which is how a generator emitting a sentinel
// block writes one.
func undocumentedSpecs(decl *ast.GenDecl) []string {
	if decl.Doc != nil {
		return nil
	}
	var out []string
	for _, spec := range decl.Specs {
		switch sp := spec.(type) {
		case *ast.TypeSpec:
			if ast.IsExported(sp.Name.Name) && sp.Doc == nil {
				out = append(out, sp.Name.Name)
			}
		case *ast.ValueSpec:
			if sp.Doc != nil {
				continue
			}
			for _, n := range sp.Names {
				if ast.IsExported(n.Name) {
					out = append(out, n.Name)
				}
			}
		}
	}
	return out
}

// AssertOrder fails when the declaration named first does not appear
// before the one named second.
//
// Slot ordering is otherwise invisible: a contribution rendered ahead
// of the type it hangs off still compiles, and only a reader notices.
// Keyed on declaration names rather than on a marker comment, so it
// survives the comment being reworded.
func (s *Source) AssertOrder(tb testing.TB, first, second string) *Source {
	tb.Helper()
	a, aok := s.declPos(first)
	b, bok := s.declPos(second)
	switch {
	case !aok:
		tb.Errorf("golangtest: %s declares no %q; it declares %v", s.path, first, s.DeclNames())
	case !bok:
		tb.Errorf("golangtest: %s declares no %q; it declares %v", s.path, second, s.DeclNames())
	case a > b:
		tb.Errorf("golangtest: %s declares %q at line %d, after %q at line %d",
			s.path, first, s.fset.Position(a).Line, second, s.fset.Position(b).Line)
	}
	return s
}

// AssertOrderAll fails unless the file declares every named
// declaration, in the order given.
//
// The list form of [Source.AssertOrder], because slot ordering is
// almost never a pair: a file assembled from three contributors takes
// two calls to pin with the pair form, and the reader has to supply
// the transitivity argument themselves. It also fails on an absent
// name rather than skipping it, so a contributor that stopped
// rendering is a failure here rather than a silent hole in the order.
func (s *Source) AssertOrderAll(tb testing.TB, names ...string) *Source {
	tb.Helper()
	prev, prevPos := "", token.NoPos
	for _, name := range names {
		pos, ok := s.declPos(name)
		if !ok {
			tb.Errorf("golangtest: %s declares no %q; it declares %v", s.path, name, s.DeclNames())
			return s
		}
		if prevPos.IsValid() && pos < prevPos {
			tb.Errorf("golangtest: %s declares %q at line %d, after %q at line %d",
				s.path, prev, s.fset.Position(prevPos).Line, name, s.fset.Position(pos).Line)
			return s
		}
		prev, prevPos = name, pos
	}
	return s
}

// declPos returns the position of the named top-level declaration.
func (s *Source) declPos(name string) (token.Pos, bool) {
	for _, d := range s.file.Decls {
		switch decl := d.(type) {
		case *ast.FuncDecl:
			if decl.Name.Name == name {
				return decl.Pos(), true
			}
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok && ts.Name.Name == name {
					return ts.Pos(), true
				}
				vs, isValue := spec.(*ast.ValueSpec)
				if !isValue {
					continue
				}
				if slices.ContainsFunc(vs.Names, func(n *ast.Ident) bool { return n.Name == name }) {
					return vs.Pos(), true
				}
			}
		}
	}
	return token.NoPos, false
}

// DeclNames lists every top-level declaration the file makes, methods
// spelled `Recv.Name`. The vocabulary a failure message quotes back.
func (s *Source) DeclNames() []string {
	var out []string
	for _, d := range s.file.Decls {
		switch decl := d.(type) {
		case *ast.FuncDecl:
			if recv := receiverName(decl); recv != "" {
				out = append(out, recv+"."+decl.Name.Name)
				continue
			}
			out = append(out, decl.Name.Name)
		case *ast.GenDecl:
			for _, spec := range decl.Specs {
				switch sp := spec.(type) {
				case *ast.TypeSpec:
					out = append(out, sp.Name.Name)
				case *ast.ValueSpec:
					for _, n := range sp.Names {
						out = append(out, n.Name)
					}
				}
			}
		}
	}
	return out
}

// receiverName returns the bare type name a method is declared on,
// with any pointer and type arguments stripped — `ItemBuilder` for
// `func (b *ItemBuilder[T]) …`. Empty for a plain function.
//
// Stripped because a caller asking about a method means the type, and
// making them spell the receiver's exact form would make the
// assertion break when the generator starts taking a pointer.
func receiverName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	return baseTypeName(fn.Recv.List[0].Type)
}

// baseTypeName strips pointers and type arguments from a type
// expression, yielding the declared identifier.
func baseTypeName(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.StarExpr:
		return baseTypeName(t.X)
	case *ast.IndexExpr:
		return baseTypeName(t.X)
	case *ast.IndexListExpr:
		return baseTypeName(t.X)
	case *ast.Ident:
		return t.Name
	default:
		return ""
	}
}

// exprString renders a type expression in canonical form, free of the
// column padding gofmt applied in the file.
//
// Why it matters: a struct field asserted as a substring carries
// whatever alignment its neighbours happened to force, so adding an
// unrelated field to the struct breaks an assertion about this one.
func exprString(e ast.Expr) string { return types.ExprString(e) }

// render prints an AST node with canonical spacing.
//
// A fresh FileSet is passed deliberately: the printer resolves
// positions through it, and one that knows nothing about these nodes
// makes it lay the code out from scratch rather than reproduce the
// original alignment.
func render(n ast.Node) string {
	var b strings.Builder
	if err := printer.Fprint(&b, token.NewFileSet(), n); err != nil {
		// Unreachable: strings.Builder never fails a write, and the
		// node came from the parser.
		return ""
	}
	return b.String()
}

// normalise collapses every run of whitespace to a single space, so
// two spellings of one signature compare equal.
func normalise(s string) string { return strings.Join(strings.Fields(s), " ") }

// numbered prefixes each line of src with its line number, so a
// failure naming a position in generated code points at something the
// reader can find in the message itself.
func numbered(src []byte) string {
	lines := strings.Split(strings.TrimRight(string(src), "\n"), "\n")
	var b strings.Builder
	for i, line := range lines {
		fmt.Fprintf(&b, "%4d | %s\n", i+1, line)
	}
	return b.String()
}
