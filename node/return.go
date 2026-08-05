// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package node

import "go.thesmos.sh/eidos/core/kind"

// Return is one return slot of a function or method: a declared type
// carrying an optional binding name.
//
// Named returns are documentation written into the signature, and a
// generator emitting a value derived from a return is the main
// consumer of that documentation. Modelling returns as bare
// [TypeRef] discarded the name at parse time, so
// `(item string, err error)` and `(string, error)` were
// indistinguishable to everything downstream — a stub generator
// deriving a field per return could only produce `Result0`,
// `Result1`.
//
// The output side has always modelled this as [emit.Return]. Making
// the source model carry it too means the two are equally expressive
// for the same concept, rather than the generator being able to emit
// a named return it cannot read.
//
// Mirrors [Param] minus Variadic — a return slot is never variadic.
type Return struct {
	BaseNode

	// Name is the return identifier when the signature declares a
	// named return. Empty for the anonymous form.
	//
	// The blank identifier normalises to empty. `_` cannot be used
	// as a derived identifier, so a consumer treating it as a name
	// would produce invalid output; leaving it in would make every
	// consumer special-case it. Blank and unnamed are therefore
	// indistinguishable here, deliberately — neither is nameable.
	Name string `json:"name,omitempty"`

	// Type is the return slot's declared type.
	Type *TypeRef `json:"type,omitempty"`

	// Owner is the function or method this return belongs to.
	// Populated by the constructing frontend.
	//
	// Owner is excluded from JSON encoding to break the host →
	// child cycle. Deserialized graphs re-wire Owner via
	// [RewireOwners].
	Owner Node `json:"-"`
}

// Kind returns [KindReturn].
func (*Return) Kind() kind.Kind { return KindReturn }

// ReturnTypes projects the declared types out of returns, dropping
// the names. It is the bridge for callers that only care about the
// type vector — signature predicates, arity checks, type matching —
// so they neither reach through `.Type` at every site nor lose the
// names for the callers that do want them.
func ReturnTypes(returns []*Return) []*TypeRef {
	if returns == nil {
		return nil
	}
	out := make([]*TypeRef, 0, len(returns))
	for _, r := range returns {
		if r == nil {
			out = append(out, nil)
			continue
		}
		out = append(out, r.Type)
	}
	return out
}

// AnonReturns wraps types as unnamed return slots — the convenience
// shape for fixtures and callers that do not model names. Mirrors
// [emit.AnonReturns].
func AnonReturns(types ...*TypeRef) []*Return {
	out := make([]*Return, 0, len(types))
	for _, t := range types {
		out = append(out, &Return{Type: t})
	}
	return out
}
