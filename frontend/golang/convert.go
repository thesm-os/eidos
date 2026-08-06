// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"go/ast"
	"go/token"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"

	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// Per-package conversion runs through [convertPackageWithCache]
// (in loader.go), which threads cache lookup / store around the
// AST→node conversion. [buildPackage] is the cache-agnostic entry
// point that produces a populated [node.Package]; the cache-aware
// wrapper handles skip-on-hit and write-on-miss.

// converter holds the per-package state used during AST → node
// conversion. The instance is internal to a single buildPackage
// call and not safe for concurrent use.
type converter struct {
	ctx  *plugin.FrontendContext
	pkg  *packages.Package
	fset *token.FileSet
	opts Options
	out  *node.Package

	// pkgFiles is the syntax file slice in deterministic order (by
	// loader-reported filename). Sorting up front keeps the produced
	// node graph byte-stable across runs.
	pkgFiles []*ast.File

	// methodsByRecv buckets pending [node.Method] entries by the
	// qualified name of their receiver type. The post-pass
	// [attachMethods] flushes the bucket into the matching
	// [node.Struct] or [node.Interface].
	methodsByRecv map[string][]*node.Method

	// structByQName / ifaceByQName / aliasByQName let later passes
	// resolve a receiver or a generic type-param reference back to
	// the in-package declaration.
	structByQName map[string]*node.Struct
	ifaceByQName  map[string]*node.Interface
	aliasByQName  map[string]*node.Alias

	// commentsByNode is the package-wide free-floating comment
	// index — every AST node maps to the comment groups [go/ast]
	// associates with it. Used by the parameter overlay to recover
	// directives that appear before or after a parameter in a
	// multiline function signature, since [ast.Field.Doc] and
	// [ast.Field.Comment] are unpopulated inside parameter lists.
	commentsByNode map[ast.Node][]*ast.CommentGroup
}

// newConverter prepares a converter for the supplied package.
func newConverter(ctx *plugin.FrontendContext, pkg *packages.Package, opts Options) *converter {
	syntax := filterSyntaxFiles(pkg, opts)
	slices.SortFunc(syntax, func(a, b *ast.File) int {
		return strings.Compare(pkg.Fset.Position(a.Pos()).Filename, pkg.Fset.Position(b.Pos()).Filename)
	})
	comments := map[ast.Node][]*ast.CommentGroup{}
	for _, file := range syntax {
		maps.Copy(comments, ast.NewCommentMap(pkg.Fset, file, file.Comments))
	}
	return &converter{
		ctx:            ctx,
		pkg:            pkg,
		fset:           pkg.Fset,
		opts:           opts,
		pkgFiles:       syntax,
		methodsByRecv:  map[string][]*node.Method{},
		structByQName:  map[string]*node.Struct{},
		ifaceByQName:   map[string]*node.Interface{},
		aliasByQName:   map[string]*node.Alias{},
		commentsByNode: comments,
		out:            &node.Package{Name: pkg.Name, Path: pkg.PkgPath},
	}
}

// run executes the conversion pipeline and returns the populated
// [node.Package].
func (c *converter) run() *node.Package {
	stampFrontendMarker(c.out)
	c.convertFiles()
	c.collectPackageDoc()
	c.convertDecls()
	c.attachMethods()
	c.detectEnums()
	c.validateDirectives()
	return c.out
}

// convertFiles produces a [node.File] entry per syntax file in the
// converter's deterministic order, populating each file's imports
// and the file-level doc-comment block. Directives in the comment
// block above `package` are intentionally not recorded on the file
// node — Go has no per-file directive concept, so they belong on
// the package (see [converter.collectPackageDoc]).
func (c *converter) convertFiles() {
	for _, syntax := range c.pkgFiles {
		pos := c.fset.Position(syntax.Package)
		f := &node.File{
			BaseNode: node.BaseNode{
				SourcePos: posOf(c.fset, syntax.Package),
				DocLines:  docLinesFromCommentGroup(syntax.Doc),
			},
			Name: filepath.Base(pos.Filename),
			Path: pos.Filename,
		}
		for _, imp := range syntax.Imports {
			rec := &node.Import{
				BaseNode: node.BaseNode{
					SourcePos: posOf(c.fset, imp.Pos()),
					DocLines:  docLinesFromCommentGroup(imp.Doc),
				},
				Path: unquoteImportPath(imp.Path.Value),
			}
			if imp.Name != nil {
				rec.Alias = imp.Name.Name
			}
			f.Imports = append(f.Imports, rec)
		}
		c.out.Files = append(c.out.Files, f)
	}
	c.dedupeImports()
}

// collectPackageDoc hoists the first non-empty package-level doc
// comment found across files into [node.Package.DocLines] together
// with its directives. Go's convention places the package doc above
// the `package` clause in a `doc.go` file, but accepting the first
// non-empty occurrence across all files matches the behaviour of
// go/doc and other tooling.
func (c *converter) collectPackageDoc() {
	for _, syntax := range c.pkgFiles {
		if syntax.Doc == nil {
			continue
		}
		// go/parser guarantees a non-nil CommentGroup carries at
		// least one Comment, so the docs slice is non-empty here.
		docs, dirs := c.docsAndDirectives(syntax.Doc, nil, nil)
		c.out.DocLines = docs
		c.out.DirectiveList = dirs
		return
	}
}

// dedupeImports populates [node.Package.Imports] with the unique
// union of every file's import paths in first-seen order. Each
// package-level [node.Import] is constructed with a fresh
// [node.BaseNode] so the per-file and package-level entries own
// independent metadata bags — value-copying the BaseNode would
// share the lazily-allocated MetaBag pointer and cause mutations
// on one to leak to the other.
func (c *converter) dedupeImports() {
	seen := map[string]struct{}{}
	for _, f := range c.out.Files {
		for _, imp := range f.Imports {
			if _, dup := seen[imp.Path]; dup {
				continue
			}
			seen[imp.Path] = struct{}{}
			docs := append([]string(nil), imp.DocLines...)
			c.out.Imports = append(c.out.Imports, &node.Import{
				BaseNode: node.BaseNode{
					SourcePos: imp.SourcePos,
					DocLines:  docs,
				},
				Path:  imp.Path,
				Alias: imp.Alias,
			})
		}
	}
}

// convertDecls walks the AST top-level declarations of every file
// in deterministic order and dispatches each decl to its kind
// handler.
func (c *converter) convertDecls() {
	for _, syntax := range c.pkgFiles {
		for _, decl := range syntax.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				c.convertGenDecl(d)
			case *ast.FuncDecl:
				c.convertFuncDecl(d)
			}
		}
	}
}

// convertGenDecl dispatches a generic-declaration block (var, const,
// type) to its per-kind converter. Tokens outside the [token.TYPE]
// / [token.VAR] / [token.CONST] family cannot reach this point —
// the [go/parser] only produces [ast.GenDecl] for those three —
// so the default branch is intentionally empty.
func (c *converter) convertGenDecl(d *ast.GenDecl) {
	switch d.Tok { //nolint:exhaustive // ast.GenDecl is only built with TYPE / VAR / CONST tokens
	case token.TYPE:
		for _, spec := range d.Specs {
			if ts, ok := spec.(*ast.TypeSpec); ok {
				c.convertTypeSpec(ts, d)
			}
		}
	case token.VAR:
		for _, spec := range d.Specs {
			if vs, ok := spec.(*ast.ValueSpec); ok {
				c.convertVarSpec(vs, d)
			}
		}
	case token.CONST:
		c.convertConstBlock(d)
	}
}

// unquoteImportPath strips the surrounding quotes from an import
// spec literal. [go/parser] guarantees every [ast.ImportSpec.Path]
// is a double-quoted [ast.BasicLit], so the function is a pure
// substring extract.
func unquoteImportPath(literal string) string {
	return literal[1 : len(literal)-1]
}
