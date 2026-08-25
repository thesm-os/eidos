// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"go/types"

	"go.thesmos.sh/eidos/node"
)

// typeRefOf converts a [types.Type] into the language-agnostic
// [node.TypeRef] model. The conversion preserves enough information
// for downstream generators to reconstruct the source form across
// every relevant variant — primitives, in-package and cross-package
// named types, pointers, slices, arrays, maps, channels (modelled
// as Named refs with `go.*` metadata), function types, generic
// instantiations and type parameters, and anonymous struct /
// interface types.
//
// The caller is expected to thread a resolved, non-nil
// [types.Type] — go/types guarantees this for every successfully
// type-checked expression the converter inspects.
func (c *converter) typeRefOf(t types.Type) *node.TypeRef {
	ref := c.buildTypeRef(t)
	c.stampTypeRefMeta(ref, t)
	return ref
}

// buildTypeRef is the per-kind dispatch behind [converter.typeRefOf].
// Kept separate so [typeRefOf] owns the meta-stamping pass without
// littering the dispatch with stamp calls per variant.
func (c *converter) buildTypeRef(t types.Type) *node.TypeRef {
	switch x := t.(type) {
	case *types.Basic:
		return &node.TypeRef{TypeKind: node.TypeRefNamed, Name: x.Name()}
	case *types.Named:
		return c.namedTypeRef(x)
	case *types.Pointer:
		return &node.TypeRef{TypeKind: node.TypeRefPointer, Elem: c.typeRefOf(x.Elem())}
	case *types.Slice:
		return &node.TypeRef{TypeKind: node.TypeRefSlice, Elem: c.typeRefOf(x.Elem())}
	case *types.Array:
		return &node.TypeRef{
			TypeKind: node.TypeRefArray,
			ArrayLen: int(x.Len()),
			Elem:     c.typeRefOf(x.Elem()),
		}
	case *types.Map:
		return &node.TypeRef{
			TypeKind: node.TypeRefMap,
			MapKey:   c.typeRefOf(x.Key()),
			MapValue: c.typeRefOf(x.Elem()),
		}
	case *types.Chan:
		return c.chanTypeRef(x)
	case *types.Signature:
		return c.funcTypeRef(x)
	case *types.TypeParam:
		return &node.TypeRef{TypeKind: node.TypeRefTypeParam, Name: x.Obj().Name()}
	case *types.Struct:
		return c.anonStructRef(x)
	case *types.Interface:
		return c.anonInterfaceRef(x)
	case *types.Alias:
		return c.typeRefOf(types.Unalias(x))
	}
	return &node.TypeRef{TypeKind: node.TypeRefNamed, Name: t.String()}
}

// namedTypeRef converts a [types.Named] into a Named [node.TypeRef],
// preserving the originating package, type arguments (for
// instantiated generics), and any back-pointer to a known
// in-package declaration.
func (c *converter) namedTypeRef(n *types.Named) *node.TypeRef {
	ref := &node.TypeRef{TypeKind: node.TypeRefNamed, Name: n.Obj().Name()}
	if pkg := n.Obj().Pkg(); pkg != nil {
		ref.Package = pkg.Path()
	}
	if targs := n.TypeArgs(); targs != nil && targs.Len() > 0 {
		ref.TypeArgs = make([]*node.TypeRef, targs.Len())
		for i := range targs.Len() {
			ref.TypeArgs[i] = c.typeRefOf(targs.At(i))
		}
	}
	return ref
}

// chanTypeRef models a Go channel as a Named ref with the synthetic
// "go.chan" qualified name. Channel-specific facts (direction,
// element type) ride on `go.*` metadata stamped by the converter —
// see [MetaChanDir] and [MetaChanElem]. The element type also rides
// on TypeArgs[0] for callers that want a structured ref rather than
// the printed form. This keeps the language-agnostic [node.TypeRef]
// free of Go-specific variants while preserving every channel detail
// consumers need.
func (c *converter) chanTypeRef(ch *types.Chan) *node.TypeRef {
	ref := &node.TypeRef{
		TypeKind: node.TypeRefNamed,
		Package:  "go",
		Name:     "chan",
		TypeArgs: []*node.TypeRef{c.typeRefOf(ch.Elem())},
	}
	stampChanMeta(ref, ch)
	return ref
}

// funcTypeRef converts a [types.Signature] (a function type) into a
// Func [node.TypeRef]. Parameter and result types are recorded
// positionally; parameter names are part of the [node.Param] model,
// not function-type identity in Go, so we record types only here.
func (c *converter) funcTypeRef(sig *types.Signature) *node.TypeRef {
	ref := &node.TypeRef{TypeKind: node.TypeRefFunc}
	if params := sig.Params(); params != nil {
		ref.FuncParams = make([]*node.TypeRef, params.Len())
		for i := range params.Len() {
			ref.FuncParams[i] = c.typeRefOf(params.At(i).Type())
		}
	}
	if results := sig.Results(); results != nil {
		ref.FuncReturns = make([]*node.TypeRef, results.Len())
		for i := range results.Len() {
			ref.FuncReturns[i] = c.typeRefOf(results.At(i).Type())
		}
	}
	return ref
}

// anonStructRef converts an inline (unnamed) [types.Struct] into a
// [node.TypeRefAnonStruct] carrying the inline fields. Field tags
// are preserved verbatim; back-pointers are wired by
// [node.RewireOwners] after the package is fully built.
func (c *converter) anonStructRef(s *types.Struct) *node.TypeRef {
	ref := &node.TypeRef{TypeKind: node.TypeRefAnonStruct}
	for i := range s.NumFields() {
		f := s.Field(i)
		if f.Embedded() {
			ref.Embeds = append(ref.Embeds, &node.Embed{Type: c.typeRefOf(f.Type())})
			continue
		}
		ref.Fields = append(ref.Fields, &node.Field{
			Name: f.Name(),
			Type: c.typeRefOf(f.Type()),
			Tag:  s.Tag(i),
		})
	}
	return ref
}

// anonInterfaceRef converts an inline (unnamed) [types.Interface]
// into a [node.TypeRefAnonInterface] carrying the inline methods
// and embedded types.
func (c *converter) anonInterfaceRef(i *types.Interface) *node.TypeRef {
	ref := &node.TypeRef{TypeKind: node.TypeRefAnonInterface}
	for m := range i.ExplicitMethods() {
		sig, _ := m.Type().(*types.Signature)
		ref.Methods = append(ref.Methods, c.methodFromSignature(m.Name(), sig))
	}
	for emb := range i.EmbeddedTypes() {
		ref.Embeds = append(ref.Embeds, &node.Embed{Type: c.typeRefOf(emb)})
	}
	return ref
}
