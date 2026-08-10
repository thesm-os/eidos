// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// FromNode returns the [emit.Ref] equivalent of r. Builtin and named
// refs map to [emit.Builtin] and [emit.External]; composites map to
// their corresponding emit constructors. Type parameters render as
// unqualified identifiers — sufficient for the in-method context
// most plugins emit.
//
// A nil r returns a nil ref rather than panicking. The frontend does
// guarantee non-nil refs for every type it parsed, but that covers
// only refs a frontend produced — and this package manufactures nils
// of its own to mean *not applicable*: [PointerElem] on a non-pointer,
// [IteratorElem] on a method returning no sequence, [MapKey] on a
// slice, and five more. Every one of those is a natural argument
// here, and refusing them made the composition a panic inside a
// generator with no position attached.
//
// Propagating instead is the smaller failure in both directions. A
// caller that branches on absence — which is what a caller asking
// `IteratorElem` is doing — gets the nil it is testing for. One that
// does not gets `ErrUnsupportedRef` from the backend's render site,
// naming the file and the type it could not spell.
//
// The produced emit ref's OriginNode points back at r so backends
// and downstream consumers can reach the source-side meta.
func FromNode(r *node.TypeRef) emit.Ref {
	if r == nil {
		return nil
	}
	ref := liftFromNode(r)
	setOrigin(ref, r)
	return ref
}

// liftFromNode performs the variant-by-variant conversion. Wrapped
// in [FromNode] so the OriginNode threading lands at a single
// site rather than at each constructor call.
func liftFromNode(r *node.TypeRef) emit.Ref {
	switch {
	case r.IsPointer():
		return emit.Ptr(FromNode(r.Elem))
	case r.IsSlice():
		return emit.SliceOf(FromNode(r.Elem))
	case r.IsArray():
		return emit.ArrayOf(FromNode(r.Elem), r.ArrayLen)
	case r.IsMap():
		return emit.MapOf(FromNode(r.MapKey), FromNode(r.MapValue))
	case r.IsFunc():
		// A func type is structural: it names no package, so the
		// fall-through below would build an external reference with
		// an empty import path and the backend would reject it
		// (writer: empty import path), aborting the whole render
		// rather than emitting bad output. Only the types *inside*
		// the signature register imports, which they do through
		// their own conversion.
		return emit.FuncOf(refSlice(r.FuncParams), refSlice(r.FuncReturns))
	case r.IsBuiltin():
		return emit.Builtin(r.Name)
	case r.IsTypeParam():
		return emit.Builtin(r.Name)
	case r.IsAnonStruct():
		// Structural, like a func type: it names no package, so the
		// fall-through below would build an external reference with an
		// empty import path and the backend would reject it (writer:
		// empty import path), aborting the render. The empty case is
		// the one real code hits constantly — `map[K]struct{}` is how
		// Go spells a set.
		return emit.AnonStructOf(anonFields(r.Fields), embedRefs(r.Embeds))
	case r.IsAnonInterface():
		// Anonymous interfaces sit in two practical buckets at the
		// plugin tier: the empty interface (the constraint-fallback
		// shape the Go frontend emits for the predeclared `any`)
		// and inline interfaces with methods or embeds (which
		// plugins do not yet preserve structurally). For the
		// former, render through the `any` builtin so
		// type-parameter constraints round-trip correctly; for
		// the latter, fall back to the same keyword — the
		// rendered output keeps compiling at the cost of losing
		// the inline shape, which is a separate follow-up.
		return emit.Builtin("any")
	}
	return emit.External(r.Package, r.Name, refSlice(r.TypeArgs)...)
}

// anonFields lifts an anonymous struct's inline fields into their
// emit carriers, preserving source order. Returns nil for no fields
// so the `struct{}` shape costs no allocation — it is the common
// case, not an edge one.
//
// Name, type and tag are the whole of an inline field: tags are part
// of a Go struct's type identity, so dropping them would make two
// structurally distinct types render identically.
func anonFields(in []*node.Field) []emit.AnonField {
	if len(in) == 0 {
		return nil
	}
	out := make([]emit.AnonField, 0, len(in))
	for _, f := range in {
		out = append(out, emit.AnonField{
			Name: f.Name,
			Type: FromNode(f.Type),
			Tag:  f.Tag,
		})
	}
	return out
}

// embedRefs lifts an anonymous struct's embedded types, preserving
// source order and returning nil for none on the same reasoning as
// [anonFields].
func embedRefs(in []*node.Embed) []emit.Ref {
	if len(in) == 0 {
		return nil
	}
	out := make([]emit.Ref, 0, len(in))
	for _, e := range in {
		out = append(out, FromNode(e.Type))
	}
	return out
}

// refSlice lifts a slice of source type references into emit form,
// preserving order. Returns an empty (non-nil) slice for empty
// input so callers can pass the result straight into constructors
// that distinguish "none" from "unset".
func refSlice(in []*node.TypeRef) []emit.Ref {
	out := make([]emit.Ref, 0, len(in))
	for _, r := range in {
		out = append(out, FromNode(r))
	}
	return out
}

// setOrigin records r as the OriginNode of ref on every concrete
// [emit.Ref] implementation. The switch enumerates the closed set
// of types [liftFromNode] can produce; future emit variants need
// a matching arm here so source-side meta stays reachable from
// the produced emit graph.
func setOrigin(ref emit.Ref, r *node.TypeRef) {
	switch v := ref.(type) {
	case *emit.BuiltinRef:
		v.OriginNode = r
	case *emit.ExternalRef:
		v.OriginNode = r
	case *emit.CompositeRef:
		v.OriginNode = r
	case *emit.TypeRef:
		v.OriginNode = r
	}
}

// ConstraintFromNode lifts a [node.Constraint] into its [emit.Constraint]
// equivalent. The any-constraint shape (nil receiver or no Embedded
// entries) lifts to nil — callers passing the result into a builder
// setter that documents "nil means any" get the right behaviour
// without further branching.
func ConstraintFromNode(c *node.Constraint) *emit.Constraint {
	if c == nil || c.IsAny() {
		return nil
	}
	refs := make([]emit.Ref, 0, len(c.Embedded))
	for _, e := range c.Embedded {
		refs = append(refs, FromNode(e))
	}
	return &emit.Constraint{Raw: c.Raw, Embedded: refs}
}
