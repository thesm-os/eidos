// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

import "go.thesmos.sh/eidos/node"

// The projection: a source callable in the form a generator renders.
//
// Every generator that emits a method answers the same questions about
// it — what a body calls each parameter, which field of a recorded
// call each return maps to, whether the source's return names survive
// onto the generated signature — and four independent implementations
// had already disagreed about them.
//
// # Why the type is here and the derivation is not
//
// Nothing in the shape is language-specific: the fields are strings,
// bools, [Ref]s and back-pointers into [node]. Deriving one is another
// matter — which identifier is legal, how a keyword is adjusted, what
// a receiver is called — and that stays with the language, in
// `lang/golang.SigOf` and its siblings.
//
// The split is what lets a plugin's neutral core name the projection.
// A generator over interfaces asks its declared rules for one through
// [SigRules]; with the type in the language package it would have to
// import that package to spell the return value, which is the import
// the rules exist to remove.

// SigParam is one positional parameter in rendered form.
type SigParam struct {
	// Name is the identifier a generated body binds this parameter
	// to: the declared name where the source gave one, `arg<N>`
	// where it did not, adjusted for keywords and made unique
	// within the list.
	Name string

	// Declared is the name the source actually wrote, empty for an
	// anonymous or blank parameter. Carried apart from Name so a
	// consumer can tell a chosen identifier from an authored one —
	// a doc comment quoting the source should quote what was
	// written.
	Declared string

	// Type is the parameter's type in emit form.
	Type Ref

	// Source is the parameter's declared type in source form, nil
	// when the projection came from the emit side. Retained because
	// every structural query in this package takes a
	// [node.TypeRef], and a consumer classifying a parameter needs
	// one.
	Source *node.TypeRef

	// Field is the exported identifier a recorded-call struct uses
	// for this parameter — the Pascal form of Name.
	Field string

	// Variadic reports whether the source declared `...T`.
	//
	// Type stays the element type, matching the model and what the
	// generated signature spells after the ellipsis. Everything
	// around it changes: a recorded field holds a slice, a
	// forwarding call needs `name...`, and a double that dropped
	// the marker takes one value where the interface takes many and
	// no longer satisfies it.
	Variadic bool
}

// SigReturn is one return slot in rendered form.
type SigReturn struct {
	// Name is the source's declared return name, empty for the
	// anonymous form. The blank identifier normalises to empty —
	// `_` cannot be used as a derived identifier, and leaving it in
	// would make every consumer special-case it.
	Name string

	// Type is the slot's type in emit form.
	Type Ref

	// Source is the slot's declared type in source form, nil when
	// the projection came from the emit side.
	Source *node.TypeRef

	// Field is the exported identifier a recorded-call struct uses.
	// Always populated: a recorded call needs one field per return
	// whether or not the source named the slot.
	//
	// The fallback reads as a failure message rather than as an
	// index. The error slot is `Err`, because that is what it is; a
	// lone value slot is `Result`, since a number distinguishes it
	// from nothing; several value slots are `Result0`, `Result1`, …
	// numbered across the value slots only, so adding an error
	// return does not renumber the fields beside it.
	Field string

	// Local is the identifier a generated body binds this slot to
	// when capturing a call's result. The declared name where the
	// signature carries names, `r<N>` otherwise — prefixed with an
	// underscore on the rare occasion a parameter already holds
	// that identifier, since shadowing a parameter would capture
	// the wrong value.
	Local string

	// Error reports whether this slot carries the builtin error.
	//
	// Recorded per slot rather than derived by position, because a
	// signature returning `(error, string)` is unusual and legal,
	// and a positional rule binds the wrong slot without failing to
	// compile.
	Error bool
}

// SigInfo is a callable projected into the form a generator renders.
type SigInfo struct {
	// Name is the callable's identifier.
	Name string

	Params  []SigParam
	Returns []SigReturn

	// TypeParams is the declaration's own type-parameter list, nil
	// for a non-generic callable.
	TypeParams []*node.TypeParam

	// Receiver is the method's receiver type, nil for a function
	// and for an interface method.
	Receiver *node.TypeRef

	// NamedReturns reports whether the generated signature may
	// carry the source's return names. All-or-nothing; see
	// [NamedReturnsUsable] for both reasons.
	NamedReturns bool

	// Source is the method this was projected from, nil when the
	// projection came from the emit side. The escape hatch: a
	// consumer needing a fact this type does not carry reads the
	// declaration rather than growing the projection.
	Source *node.Method

	// receiverIdent is the identifier the generated method binds
	// its receiver to, held so the collision guards can reason
	// about it.
	//
	// A field rather than an accessor: the deriving package sets it,
	// and it lives here. A template reads it the same either way.
	ReceiverIdent string
}

// ErrReturn returns the slot carrying the error, or nil.
//
// Found by flag rather than by position, so a body assigning an
// injected failure names the right field on a signature returning
// `(error, bool)`.
func (s *SigInfo) ErrReturn() *SigReturn {
	if s == nil {
		return nil
	}
	for i := range s.Returns {
		if s.Returns[i].Error {
			return &s.Returns[i]
		}
	}
	return nil
}

// HasResults reports whether the callable returns anything, which
// decides whether a generated body returns at all.
func (s *SigInfo) HasResults() bool { return s != nil && len(s.Returns) > 0 }

// ReturnsError reports whether any slot carries the builtin error.
func (s *SigInfo) ReturnsError() bool { return s.ErrReturn() != nil }

// ParamByField returns the parameter whose recorded-call field is
// named field, and whether there is one.
//
// [SigParam.Field] is this package's own projection — the Pascal form it
// chose, after keyword adjustment and uniquing — so a consumer holding
// a field name has no way back to the parameter without repeating that
// derivation and trusting the two to agree. A generator reading a
// directive that names a recorded field is exactly that consumer.
//
// Matched on Field rather than Name because the field is what appears
// in the generated struct, which is the name a directive or a template
// is written against.
func (s *SigInfo) ParamByField(field string) (SigParam, bool) {
	if s == nil {
		return SigParam{}, false
	}
	for _, p := range s.Params {
		if p.Field == field {
			return p, true
		}
	}
	return SigParam{}, false
}

// IsGeneric reports whether the callable carries type parameters.
func (s *SigInfo) IsGeneric() bool { return s != nil && len(s.TypeParams) > 0 }

// Variadic reports whether the last parameter is variadic.
func (s *SigInfo) Variadic() bool {
	return s != nil && len(s.Params) > 0 && s.Params[len(s.Params)-1].Variadic
}

// Idents returns the parameter identifiers in order.
func (s *SigInfo) Idents() []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s.Params))
	for i := range s.Params {
		out[i] = s.Params[i].Name
	}
	return out
}

// Locals returns the capture identifiers in order.
func (s *SigInfo) Locals() []string {
	if s == nil {
		return nil
	}
	out := make([]string, len(s.Returns))
	for i := range s.Returns {
		out[i] = s.Returns[i].Local
	}
	return out
}

// Taken returns every identifier the generated method already
// occupies — its receiver and its parameters.
//
// What a caller passes to [UniqueIdent] when choosing a name of its
// own, so a helper variable a generator introduces cannot shadow
// something the signature declared.
func (s *SigInfo) Taken() []string {
	if s == nil {
		return nil
	}
	out := make([]string, 0, len(s.Params)+1)
	if s.ReceiverIdent != "" {
		out = append(out, s.ReceiverIdent)
	}
	return append(out, s.Idents()...)
}
