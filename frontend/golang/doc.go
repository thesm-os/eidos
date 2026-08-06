// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package golang is the Go-language frontend for eidos. It loads Go
// source packages via [golang.org/x/tools/go/packages] and converts
// their declarations into the language-agnostic [node] model — every
// struct, interface, method, field, function, variable, constant,
// type alias, embedded type, and generic type parameter from the
// loaded packages surfaces as the corresponding node kind with
// correct back-pointers, doc comments preserved verbatim, and
// `+gen:` / `-gen:` directives parsed against the pipeline's
// directive registry.
//
// # Pipeline integration
//
// The frontend implements [plugin.Frontend]; register it on a
// [pipeline.Builder] via [pipeline.Builder.WithFrontend]:
//
//	pipeline.New().
//	    WithFrontend(golang.New()).
//	    WithBackend(backend.New()).
//	    Build()
//
// Load is invoked once per [plugin.FrontendContext.Pattern]. Each
// pattern is interpreted by [golang.org/x/tools/go/packages.Load]
// using the conventional Go-toolchain rules — module paths, "./..."
// recursive scopes, and explicit file lists are all accepted.
//
// # Language-agnostic shapes; Go-specific facts in metadata
//
// The frontend never leaks Go-specific shapes into [node] —
// channels become Named refs carrying channel metadata; iter.Seq /
// iter.Seq2 patterns surface as metadata on an ordinary
// [node.Function]; type-set generic constraints attach metadata to
// the [node.TypeParam] rather than inflating [node.Constraint] with
// Go-specific fields.
//
// # Meta-key catalog
//
// The catalog below is the complete set of keys the converter
// stamps. Each is an exported `Meta*` registry singleton declared
// in meta.go, read typed through [meta.Key.Get] and string-keyed
// from templates through the `metaBool` / `metaStr` funcmap
// helpers. `docs/frontend/golang.md` lays out the same catalog
// grouped by host node kind, with worked read examples.
//
// Every stamp records full provenance — author `"golang"` at
// [meta.AuthorityPlugin], positioned at the declaration that
// motivated it — so `eidos explain` traces each fact back to its
// source expression. Boolean keys are written only when the fact
// holds: absence is the negative, a `false` value is never stamped.
//
//   - frontend — cross-frontend provenance marker on every produced
//     [node.Package]; string value `"golang"`. The one key outside
//     the `go.*` namespace and the only one declared through
//     [meta.EnsureKey] rather than [meta.NewKey], because the
//     protobuf frontend declares the same name independently.
//     Bridge annotators and the cross-namespace audit step filter
//     their walks by reading it.
//   - go.isChannel — bool on the [node.TypeRef] a Go channel
//     converts to. [node.TypeRef] has no channel variant, so a
//     channel becomes a Named ref under the synthetic qualified
//     name `go.chan`; this key is what tells that ref apart from a
//     user-declared type of the same name.
//   - go.chanDir — string on the same ref: `"both"`, `"send"`, or
//     `"recv"`, translated from the type checker's channel
//     direction.
//   - go.chanElem — string on the same ref: the element type
//     printed in Go source form (`"int"`, `"context.Context"`,
//     `"*pkg.Type"`). The structured view of the same fact rides on
//     the ref's first type argument; this key is the
//     templates-friendly one.
//   - go.isContext — bool on a [node.TypeRef] naming
//     `context.Context`.
//   - go.isError — bool on a [node.TypeRef] naming the predeclared
//     `error` interface.
//   - go.isStringer — bool on a [node.TypeRef] whose type
//     implements `fmt.Stringer` in either its value or its pointer
//     form.
//   - go.isComparable — bool on a [node.TypeRef] whose type
//     satisfies Go's `comparable` constraint — usable as a map key,
//     usable as a type argument to a `comparable`-bounded
//     parameter.
//   - go.isInterface — bool on a [node.TypeRef] whose underlying
//     type is an interface, type parameters excluded. A Named ref
//     carries a package and an identifier and nothing about what
//     they resolve to, so `io.Reader` and `time.Duration` are
//     otherwise indistinguishable to a plugin; consumers that must
//     tell a collaborator from a plain value read this key instead
//     of resolving names, which they cannot do for types outside
//     the loaded packages. A type parameter is excluded on purpose:
//     its constraint is its underlying type, so `K comparable`
//     would otherwise report as an interface and misroute every
//     generic signature.
//   - go.embedsInterface — bool on a [node.Struct] with at least
//     one embedded field whose underlying type is an interface —
//     Go's promotion-by-embedding case.
//   - go.isEmptyInterface — bool on a [node.Interface] that
//     declares no explicit methods and no embeds.
//   - go.isConstraintInterface — bool on a [node.Interface]
//     embedding at least one type-set union: the declaration is a
//     generic constraint, not a method-set contract.
//   - go.underlyingKind — string on a [node.Alias], one of
//     `"basic"`, `"func"`, `"map"`, `"slice"`, `"array"`,
//     `"pointer"`, `"chan"`. Struct and interface underlying types
//     never appear — those convert to [node.Struct] /
//     [node.Interface] instead of an alias — and a shape outside
//     the enumerated set stamps the empty string rather than
//     guessing, so consumers can detect it.
//   - go.isIterSeq — bool on a [node.Function] whose single result
//     is `iter.Seq[T]`.
//   - go.isIterSeq2 — bool on a [node.Function] whose single result
//     is `iter.Seq2[K, V]`.
//   - go.iterKeyType — string on a `go.isIterSeq2` function: the
//     printed source form of the sequence's key type argument.
//   - go.iterValueType — string on a `go.isIterSeq` or
//     `go.isIterSeq2` function: the printed source form of the
//     sequence's value type argument.
//   - go.iotaValue — int on every [node.Constant] whose value is an
//     integer exactly representable as an int64, and forwarded onto
//     the [node.EnumVariant] promoted from such a constant. Named
//     for the iota-driven enum case it exists to serve; the stamp
//     itself is not restricted to iota-derived constants.
//   - go.receiverIsPointer — bool on a [node.Method] declared on a
//     pointer receiver (`func (*T) Foo()`).
//   - go.constraintTerms — []ConstraintTerm on a [node.TypeParam]
//     whose constraint declares at least one type-set term (the
//     `~int | ~string` form). Each term carries the term's
//     [node.TypeRef] and whether the `~` operator was written; the
//     bag holds the JSON form documented on [ConstraintTerm].
//   - go.tag.<name> — string per struct-tag entry on a [node.Field]:
//     the tag `json:"id" db:"id_col"` stamps `go.tag.json` = `"id"`
//     and `go.tag.db` = `"id_col"`. Tag names are not known until a
//     field is read, so these keys are registered dynamically
//     through [meta.EnsureKey] under the [MetaTagPrefix] namespace
//     instead of an exported per-key singleton.
//
// # Cache integration
//
// Each Load call hashes its input file bytes plus [Version] and
// composes a [cache.NewKey]-shaped cache key. On hit the cached
// JSON-serialized [node.Package] is deserialized and re-wired via
// [node.RewireOwners]; on miss the AST conversion runs and the
// result is written back to the cache for the next run.
//
// # Concurrency
//
// Load is safe to call concurrently across patterns — each
// invocation builds its own package set and writes through the
// per-package locks the [store] enforces. Parallel-frontend
// dispatch is opt-in at the pipeline level via
// [pipeline.Builder.WithParallel].
package golang
