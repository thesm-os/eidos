// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package deprecated

import "go.thesmos.sh/eidos/plugins/annotator/shape"

// Name is the canonical mixin name this package stamps.
const Name = "deprecated"

// Mixin returns the [shape.Mixin] this package contributes.
// Documentary, and unlike its two siblings this package had not said
// so: the docblock described what a generator may do with the stamp —
// skip the callable, emit a warning — which is a hint about handling
// rather than a claim about behaviour. A consumer had already read it
// as documentary and carried that reading privately; the judgement
// belongs here, where the classification is declared.
//
// Nothing about a callable's behaviour follows from its being
// scheduled for removal, so there is no invariant to check and no
// rule for a coverage report to be missing.
func Mixin() shape.Mixin {
	return shape.Mixin{Name: Name, Documentary: true}
}
