// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"

	ts "github.com/tree-sitter/go-tree-sitter"
	tsgrammar "github.com/tree-sitter/tree-sitter-typescript/bindings/go"
)

// ErrParse reports that the parser returned no tree for a file.
//
// Distinct from a file containing syntax errors, which tree-sitter
// recovers from and reports through the tree's own error nodes: this
// is the parser declining to produce a tree at all.
var ErrParse = errors.New("frontend: parser returned no tree")

// grammar identifies which of the two TypeScript grammars a file is
// parsed with.
//
// They are not interchangeable, and the difference is not cosmetic:
// `<T>value` is a type assertion in `.ts` and the opening of a JSX
// element in `.tsx`, so the same bytes parse to different trees. A
// single grammar for both would silently mis-parse one of them.
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
// every binary linking this package whether or not it ever parses.
var languages = sync.OnceValue(func() [2]*ts.Language { //nolint:gochecknoglobals // immutable after first call
	return [2]*ts.Language{
		grammarTS:  ts.NewLanguage(tsgrammar.LanguageTypescript()),
		grammarTSX: ts.NewLanguage(tsgrammar.LanguageTSX()),
	}
})

// parserPools holds a [sync.Pool] of parsers per grammar.
//
// A tree-sitter parser is not safe for concurrent use and allocating
// one costs a C allocation plus the grammar bind, so the pipeline
// dispatching Load across goroutines would either serialise on a
// single parser or pay that cost per file. Pooling per grammar gives
// each goroutine its own without either.
var parserPools = sync.OnceValue(func() [2]*sync.Pool { //nolint:gochecknoglobals // immutable after first call
	langs := languages()
	pools := [2]*sync.Pool{}
	for g := range pools {
		lang := langs[g]
		pools[g] = &sync.Pool{New: func() any {
			p := ts.NewParser()
			// SetLanguage fails only on an ABI mismatch between the
			// grammar and the runtime, which is fixed at build time.
			// A parser that failed to bind is returned unbound and
			// reports the failure at Parse, where a file path is in
			// hand to name.
			_ = p.SetLanguage(lang)
			return p
		}}
	}
	return pools
})

// parsed is one file's syntax tree plus the bytes it indexes into.
//
// The tree holds byte offsets, not text, so every read of a node's
// source needs the original bytes alongside it. Keeping the two
// together is what stops a caller pairing a tree with the wrong
// buffer.
type parsed struct {
	// path is the file the tree came from, as supplied.
	path string

	// src is the file's bytes. Node offsets index into this.
	src []byte

	// tree is the syntax tree. The caller closes it via [parsed.close].
	tree *ts.Tree
}

// root returns the tree's root node.
func (p *parsed) root() *ts.Node { return p.tree.RootNode() }

// text returns n's source text.
func (p *parsed) text(n *ts.Node) string {
	if n == nil {
		return ""
	}
	return n.Utf8Text(p.src)
}

// close releases the tree's C memory. Safe on a nil receiver so a
// deferred close after a failed parse needs no guard.
func (p *parsed) close() {
	if p != nil && p.tree != nil {
		p.tree.Close()
	}
}

// parseFile parses src as the file at path.
//
// The returned [parsed] owns a C-allocated tree and the caller must
// call close on it. A nil tree from the parser is reported as an
// error rather than returned, because every downstream walk would
// otherwise dereference it.
func parseFile(path string, src []byte) (*parsed, error) {
	g := grammarFor(path)
	pool := parserPools()[g]

	p, _ := pool.Get().(*ts.Parser)
	// Returned unreset: a parser carries no state between Parse calls
	// that affects the next one — the previous tree is passed
	// explicitly, and this call passes nil.
	defer pool.Put(p)

	tree := p.Parse(src, nil)
	if tree == nil {
		return nil, fmt.Errorf("%w: %s", ErrParse, path)
	}
	return &parsed{path: path, src: src, tree: tree}, nil
}
