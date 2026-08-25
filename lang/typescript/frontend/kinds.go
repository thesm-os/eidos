// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

// Grammar node kinds referenced from more than one converter.
//
// Named where they are shared rather than everywhere they appear: a
// constant per kind would be a second vocabulary to keep in step with
// the grammar, and `case "union_type"` reads as the grammar spells
// it. These three earn a name because two converters each match them,
// and a typo in one would be a silently unmatched case rather than a
// build error.
const (
	// kindIdentifier is a value-position identifier.
	kindIdentifier = "identifier"

	// kindDecorator is a decorator. It is matched in three places
	// because the grammar attaches one in three different positions
	// depending on how the declaration was written.
	kindDecorator = "decorator"

	// kindIndexSignature is an index signature. It is matched in four
	// places because both a declaration body and a type expression
	// may carry one, and each has to tell it from a mapped type.
	kindIndexSignature = "index_signature"

	// kindRequiredParam and kindOptionalParam are the two parameter
	// forms. They appear both in a parameter list and as the labelled
	// elements of a tuple type, which is why they are shared.
	kindRequiredParam = "required_parameter"
	kindOptionalParam = "optional_parameter"
)
