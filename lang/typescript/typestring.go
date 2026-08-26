// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript

import (
	"strconv"
	"strings"

	"go.thesmos.sh/eidos/node"
)

// TypeString renders a source type reference as the TypeScript
// expression an author would recognise.
//
// # Why this is not the backend's renderType
//
// The backend's spelling is bound to a render state, because it
// registers the file's imports as a side effect. Nothing here can do
// that, and nothing here should: these produce text for a human — a
// diagnostic naming the type that failed, a doc comment describing
// what a generated method takes. Text cannot ask for an import, so a
// caller putting the result into generated source emits a file naming
// a module it never imports.
//
// The rule is worth stating because the mistake is easy and the
// failure is late: the file parses, and the consumer's compiler
// reports an unresolved name in code they did not write.
//
// Empty for nil and for a shape this package cannot spell, so a
// caller interpolating the result needs no guard — and an empty type
// in a message is visibly wrong, where a partial one reads as
// correct.
func TypeString(t *node.TypeRef) string {
	return typeText(t, maxConvDepth)
}

// typeText is the recursive renderer with the depth budget threaded
// through.
func typeText(t *node.TypeRef, depth int) string {
	if t == nil || depth <= 0 {
		return ""
	}

	switch {
	case IsUnion(t):
		return joinMembers(t, " | ", depth)
	case IsIntersection(t):
		return joinMembers(t, " & ", depth)
	case IsTuple(t):
		return "[" + strings.Join(memberTexts(t, depth), ", ") + "]"
	case IsOperator(t):
		text, _ := MetaTypeText.Get(t.Meta())
		return text
	case t.IsSlice():
		return arrayText(typeText(t.Elem, depth-1))
	case t.IsArray():
		return arrayText(typeText(t.Elem, depth-1))
	case t.IsMap():
		return "Record<" + typeText(t.MapKey, depth-1) + ", " + typeText(t.MapValue, depth-1) + ">"
	case t.IsPointer():
		inner := typeText(t.Elem, depth-1)
		if inner == "" {
			return ""
		}
		return inner + " | " + TypeNull
	case t.IsFunc():
		return funcText(t, depth)
	case t.IsAnonStruct():
		return anonStructText(t, depth)
	case t.IsAnonInterface():
		return "object"
	default:
		return namedText(t, depth)
	}
}

// joinMembers renders a marker ref's members joined by sep.
func joinMembers(t *node.TypeRef, sep string, depth int) string {
	parts := memberTexts(t, depth)
	if len(parts) == 0 {
		return TypeNever
	}
	return strings.Join(parts, sep)
}

// memberTexts renders each of a marker ref's members.
func memberTexts(t *node.TypeRef, depth int) []string {
	members := Members(t)
	out := make([]string, 0, len(members))
	for _, m := range members {
		if text := typeText(m, depth-1); text != "" {
			out = append(out, text)
		}
	}
	return out
}

// arrayText spells the array form, parenthesising where a postfix
// `[]` would otherwise bind too tightly — `A | B[]` reads as an array
// of B beside a plain A.
func arrayText(elem string) string {
	if elem == "" {
		return ""
	}
	if bindsLoosely(elem) {
		return "(" + elem + ")[]"
	}
	return elem + "[]"
}

// bindsLoosely reports whether a rendered type must be parenthesised
// before a postfix operator binds to it.
func bindsLoosely(rendered string) bool {
	return strings.Contains(rendered, " | ") ||
		strings.Contains(rendered, " & ") ||
		strings.Contains(rendered, " => ")
}

// funcText spells a function type, naming parameters positionally
// because TypeScript requires a name in a signature.
func funcText(t *node.TypeRef, depth int) string {
	params := make([]string, 0, len(t.FuncParams))
	for i, p := range t.FuncParams {
		params = append(params, "arg"+strconv.Itoa(i)+": "+typeText(p, depth-1))
	}

	ret := TypeVoid
	switch len(t.FuncReturns) {
	case 0:
	case 1:
		ret = typeText(t.FuncReturns[0], depth-1)
	default:
		parts := make([]string, 0, len(t.FuncReturns))
		for _, r := range t.FuncReturns {
			parts = append(parts, typeText(r, depth-1))
		}
		ret = "[" + strings.Join(parts, ", ") + "]"
	}
	return "(" + strings.Join(params, ", ") + ") => " + ret
}

// anonStructText spells an inline object type.
//
// The field list is rendered rather than elided: an inline object is
// identified by its members, and a caller reading `{…}` in a message
// learns nothing the surrounding context did not already say.
func anonStructText(t *node.TypeRef, depth int) string {
	if len(t.Fields) == 0 {
		return "{}"
	}
	parts := make([]string, 0, len(t.Fields))
	for _, f := range t.Fields {
		part := PropertyKey(f.Name)
		if opt, _ := MetaOptional.Get(f.Meta()); opt {
			part += "?"
		}
		parts = append(parts, part+": "+typeText(f.Type, depth-1))
	}
	return "{ " + strings.Join(parts, "; ") + " }"
}

// namedText spells a named reference with its qualifier and any type
// arguments.
func namedText(t *node.TypeRef, depth int) string {
	name := t.Name
	if t.Package != "" {
		name = t.Package + "." + t.Name
	}
	if len(t.TypeArgs) == 0 {
		return name
	}
	args := make([]string, 0, len(t.TypeArgs))
	for _, a := range t.TypeArgs {
		args = append(args, typeText(a, depth-1))
	}
	return name + "<" + strings.Join(args, ", ") + ">"
}
