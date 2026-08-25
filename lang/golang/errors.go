// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
)

// Go's error protocol, projected.
//
// The predicates behind this already live in sigshape.go and embed.go:
// [ImplementsError] over a method set, [IsIsMethod] and
// [IsUnwrapMethod] over the two optional halves, [PromotedMethods] and
// [ExportedFieldSet] over what embedding contributes. What this file
// adds is the one composition a generator needs and got wrong twice —
// the method set is walked through embeds and the literal's field set
// is not.

// ErrorInfoOf projects a Go struct into the error contract it carries,
// reporting false for one that carries none.
//
// Matched on the method set rather than on a name. A type declaring
// `Error()` with no result is not an error, and a generated check
// calling it as one does not compile — which puts the failure in the
// consuming repository rather than a diagnostic in this one.
//
// The set is the declared methods plus everything embedding
// contributes. `type NotFoundError struct { *BaseError; Key string }`
// is the dominant Go idiom for a family of custom errors, and reading
// only its declarations finds no Error method at all: a generator
// covering the package's error contract covers half of it, silently.
//
// The field set is *not* walked the same way, and the asymmetry is
// deliberate. A promoted field is named by a selector, and a selector
// is not a composite-literal key — setting `Base.Cause` needs the
// literal nested one level per embed, which the caller does not spell.
// [emit.ErrorInfo.Cause] is the exception, because the check that
// needs it assigns rather than builds a literal, and promotion makes
// that legal however deep the field sits.
func ErrorInfoOf(s *node.Struct, r Resolver) (emit.ErrorInfo, bool) {
	if s == nil {
		return emit.ErrorInfo{}, false
	}
	methods, methodEmbeds := errorMethodSet(s, r)
	if !ImplementsError(methods) {
		return emit.ErrorInfo{}, false
	}
	fields, fieldEmbeds := ExportedFieldSet(s, r)
	return emit.ErrorInfo{
		Addressed:  node.PointerReceiver(methods, MethodError),
		Compares:   IsIsMethod(node.MethodByName(methods, MethodIs)),
		Unwraps:    IsUnwrapMethod(node.MethodByName(methods, MethodUnwrap)),
		Cause:      causeField(fields),
		Members:    errorMembers(fields, r),
		Unresolved: unreachedEmbeds(methodEmbeds, fieldEmbeds),
	}, true
}

// errorMethodSet returns the struct's full method set — declared plus
// promoted — and the embeds the run could not follow.
//
// Promotion order is Go's: a declared member shadows a promoted one,
// which is why the declared methods come first.
func errorMethodSet(s *node.Struct, r Resolver) ([]*node.Method, []UnresolvedEmbed) {
	promoted, unresolved := PromotedMethods(s, r)
	out := make([]*node.Method, 0, len(s.Methods)+len(promoted))
	out = append(out, s.Methods...)
	for _, p := range promoted {
		out = append(out, p.Method)
	}
	return out, unresolved
}

// causeField names the exported field holding the wrapped error, or
// empty when the type carries none.
//
// The first such field rather than the only one: a type carrying two
// has no answer to "which did you mean", which is the rule
// [node.FieldOfType] states for the same question one level down.
//
// Reachable at any depth, unlike [errorMembers]. A check builds its
// subject by declaring it and assigning — `var got T; got.Cause = c` —
// and promotion makes that selector legal however deep the field sits.
// A pointer embed still refuses: the zero of an embedded pointer is
// nil, and assigning through it panics on a type whose contract is
// fine.
func causeField(fields []PromotedField) string {
	for _, f := range fields {
		if !f.ThroughPointer && f.Field != nil && IsError(f.Field.Type) {
			return f.Field.Name
		}
	}
	return ""
}

// errorMembers lifts the exported fields a composite literal in the
// generated file can set, with a value to write into each.
//
// Depth zero only — see [ErrorInfoOf] for why the field walk stops
// where the method walk does not. A promoted field emitted anyway
// produces `invalid field name Base.Cause in struct literal` in the
// consumer's build.
func errorMembers(fields []PromotedField, r Resolver) []emit.ErrorMember {
	out := make([]emit.ErrorMember, 0, len(fields))
	for _, f := range fields {
		if f.Depth != 0 || f.ThroughPointer || f.Field == nil || f.Field.Type == nil {
			continue
		}
		sample, _ := SampleRefFor(f.Field.Type, f.Field.Name, r)
		out = append(out, emit.ErrorMember{
			Name:   f.Field.Name,
			Sample: sample,
			// Only a string arrives in a message unchanged. A number is
			// rendered through a format whose width and base are not
			// visible here, so asserting that `42` appears in the output
			// fails against a type reporting the same value as `042`.
			Verbatim: IsString(f.Field.Type),
		})
	}
	return out
}

// unreachedEmbeds names every embed a walk could not follow, deduped
// across the walks.
//
// One unfollowed embed is one thing wrong with the source, and
// reporting it once per question asked reads as two. Written the way
// the author spelled it, so a diagnostic names something they can find.
func unreachedEmbeds(sets ...[]UnresolvedEmbed) []string {
	var out []string
	seen := make(map[string]struct{})
	for _, embeds := range sets {
		for _, e := range embeds {
			if _, dup := seen[e.Written]; dup {
				continue
			}
			seen[e.Written] = struct{}{}
			out = append(out, e.Written)
		}
	}
	return out
}
