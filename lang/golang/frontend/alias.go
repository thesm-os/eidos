// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"go/ast"
	"go/types"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/node"
)

// convertAlias converts a non-struct, non-interface type-spec into a
// [node.Alias] and appends it to the converter's package. Covers
// both forms:
//
//   - `type X = Y` — true alias (Go 1.9+); [node.Alias.IsAlias] true.
//   - `type X Y`  — type definition; [node.Alias.IsAlias] false.
//
// The target type is taken from the type-checker's resolved view of
// the underlying type when the spec is a type definition, and from
// the alias's right-hand side when the spec is a true alias.
// Generic type parameters declared on the alias are converted via
// [typeParamsFromList].
func (c *converter) convertAlias(
	ts *ast.TypeSpec,
	obj *types.TypeName,
	docs []string,
	dirs []*directive.Directive,
) {
	a := &node.Alias{
		BaseNode: node.BaseNode{
			SourcePos:     posOf(c.fset, ts.Pos()),
			DocLines:      docs,
			DirectiveList: dirs,
		},
		Name:    obj.Name(),
		Package: c.out.Path,
		Target:  c.aliasTarget(ts, obj),
		IsAlias: ts.Assign.IsValid(),
	}
	a.TypeParams = c.typeParamsFromList(a, ts.TypeParams)
	c.stampAliasMeta(a, obj)
	c.out.Aliases = append(c.out.Aliases, a)
	c.aliasByQName[a.QName()] = a
}

// aliasTarget returns the [node.TypeRef] the alias points at. True
// aliases (`type X = Y`) take the RHS expression's type; type
// definitions (`type X Y`) take the underlying type so consumers
// see what the new named type wraps. go/types always records a
// TypeAndValue for a top-level alias RHS (an Invalid basic when
// the source is malformed), so the lookup is always populated.
func (c *converter) aliasTarget(ts *ast.TypeSpec, obj *types.TypeName) *node.TypeRef {
	if ts.Assign.IsValid() {
		return c.typeRefOf(c.pkg.TypesInfo.Types[ts.Type].Type)
	}
	// Type definitions (`type X Y`) always produce a *types.Named
	// from go/types; the underlying type is what consumers want.
	return c.typeRefOf(obj.Type().Underlying())
}
