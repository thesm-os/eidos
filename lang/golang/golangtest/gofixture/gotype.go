// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package gofixture

import (
	"strconv"
	"strings"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// maxTypeDepth bounds the type-expression walk.
//
// A type expression nests — `map[string][]*[]T` is four levels — and
// the model admits a cycle no parsed source could produce, because a
// fixture author wires [node.TypeRef] values by hand and nothing
// stops one pointing at itself. Sixteen is past anything a fixture
// spells and shallow enough that the cycle surfaces as a named
// failure rather than as a stack overflow that takes the whole test
// binary with it.
const maxTypeDepth = 16

// typeExpr renders one type reference as the Go expression an author
// would have typed, registering any import the expression needs.
//
// # Why this is not [golang.TypeString]
//
// TypeString is built for messages, and it says so: it renders an
// anonymous struct as `struct{…}` and cannot register an import,
// because text in a diagnostic has nowhere to put one. Both
// behaviours are correct there and fatal here — a support package
// whose field type is `struct{…}` does not compile, and one naming
// `context.Context` without importing it does not either. This
// renderer therefore spells every shape in full and refuses the ones
// it cannot, which is the whole point of the projection.
func (p *goPrinter) typeExpr(t *node.TypeRef, depth int) string {
	if t == nil {
		return p.fail("a nil type reference")
	}
	if depth <= 0 {
		return p.fail("a type reference nested past " +
			strconv.Itoa(maxTypeDepth) + " levels (a cycle in the fixture graph?)")
	}
	switch {
	case golang.IsChannel(t):
		return p.chanExpr(t, depth)
	case t.IsPointer():
		return "*" + p.typeExpr(t.Elem, depth-1)
	case t.IsSlice():
		return "[]" + p.typeExpr(t.Elem, depth-1)
	case t.IsArray():
		return "[" + strconv.Itoa(t.ArrayLen) + "]" + p.typeExpr(t.Elem, depth-1)
	case t.IsMap():
		return "map[" + p.typeExpr(t.MapKey, depth-1) + "]" +
			p.typeExpr(t.MapValue, depth-1)
	case t.IsFunc():
		return p.funcExpr(t, depth)
	case t.IsTypeParam():
		if t.Name == "" {
			return p.fail("a type-parameter reference with no name")
		}
		return t.Name
	case t.IsAnonStruct():
		return p.anonStructExpr(t, depth)
	case t.IsAnonInterface():
		return p.anonInterfaceExpr(t, depth)
	case t.TypeKind == node.TypeRefNamed:
		return p.namedExpr(t, depth)
	default:
		return p.fail("a type reference of kind " + t.TypeKind.String())
	}
}

// namedExpr renders a named reference, qualified only when it points
// outside the fixture's own package.
//
// The elision is what makes the projection compile at all: the
// fixture stamps every declaration with its own package path, so a
// struct's own method receiver arrives as `example.com/test.User`.
// Printed with its qualifier that is a package importing itself.
func (p *goPrinter) namedExpr(t *node.TypeRef, depth int) string {
	if t.Name == "" {
		return p.fail("a named type reference with no name")
	}
	name := t.Name
	if t.Package != "" && t.Package != p.pkg.Path {
		name = p.qualifier(t.Package) + "." + name
	}
	if len(t.TypeArgs) == 0 {
		return name
	}
	return name + "[" + p.joinTypes(t.TypeArgs, depth-1) + "]"
}

// chanExpr reassembles a channel from the metadata a Go frontend
// stamps beside it. The model has no channel variant — a channel is
// a named ref carrying its direction and element as meta — so a
// projection reading only the shape would print the synthetic name
// and produce an undefined identifier.
func (p *goPrinter) chanExpr(t *node.TypeRef, depth int) string {
	elem := golang.ChanElem(t)
	if elem == nil {
		return p.fail("a channel type reference with no element type")
	}
	rendered := p.typeExpr(elem, depth-1)
	switch golang.ChanDir(t) {
	case golang.ChanSend:
		return "chan<- " + rendered
	case golang.ChanRecv:
		return "<-chan " + rendered
	default:
		return "chan " + rendered
	}
}

// funcExpr renders a function type. [node.TypeRef] carries only the
// parameter and return types, never their names, so the spelling is
// necessarily the anonymous one — which is what a fixture that
// declared the type could have written anyway.
func (p *goPrinter) funcExpr(t *node.TypeRef, depth int) string {
	params := "func(" + p.joinTypes(t.FuncParams, depth-1) + ")"
	switch len(t.FuncReturns) {
	case 0:
		return params
	case 1:
		return params + " " + p.typeExpr(t.FuncReturns[0], depth-1)
	default:
		return params + " (" + p.joinTypes(t.FuncReturns, depth-1) + ")"
	}
}

// anonStructExpr spells an inline struct in full.
//
// Semicolon-separated rather than laid out: gofmt expands the form
// into lines, and composing it on one line keeps this renderer free
// of the indentation bookkeeping a nested anonymous struct would
// otherwise need at every level.
func (p *goPrinter) anonStructExpr(t *node.TypeRef, depth int) string {
	members := make([]string, 0, len(t.Embeds)+len(t.Fields))
	for _, e := range t.Embeds {
		members = append(members, p.typeExpr(e.Type, depth-1))
	}
	for _, f := range t.Fields {
		if f.Name == "" {
			return p.fail("an anonymous-struct field with no name")
		}
		member := f.Name + " " + p.typeExpr(f.Type, depth-1)
		if tag := p.fieldTag(f); tag != "" {
			member += " " + tag
		}
		members = append(members, member)
	}
	if len(members) == 0 {
		return "struct{}"
	}
	return "struct{ " + strings.Join(members, "; ") + " }"
}

// anonInterfaceExpr spells an inline interface in full. An empty one
// is `any` rather than `interface{}` — the same type, in the
// spelling gofmt and every modern Go author use.
func (p *goPrinter) anonInterfaceExpr(t *node.TypeRef, depth int) string {
	members := make([]string, 0, len(t.Embeds)+len(t.Methods))
	for _, e := range t.Embeds {
		members = append(members, p.typeExpr(e.Type, depth-1))
	}
	for _, m := range t.Methods {
		members = append(members, p.methodSpec(m))
	}
	if len(members) == 0 {
		return typeAny
	}
	return "interface{ " + strings.Join(members, "; ") + " }"
}

// typeAny is Go's spelling of the empty interface, named once so the
// two places that reach for it cannot disagree.
const typeAny = "any"

// joinTypes renders a positional type list — type arguments, a
// function type's parameters or its returns.
func (p *goPrinter) joinTypes(refs []*node.TypeRef, depth int) string {
	parts := make([]string, 0, len(refs))
	for _, r := range refs {
		parts = append(parts, p.typeExpr(r, depth))
	}
	return strings.Join(parts, ", ")
}

// typeParamDecl renders a generic declaration's parameter list.
func (p *goPrinter) typeParamDecl(params []*node.TypeParam) string {
	if len(params) == 0 {
		return ""
	}
	parts := make([]string, 0, len(params))
	for _, tp := range params {
		if tp.Name == "" {
			return p.fail("a type parameter with no name")
		}
		parts = append(parts, tp.Name+" "+p.constraintExpr(tp.Constraint))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// typeParamUse renders the argument list a generic declaration's own
// members refer to it by — the `[T]` in `func (l *List[T]) Get() T`.
// A method on a generic type is invalid Go without it.
func (p *goPrinter) typeParamUse(params []*node.TypeParam) string {
	if len(params) == 0 {
		return ""
	}
	names := make([]string, 0, len(params))
	for _, tp := range params {
		if tp.Name == "" {
			return p.fail("a type parameter with no name")
		}
		names = append(names, tp.Name)
	}
	return "[" + strings.Join(names, ", ") + "]"
}

// constraintExpr renders a type parameter's bound.
//
// [node.Constraint.Embedded] models embedding rather than union, so
// several bounds compose into an inline interface embedding each —
// `interface{ A; B }` — not into the `A | B` a type set would spell.
// Getting that backwards produces a constraint that compiles and
// admits the wrong types.
//
// A constraint carrying only [node.Constraint.Raw] is printed
// verbatim, because that is the one case where the structured field
// says nothing: Go's type-set form (`~int | ~string`) has no Embedded
// representation at all, and [node.Constraint.IsAny] reads it as the
// unbounded constraint. Printing `any` for it would compile and admit
// every type the author excluded. Verbatim text registers no import,
// so a raw form naming another package needs that package imported by
// the fixture — the one place this projection is not correct by
// construction, and the reason a bound with structured embeds still
// prefers them.
func (p *goPrinter) constraintExpr(c *node.Constraint) string {
	if c.IsAny() {
		if raw := rawBound(c); raw != "" {
			return raw
		}
		return typeAny
	}
	if len(c.Embedded) == 1 {
		return p.typeExpr(c.Embedded[0], maxTypeDepth)
	}
	parts := make([]string, 0, len(c.Embedded))
	for _, e := range c.Embedded {
		parts = append(parts, p.typeExpr(e, maxTypeDepth))
	}
	return "interface{ " + strings.Join(parts, "; ") + " }"
}

// rawBound returns the constraint's printed source form, or empty for
// a nil constraint. Split out so the nil check sits at one site rather
// than beside every read of a field that only a frontend populates.
func rawBound(c *node.Constraint) string {
	if c == nil {
		return ""
	}
	return c.Raw
}

// fieldTag renders a struct tag as the backquoted literal Go wants,
// tolerating both conventions a fixture might have used.
//
// A Go frontend records the tag without its backquotes, because
// that is what go/types hands it; the builder's own doc example
// shows them included. Both spellings therefore exist in the wild
// and both have to land on exactly one pair of quotes here — two
// pairs is a parse error, none is a syntax error in the field list.
func (p *goPrinter) fieldTag(f *node.Field) string {
	raw := strings.Trim(f.Tag, "`")
	if raw == "" {
		return ""
	}
	if strings.Contains(raw, "`") {
		return p.fail("a struct tag containing a backquote, which no Go " +
			"raw string literal can hold")
	}
	return "`" + raw + "`"
}
