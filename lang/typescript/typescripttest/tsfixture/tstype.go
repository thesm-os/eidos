// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tsfixture

import (
	"strconv"
	"strings"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// maxTypeDepth bounds the type walk.
//
// A fixture cannot build a cycle through the constructors — every one
// takes its element by value and returns a fresh ref — but a test
// assembling a graph by hand can, and a projection that recursed
// forever would hang the run rather than fail it.
const maxTypeDepth = 32

// typeOf spells one type reference, registering whatever import it
// needs.
func (p *tsPrinter) typeOf(t *node.TypeRef) string {
	return p.typeAt(t, maxTypeDepth)
}

// typeAt is the recursive worker, with the depth budget.
func (p *tsPrinter) typeAt(t *node.TypeRef, depth int) string {
	if t == nil {
		p.fail("a declaration with no type where one is required")
	}
	if depth <= 0 {
		p.fail("a type nested deeper than the projection walks")
	}

	if typescript.IsMarker(t) {
		return p.markerType(t, depth)
	}

	switch t.TypeKind {
	case node.TypeRefNamed:
		return p.namedType(t, depth)
	case node.TypeRefTypeParam:
		return t.Name
	case node.TypeRefSlice, node.TypeRefArray:
		return arrayOf(p.typeAt(t.Elem, depth-1))
	case node.TypeRefMap:
		return "Record<" + p.typeAt(t.MapKey, depth-1) + ", " +
			p.typeAt(t.MapValue, depth-1) + ">"
	case node.TypeRefFunc:
		return p.funcType(t, depth)
	case node.TypeRefAnonStruct:
		return p.objectType(t, depth)
	case node.TypeRefAnonInterface:
		// An interface with no members is what `object` names, and the
		// fixture has no way to give an anonymous one members.
		return "object"
	case node.TypeRefPointer:
		// Reachable only from a graph another language's fixture built.
		// Spelled as the nullable union rather than refused, because
		// that is the projection the backend makes and a support file
		// disagreeing with it would not type-check against the output.
		return p.typeAt(t.Elem, depth-1) + " | " + typescript.TypeNull
	default:
		return p.fail("a type of kind " + t.TypeKind.String())
	}
}

// namedType spells a named reference, importing it when it names
// another module.
func (p *tsPrinter) namedType(t *node.TypeRef, depth int) string {
	if t.Name == "" {
		p.fail("a named type with no name")
	}
	// A literal type is its own text — `'admin'`, `42` — and has no
	// declaration to import.
	if lit, ok := typescript.MetaLiteralType.Get(t.Meta()); ok && lit != "" {
		return lit
	}
	if t.Package != "" {
		p.record(t.Package, t.Name)
	}
	return t.Name + p.typeArgs(t.TypeArgs, depth)
}

// markerType spells one of `lang/typescript`'s structural markers.
func (p *tsPrinter) markerType(t *node.TypeRef, depth int) string {
	members := typescript.Members(t)
	switch t.Name {
	case typescript.RefUnion:
		if len(members) == 0 {
			return typescript.TypeNever
		}
		return strings.Join(p.each(members, depth), " | ")
	case typescript.RefIntersection:
		if len(members) == 0 {
			return typescript.TypeNever
		}
		return strings.Join(p.each(members, depth), " & ")
	case typescript.RefTuple:
		return "[" + strings.Join(p.each(members, depth), ", ") + "]"
	case typescript.RefOperator:
		text, ok := typescript.MetaTypeText.Get(t.Meta())
		if !ok || text == "" {
			p.fail("an operator type carrying no source text")
		}
		return text
	default:
		return p.fail("a structural marker named " + t.Name)
	}
}

// funcType spells `(a: A) => R`.
//
// Parameters are named positionally because TypeScript requires it:
// `(string) => void` declares a parameter *called* string with an
// inferred type, which is a different signature.
func (p *tsPrinter) funcType(t *node.TypeRef, depth int) string {
	parts := make([]string, 0, len(t.FuncParams))
	for i, param := range t.FuncParams {
		parts = append(parts, "arg"+strconv.Itoa(i)+": "+p.typeAt(param, depth-1))
	}
	return "(" + strings.Join(parts, ", ") + ") => " + p.funcReturn(t, depth)
}

// funcReturn spells a function type's return — void for none, the
// type for one, the tuple for several.
func (p *tsPrinter) funcReturn(t *node.TypeRef, depth int) string {
	switch len(t.FuncReturns) {
	case 0:
		return typescript.TypeVoid
	case 1:
		return p.typeAt(t.FuncReturns[0], depth-1)
	default:
		return "[" + strings.Join(p.each(t.FuncReturns, depth), ", ") + "]"
	}
}

// objectType spells an inline `{ a: string; b?: number }`.
func (p *tsPrinter) objectType(t *node.TypeRef, depth int) string {
	if len(t.Fields) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(t.Fields))
	for _, f := range t.Fields {
		if f == nil || f.Name == "" {
			p.fail("an inline object member with no name")
		}
		part := typescript.PropertyKey(f.Name)
		if opt, _ := typescript.MetaOptional.Get(f.Meta()); opt {
			part += "?"
		}
		parts = append(parts, part+": "+p.typeAt(f.Type, depth-1))
	}
	return "{ " + strings.Join(parts, "; ") + " }"
}

// typeArgs spells a generic argument list, empty for none.
func (p *tsPrinter) typeArgs(args []*node.TypeRef, depth int) string {
	if len(args) == 0 {
		return ""
	}
	return "<" + strings.Join(p.each(args, depth), ", ") + ">"
}

// each spells every reference in a list.
func (p *tsPrinter) each(refs []*node.TypeRef, depth int) []string {
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, p.typeAt(r, depth-1))
	}
	return out
}

// record notes that the file takes name from the given specifier.
//
// A set per specifier rather than one entry per name, because
// TypeScript binds a set of names per import statement and a file
// naming two types from one module writes one line. Two names that
// collide across modules are not resolved here: the fixture chose
// them, and a projection renaming one would produce a support file
// declaring something the graph does not.
func (p *tsPrinter) record(specifier, name string) {
	names, ok := p.imports[specifier]
	if !ok {
		names = map[string]struct{}{}
		p.imports[specifier] = names
	}
	names[name] = struct{}{}
}
