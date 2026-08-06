// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"go/ast"
	"go/types"

	"go.thesmos.sh/eidos/node"
)

// convertConstBlock converts an entire `const ( … )` block into one
// or more [node.Constant] entries. The block is treated as a unit
// so [detectEnums] can later coalesce typed-iota groups into a
// single [node.Enum] without losing the per-constant detail.
//
// Iota inheritance is honoured — entries that omit a type
// expression but inherit a typed constant's type from a preceding
// spec carry that inherited type on [node.Constant.Type]. Untyped
// constants (`const X = 1`) leave Type nil to match the
// [node.Constant] contract.
func (c *converter) convertConstBlock(d *ast.GenDecl) {
	for _, spec := range d.Specs {
		// go/parser only emits *ast.ValueSpec inside a token.CONST
		// GenDecl.Specs; the cast is total.
		c.convertConstSpec(spec.(*ast.ValueSpec), d) //nolint:forcetypeassert // parser invariant
	}
}

// convertConstSpec produces one [node.Constant] per declared name
// on a `const` spec. The constant's value is the type-checker's
// resolved constant value when available, falling back to the
// source-printed initialiser expression otherwise — both forms
// preserve the original literal accurately enough for downstream
// generators.
func (c *converter) convertConstSpec(vs *ast.ValueSpec, owner *ast.GenDecl) {
	docs, dirs := c.docsAndDirectives(vs.Doc, owner.Doc, vs.Comment)
	for _, name := range vs.Names {
		if name.Name == "_" {
			// Blank-identifier consts (`const _ = …`) hold an iota
			// slot without contributing a referencable identifier;
			// skip them so duplicate `_` entries across a const
			// block don't collide on qualified-name registration.
			continue
		}
		// go/types records a *types.Const for a const-decl name even
		// when the value or type annotation fails to resolve — but
		// records nil for the losing side of a redeclaration, so the
		// lookup is only total for a package that type-checks. A
		// frontend meets whatever the author last saved, so it must
		// not be. The redeclaration is already reported as a
		// diagnostic by the loader; skipping the name keeps the rest
		// of the block convertible instead of aborting the walk on a
		// nil-interface conversion.
		obj, ok := c.pkg.TypesInfo.Defs[name].(*types.Const)
		if !ok {
			continue
		}
		cst := &node.Constant{
			BaseNode: node.BaseNode{
				SourcePos:     posOf(c.fset, name.Pos()),
				DocLines:      docs,
				DirectiveList: dirs,
			},
			Name:    obj.Name(),
			Package: c.out.Path,
			Value:   c.constValue(obj),
		}
		// Populate Type for typed constants — either explicitly
		// typed (vs.Type != nil) or implicitly typed via iota
		// inheritance from a preceding spec. Untyped constants
		// (Go's `const X = 1` with no type bound on the block)
		// leave Type nil to match the [node.Constant] contract.
		if isTypedConst(obj) {
			cst.Type = c.typeRefOf(obj.Type())
			if cst.Type != nil && vs.Type != nil {
				cst.Type.SourcePos = posOf(c.fset, vs.Type.Pos())
			}
		}
		c.stampConstantMeta(cst, obj)
		c.out.Constants = append(c.out.Constants, cst)
	}
}

// isTypedConst reports whether obj has a non-untyped constant type
// — i.e. it has either an explicit type expression in source or
// inherits a typed bound from a preceding iota-group entry.
// Untyped constants (`const X = 1` with no type bound) report
// false.
func isTypedConst(obj *types.Const) bool {
	basic, ok := obj.Type().(*types.Basic)
	if !ok {
		return true
	}
	return basic.Info()&types.IsUntyped == 0
}

// constValue returns the source-faithful textual form of a
// constant's value via [go/constant.Value.ExactString]. [go/types]
// always populates a [types.Const]'s value for any const the type-
// checker resolved (the converter has already filtered unresolved
// consts via the [*types.Const] type-assertion in
// [convertConstSpec]), so the function is a pure projection.
func (*converter) constValue(obj *types.Const) string {
	return obj.Val().ExactString()
}
