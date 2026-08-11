// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"go/ast"
	"go/types"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// convertInterface converts an interface type-spec into a
// [node.Interface] and appends it to the converter's package.
// Method docs, embed docs, and type-expression positions are
// recovered from the AST interface body so consumers see the same
// shape they would for a freshly-parsed package.
//
// Constraint-interface facts (Go's `~int | ~string` form when the
// interface is used as a generic bound) ride on `go.*` metadata
// rather than first-class node fields, keeping the
// [node.Interface] shape language-agnostic.
func (c *converter) convertInterface(
	ts *ast.TypeSpec,
	obj *types.TypeName,
	it *types.Interface,
	docs []string,
	dirs []*directive.Directive,
) {
	i := &node.Interface{
		BaseNode: node.BaseNode{
			SourcePos:     posOf(c.fset, ts.Pos()),
			DocLines:      docs,
			DirectiveList: dirs,
		},
		Name:    obj.Name(),
		Package: c.out.Path,
	}
	i.TypeParams = c.typeParamsFromList(i, ts.TypeParams)
	c.populateInterfaceBody(i, ts.Type, it)
	c.stampInterfaceMeta(i, it, obj)
	c.out.Interfaces = append(c.out.Interfaces, i)
	c.ifaceByQName[i.QName()] = i
}

// populateInterfaceBody walks an AST interface expression in
// lock-step with the type-checker's [types.Interface] view to
// produce [node.Method] and [node.Embed] entries.
func (c *converter) populateInterfaceBody(i *node.Interface, expr ast.Expr, it *types.Interface) {
	astIface, ok := expr.(*ast.InterfaceType)
	if !ok || astIface.Methods == nil {
		c.populateInterfaceFromTypeOnly(i, it)
		return
	}
	for _, field := range astIface.Methods.List {
		docs := docLinesFromCommentGroup(field.Doc)
		// Both comment groups contribute directives. A field is the
		// one shape where Go offers two, and the trailing one is
		// where per-field metadata naturally goes — it sits on the
		// line it describes. Reading only Doc dropped it silently,
		// which is the worst available failure: a plugin saw no
		// directive and emitted output as though the line had never
		// been written.
		//
		// Leading first, so DirectiveList arrives in source order.
		//
		// Docs deliberately take only the leading group. A trailing
		// comment is a note on the line, not the entity's doc
		// comment, and folding it in would put it in generated godoc.
		dirs := c.parseDirectives(field.Doc)
		dirs = append(dirs, c.parseDirectives(field.Comment)...)
		typePos := posOf(c.fset, field.Type.Pos())
		if len(field.Names) == 0 {
			c.appendInterfaceEmbed(i, field, docs, dirs, typePos)
			continue
		}
		for _, name := range field.Names {
			c.appendInterfaceMethod(i, it, name, field, docs, dirs)
		}
	}
}

// populateInterfaceFromTypeOnly is the fallback when the AST
// interface expression cannot be resolved. The type-checker's view
// produces structurally-correct methods and embeds without per-entry
// docs and source-expression positions.
func (c *converter) populateInterfaceFromTypeOnly(i *node.Interface, it *types.Interface) {
	for m := range it.ExplicitMethods() {
		sig, _ := m.Type().(*types.Signature)
		method := c.methodFromSignature(m.Name(), sig)
		method.SourcePos = posOf(c.fset, m.Pos())
		i.Methods = append(i.Methods, method)
	}
	for emb := range it.EmbeddedTypes() {
		i.Embeds = append(i.Embeds, &node.Embed{
			Type:     c.typeRefOf(emb),
			Resolved: c.projectForeignInterface(emb),
		})
	}
}

// appendInterfaceEmbed appends one [node.Embed] for an embedded
// interface or constraint expression. Constraint-interface facts
// (type-set unions, `~T` approximate types) ride on metadata
// downstream; this entry-point only records the structural
// position + type-ref.
func (c *converter) appendInterfaceEmbed(
	i *node.Interface,
	field *ast.Field,
	docs []string,
	dirs []*directive.Directive,
	typePos position.Pos,
) {
	ref := c.typeRefForInterfaceEmbed(field.Type)
	if ref != nil {
		ref.SourcePos = typePos
	}
	embed := &node.Embed{
		BaseNode: node.BaseNode{
			SourcePos:     posOf(c.fset, field.Pos()),
			DocLines:      docs,
			DirectiveList: dirs,
		},
		Type: ref,
	}
	if c.pkg.TypesInfo != nil {
		embed.Resolved = c.projectForeignInterface(c.pkg.TypesInfo.TypeOf(field.Type))
	}
	i.Embeds = append(i.Embeds, embed)
}

// typeRefForInterfaceEmbed resolves an interface-embed AST
// expression to a [node.TypeRef]. The expression may be a plain
// named type, a type-set union, or a `~T` approximate-type
// expression — all map to a type-ref via the type-checker's
// resolved view. go/types always records a TypeAndValue for a
// top-level interface-embed expression (an Invalid basic on
// malformed source); typeRefOf handles a nil input by returning
// nil so a missing map entry degrades cleanly.
func (c *converter) typeRefForInterfaceEmbed(expr ast.Expr) *node.TypeRef {
	return c.typeRefOf(c.pkg.TypesInfo.Types[expr].Type)
}

// appendInterfaceMethod appends one [node.Method] for an explicit
// interface method declaration. Parameter docs and type-expression
// positions are recovered via the shared
// [converter.paramsAndReturnsFromSignature] / overlay helpers.
func (c *converter) appendInterfaceMethod(
	i *node.Interface,
	it *types.Interface,
	name *ast.Ident,
	field *ast.Field,
	docs []string,
	dirs []*directive.Directive,
) {
	// populateInterfaceBody only calls this when len(field.Names) > 0,
	// which guarantees field.Type is a *ast.FuncType per the Go
	// grammar; embeds (no Names) go through appendInterfaceEmbed.
	// name.Name is an explicit method of it for any interface the
	// type-checker accepted — but only for those.
	tFunc := field.Type.(*ast.FuncType) //nolint:forcetypeassert // grammar invariant
	var obj *types.Func
	for m := range it.ExplicitMethods() {
		if m.Name() == name.Name {
			obj = m
			break
		}
	}
	if obj == nil {
		// The AST carries a method the type-checker refused to
		// record. `_()` is the reachable case: go/types rejects the
		// blank method name and omits it from ExplicitMethods, while
		// the AST field survives — so the scan above completes with
		// no match.
		//
		// Skip the method and keep converting. The loader already
		// surfaced the type error that caused this ("methods must
		// have a unique non-blank name"), so a second diagnostic
		// here would only restate it; what matters is that the rest
		// of the interface, and every other declaration in the
		// package, still converts. Dereferencing obj instead
		// unwound out of converter.run and discarded all of it,
		// leaving the user a framework panic stapled to a valid
		// syntax error.
		return
	}
	sig, _ := obj.Type().(*types.Signature)
	m := &node.Method{
		BaseNode: node.BaseNode{
			SourcePos:     posOf(c.fset, name.Pos()),
			DocLines:      docs,
			DirectiveList: dirs,
		},
		Name: name.Name,
	}
	m.Params, m.Returns = c.paramsAndReturnsFromSignature(sig, m, tFunc)
	i.Methods = append(i.Methods, m)
}

// methodFromSignature builds a [node.Method] from a name and a
// [types.Signature]. Used as a fallback when the AST interface
// expression cannot be matched; production conversion goes through
// [appendInterfaceMethod] which preserves source-level docs and
// positions.
//
// sig is guaranteed non-nil: every caller obtains it from
// `m.Type().(*types.Signature)` on a *types.Func that the type
// checker has already resolved.
func (c *converter) methodFromSignature(name string, sig *types.Signature) *node.Method {
	m := &node.Method{Name: name}
	if params := sig.Params(); params != nil {
		for p := range params.Variables() {
			m.Params = append(m.Params, &node.Param{
				BaseNode: node.BaseNode{SourcePos: posOf(c.fset, p.Pos())},
				Name:     p.Name(),
				Type:     c.typeRefOf(p.Type()),
				Owner:    m,
			})
		}
		if sig.Variadic() && len(m.Params) > 0 {
			last := m.Params[len(m.Params)-1]
			last.Variadic = true
			if slice, ok := params.At(params.Len() - 1).Type().(*types.Slice); ok {
				last.Type = c.typeRefOf(slice.Elem())
			}
		}
	}
	if results := sig.Results(); results != nil {
		for r := range results.Variables() {
			m.Returns = append(m.Returns, &node.Return{
				Name: returnBindingName(r),
				Type: c.typeRefOf(r.Type()),
			})
		}
	}
	return m
}

// projectForeignInterface renders an embedded interface declared
// outside the package being converted as a [node.Interface] the
// method-set walk can complete against.
//
// The type-checker has the whole method set in hand at conversion
// time, and recording only a type reference discarded it — which took
// every interface embedding `io.Closer` or `fmt.Stringer` out of scope
// for generation, because a projection missing a method describes a
// type that does not satisfy what it claims to.
//
// Nil for anything declared in this package, whose own conversion
// produces the better answer, and for a non-interface embed, which is
// a type-set term rather than a method contributor.
//
// The full method set rather than the explicit one, so the projection
// arrives flattened and the walk needs no second level of recursion
// into it. That also means the result carries no embeds of its own.
//
// The returned interface is not registered in ifaceByQName or emitted
// into the package: it hangs off the embed that needed it, so nothing
// walking the graph sees a declaration no pattern asked for.
func (c *converter) projectForeignInterface(t types.Type) *node.Interface {
	named, ok := t.(*types.Named)
	if !ok || named.Obj() == nil || named.Obj().Pkg() == nil {
		return nil
	}
	if named.Obj().Pkg().Path() == c.pkg.PkgPath {
		return nil
	}
	it, ok := named.Underlying().(*types.Interface)
	if !ok {
		return nil
	}
	projected := &node.Interface{
		Name:    named.Obj().Name(),
		Package: named.Obj().Pkg().Path(),
	}
	for m := range it.Methods() {
		sig, isSig := m.Type().(*types.Signature)
		if !isSig {
			continue
		}
		method := c.methodFromSignature(m.Name(), sig)
		method.SourcePos = posOf(c.fset, m.Pos())
		method.Owner = projected
		projected.Methods = append(projected.Methods, method)
	}
	if len(projected.Methods) == 0 {
		// An interface contributing no methods completes no walk, and
		// recording it would turn "nothing to add" into a resolution
		// the walk reports as successful.
		return nil
	}
	return projected
}
