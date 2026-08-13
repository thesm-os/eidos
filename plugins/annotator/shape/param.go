// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape

// ParamKind says how the refinement resolver treats a directive
// parameter's value.
//
// The kind is a property of the key, so it is declared beside the key
// rather than in a parallel list. An earlier form carried the keys in
// one slice and each resolvable subset in another, which meant every
// key was declared twice and nothing checked the two agreed: a key
// named as a sibling but omitted from the key list resolved correctly
// and stayed invisible to every [Contract.Validate] and
// [Mixin.Validate] hook, because those read the key list alone.
type ParamKind uint8

const (
	// KindOpaque leaves the value verbatim. The right kind for
	// anything with no declaration to resolve against — a literal, a
	// quantity, a field name, an encoded graph.
	KindOpaque ParamKind = iota

	// KindCallable names a sibling callable, resolved through the
	// host's own scope: an owner's method set for a method, the
	// package's functions for a function.
	KindCallable

	// KindVar names a package-level var, resolved through the
	// package. A sentinel is not declared on the receiver, so this
	// scope is the package's for a method host as much as a function
	// one.
	KindVar

	// KindMember names a member of the type the host answers — a
	// method on the handle a role's callable returns. Resolved
	// through that type's own declaration, which the resolver can
	// only see when the run loaded it.
	KindMember
)

// String renders the kind for diagnostics.
func (k ParamKind) String() string {
	switch k {
	case KindOpaque:
		return "opaque"
	case KindCallable:
		return "callable"
	case KindVar:
		return "var"
	case KindMember:
		return "member"
	default:
		return "unknown"
	}
}

// scopeNoun renders what a kind's value must name, for the resolver's
// "names no <noun>" diagnostic. Distinct from [ParamKind.String],
// which is the bare token a reader wants when the kind is the subject
// rather than the object of a sentence.
func (k ParamKind) scopeNoun() string {
	switch k {
	case KindCallable:
		return "callable in scope"
	case KindVar:
		return "package-level var in scope"
	case KindMember:
		return "member of the type it answers"
	case KindOpaque:
		return "resolvable declaration"
	default:
		return "resolvable declaration"
	}
}

// Param is one KV key a directive accepts, with how its value
// resolves.
//
// Declaring the kind here rather than in a parallel list is what makes
// the key's treatment single-sourced: a reader of the declaration sees
// what the value must name, and a fifth kind costs a constant rather
// than a field on two schemas.
type Param struct {
	// Key is the KV key as written in the directive.
	Key string

	// Kind says what the value names and therefore how it resolves.
	// The zero value is [KindOpaque], so a param needing no
	// resolution may leave it unset.
	Kind ParamKind
}

// ParamKeys returns the declared keys in declaration order.
//
// The order is the declaration's, not sorted: a diagnostic listing
// accepted keys reads best in the order the author wrote them, and
// resolution visits them in the same order so two runs report the same
// failure first.
func ParamKeys(params []Param) []string {
	if len(params) == 0 {
		return nil
	}
	out := make([]string, 0, len(params))
	for _, p := range params {
		out = append(out, p.Key)
	}
	return out
}

// ParamKeysOfKind returns the declared keys of one kind, in
// declaration order.
func ParamKeysOfKind(params []Param, kind ParamKind) []string {
	var out []string
	for _, p := range params {
		if p.Kind == kind {
			out = append(out, p.Key)
		}
	}
	return out
}
