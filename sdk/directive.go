// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import "go.thesmos.sh/eidos/core/directive"

// Directive is one parsed `+gen:` or `-gen:` directive instance.
// Frontends parse directives from source comments and attach them
// to the originating node; annotators / generators read them via
// [node.Node.Directives] or the node's HasDirective / Directive
// accessors.
type Directive = directive.Directive

// DirectiveName is the bare name of a directive — what follows
// the "+gen:" or "-gen:" prefix and precedes the first
// whitespace. Defined as its own string-alias type so [Schema],
// [Directive], and [Registry] signatures are self-documenting;
// literal strings convert implicitly.
type DirectiveName = directive.Name

// DirectiveSchema is the contract for a single directive name —
// which node kinds it may attach to, which other directives it
// requires or excludes, what positional and key/value arguments
// it accepts, and whether it permits the negated form.
type DirectiveSchema = directive.Schema

// SchemaBuilder is the fluent constructor for a [DirectiveSchema].
// Construct via [NewDirective].
type SchemaBuilder = directive.SchemaBuilder

// PositionalArg describes one positional argument slot in a
// [DirectiveSchema]'s argument vector.
type PositionalArg = directive.PositionalArg

// PositionalOption is a functional option applied by
// [SchemaBuilder.Positional]. The helpers [Required], [OneOf],
// [Default], and [Describe] produce ready-to-use options.
type PositionalOption = directive.PositionalOption

// Parser turns directive-bearing comment lines into [Directive]
// values. Most plugin authors use the singleton [DefaultParser]
// rather than constructing their own.
type Parser = directive.Parser

// Registry stores [DirectiveSchema] values keyed by name. The
// pipeline maintains the project-wide registry; plugin code
// typically reads through the pipeline's [FrontendContext.Registry]
// rather than constructing its own.
type Registry = directive.Registry

// NewDirective begins a builder for the directive with the given
// bare name. Chain [SchemaBuilder.On] / [SchemaBuilder.Requires]
// / [SchemaBuilder.Positional] / … to declare the schema, then
// call Build to obtain the [DirectiveSchema] value.
var NewDirective = directive.NewSchema

// FromSchema wraps an existing [DirectiveSchema] in a builder so
// schemas loaded from disk can flow through the same registration
// pipeline as programmatic ones.
var FromSchema = directive.FromSchema

// NewParser returns a [Parser] configured for the supplied
// prefix. Most plugin code uses [DefaultParser] instead.
var NewParser = directive.NewParser

// DefaultParser returns the singleton [Parser] configured for
// the project-wide [DefaultPrefix] ("gen"). Pipelines and
// frontends call this so every directive across the process
// parses identically.
var DefaultParser = directive.DefaultParser

// DefaultPrefix is the project-wide directive prefix every eidos
// frontend, generator, and tool uses: "gen". Comments of the
// form `+gen:NAME …` and `-gen:NAME …` carry directives.
const DefaultPrefix = directive.DefaultPrefix

// NewRegistry returns an empty [Registry] ready for use. Most
// plugin code reads from the pipeline's shared registry rather
// than constructing one.
var NewRegistry = directive.NewRegistry

// Required marks the next positional slot as mandatory; the
// directive parser rejects calls that omit it.
var Required = directive.Required

// OneOf restricts the next positional slot's value to one of
// the supplied strings. Values are compared verbatim.
var OneOf = directive.OneOf

// Default sets the value the next positional slot resolves to
// when the source omits it. Default on a Required slot is a
// no-op.
var Default = directive.Default

// Describe attaches a human-readable description to the next
// positional slot. Validation does not consult Description;
// it is purely informational.
var Describe = directive.Describe

// Validate checks every directive in directives against the
// schemas in registry, in the context of a node of the given
// kind, and emits positioned diagnostics into sink for each rule
// violation. Returns true iff every directive passes.
var Validate = directive.Validate

// HasPositive reports whether list contains a directive named
// name with [Directive.Negated] false — the `+gen:NAME` form.
var HasPositive = directive.HasPositive

// HasNegated reports whether list contains a directive named
// name with [Directive.Negated] true — the `-gen:NAME` form.
var HasNegated = directive.HasNegated

// Last returns the final directive in list named name, or nil when
// none matches.
//
// The counterpart to the node's own `Directive` method, which returns
// the *first* match and answers "is this declared". A repeatable
// value-carrying directive needs the opposite rule: an author writing
//
//	//+gen:default limit=10
//	//+gen:default limit=50
//
// has said the limit is 50, and a generator reading the first emits
// 10 — a value the source contradicts two lines below, with no
// diagnostic, because both directives are well-formed.
//
// Which rule applies is a property of the directive's schema, so it
// is the plugin's to choose. Declaring a directive through this
// package and having to reach past it to read one was the asymmetry
// this closes.
//
//nolint:gochecknoglobals // alias re-export of a stable accessor.
var Last = directive.Last

// OutDirective is the canonical `+gen:out` directive name —
// the per-source routing override the Layout phase consumes.
// Plugins that consult or honour the per-source routing
// override read this constant instead of the literal
// "out" string.
const OutDirective DirectiveName = "out"

// ValueDirective is the canonical `+gen:value <override>`
// directive name — a per-source string override on any node
// whose identity translates into a rendered or serialised
// string form (enum-variant string forms, sentinel error
// codes, struct-field tags, identifier renames, ...).
// Plugins that translate node identity into strings read
// this constant instead of the literal "value" string so
// the framework owns the shared schema.
const ValueDirective DirectiveName = "value"

// Error sentinels surfaced by the directive parser, registry,
// and validator. Plugin code that wants to distinguish failure
// modes wraps them with [errors.Is].
var (
	// ErrMalformedDirective is returned by [Parser.Parse] when
	// the input is not a well-formed directive.
	ErrMalformedDirective = directive.ErrMalformedDirective

	// ErrInvalidPrefix is returned by [NewParser] when the
	// supplied prefix is empty or contains reserved characters.
	ErrInvalidPrefix = directive.ErrInvalidPrefix

	// ErrSchemaConflict is returned by [Registry.Register] when
	// a schema with the same [DirectiveName] is already
	// registered.
	ErrSchemaConflict = directive.ErrSchemaConflict

	// ErrEmptyKey is returned by [Parser.Parse] for a key/value
	// pair whose key is empty (`+gen:repo =value`). It is a
	// distinct failure from [ErrMalformedDirective] — the line is
	// recognisably a directive, so a plugin reporting on its own
	// syntax can tell "not a directive at all" from "a directive
	// with a broken pair".
	ErrEmptyKey = directive.ErrEmptyKey
)
