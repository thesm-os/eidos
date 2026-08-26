// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript

import (
	"strings"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// TypeError is the built-in class every TypeScript error derives
// from, and the name a heritage clause has to reach for a declaration
// to take part in the protocol.
const TypeError = "Error"

// errorSuffix is the trailer a declared error is named under.
const errorSuffix = "Error"

// causeMember is the property ES2022 gives an error for the error it
// wraps, and the one `new Error(msg, { cause })` populates.
const causeMember = "cause"

// SentinelName spells the identifier a declared error is named under
// — `NotFound` giving `NotFoundError`.
//
// A suffix rather than Go's `Err` prefix, because TypeScript's errors
// are classes rather than values and a class is named for what it is.
// The whole standard library follows it: TypeError, RangeError,
// SyntaxError.
//
// A base already carrying the suffix is returned unchanged, so
// applying the rule twice is a no-op — which matters because a
// generator naming a declared error and a detector finding one run
// the same rule from opposite ends.
func SentinelName(base string) string {
	if base == "" {
		return ""
	}
	if IsSentinelName(base) {
		return TypeName(base)
	}
	return TypeName(base, errorSuffix)
}

// IsSentinelName reports whether an identifier follows that
// convention.
//
// The bare word `Error` does not: it is the base class, and a
// generator treating it as a declared error would emit checks against
// the language's own type.
func IsSentinelName(ident string) bool {
	return len(ident) > len(errorSuffix) && strings.HasSuffix(ident, errorSuffix)
}

// ErrorOf is [Source.ErrorOf] as a plain function.
//
// Matched on the heritage rather than on the name. A class called
// `ValidationError` that extends nothing is not an error — `throw`
// accepts it and every consumer catching by `instanceof Error` misses
// it — so a generated check calling it one asserts something the
// declaration does not say.
//
// The chain is walked through the resolver, because
// `class NotFoundError extends BaseError` is the dominant idiom for a
// family of errors and reading only its own heritage finds a name
// this projection has never heard of.
func ErrorOf(s *node.Struct, r node.Resolver) (emit.ErrorInfo, bool) {
	if s == nil {
		return emit.ErrorInfo{}, false
	}
	derived, unresolved := derivesFromError(s, r, 0)
	if !derived {
		// Named even on a refusal, so a caller can tell a declaration
		// that certainly is not an error from one whose base the run
		// never loaded. Both answer false, and only one is worth
		// telling the author about.
		return emit.ErrorInfo{Unresolved: unresolved}, false
	}
	return emit.ErrorInfo{
		// TypeScript has no addressability, and no equality protocol
		// for errors: a caller compares with `instanceof` and reads
		// `cause` itself. Both stay false so a generator emits no check
		// for a contract the language does not have.
		Unwraps:    hasCause(s),
		Cause:      causeOf(s),
		Members:    errorMembers(s),
		Unresolved: unresolved,
	}, true
}

// maxHeritageDepth bounds the walk up the extends chain.
//
// A cycle is not expressible in TypeScript, but a graph assembled
// from several runs can hold one, and the walk is over data this
// package did not build.
const maxHeritageDepth = 16

// derivesFromError reports whether s extends Error, directly or
// through a chain, and names every link the run could not follow.
//
// Only `extends` counts. A class that implements an error-shaped
// interface satisfies the shape and is still not an `Error`, which is
// what `instanceof` and every runtime handler test.
func derivesFromError(s *node.Struct, r node.Resolver, depth int) (bool, []string) {
	if s == nil || depth > maxHeritageDepth {
		return false, nil
	}

	var unresolved []string
	for _, e := range s.Embeds {
		if e == nil || e.Type == nil {
			continue
		}
		if kind, ok := MetaHeritage.Get(e.Meta()); ok && kind != HeritageExtends {
			continue
		}
		if e.Type.Name == TypeError {
			return true, unresolved
		}

		base := resolveStruct(e.Type, r)
		if base == nil {
			unresolved = append(unresolved, TypeString(e.Type))
			continue
		}
		derived, deeper := derivesFromError(base, r, depth+1)
		unresolved = append(unresolved, deeper...)
		if derived {
			return true, unresolved
		}
	}
	return false, unresolved
}

// resolveStruct follows a type reference to the class it names, or
// nil for one the run did not load.
func resolveStruct(t *node.TypeRef, r node.Resolver) *node.Struct {
	if t == nil || r == nil {
		return nil
	}
	resolved, ok := r.Resolve(t)
	if !ok {
		return nil
	}
	s, _ := resolved.(*node.Struct)
	return s
}

// hasCause reports whether the declaration names the error it wraps.
func hasCause(s *node.Struct) bool { return causeOf(s) != "" }

// causeOf names the member holding the wrapped error, empty when the
// declaration carries none.
//
// Matched on the member name rather than on its type. `cause` is
// typed `unknown` on the built-in — anything may be thrown — so a
// type-directed search finds nothing on the one declaration that
// certainly has it.
func causeOf(s *node.Struct) string {
	for _, f := range s.Fields {
		if f != nil && f.Name == causeMember {
			return f.Name
		}
	}
	return ""
}

// errorMembers lifts the members a generated literal can set, with a
// value to write into each.
//
// The settable set rather than every field, for the reason
// [Settable] states: a readonly, static or non-public member cannot
// be assigned from another module, and a check that tried would fail
// in the consuming repository rather than here.
func errorMembers(s *node.Struct) []emit.ErrorMember {
	members := Settable(s)
	out := make([]emit.ErrorMember, 0, len(members))
	for _, m := range members {
		if m.Source == nil || m.Source.Type == nil {
			continue
		}
		sample, _ := SamplesOf(m.Source.Type, m.Name, nil)
		out = append(out, emit.ErrorMember{
			Name:   m.Name,
			Sample: sample,
			// Only a string reaches a message unchanged. A number is
			// rendered by whatever concatenates it, and `1e21` is a
			// faithful rendering of a value a check would look for as
			// `1000000000000000000000`.
			Verbatim: isStringRef(m.Source.Type),
		})
	}
	return out
}

// isStringRef reports whether a type is TypeScript's string.
func isStringRef(t *node.TypeRef) bool {
	return t != nil && t.TypeKind == node.TypeRefNamed &&
		t.Package == "" && t.Name == ScalarString
}
