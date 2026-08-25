// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"go/ast"
	"go/types"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/node"
)

// typeParamsFromList converts an AST type-parameter list into the
// language-agnostic [node.TypeParam] slice. owner is the host
// declaration (struct, interface, function, method, alias) the
// type parameters belong to; the back-pointer is set here so
// downstream consumers can reach the host without an extra
// [node.RewireOwners] pass.
//
// Each parameter's constraint is built from the type-checker's
// resolved bound. Type-set constraints (Go's `~int | ~string`
// form) stamp [MetaConstraintTerms] on the resulting [node.TypeParam]
// rather than inflating [node.Constraint] with Go-specific fields,
// keeping the node model language-agnostic.
func (c *converter) typeParamsFromList(owner node.Node, fl *ast.FieldList) []*node.TypeParam {
	if fl == nil {
		return nil
	}
	var out []*node.TypeParam
	for _, field := range fl.List {
		fallback := c.constraintFallbackFromExpr(field.Type)
		rawExpr := c.printExpr(field.Type)
		for _, name := range field.Names {
			constraint, terms := c.buildConstraintForName(name, fallback)
			if constraint != nil {
				constraint.Raw = rawExpr
			}
			tp := &node.TypeParam{
				BaseNode:   node.BaseNode{SourcePos: posOf(c.fset, name.Pos())},
				Name:       name.Name,
				Constraint: constraint,
				Owner:      owner,
			}
			if len(terms) > 0 {
				MetaConstraintTerms.SetAt(tp.EnsureMeta(), terms, meta.AuthorityPlugin, FrontendName, tp.Pos())
			}
			out = append(out, tp)
		}
	}
	return out
}

// constraintFallbackFromExpr returns a best-effort [node.TypeRef]
// for an AST constraint expression. Type-set expressions yield nil
// — type-set detail rides on metadata after the type checker
// resolves the bound — but named-bound expressions (interfaces,
// `comparable`, qualified-name references) become a [node.TypeRef]
// the caller can fall back to when the type checker did not
// produce an explicit interface form.
func (c *converter) constraintFallbackFromExpr(expr ast.Expr) *node.TypeRef {
	if expr == nil || isTypeSetExpr(expr) {
		return nil
	}
	if tv, ok := c.pkg.TypesInfo.Types[expr]; ok && tv.Type != nil {
		return c.typeRefOf(tv.Type)
	}
	return nil
}

// buildConstraintForName resolves the type-checker's bound for the
// AST identifier and converts it into a [node.Constraint] paired
// with the type-set terms (if any). The terms are returned
// separately so the caller can stamp [MetaConstraintTerms] on the
// owning [node.TypeParam].
//
// Returns (nil, nil) when the type-checker did not produce a
// resolved [types.TypeParam] for name — this only happens when the
// package contains type-check errors, in which case the loader has
// already emitted a positioned diagnostic and the resulting
// [node.TypeParam] simply carries no constraint (the
// language-agnostic equivalent of `any`).
func (c *converter) buildConstraintForName(
	name *ast.Ident,
	fallback *node.TypeRef,
) (*node.Constraint, []ConstraintTerm) {
	obj, _ := c.pkg.TypesInfo.Defs[name].(*types.TypeName)
	if obj == nil {
		return nil, nil
	}
	// For a resolved type-param identifier go/types guarantees the
	// object's Type is a *types.TypeParam. The Underlying of its
	// Constraint, however, is *types.Basic (Invalid) on malformed
	// source where the bound failed to resolve — the only reachable
	// non-Interface case.
	tp := obj.Type().(*types.TypeParam) //nolint:forcetypeassert // go/types invariant
	iface, ok := tp.Constraint().Underlying().(*types.Interface)
	if !ok {
		return nil, nil
	}
	return c.constraintFromInterface(iface, fallback)
}

// constraintFromInterface converts a constraint-interface (Go's
// implicit interface form behind every type-parameter bound) into
// a [node.Constraint] + a type-set term slice. Method-set entries
// are not modelled here — Go constraints with explicit methods are
// rare in practice; when they appear, the underlying interface
// shape comes through via the constraint's Embedded type-ref to
// the constraint-interface itself.
func (c *converter) constraintFromInterface(
	iface *types.Interface,
	fallback *node.TypeRef,
) (*node.Constraint, []ConstraintTerm) {
	out := &node.Constraint{}
	var terms []ConstraintTerm
	for emb := range iface.EmbeddedTypes() {
		if union, isUnion := emb.(*types.Union); isUnion {
			terms = append(terms, c.constraintTermsFromUnion(union)...)
			continue
		}
		if ref := c.typeRefOf(emb); ref != nil {
			out.Embedded = append(out.Embedded, ref)
		}
	}
	if out.IsAny() && fallback != nil {
		out.Embedded = []*node.TypeRef{fallback}
	}
	if out.IsAny() && len(terms) == 0 {
		return nil, nil
	}
	return out, terms
}

// isTypeSetExpr reports whether an AST constraint expression is a
// type-set (contains `~` or `|`). The check is intentionally
// shallow — type-set expressions are recognised before recursive
// type-ref conversion runs.
func isTypeSetExpr(expr ast.Expr) bool {
	switch e := expr.(type) {
	case *ast.UnaryExpr:
		return e.Op.String() == "~"
	case *ast.BinaryExpr:
		return e.Op.String() == "|"
	case *ast.InterfaceType:
		// go/parser always allocates a non-nil *ast.FieldList for
		// InterfaceType.Methods, even when the interface is empty.
		for _, f := range e.Methods.List {
			if isTypeSetExpr(f.Type) {
				return true
			}
		}
	}
	return false
}
