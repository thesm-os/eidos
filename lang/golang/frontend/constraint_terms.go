// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import "go/types"

// constraintTermsFromUnion converts a [types.Union] (the type-checker
// representation of a `~int | ~string` constraint expression) into
// the [ConstraintTerm] slice [MetaConstraintTerms] carries.
func (c *converter) constraintTermsFromUnion(u *types.Union) []ConstraintTerm {
	out := make([]ConstraintTerm, 0, u.Len())
	for term := range u.Terms() {
		out = append(out, ConstraintTerm{
			Type:        c.typeRefOf(term.Type()),
			Approximate: term.Tilde(),
		})
	}
	return out
}
