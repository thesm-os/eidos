// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"go/ast"
	"go/printer"
	"go/types"
	"strings"

	"go.thesmos.sh/eidos/node"
)

// convertVarSpec converts a `var` spec (one entry within a
// `var ( … )` block or a single `var` line) into one or more
// [node.Variable] entries. Each declared name on the LHS produces
// a variable; the type information is the source-level declared
// type — variables whose type was inferred from their initialiser
// leave [node.Variable.Type] nil, per [node.Variable]'s contract.
func (c *converter) convertVarSpec(vs *ast.ValueSpec, owner *ast.GenDecl) {
	docs, dirs := c.docsAndDirectives(vs.Doc, owner.Doc, vs.Comment)
	for i, name := range vs.Names {
		if name.Name == "_" {
			// Blank-identifier declarations (`var _ = …`) are
			// syntactic discard-markers in Go; they carry no name
			// downstream consumers can reference, and recording them
			// produces duplicate qualified names when the same
			// pattern appears in multiple files of one package.
			continue
		}
		obj, ok := c.pkg.TypesInfo.Defs[name].(*types.Var)
		if !ok || obj == nil {
			continue
		}
		v := &node.Variable{
			BaseNode: node.BaseNode{
				SourcePos:     posOf(c.fset, name.Pos()),
				DocLines:      docs,
				DirectiveList: dirs,
			},
			Name:    obj.Name(),
			Package: c.out.Path,
		}
		if vs.Type != nil {
			v.Type = c.typeRefOf(obj.Type())
			if v.Type != nil {
				v.Type.SourcePos = posOf(c.fset, vs.Type.Pos())
			}
		}
		v.InitExpr = c.varInitExpr(vs, i)
		c.out.Variables = append(c.out.Variables, v)
	}
}

// varInitExpr returns the verbatim source form of the i-th
// initialiser expression on a `var` spec, or "" when no
// initialiser exists at that position. Multiple names sharing one
// RHS expression (Go's `var a, b = oneCall()` form) all receive
// the same source text — the printed form of the single RHS
// expression.
func (c *converter) varInitExpr(vs *ast.ValueSpec, idx int) string {
	switch {
	case len(vs.Values) == 0:
		return ""
	case len(vs.Values) == len(vs.Names):
		return c.printExpr(vs.Values[idx])
	default:
		return c.printExpr(vs.Values[0])
	}
}

// printExpr returns the gofmt-printed source form of expr using the
// converter's [token.FileSet]. Used to preserve verbatim initialiser
// text and constant-value text. All callers pass a non-nil
// expression resolved from the AST.
func (c *converter) printExpr(expr ast.Expr) string {
	var b strings.Builder
	_ = printer.Fprint(&b, c.fset, expr) //nolint:errcheck // strings.Builder never returns an error
	return b.String()
}
