// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"go/ast"
	"go/types"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/node"
)

// convertFuncDecl converts a function declaration into either a
// [node.Function] (top-level) or a [node.Method] (when the decl
// carries a receiver). Top-level functions land in the package's
// Functions slice immediately; methods are buffered for
// [attachMethods], which flushes them onto the matching
// [node.Struct] or [node.Interface] once every type has been
// declared.
func (c *converter) convertFuncDecl(fd *ast.FuncDecl) {
	docs, dirs := c.docsAndDirectives(fd.Doc, nil, nil)
	// go/types records a *types.Func for every func-decl name it
	// accepts — but records nil for the losing side of a
	// redeclaration, so the lookup is only total for a package that
	// type-checks. A frontend meets whatever the author last saved,
	// so it must not be. The redeclaration is already reported as a
	// diagnostic by the loader; skipping the decl keeps the rest of
	// the package convertible instead of aborting the walk on a
	// nil-interface conversion.
	obj, ok := c.pkg.TypesInfo.Defs[fd.Name].(*types.Func)
	if !ok {
		return
	}
	// Every *types.Func.Type() is a *types.Signature; that assertion
	// really is total once obj itself is non-nil.
	sig := obj.Type().(*types.Signature) //nolint:forcetypeassert // go/types invariant

	if fd.Recv == nil {
		c.appendFunction(fd, obj, sig, docs, dirs)
		return
	}
	c.bufferMethod(fd, obj, sig, docs, dirs)
}

// appendFunction builds a top-level [node.Function] and appends it
// to the converter's output package.
func (c *converter) appendFunction(
	fd *ast.FuncDecl,
	obj *types.Func,
	sig *types.Signature,
	docs []string,
	dirs []*directive.Directive,
) {
	fn := &node.Function{
		BaseNode: node.BaseNode{
			SourcePos:     posOf(c.fset, fd.Pos()),
			DocLines:      docs,
			DirectiveList: dirs,
		},
		Name:    obj.Name(),
		Package: c.out.Path,
	}
	fn.TypeParams = c.typeParamsFromList(fn, fd.Type.TypeParams)
	fn.Params, fn.Returns = c.paramsAndReturnsFromSignature(sig, fn, fd.Type)
	c.stampFunctionMeta(fn, sig)
	c.out.Functions = append(c.out.Functions, fn)
}

// bufferMethod records the FuncDecl as a [node.Method] keyed by the
// receiver type's qualified name. [attachMethods] flushes the
// buffer onto the matching declaration after every type has been
// converted.
func (c *converter) bufferMethod(
	fd *ast.FuncDecl,
	obj *types.Func,
	sig *types.Signature,
	docs []string,
	dirs []*directive.Directive,
) {
	m := &node.Method{
		BaseNode: node.BaseNode{
			SourcePos:     posOf(c.fset, fd.Pos()),
			DocLines:      docs,
			DirectiveList: dirs,
		},
		Name: obj.Name(),
	}
	m.TypeParams = c.typeParamsFromList(m, fd.Type.TypeParams)
	m.Params, m.Returns = c.paramsAndReturnsFromSignature(sig, m, fd.Type)
	c.fillReceiver(m, sig, fd.Recv)
	c.stampMethodMeta(m)

	qname := c.receiverQName(sig)
	c.methodsByRecv[qname] = append(c.methodsByRecv[qname], m)
}

// paramsAndReturnsFromSignature builds the [node.Param] slice and
// the return-type [node.TypeRef] slice for a function or method.
// owner is the host the params back-pointer at; astFn supplies AST
// positions for each parameter and the optional source-level doc
// comments declared inline before each parameter group.
func (c *converter) paramsAndReturnsFromSignature(
	sig *types.Signature,
	owner node.Node,
	astFn *ast.FuncType,
) ([]*node.Param, []*node.Return) {
	params := c.paramsFromSignature(sig, owner, astFn)
	returns := c.returnsFromSignature(sig, astFn)
	return params, returns
}

// paramsFromSignature returns the [node.Param] slice for sig,
// overlaying AST-derived doc comments and type-expression positions
// when astFn supplies them. The slice respects variadic semantics:
// the last parameter's [node.Param.Type] is the element type (Go's
// `...T` form).
func (c *converter) paramsFromSignature(
	sig *types.Signature,
	owner node.Node,
	astFn *ast.FuncType,
) []*node.Param {
	tp := sig.Params()
	if tp == nil {
		return nil
	}
	out := make([]*node.Param, 0, tp.Len())
	for p := range tp.Variables() {
		out = append(out, &node.Param{
			BaseNode: node.BaseNode{SourcePos: posOf(c.fset, p.Pos())},
			Name:     p.Name(),
			Type:     c.typeRefOf(p.Type()),
			Owner:    owner,
		})
	}
	if sig.Variadic() && len(out) > 0 {
		last := out[len(out)-1]
		last.Variadic = true
		if slice, ok := tp.At(tp.Len() - 1).Type().(*types.Slice); ok {
			last.Type = c.typeRefOf(slice.Elem())
		}
	}
	c.overlayParamDocsAndTypePos(out, astFn)
	return out
}

// returnsFromSignature returns the result-type slice for sig in
// positional order, with AST-derived positions overlaid when the
// supplied [ast.FuncType] has them.
func (c *converter) returnsFromSignature(sig *types.Signature, astFn *ast.FuncType) []*node.Return {
	results := sig.Results()
	if results == nil {
		return nil
	}
	out := make([]*node.Return, 0, results.Len())
	refs := make([]*node.TypeRef, 0, results.Len())
	for r := range results.Variables() {
		ref := c.typeRefOf(r.Type())
		refs = append(refs, ref)
		out = append(out, &node.Return{Name: returnBindingName(r), Type: ref})
	}
	c.overlayReturnTypePos(refs, astFn)
	return out
}

// returnBindingName reports the declared name of a result variable,
// normalising the blank identifier to empty.
//
// go/types reports an anonymous result with an empty name and a
// blank one as "_". Neither can be used as a derived identifier, so
// collapsing them here keeps every consumer from special-casing
// "_" — at the cost of making the two indistinguishable downstream,
// which is intended: what a consumer needs to know is whether a
// usable name exists.
func returnBindingName(v *types.Var) string {
	if name := v.Name(); name != "_" {
		return name
	}
	return ""
}

// fillReceiver records the receiver type ref and the source-level
// receiver variable name on m. The AST receiver list is the
// authoritative source for the receiver-variable name — the type
// checker reports anonymous receivers (the `func (*Foo) Bar()`
// form) the same way it reports named ones, so the AST is the
// only place where the source-level name is recoverable.
func (c *converter) fillReceiver(m *node.Method, sig *types.Signature, recv *ast.FieldList) {
	// bufferMethod (the only caller) is only entered when fd.Recv is
	// non-nil and the signature has a Recv resolved by go/types.
	m.Receiver = c.typeRefOf(sig.Recv().Type())
	for _, field := range recv.List {
		if m.Receiver != nil && field.Type != nil {
			m.Receiver.SourcePos = posOf(c.fset, field.Type.Pos())
		}
		for _, n := range field.Names {
			m.ReceiverName = n.Name
			return
		}
	}
}

// receiverQName returns the qualified name of the receiver type a
// method is declared on. Pointer receivers are normalised — the
// method attaches to the named type itself, not to the pointer
// wrapper, matching the Go convention that methods on `*T` and `T`
// share a method set per type. Methods can only be declared on
// named types in the same package, so the receiver type is always
// resolvable to a *types.Named with a non-nil Pkg.
func (*converter) receiverQName(sig *types.Signature) string {
	t := sig.Recv().Type()
	if p, ok := t.(*types.Pointer); ok {
		t = p.Elem()
	}
	named := t.(*types.Named) //nolint:forcetypeassert // method receiver is guaranteed a named in-package type
	return named.Obj().Pkg().Path() + "." + named.Obj().Name()
}

// overlayParamDocsAndTypePos walks the AST parameter list and
// applies three pieces of information the type checker drops: the
// per-parameter doc comment, any `+gen:` directives associated
// with the parameter, and the source position of the type
// expression. The AST and signature lists run in lock-step (Go
// expands `a, b int` into two type-checker variables); a single
// AST field can map to multiple signature params, all sharing the
// same docs and directives.
//
// [ast.Field.Doc] and [ast.Field.Comment] are unpopulated inside
// parameter lists — comment association via the parser is a
// top-level-decl affordance — so the overlay looks up associated
// comments via the converter's package-wide
// [ast.NewCommentMap] index. Comments before and after the
// parameter both attach.
func (c *converter) overlayParamDocsAndTypePos(params []*node.Param, astFn *ast.FuncType) {
	// paramsFromSignature passes the same astFn it received from its
	// caller; go/parser guarantees a non-nil *ast.FuncType.Params for
	// any function declaration (even an empty parameter list yields
	// a non-nil FieldList). The AST and signature run in lock-step so
	// idx never overruns params either.
	idx := 0
	for _, field := range astFn.Params.List {
		docs, dirs := c.paramDocsAndDirectives(field)
		typePos := posOf(c.fset, field.Type.Pos())
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for range count {
			if len(docs) > 0 {
				params[idx].DocLines = docs
			}
			if len(dirs) > 0 {
				params[idx].DirectiveList = dirs
			}
			if params[idx].Type != nil {
				params[idx].Type.SourcePos = typePos
			}
			idx++
		}
	}
}

// paramDocsAndDirectives returns the doc lines and parsed
// directives the [ast.CommentMap] associates with field. Comments
// before and after the field are both considered — Go's comment-
// map associates each free-floating comment with the closest
// AST node, which for params means either the preceding-line or
// trailing-line comment.
func (c *converter) paramDocsAndDirectives(field *ast.Field) ([]string, []*directive.Directive) {
	groups := c.commentsByNode[field]
	docs := make([]string, 0, len(groups))
	dirs := make([]*directive.Directive, 0, len(groups))
	for _, cg := range groups {
		docs = append(docs, docLinesFromCommentGroup(cg)...)
		dirs = append(dirs, c.parseDirectives(cg)...)
	}
	return docs, dirs
}

// overlayReturnTypePos walks the AST result list and assigns the
// source position of each return-type expression onto the
// corresponding [node.TypeRef].
func (c *converter) overlayReturnTypePos(returns []*node.TypeRef, astFn *ast.FuncType) {
	// returnsFromSignature only reaches here when sig.Results() is
	// non-nil, which implies astFn.Results is non-nil too — the AST
	// and signature describe the same source.
	idx := 0
	for _, field := range astFn.Results.List {
		typePos := posOf(c.fset, field.Type.Pos())
		count := len(field.Names)
		if count == 0 {
			count = 1
		}
		for range count {
			if returns[idx] != nil {
				returns[idx].SourcePos = typePos
			}
			idx++
		}
	}
}
