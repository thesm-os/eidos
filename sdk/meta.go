// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import (
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/node"
)

// The metadata surface — how an annotator states a fact about a
// declaration and how a generator reads it back.
//
// This is the only channel between an annotator and a generator.
// Everything a shape detector infers, every `<lang>.*` fact a
// frontend records, and every directive override the pipeline
// applies lands in a [Bag] under a typed [Key].

// Bag is the per-owner metadata store hanging off every source
// node and emit value. Reached via `n.Meta()` for reads and
// `n.EnsureMeta()` for writes.
//
// A nil Bag reads as empty and panics on write, deliberately: an
// owner holds nil until something writes, because allocating one
// to answer a question is itself a write and therefore a race
// between two concurrent readers. The panic marks a caller that
// skipped EnsureMeta, and reaches the user as an attributed
// diagnostic rather than a crash.
type Bag = meta.Bag

// Key is a typed metadata key. Declare one per fact at package
// level, then read and write it through its methods — the type
// parameter is what stops a generator reading a bool out of a
// slot an annotator wrote a string into.
type Key[T any] = meta.Key[T]

// MetaParser converts a directive's raw string argument into a
// key's value type, so `+gen:` overrides can reach a typed key
// without the plugin doing untyped access. Named with the prefix
// because [Parser] is the directive parser.
type MetaParser[T any] = meta.Parser[T]

// MetaProvenance is one recorded metadata operation — the value,
// who set it, at which [MetaAuthority], and where. Read through
// [Bag.Winning] or [Bag.History] by tooling that has to explain
// why a fact is stamped.
type MetaProvenance = meta.Provenance

// MetaAuthority ranks the source of a metadata write. The highest
// authority wins a name; within one authority a tombstone beats a
// value and the last write beats earlier ones.
type MetaAuthority = meta.Authority

// The three authorities, lowest to highest. A plugin's own
// inferences are [AuthorityPlugin]; a `+gen:` override in source
// is [AuthorityDirective]; a human's explicit statement is
// [AuthorityManual]. An annotator that writes at the wrong
// authority silently overrules the source it was meant to obey.
const (
	AuthorityPlugin    = meta.AuthorityPlugin
	AuthorityDirective = meta.AuthorityDirective
	AuthorityManual    = meta.AuthorityManual
)

// The stock parsers, for the value types directives carry.
//
//nolint:gochecknoglobals // alias re-exports of stable helpers.
var (
	// StringParser takes the raw argument verbatim.
	StringParser = meta.StringParser

	// StringListParser splits a comma-separated argument.
	StringListParser = meta.StringListParser

	// BoolParser accepts the conventional truthy / falsy spellings.
	BoolParser = meta.BoolParser

	// IntParser parses a decimal integer.
	IntParser = meta.IntParser

	// NodeRefParser validates an argument naming another
	// declaration.
	NodeRefParser = meta.NodeRefParser
)

// NewBag returns an empty [Bag]. Plugin code calls `EnsureMeta()`
// on the owner instead; this exists for the caller assembling a
// node or emit value from scratch.
//
//nolint:gochecknoglobals // alias re-export of a stable factory.
var NewBag = meta.NewBag

// NewKey declares and globally registers a typed metadata key.
//
// Registration is what lets the directive-override step find the
// parser by name, so a key declared without it is invisible to
// `+gen:` overrides. Registering the same name twice panics with an
// error wrapping [ErrDuplicateKey] — at init, for a package-level
// `var`, which is where a duplicate belongs rather than at the first
// read. Use [EnsureKey] where two packages may legitimately declare
// the same key.
//
// Generic re-exports are thin wrappers rather than `var` bindings
// because Go cannot bind an uninstantiated generic function to a
// variable. The wrapper preserves type identity: the [Key] it
// returns is the same type [meta.NewKey] returns.
func NewKey[T any](name string, parser MetaParser[T]) Key[T] {
	return meta.NewKey(name, parser)
}

// EnsureKey declares a typed metadata key, returning the existing
// registration when the name is already taken rather than
// failing.
//
// This is the one to reach for by default. A key shared across
// plugins — a shape fact several detectors stamp, say — is
// declared in each of them, and NewKey would make load order
// decide whether the process starts.
func EnsureKey[T any](name string, parser MetaParser[T]) Key[T] {
	return meta.EnsureKey(name, parser)
}

// AnyKey is the type-erased view of a [Key] — the operations that do
// not need its static type. The directive-override step holds one to
// stamp a directive's raw string into a [Bag] through the key's
// registered parser, without knowing what the key carries.
type AnyKey = meta.AnyKey

// Observer is the callback [Bag.AddObserver] fires on every Set
// against the bag, carrying the name written.
type Observer = meta.Observer

// LookupKey returns the registered key of a given name.
//
// Most plugins hold their own typed [Key] and never need this — it is
// for a caller resolving a name it was handed rather than one it
// declared, which is what the directive-override step does. Reports
// an error wrapping [ErrUnregisteredKey] when the name was never
// registered.
//
// Named LookupKey rather than Lookup because a bare Lookup on a façade
// spanning the whole plugin surface names no registry.
//
//nolint:gochecknoglobals // alias re-export of a stable function.
var LookupKey = meta.Lookup

// ParseAuthority converts an authority's string spelling to its
// [MetaAuthority] constant, reporting an error wrapping
// [ErrUnknownAuthority] for a string naming none.
//
//nolint:gochecknoglobals // alias re-export of a stable function.
var ParseAuthority = meta.ParseAuthority

// Error sentinels surfaced by key registration, value parsing, and
// authority lookup. Plugin code that wants to distinguish failure
// modes wraps them with [errors.Is].
var (
	// ErrParse is wrapped by every stock parser when its input is not
	// a value of the key's type — [IntParser] on "abc" reports it.
	ErrParse = meta.ErrParse

	// ErrDuplicateKey is wrapped by the value [NewKey] panics with
	// when the name is already registered. [EnsureKey] returns the
	// existing key rather than raising it.
	ErrDuplicateKey = meta.ErrDuplicateKey

	// ErrUnregisteredKey is returned by [LookupKey] when no key
	// matches the name.
	ErrUnregisteredKey = meta.ErrUnregisteredKey

	// ErrUnknownAuthority is returned by [ParseAuthority] for a string
	// naming no authority.
	ErrUnknownAuthority = meta.ErrUnknownAuthority
)

// MetaFrontend is the key every frontend stamps on the packages it
// produces, naming the language it parsed — `"golang"`, `"protobuf"`.
//
// Re-exported from [node.MetaFrontend] rather than declared here. A
// meta key is interned by name, so a package re-declaring it by string
// forfeits the compile-time link to everyone else using it — and this
// façade cannot be the one declaration, because a frontend sits below
// it and cannot import it. The three that stamp the key were spelling
// it themselves for exactly that reason.
//
// Read it through [LanguageOf] rather than directly, so no caller has
// to decide separately what an unstamped package means.
//
//nolint:gochecknoglobals // re-exported meta key registration.
var MetaFrontend = node.MetaFrontend

// LanguageOf returns the language pkg's declarations were written in,
// or empty when nothing stamped one.
//
// The lookup an annotator makes to find the [SourceRules] that apply
// to what it is reading:
//
//	rules, ok := p.Source(sdk.LanguageOf(pkg))
//
// The answer is a language name from the same namespace a backend
// answers to, which is what lets one [LanguageSupport] declaration
// carry both halves. It is *not* interchangeable with the language a
// run renders: a run parsing Go and emitting TypeScript has both, and
// a plugin reading a Go struct tag under the render language would be
// applying the wrong language's rules to source.
//
// Empty is the honest answer for a fixture, a bridge or a synthesised
// graph — nothing produced it, so no language claims it, and a caller
// looking the result up finds no rules rather than the wrong ones.
func LanguageOf(pkg *Package) string {
	if pkg == nil {
		return ""
	}
	name, _ := MetaFrontend.Get(pkg.Meta())
	return name
}
