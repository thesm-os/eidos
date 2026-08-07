// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"slices"

	"go.thesmos.sh/eidos/node"
)

// Structural interface satisfaction, and the deep answers a
// resolver makes possible.
//
// [store.Reader.Implementers] answers this for one interface across
// the whole store, and the logic behind it is not reachable for an
// arbitrary pair — so a generator asking "does the double I just
// built satisfy the interface it doubles" has no way to ask.

// MissingMethod is one reason a method set fails to satisfy an
// interface.
type MissingMethod struct {
	// Name is the interface method's identifier.
	Name string

	// Declared is the same-named method the candidate declares, nil
	// when it declares none.
	//
	// The distinction is what makes a diagnostic useful: a missing
	// method and a method of the wrong signature are different
	// mistakes, and the second is the one an author reads twice
	// before seeing.
	Declared *node.Method

	// Want is the interface's declaration, for a message that can
	// print both sides.
	Want *node.Method
}

// Satisfies reports whether a method set satisfies an interface,
// and names every method that fails.
//
// Structural: names and signatures are compared, and nothing is
// read from a stamp. That is what lets it answer for a set a
// generator just built, which no frontend has seen.
//
// Pass a resolved interface method set where the interface embeds —
// [node.MethodSet] — because an interface composed of embeds
// declares nothing of its own and this would report it satisfied by
// anything.
//
// # What it cannot see
//
// A parameter type is compared by [node.TypeRef.Equal], which is
// structural. Two references to the same type spelled differently —
// one qualified, one not, because one side was written inside the
// declaring package — compare unequal. A generator comparing a
// double it emitted against the source interface it copied from
// sees them agree, because both came from the same references;
// a generator comparing two independently authored declarations may
// not.
func Satisfies(have, want []*node.Method) (bool, []MissingMethod) {
	byName := make(map[string]*node.Method, len(have))
	for _, m := range have {
		if m != nil {
			byName[m.Name] = m
		}
	}
	var missing []MissingMethod
	for _, w := range want {
		if w == nil {
			continue
		}
		got, declared := byName[w.Name]
		if declared && SameSignature(got, w) {
			continue
		}
		entry := MissingMethod{Name: w.Name, Want: w}
		if declared {
			entry.Declared = got
		}
		missing = append(missing, entry)
	}
	return len(missing) == 0, missing
}

// SameSignature reports whether two methods declare the same
// parameter and return types, in order.
//
// Names are ignored on both sides: a parameter's identifier and a
// return's binding name are documentation, and two methods
// differing only in them are the same method as far as satisfaction
// goes. The variadic marker is not ignored — `Print(...string)` and
// `Print([]string)` are different methods that a name-and-type
// comparison would otherwise conflate.
func SameSignature(a, b *node.Method) bool {
	if a == nil || b == nil {
		return a == b
	}
	if len(a.Params) != len(b.Params) || len(a.Returns) != len(b.Returns) {
		return false
	}
	for i := range a.Params {
		if !sameParam(a.Params[i], b.Params[i]) {
			return false
		}
	}
	for i := range a.Returns {
		if !sameReturn(a.Returns[i], b.Returns[i]) {
			return false
		}
	}
	return true
}

// sameParam compares one parameter's type and variadic marker.
func sameParam(a, b *node.Param) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Variadic == b.Variadic && a.Type.Equal(b.Type)
}

// sameReturn compares one return slot's type.
func sameReturn(a, b *node.Return) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Type.Equal(b.Type)
}

// UnderlyingOf resolves a named type to the type it is defined as.
//
// `type Weekday int` underlies `int`; a struct underlies itself.
// Follows a chain of defined types, so `type A B` with `type B int`
// underlies `int` — which is what a caller deciding how to spell a
// value needs, and what the model does not carry.
//
// Returns t unchanged when it is not a named type, and nil when the
// resolver cannot reach the declaration: a smaller answer rather
// than a wrong one.
func UnderlyingOf(t *node.TypeRef, r Resolver) *node.TypeRef {
	return underlyingOf(t, r, maxResolveDepth)
}

// underlyingOf is [UnderlyingOf] with the chain budget threaded
// through.
func underlyingOf(t *node.TypeRef, r Resolver, depth int) *node.TypeRef {
	if t == nil || depth <= 0 {
		return nil
	}
	if r == nil || t.TypeKind != node.TypeRefNamed || t.IsBuiltin() {
		return t
	}
	decl, found := r.Resolve(t)
	if !found {
		return nil
	}
	alias, ok := decl.(*node.Alias)
	if !ok || alias.Target == nil {
		// A struct, interface or enum underlies itself: the declaration
		// is the type.
		return t
	}
	return underlyingOf(alias.Target, r, depth-1)
}

// ComparableDeep reports whether values of t may be compared with
// `==`, resolving named types through r.
//
// The honest version of [Keyable], which answers from the reference
// alone and therefore cannot see that a struct holds a slice field.
// Go's rule is recursive: a struct is comparable when every field
// is, an array when its element is, and slices, maps and functions
// never are.
//
// The second result is false when the walk hit a type the resolver
// could not reach. A caller must not read the first result then —
// an unreachable type is not evidence of comparability, and
// emitting a map keyed on it produces a compile error in the
// consumer's build.
func ComparableDeep(t *node.TypeRef, r Resolver) (equalable, known bool) {
	return comparableDeep(t, r, maxResolveDepth, map[string]struct{}{})
}

// comparableDeep is [ComparableDeep] with the recursion budget and
// the cycle guard threaded through.
func comparableDeep(
	t *node.TypeRef, r Resolver, depth int, seen map[string]struct{},
) (equalable, known bool) {
	if t == nil || depth <= 0 {
		return false, false
	}
	switch {
	case t.IsSlice(), t.IsMap(), t.IsFunc():
		return false, true
	case t.IsPointer(), IsChannel(t):
		// A pointer and a channel compare by identity whatever they
		// point at, so the element is not walked.
		return true, true
	case t.IsArray():
		return comparableDeep(t.Elem, r, depth-1, seen)
	case t.IsAnonStruct():
		return fieldsComparable(t.Fields, r, depth, seen)
	case t.IsAnonInterface(), IsInterface(t):
		// An interface value compares when its dynamic type does,
		// which is not knowable statically. Reported as comparable
		// because the code compiles; a run-time panic is the caller's
		// to reason about, and refusing would rule out every
		// interface-keyed map Go admits.
		return true, true
	case t.IsBuiltin(), t.IsTypeParam():
		return true, true
	}

	key := QName(t)
	if _, looping := seen[key]; looping {
		return true, true
	}
	seen[key] = struct{}{}

	if r == nil {
		return false, false
	}
	decl, found := r.Resolve(t)
	if !found {
		return false, false
	}
	switch v := decl.(type) {
	case *node.Struct:
		return fieldsComparable(v.Fields, r, depth-1, seen)
	case *node.Alias:
		return comparableDeep(v.Target, r, depth-1, seen)
	case *node.Interface:
		return true, true
	case *node.Enum:
		return true, true
	default:
		return false, false
	}
}

// fieldsComparable reports whether every field of a struct is
// comparable.
//
// One uncomparable field makes the whole struct uncomparable, and
// one unreachable field makes the answer unknown — the first
// short-circuits, the second does not, because a later field may be
// definitively uncomparable and that is the stronger answer.
func fieldsComparable(
	fields []*node.Field, r Resolver, depth int, seen map[string]struct{},
) (equalable, known bool) {
	known = true
	for _, f := range fields {
		if f == nil {
			continue
		}
		ok, sure := comparableDeep(f.Type, r, depth-1, seen)
		if sure && !ok {
			return false, true
		}
		if !sure {
			known = false
		}
	}
	return known, known
}

// RecommendedReceiver reports whether a type's methods should be
// declared on the pointer receiver.
//
// Go's rule is consistency: if any method needs a pointer — because
// it mutates, or because the type holds a lock or is large — every
// method should take one, so the method set is the same through
// both forms and a value of the type is never accidentally a
// partial implementation.
//
// True when any existing method already takes a pointer. A
// generator adding methods to a type reads this rather than
// deciding per method, which is how a type ends up with a mixed
// receiver set that satisfies an interface through `*T` and not
// through `T`.
func RecommendedReceiver(methods []*node.Method) (byPointer bool) {
	return slices.ContainsFunc(methods, ReceiverIsPointerDecl)
}

// ReceiverIsPointerDecl reports whether m is declared on a pointer
// receiver, read from the declaration rather than from a stamp.
//
// The structural twin of [ReceiverIsPointer], which reads
// `go.receiverIsPointer`. Both are kept: the stamp is what a
// frontend derived, and this is what a graph with no frontend
// behind it can still answer.
func ReceiverIsPointerDecl(m *node.Method) bool {
	return m != nil && m.Receiver != nil && m.Receiver.IsPointer()
}
