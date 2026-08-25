// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"go/ast"
	"go/types"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// convertStruct converts a struct type-spec into a [node.Struct] and
// appends it to the converter's package. Methods declared on the
// type elsewhere in the source are not attached here — they are
// buffered by [convertFuncDecl] and flushed by [attachMethods] once
// every type has been declared.
//
// Field and embed conversion walks the AST struct expression so
// per-field doc comments and type-expression positions are
// preserved alongside the type-checker's resolved type information.
func (c *converter) convertStruct(
	ts *ast.TypeSpec,
	obj *types.TypeName,
	st *types.Struct,
	docs []string,
	dirs []*directive.Directive,
) {
	s := &node.Struct{
		BaseNode: node.BaseNode{
			SourcePos:     posOf(c.fset, ts.Pos()),
			DocLines:      docs,
			DirectiveList: dirs,
		},
		Name:    obj.Name(),
		Package: c.out.Path,
	}
	s.TypeParams = c.typeParamsFromList(s, ts.TypeParams)
	c.populateStructBody(s, ts.Type, st)
	c.stampStructMeta(s, st)
	for _, f := range s.Fields {
		c.stampFieldTagMeta(f)
	}
	c.out.Structs = append(c.out.Structs, s)
	c.structByQName[s.QName()] = s
}

// populateStructBody walks an AST struct expression in lock-step
// with the type-checker's [types.Struct] view to produce [node.Field]
// and [node.Embed] entries. The AST is the authoritative source
// for per-field doc comments, type-expression positions, and the
// declared tag literal; go/types fills in the resolved type
// information.
func (c *converter) populateStructBody(s *node.Struct, expr ast.Expr, st *types.Struct) {
	astStruct, ok := expr.(*ast.StructType)
	if !ok || astStruct.Fields == nil {
		c.populateStructFromTypeOnly(s, st)
		return
	}
	idx := 0
	for _, field := range astStruct.Fields.List {
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
			c.appendStructEmbed(s, st, idx, field, docs, dirs, typePos)
			idx++
			continue
		}
		for _, name := range field.Names {
			c.appendStructField(s, st, idx, name, field, docs, dirs, typePos)
			idx++
		}
	}
}

// populateStructFromTypeOnly is the fallback when the AST struct
// expression cannot be resolved (synthetic or parse-errored). The
// type-checker's view still produces structurally-correct fields,
// just without per-field docs and source-expression positions.
func (c *converter) populateStructFromTypeOnly(s *node.Struct, st *types.Struct) {
	for i := range st.NumFields() {
		v := st.Field(i)
		if v.Embedded() {
			s.Embeds = append(s.Embeds, &node.Embed{
				BaseNode: node.BaseNode{SourcePos: posOf(c.fset, v.Pos())},
				Type:     c.typeRefOf(v.Type()),
			})
			continue
		}
		s.Fields = append(s.Fields, &node.Field{
			BaseNode: node.BaseNode{SourcePos: posOf(c.fset, v.Pos())},
			Name:     v.Name(),
			Type:     c.typeRefOf(v.Type()),
			Tag:      st.Tag(i),
		})
	}
}

// appendStructEmbed appends a [node.Embed] for an anonymous (embedded)
// field at AST index idx. The embedded type's ref carries the AST
// type-expression position; the embed itself carries the AST field
// position so consumers can navigate from the embed to the source
// line.
func (c *converter) appendStructEmbed(
	s *node.Struct,
	st *types.Struct,
	idx int,
	field *ast.Field,
	docs []string,
	dirs []*directive.Directive,
	typePos position.Pos,
) {
	v := st.Field(idx)
	ref := c.typeRefOf(v.Type())
	if ref != nil {
		ref.SourcePos = typePos
	}
	s.Embeds = append(s.Embeds, &node.Embed{
		BaseNode: node.BaseNode{
			SourcePos:     posOf(c.fset, field.Pos()),
			DocLines:      docs,
			DirectiveList: dirs,
		},
		Type: ref,
	})
}

// appendStructField appends a [node.Field] for the named field at
// AST index idx. Names sharing one AST field expression (`A, B
// int`) all carry the same docs, tag, and type-expression position.
func (c *converter) appendStructField(
	s *node.Struct,
	st *types.Struct,
	idx int,
	name *ast.Ident,
	field *ast.Field,
	docs []string,
	dirs []*directive.Directive,
	typePos position.Pos,
) {
	v := st.Field(idx)
	ref := c.typeRefOf(v.Type())
	if ref != nil {
		ref.SourcePos = typePos
	}
	tag := unquoteTagLiteral(field.Tag)
	s.Fields = append(s.Fields, &node.Field{
		BaseNode: node.BaseNode{
			SourcePos:     posOf(c.fset, name.Pos()),
			DocLines:      docs,
			DirectiveList: dirs,
		},
		Name: name.Name,
		Type: ref,
		Tag:  tag,
	})
}

// unquoteTagLiteral strips the surrounding backticks (or quotes)
// from an AST tag literal. A nil tag (the common no-tag-on-field
// case) yields the empty string; otherwise [go/parser] guarantees
// the literal is wrapped in matching delimiters and the function
// returns the contained bytes verbatim.
func unquoteTagLiteral(tag *ast.BasicLit) string {
	if tag == nil {
		return ""
	}
	v := tag.Value
	return v[1 : len(v)-1]
}
