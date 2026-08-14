// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package directive

import "go.thesmos.sh/eidos/core/kind"

// SchemaBuilder is the fluent constructor for [Schema]. Every method
// returns the receiver, so call chains read top-down:
//
//	directive.NewSchema("mock").
//	    On(node.KindInterface).
//	    Requires("repo").
//	    RequiredKeys("target").
//	    Positional("variant", directive.OneOf("default", "stub")).
//	    Build()
//
// [Schema.AllowNegated] defaults to true; call [SchemaBuilder.DenyNegation]
// for directives whose negated form has no meaning.
type SchemaBuilder struct {
	s Schema
}

// NewSchema starts a builder for the directive with the given bare name.
func NewSchema(name Name) *SchemaBuilder {
	return &SchemaBuilder{s: Schema{Name: name, AllowNegated: true}}
}

// FromSchema wraps an existing [Schema] in a builder so configuration
// loaded from disk can flow through the same registration pipeline.
func FromSchema(s Schema) *SchemaBuilder { return &SchemaBuilder{s: s} }

// On restricts the directive to specific node kinds. Empty means
// "any kind"; subsequent calls append.
func (b *SchemaBuilder) On(kinds ...kind.Kind) *SchemaBuilder {
	b.s.AppliesTo = append(b.s.AppliesTo, kinds...)
	return b
}

// Requires lists directives that must also be present on the same
// node when this directive appears.
func (b *SchemaBuilder) Requires(names ...Name) *SchemaBuilder {
	b.s.Requires = append(b.s.Requires, names...)
	return b
}

// ExclusiveWith lists directives that must not appear on the same node.
func (b *SchemaBuilder) ExclusiveWith(names ...Name) *SchemaBuilder {
	b.s.MutuallyExclusiveWith = append(b.s.MutuallyExclusiveWith, names...)
	return b
}

// RequiredKeys names KV keys that must be present.
func (b *SchemaBuilder) RequiredKeys(keys ...string) *SchemaBuilder {
	b.s.RequiredKeys = append(b.s.RequiredKeys, keys...)
	return b
}

// AllowedKeys restricts the permitted KV keys when set. Subsequent
// calls append; an empty AllowedKeys list accepts any key. A
// directive that accepts none says so with [SchemaBuilder.DenyKeys],
// since calling this with no arguments cannot be told from never
// calling it.
func (b *SchemaBuilder) AllowedKeys(keys ...string) *SchemaBuilder {
	b.s.AllowedKeys = append(b.s.AllowedKeys, keys...)
	return b
}

// DenyKeys rejects every KV pair on the directive.
//
// For a directive whose contract is that it takes no arguments. Such a
// schema could not previously state it: an empty AllowedKeys means
// unrestricted, so a stray key parsed, validated and stamped nothing,
// and an author who wrote one believed it had an effect.
func (b *SchemaBuilder) DenyKeys() *SchemaBuilder {
	b.s.DenyKeys = true
	return b
}

// Positional appends one positional-arg slot.
func (b *SchemaBuilder) Positional(name string, opts ...PositionalOption) *SchemaBuilder {
	pa := PositionalArg{Name: name}
	for _, opt := range opts {
		opt(&pa)
	}
	b.s.PositionalArgs = append(b.s.PositionalArgs, pa)
	return b
}

// AllowExtraPositional permits more positional args than declared in
// PositionalArgs.
func (b *SchemaBuilder) AllowExtraPositional() *SchemaBuilder {
	b.s.AllowExtraPositional = true
	return b
}

// DenyNegation marks the directive as not accepting the `-gen:` form.
// Use on action directives (like `+gen:mock`) where negation would be
// semantically empty.
func (b *SchemaBuilder) DenyNegation() *SchemaBuilder {
	b.s.AllowNegated = false
	return b
}

// Describe attaches a human-readable description for documentation
// output.
func (b *SchemaBuilder) Describe(text string) *SchemaBuilder {
	b.s.Description = text
	return b
}

// Build returns the configured [Schema] by value.
func (b *SchemaBuilder) Build() Schema { return b.s }
