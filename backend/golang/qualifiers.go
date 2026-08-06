// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"go/ast"
	"go/parser"
	"go/token"

	"go.thesmos.sh/eidos/writer"
)

// fileRefs is everything one AST traversal of a rendered body
// yields. The three sets answer three different questions and are
// collected together because the traversal — not the bookkeeping —
// is the cost.
type fileRefs struct {
	// parsed reports whether the traversal ran. The zero value is
	// false, which is what makes the sets safe to consult without a
	// separate ok flag threaded through every caller: an empty
	// qualifier set is indistinguishable from "nothing is
	// referenced" and would prune an entire import block.
	parsed bool

	// qualifiers holds the X of every selector expression that is a
	// bare identifier: the `pkg` of `pkg.Symbol`. An import whose
	// alias is absent here is referenced by nothing in the body.
	qualifiers map[string]struct{}

	// declared holds every name the file binds, anywhere, at any
	// scope. Deliberately scope-blind — see [collectRefs].
	declared map[string]struct{}

	// topLevel holds the package-scope names this file declares. A
	// sibling file rendered into the same package can select on
	// these without importing anything, which is what makes the
	// unresolved-qualifier check package-scoped rather than
	// file-scoped.
	topLevel map[string]struct{}
}

// collectRefs parses src and returns the reference sets one
// traversal yields. The zero [fileRefs] — parsed false, every set
// nil — is returned when src does not parse, and every consumer
// treats it as "assume nothing".
//
// # Why not the resolver's own collector
//
// goimports builds the same set by skipping identifiers the parser
// resolved (`xident.Obj != nil`). That needs [ast.Object], which
// [go/parser] documents as deprecated and recommends against via
// [parser.SkipObjectResolution]; goimports is stuck with it and
// panics if it is ever unavailable. If object resolution is
// eventually removed, `Obj` goes nil everywhere and a rule built on
// it inverts silently — every identifier reads as unresolved. The
// explicit declared set has no such failure mode: it is built from
// syntax that cannot go away.
//
// # Scope-blindness is deliberate
//
// declared records a binding regardless of the block it occurs in,
// so a local named `time` anywhere in the file masks a genuine
// `time.Now()` reference elsewhere in it. The subtraction is
// one-sided on purpose. Over-counting bindings keeps an import and
// suppresses a report; under-counting deletes an import and invents
// a report. Only the second silently changes what the generated
// code does.
//
// Allocation: four maps plus the parsed AST, all released when the
// caller returns. Cost is dominated by the parse — roughly 3 µs at
// one declaration and 3 ms at a thousand — not by the walk, which
// is under 15% of that.
func collectRefs(src []byte, filename string) fileRefs {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.SkipObjectResolution)
	if err != nil {
		return fileRefs{}
	}

	refs := fileRefs{
		parsed:     true,
		qualifiers: map[string]struct{}{},
		declared:   map[string]struct{}{},
		topLevel:   map[string]struct{}{},
	}
	collectTopLevel(f, refs.topLevel, refs.declared)

	ast.Inspect(f, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			// Only a bare identifier in X position can be a package
			// qualifier. In `a.b.c` the outer X is itself a selector,
			// so this records `a` once and nothing else — which is
			// exactly right, since only `a` could be an import.
			if id, ok := x.X.(*ast.Ident); ok {
				refs.qualifiers[id.Name] = struct{}{}
			}
		case *ast.FuncDecl:
			addFieldNames(x.Recv, refs.declared)
			refs.declared[x.Name.Name] = struct{}{}
		case *ast.FuncType:
			addFieldNames(x.TypeParams, refs.declared)
			addFieldNames(x.Params, refs.declared)
			addFieldNames(x.Results, refs.declared)
		case *ast.ValueSpec:
			addIdents(x.Names, refs.declared)
		case *ast.TypeSpec:
			refs.declared[x.Name.Name] = struct{}{}
			addFieldNames(x.TypeParams, refs.declared)
		case *ast.AssignStmt:
			if x.Tok == token.DEFINE {
				addExprIdents(x.Lhs, refs.declared)
			}
		case *ast.RangeStmt:
			if x.Tok == token.DEFINE {
				addExprIdents([]ast.Expr{x.Key, x.Value}, refs.declared)
			}
		case *ast.TypeSwitchStmt:
			// `switch v := x.(type)` binds v; the assign arrives as a
			// DEFINE AssignStmt and is handled above when Inspect
			// descends into it.
			return true
		}
		return true
	})
	return refs
}

// collectTopLevel records the package-scope names f declares into
// both topLevel and declared. Methods are excluded: a method name
// lives in its receiver's namespace, not the package's, so a
// sibling file cannot select on it without naming the receiver.
func collectTopLevel(f *ast.File, topLevel, declared map[string]struct{}) {
	for _, d := range f.Decls {
		switch x := d.(type) {
		case *ast.FuncDecl:
			if x.Recv != nil {
				continue
			}
			topLevel[x.Name.Name] = struct{}{}
			declared[x.Name.Name] = struct{}{}
		case *ast.GenDecl:
			for _, spec := range x.Specs {
				switch s := spec.(type) {
				case *ast.ValueSpec:
					addIdents(s.Names, topLevel)
					addIdents(s.Names, declared)
				case *ast.TypeSpec:
					topLevel[s.Name.Name] = struct{}{}
					declared[s.Name.Name] = struct{}{}
				}
			}
		}
	}
}

// addFieldNames records the names bound by a field list — receiver,
// parameters, results, type parameters. A nil list and an unnamed
// entry (an anonymous result, an embedded type) contribute nothing.
func addFieldNames(list *ast.FieldList, into map[string]struct{}) {
	if list == nil {
		return
	}
	for _, f := range list.List {
		addIdents(f.Names, into)
	}
}

// addIdents records each identifier's name, skipping the blank
// identifier — `_` binds nothing and treating it as a declared name
// would mask an import aliased to it.
func addIdents(idents []*ast.Ident, into map[string]struct{}) {
	for _, id := range idents {
		if id == nil || id.Name == writer.BlankAlias {
			continue
		}
		into[id.Name] = struct{}{}
	}
}

// addExprIdents records the identifiers among exprs, ignoring
// anything that is not a bare name. A short variable declaration's
// left-hand side may hold a selector or index expression when it
// mixes assignment with declaration; those bind nothing new.
func addExprIdents(exprs []ast.Expr, into map[string]struct{}) {
	for _, e := range exprs {
		if id, ok := e.(*ast.Ident); ok && id.Name != writer.BlankAlias {
			into[id.Name] = struct{}{}
		}
	}
}

// pruneImports returns the entries of tracked that the rendered body
// can actually reach, preserving order — sorting and grouping belong
// to [writeImportBlock].
//
// An unused import is a compile error in Go. Until the goimports
// resolve pass is removed this duplicates a deletion that pass
// already performs; afterwards it is the only thing standing between
// an unreferenced [emit.File.Imports] entry and uncompilable output.
//
// Three keep rules, in order of how often they fire:
//
//   - the alias appears in the body as a selector qualifier;
//   - the alias is `_` or `.`, which no body text can name, so
//     absence from qualifiers carries no information;
//   - the walk did not run, in which case nothing is known and
//     nothing is dropped. [runGoFormat] owns the diagnostic for that
//     case; a second one would say less about the same bytes.
func pruneImports(tracked []writer.Import, refs fileRefs) []writer.Import {
	if !refs.parsed {
		return tracked
	}
	out := tracked[:0:0]
	for _, imp := range tracked {
		if imp.Alias == writer.BlankAlias || imp.Alias == writer.DotAlias {
			out = append(out, imp)
			continue
		}
		if _, referenced := refs.qualifiers[imp.Alias]; referenced {
			out = append(out, imp)
		}
	}
	return out
}

// shadowedImports returns the paths kept by [pruneImports] whose
// alias is also a name the file declares, in import-set order —
// which is [writer.ImportSet.Imp] call order, and so already
// deterministic for a given emit graph.
//
// These are the imports the rule cannot judge. A local named
// `strings` produces `strings.Builder` selectors of its own, so an
// unreferenced `"strings"` import reads as referenced and survives —
// and the file then fails to build with "imported and not used",
// with nothing in the run pointing at why.
//
// goimports resolved this correctly and deleted the import, so
// naming it is what keeps the finalise-chain replacement from
// trading a repair for a silent breakage. It is reported rather than
// acted on because the alternative — dropping the import — is the
// under-counting direction, which changes what compiling code does.
func shadowedImports(kept []writer.Import, refs fileRefs) []writer.Import {
	if !refs.parsed {
		return nil
	}
	var out []writer.Import
	for _, imp := range kept {
		if imp.Alias == writer.BlankAlias || imp.Alias == writer.DotAlias {
			continue
		}
		if _, shadowed := refs.declared[imp.Alias]; shadowed {
			out = append(out, imp)
		}
	}
	return out
}
