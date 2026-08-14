// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package directive

import "go.thesmos.sh/eidos/core/kind"

// Schema declares the contract for a single directive Name.
//
// Registration is opt-in per directive: unregistered names parse and
// remain inert from the validator's perspective. Once a Schema is
// registered, [Validate] enforces every field — AppliesTo,
// Requires / MutuallyExclusiveWith, Required / Allowed keys, and
// the PositionalArgs slot vector.
//
// Schemas are value types; copy freely. Construct via [NewSchema]
// for fluent builder ergonomics rather than struct-literal use.
type Schema struct {
	// Name is the bare directive name this schema covers.
	Name Name

	// AppliesTo restricts the directive to specific node kinds.
	// Empty means "any kind".
	AppliesTo []kind.Kind

	// Requires lists directive names that must also be present on
	// the same node when this directive appears.
	Requires []Name

	// MutuallyExclusiveWith lists directive names that must not
	// co-occur with this one on the same node.
	MutuallyExclusiveWith []Name

	// RequiredKeys lists KV keys that must be present.
	RequiredKeys []string

	// AllowedKeys, when non-empty, restricts which KV keys are
	// permitted. An empty AllowedKeys accepts any keys (still
	// subject to RequiredKeys) — see [Schema.DenyKeys] for the
	// directive that accepts none.
	AllowedKeys []string

	// DenyKeys rejects every KV pair, for a directive whose whole
	// contract is that it takes no arguments.
	//
	// A separate flag rather than a shape of [Schema.AllowedKeys],
	// because an empty list already means unrestricted: a directive
	// accepting no keys and one accepting any are indistinguishable
	// otherwise, and the only way to reject a stray key would be to
	// enumerate a key the directive does not want. Keeping the zero
	// value's meaning also leaves every existing schema unchanged.
	DenyKeys bool

	// PositionalArgs describes positional argument slots in order.
	PositionalArgs []PositionalArg

	// AllowExtraPositional permits more positional args than the
	// PositionalArgs list describes, for variadic directives.
	AllowExtraPositional bool

	// AllowNegated controls whether the `-gen:` form is acceptable
	// for this directive. Default true; action-only directives
	// (where negation has no meaning) can opt out via
	// [SchemaBuilder.DenyNegation].
	AllowNegated bool

	// Description is informational, used by the documentation
	// generator. Validation does not consult it.
	Description string
}
