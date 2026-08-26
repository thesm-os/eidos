// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescripttest

import (
	"path/filepath"
	"sync"

	ts "github.com/tree-sitter/go-tree-sitter"
	tsgrammar "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// The parser this package reads generated output with.
//
// Its own rather than the frontend's, and the duplication is
// deliberate. A harness that asserted on generated output by parsing
// it with the frontend under test would confirm itself: a frontend
// bug that mis-read a construct would make the backend that emitted
// it look correct. Sharing the grammar and nothing else is what keeps
// the two independent — and the shared part is two lines.
//
// Linking tree-sitter means this package needs cgo. The frontend
// already does and lives in the same module, so nothing that could
// build before stops building; a consumer wanting only the structural
// assertions still pays a C toolchain, which is the cost of checking
// TypeScript from Go at all.

// grammar identifies which of the two TypeScript grammars a file is
// parsed with.
//
// They are not interchangeable, and the difference is not cosmetic:
// `<T>value` is a type assertion in `.ts` and the opening of a JSX
// element in `.tsx`, so the same bytes parse to different trees.
type grammar int

const (
	// grammarTS is the `.ts` / `.mts` / `.cts` grammar.
	grammarTS grammar = iota

	// grammarTSX is the `.tsx` grammar, which admits JSX syntax and
	// gives up angle-bracket type assertions to get it.
	grammarTSX
)

// grammarFor returns the grammar a file's extension selects.
func grammarFor(path string) grammar {
	if filepath.Ext(path) == ".tsx" {
		return grammarTSX
	}
	return grammarTS
}

// languages holds one [ts.Language] per grammar, built once.
//
// Constructed lazily rather than in an init: building a language
// crosses into C, and a package-level init that does so runs for
// every test binary linking this package whether or not it ever
// parses.
var languages = sync.OnceValue(func() [2]*ts.Language { //nolint:gochecknoglobals // immutable after first call
	return [2]*ts.Language{
		grammarTS:  ts.NewLanguage(tsgrammar.LanguageTypescript()),
		grammarTSX: ts.NewLanguage(tsgrammar.LanguageTSX()),
	}
})

// parserPools holds a [sync.Pool] of parsers per grammar.
//
// A tree-sitter parser is not safe for concurrent use and allocating
// one costs a C allocation plus the grammar bind. Assertions in this
// package are documented as safe from parallel subtests, so they will
// be called concurrently; pooling per grammar gives each goroutine
// its own parser without serialising them.
var parserPools = sync.OnceValue(func() [2]*sync.Pool { //nolint:gochecknoglobals // immutable after first call
	langs := languages()
	pools := [2]*sync.Pool{}
	for g := range pools {
		lang := langs[g]
		pools[g] = &sync.Pool{New: func() any {
			p := ts.NewParser()
			// SetLanguage fails only on an ABI mismatch between the
			// grammar and the runtime, which is fixed at build time.
			_ = p.SetLanguage(lang)
			return p
		}}
	}
	return pools
})

// parseTree parses src as the file at path, returning nil when the
// parser declined to produce a tree at all.
//
// Distinct from a file containing syntax errors, which tree-sitter
// recovers from and reports through the tree's own error nodes — see
// [firstError].
func parseTree(path string, src []byte) *ts.Tree {
	pool := parserPools()[grammarFor(path)]
	p, _ := pool.Get().(*ts.Parser)
	defer pool.Put(p)
	return p.Parse(src, nil)
}

// firstError returns the first node in the tree that tree-sitter
// could not parse, or nil for a clean file.
//
// A pre-order walk, so the node reported is the earliest failure
// rather than the deepest: a stray brace makes everything after it
// look wrong, and naming the last of those sends the reader to the
// end of a file whose problem is at the top.
func firstError(n *ts.Node) *ts.Node {
	if n == nil {
		return nil
	}
	if n.IsError() || n.IsMissing() {
		return n
	}
	// HasError is what makes the walk cheap: a subtree with nothing
	// wrong in it is skipped whole rather than descended.
	if !n.HasError() {
		return nil
	}
	for i := range n.ChildCount() {
		if got := firstError(n.Child(i)); got != nil {
			return got
		}
	}
	return nil
}
