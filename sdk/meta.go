// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import "go.thesmos.sh/eidos/core/meta"

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
// `+gen:` overrides. Registering the same name twice returns
// [meta.ErrDuplicateKey] — which for a package-level `var` means a
// panic at init. Use [EnsureKey] where two packages may legitimately
// declare the same key.
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
