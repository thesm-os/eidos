// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/writer"
)

// fileRefs is everything one AST traversal of a rendered body
// yields. The sets answer different questions and are collected
// together because the traversal — not the bookkeeping — is the
// cost.
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

	// typeQualifiers holds the qualifiers that appear inside a type
	// position — a struct field's type, a parameter's, a result's, a
	// declared variable's, a composite literal's. No local can stand
	// in X position there, so an alias in this set is proof its
	// import is used, whatever declared holds. This is what keeps
	// the scope-blind caution from calling `tx tx.Tx` a shadow: the
	// parameter's name binds `tx` and its type uses the package, in
	// the one grammatical position the two cannot be confused.
	typeQualifiers map[string]struct{}

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
// Allocation: five maps plus the parsed AST, all released when the
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
		parsed:         true,
		qualifiers:     map[string]struct{}{},
		declared:       map[string]struct{}{},
		typeQualifiers: map[string]struct{}{},
		topLevel:       map[string]struct{}{},
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
		case *ast.Field:
			// One case covers every signature and struct slot: a
			// struct's fields, a func's receiver, params, results and
			// type parameters, and an interface's methods and embeds
			// all arrive as fields.
			addTypeQualifiers(x.Type, refs.typeQualifiers)
		case *ast.CompositeLit:
			addTypeQualifiers(x.Type, refs.typeQualifiers)
		case *ast.TypeAssertExpr:
			// Nil for `.(type)` in a type switch, which asserts no
			// type; addTypeQualifiers tolerates it.
			addTypeQualifiers(x.Type, refs.typeQualifiers)
		case *ast.FuncDecl:
			addFieldNames(x.Recv, refs.declared)
			refs.declared[x.Name.Name] = struct{}{}
		case *ast.FuncType:
			addFieldNames(x.TypeParams, refs.declared)
			addFieldNames(x.Params, refs.declared)
			addFieldNames(x.Results, refs.declared)
		case *ast.ValueSpec:
			addIdents(x.Names, refs.declared)
			addTypeQualifiers(x.Type, refs.typeQualifiers)
		case *ast.TypeSpec:
			refs.declared[x.Name.Name] = struct{}{}
			addFieldNames(x.TypeParams, refs.declared)
			addTypeQualifiers(x.Type, refs.typeQualifiers)
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

// addTypeQualifiers records every qualifier inside one type
// expression — the `tx` of a `tx.Tx` field, parameter, result,
// element or embedded type, however deeply the type composes it.
//
// The whole subtree is walked rather than the top level matched,
// because a qualifier nests inside every composite type form:
// `[]tx.Tx`, `map[string]tx.Tx`, `func(tx.Tx) error`, `List[tx.Tx]`.
// A nil expr — an unconstrained type assertion's `.(type)` —
// contributes nothing.
func addTypeQualifiers(expr ast.Expr, into map[string]struct{}) {
	if expr == nil {
		return
	}
	ast.Inspect(expr, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if id, ok := sel.X.(*ast.Ident); ok {
				into[id.Name] = struct{}{}
			}
		}
		return true
	})
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
// A qualifier in a type position settles the question the other way
// and is checked first: no local can stand in X position inside a
// struct field's type, a parameter's or a result's, so an alias used
// there is proven live and reporting it would call working code
// broken. `func Commit(ctx context.Context, tx tx.Tx)` is the shape
// — a package whose name is also its natural parameter name — and it
// warned on every run until the positions were told apart.
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
		if _, proven := refs.typeQualifiers[imp.Alias]; proven {
			continue
		}
		if _, shadowed := refs.declared[imp.Alias]; shadowed {
			out = append(out, imp)
		}
	}
	return out
}

// packageKey identifies the Go package a target renders into. Two
// targets sharing one are two files of the same package, so a
// package-scope name either declares can satisfy a selector in the
// other without any import.
type packageKey struct {
	dir string
	pkg string
}

// keyFor returns the [packageKey] of a target.
func keyFor(t emit.Target) packageKey {
	return packageKey{dir: t.Dir, pkg: t.Package}
}

// boundNames returns the local names the surviving imports bind.
//
// The blank alias binds nothing, and the dot alias binds an unknown
// set — see [unresolvedCandidates], which refuses to judge a file
// carrying one at all.
func boundNames(imports []writer.Import) map[string]struct{} {
	out := make(map[string]struct{}, len(imports))
	for _, imp := range imports {
		if imp.Alias == writer.BlankAlias || imp.Alias == writer.DotAlias {
			continue
		}
		out[imp.Alias] = struct{}{}
	}
	return out
}

// unresolvedCandidates returns the qualifiers this file names that
// nothing in it can bind: not an import, and not a name the file
// declares. The result is sorted, because map iteration order would
// otherwise make `-diag-format json` output differ between runs of
// the same input.
//
// Candidates are not yet a verdict. A sibling target rendered into
// the same package may declare the name at package scope, which is
// reachable without an import and is exactly the shape multi-output
// routing produces — [Backend.Render] subtracts that union before
// reporting.
//
// A dot import suspends the check entirely. It merges an unknown
// set of exported names into file scope, so any qualifier could
// legitimately come from it and every report would be a guess. The
// one-sided direction holds: a missed report costs a `go build`
// error the developer already gets.
func unresolvedCandidates(refs fileRefs, tracked []writer.Import) []string {
	if !refs.parsed {
		return nil
	}
	for _, imp := range tracked {
		if imp.Alias == writer.DotAlias {
			return nil
		}
	}
	bound := boundNames(tracked)
	var out []string
	for q := range refs.qualifiers {
		if _, declared := refs.declared[q]; declared {
			continue
		}
		if _, imported := bound[q]; imported {
			continue
		}
		out = append(out, q)
	}
	slices.Sort(out)
	return out
}

// unionTopLevel groups the package-scope names every rendered target
// declares by the package it renders into.
//
// Computed across the whole run because that is the smallest scope
// at which the question is answerable: a target selecting on a name
// its sibling declares is correct Go, and no per-target view can
// tell that from a missing import. Skipped targets contribute
// nothing — they produced no file.
func unionTopLevel(keys []emit.Target, results []renderResult) map[packageKey]map[string]struct{} {
	out := map[packageKey]map[string]struct{}{}
	for i, res := range results {
		if res.skip || len(res.topLevel) == 0 {
			continue
		}
		k := keyFor(keys[i])
		names, ok := out[k]
		if !ok {
			names = map[string]struct{}{}
			out[k] = names
		}
		for n := range res.topLevel {
			names[n] = struct{}{}
		}
	}
	return out
}

// unresolvedAfterPackage returns the candidates no sibling in the
// same package declares — the reportable set.
func unresolvedAfterPackage(candidates []string, declared map[string]struct{}) []string {
	if len(candidates) == 0 {
		return nil
	}
	out := candidates[:0:0]
	for _, q := range candidates {
		if _, ok := declared[q]; ok {
			continue
		}
		out = append(out, q)
	}
	return out
}
