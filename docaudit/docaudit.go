// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package docaudit drives the "documented vs implemented"
// audit every plugin package runs against its own `doc.go`.
// Each meta-key constructor literal under the package source
// must appear in the package doc; otherwise the audit fails and
// the offending key surfaces in the failing-test output.
//
// The audit ships as a black-box helper rather than per-package
// duplication so a single update to the key-discovery rule
// (new constructor name, additional skip pattern, ...)
// propagates without touching every doc-audit caller.
package docaudit

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// TB is the narrow subset of [testing.TB] the audit needs.
// Defined as a local interface so the audit accepts both
// real *testing.T values and per-test fakes that drive failure-
// path coverage on the helper itself.
type TB interface {
	Helper()
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
}

// AssertEveryMetaKeyDocumented walks every `.go` source file
// under packageDir (excluding `_test.go`), extracts the literal-
// string first argument of every `meta.NewKey(...)` /
// `meta.EnsureKey(...)` call, and asserts each resulting key
// appears verbatim in packageDir/doc.go. Dynamic-name calls
// whose first argument is not a literal string are skipped;
// those land under a documented namespace prefix, which the
// caller audits separately via the prefix presence in doc.go.
//
// Pass the test's `t` and the absolute path of the package
// directory. The package's doc.go is read by joining packageDir
// with the literal filename.
func AssertEveryMetaKeyDocumented(t TB, packageDir string) {
	t.Helper()
	keys, err := collectMetaKeyLiterals(packageDir)
	if err != nil {
		t.Fatalf("docaudit: collect meta-key literals from %s: %v", packageDir, err)
	}
	if len(keys) == 0 {
		t.Fatalf("docaudit: no meta-key literals discovered under %s — audit is mis-wired",
			packageDir)
	}
	docPath := filepath.Join(packageDir, "doc.go")
	body, err := os.ReadFile(docPath) //nolint:gosec // path supplied by test code, not user input.
	if err != nil {
		t.Fatalf("docaudit: read %s: %v", docPath, err)
	}
	doc := string(body)
	for _, key := range keys {
		if !mentionsKey(doc, key) {
			t.Errorf("docaudit: doc.go missing mention of meta key %q (declared in package source)",
				key)
		}
	}
}

// MetaKeys returns every literal metadata-key name declared under
// packageDir, sorted and de-duplicated.
//
// It exposes the same collection [AssertEveryMetaKeyDocumented] uses,
// for callers that need the key set itself rather than the
// documentation assertion. The motivating case is a frontend pinning
// its cache-invalidating version constant to its stamping surface: a
// key added without a version bump leaves every warm cache serving a
// node graph that predates the key, and deriving the set here means
// the guard cannot drift from the code the way a hand-maintained list
// would.
//
// Calls whose first argument is not a string literal are skipped, as
// they are for the documentation audit — those are the dynamic-name
// pattern, documented by namespace prefix instead.
func MetaKeys(packageDir string) ([]string, error) {
	return collectMetaKeyLiterals(packageDir)
}

// collectMetaKeyLiterals returns the sorted, de-duplicated list
// of literal-string first arguments to every meta-key
// constructor call found under dir's non-test Go sources.
// Returns [ErrEmptyDirectory] when no source files parse.
func collectMetaKeyLiterals(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err //nolint:wrapcheck // pass-through for caller's t.Fatalf wrap
	}
	fset := token.NewFileSet()
	seen := map[string]struct{}{}
	parsedAny := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err //nolint:wrapcheck // pass-through for caller's t.Fatalf wrap
		}
		parsedAny = true
		ast.Inspect(file, func(n ast.Node) bool {
			literal, ok := metaKeyLiteralFrom(n)
			if !ok {
				return true
			}
			seen[literal] = struct{}{}
			return true
		})
	}
	if !parsedAny {
		return nil, ErrEmptyDirectory
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	slices.Sort(out)
	return out, nil
}

// calleeSelector unwraps a call's function expression to the
// underlying selector, seeing through explicit generic instantiation.
//
// [meta.NewKey] and [meta.EnsureKey] are generic. Written with the
// type argument inferred — meta.NewKey("go.isChannel", ...) — the
// callee is a plain [ast.SelectorExpr]. Written with it explicit —
// meta.NewKey[bool]("go.isChannel", ...) — the parser wraps that
// selector in an [ast.IndexExpr], or an [ast.IndexListExpr] for two
// or more type arguments.
//
// Both forms compile and both declare a published key. Matching only
// the bare selector meant the explicit form was skipped in silence,
// so a key declared that way was exempt from the documentation audit
// and from the frontends' stamping-surface guard while appearing to
// be covered by both. That is the failure mode this package exists to
// prevent, reproduced inside the package itself.
func calleeSelector(fun ast.Expr) (*ast.SelectorExpr, bool) {
	switch f := fun.(type) {
	case *ast.SelectorExpr:
		return f, true
	case *ast.IndexExpr:
		sel, ok := f.X.(*ast.SelectorExpr)
		return sel, ok
	case *ast.IndexListExpr:
		sel, ok := f.X.(*ast.SelectorExpr)
		return sel, ok
	default:
		return nil, false
	}
}

// metaKeyLiteralFrom returns the first-argument literal string
// of n when n is a `meta.NewKey(...)` or `meta.EnsureKey(...)`
// call; ok=false otherwise. Calls whose first argument is not a
// literal string (the dynamic-name pattern) return ok=false too
// — those are documented by namespace prefix, not per-key
// enumeration.
func metaKeyLiteralFrom(n ast.Node) (string, bool) {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return "", false
	}
	sel, ok := calleeSelector(call.Fun)
	if !ok {
		return "", false
	}
	pkgIdent, ok := sel.X.(*ast.Ident)
	if !ok || pkgIdent.Name != "meta" {
		return "", false
	}
	if sel.Sel.Name != "NewKey" && sel.Sel.Name != "EnsureKey" {
		return "", false
	}
	if len(call.Args) == 0 {
		return "", false
	}
	lit, ok := call.Args[0].(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	value, err := unquoteDoubleQuoted(lit.Value)
	if err != nil {
		return "", false
	}
	return value, true
}

// mentionsKey reports whether doc documents key as a key rather than
// merely containing its text.
//
// A plain substring test passes coincidentally in two ways this
// package cannot afford, because a gate that passes by accident is
// worse than an absent one — it produces a green tick a reader
// trusts:
//
//   - A short key is a common word. `frontend` matched any doc.go that
//     used the word "frontend" in prose, so it was never actually
//     audited.
//   - A key that is a prefix of a sibling rides on the sibling.
//     `go.isIterSeq` matched inside `go.isIterSeq2`, so documenting
//     one silently discharged the obligation for both.
//
// A match therefore has to end at a boundary: the character following
// the occurrence must not be one that could continue a key name.
// Leading context is deliberately not constrained — keys appear after
// backticks, brackets and hyphens in ordinary documentation prose, and
// no false pass observed here came from the left.
func mentionsKey(doc, key string) bool {
	for offset := 0; ; {
		i := strings.Index(doc[offset:], key)
		if i < 0 {
			return false
		}
		end := offset + i + len(key)
		if !continuesKey(doc, end) {
			return true
		}
		offset = end
	}
}

// continuesKey reports whether the text at index i extends a key name
// that ends just before it.
//
// The dot needs care, because it is both a key separator and the most
// common way an English sentence ends. Treating it as key-continuing
// unconditionally rejected "the go.chanDir key." — a perfectly good
// mention terminated by a full stop — and the package's own fixture
// caught it. A dot therefore continues a key only when an identifier
// character follows it, which distinguishes `shape.key_type` from
// `shape.` at the end of a clause.
func continuesKey(doc string, i int) bool {
	if i >= len(doc) {
		return false
	}
	if doc[i] == '.' {
		return i+1 < len(doc) && isIdentRune(rune(doc[i+1]))
	}
	return isIdentRune(rune(doc[i]))
}

// isIdentRune reports whether r can appear inside one dotted segment
// of a metadata-key name. The vocabulary is the one this repo's keys
// use: lower-camel segments with underscores.
func isIdentRune(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return true
	case r == '_':
		return true
	default:
		return false
	}
}

// unquoteDoubleQuoted strips the surrounding double quotes from
// raw and returns the inner content. The meta-key constructors
// only ever take double-quoted literals in this codebase; the
// helper deliberately rejects raw-string and single-quoted forms
// to keep the audit's input contract narrow.
func unquoteDoubleQuoted(raw string) (string, error) {
	if len(raw) < 2 || raw[0] != '"' || raw[len(raw)-1] != '"' {
		return "", ErrUnsupportedLiteral
	}
	return raw[1 : len(raw)-1], nil
}

// ErrEmptyDirectory signals that a doc-audit walk under the
// supplied directory found no parseable Go source files. The
// audit treats the situation as a wiring error rather than an
// implicit pass.
var ErrEmptyDirectory = errors.New("docaudit: no Go source files in package directory")

// ErrUnsupportedLiteral signals that a meta-key constructor call
// carried a literal kind the audit's narrow grammar doesn't
// accept (raw string, single-quoted, multi-line backtick). The
// audit skips the call rather than guessing the key string.
var ErrUnsupportedLiteral = errors.New("docaudit: unsupported literal form")
