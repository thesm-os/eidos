// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"go/ast"
	"go/types"
)

// convertTypeSpec routes a single [ast.TypeSpec] (one entry in a
// `type ( … )` block) to the per-kind converter. The owning
// [ast.GenDecl] carries any block-level doc / directive comments
// that should be inherited when the spec itself has none.
func (c *converter) convertTypeSpec(ts *ast.TypeSpec, owner *ast.GenDecl) {
	docs, dirs := c.docsAndDirectives(ts.Doc, owner.Doc, ts.Comment)

	obj, ok := c.pkg.TypesInfo.Defs[ts.Name].(*types.TypeName)
	if !ok || obj == nil {
		// A frontend-stage type-check failure means we have no
		// resolved type info for this spec. The associated
		// packages.Error has already surfaced as a positioned
		// diagnostic; skipping the conversion keeps the rest of the
		// package usable.
		return
	}

	switch underlying := obj.Type().Underlying().(type) {
	case *types.Struct:
		c.convertStruct(ts, obj, underlying, docs, dirs)
	case *types.Interface:
		c.convertInterface(ts, obj, underlying, docs, dirs)
	default:
		// Everything else (named primitives, type aliases, function-
		// type aliases, channel-type aliases) becomes a [node.Alias].
		c.convertAlias(ts, obj, docs, dirs)
	}
}
