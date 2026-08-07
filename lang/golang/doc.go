// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package golang holds the Go-language conventions every
// plugin generating Go output shares. It is the Go side of
// the per-language adapter pattern eidos plugins use to keep
// their cores language-agnostic.
//
// # Surface
//
// A Go generator does four things, in order: it asks questions
// of a source declaration, projects it into renderable data,
// spells that data as Go, and declares itself to the pipeline.
// The first three live here. Each group was answered privately
// — and differently — by two or more consumers before it did.
//
// Ask, over [node] values:
//
//   - Signature queries (query.go): [Callable], [HasContext],
//     [StripContext], [TrailingVariadic], [StripVariadic],
//     [ErrorSlot], [ErrorReturn], [StripError], [Deref],
//     [PointerElem], [SliceElem], [ArrayElem], [MapKey],
//     [MapValue], [FuncSignature], [IteratorOfType].
//   - Type predicates (query.go, golang.go): [IsBool],
//     [IsString], [IsNumeric], [IsInteger], [IsAny],
//     [IsBuiltinNamed], [Nilable], [Keyable], [IsExported],
//     [IsByteSlice], [IsSlice], [IsMap].
//   - Well-known shapes (sigshape.go): [IsErrorMethod],
//     [IsUnwrapMethod], [IsStringMethod], [IsWriteMethod],
//     the four codec pairs plus [Codecs], [IsScanMethod],
//     [IsValuerMethod], [ImplementsSorter], [IsEqualMethod],
//     [IsCloneMethod], [IsValidateMethod], [SignatureMatches] —
//     the signatures the standard library gives meaning to, matched
//     on types rather than on a return's binding name.
//   - Embedding (embed.go): [EmbedIdent], [EmbedTarget],
//     [FieldSet], [PromotedFields], [ExportedFieldSet],
//     [PromotedMethods], [EmbedsType] — Go's promotion rules in
//     full, since a generator reading `s.Fields` reads what the
//     source typed rather than what the struct has.
//   - Satisfaction (satisfies.go): [Satisfies], [SameSignature],
//     [UnderlyingOf], [ComparableDeep], [RecommendedReceiver].
//   - Enums (enum.go): [EnumFormOf], [VariantText], [EnumTexts],
//     [DuplicateText], [ZeroVariant], [EnumValues],
//     [OutOfRangeValue], [OutOfRangeText], [EnumMethods],
//     [IsIotaDerived] — the six facts every enum generator derives,
//     including the one that matters: a string enum's textual form
//     is its declared value, not its identifier.
//   - Metadata accessors (accessors.go): [IsError], [IsContext],
//     [IsInterface], [ReceiverIsPointer], [Tag] — typed readers
//     over the `go.*` vocabulary below.
//
// Project, from either model:
//
//   - [Sig], [Param], [Return] with [SigOf], [SigOfFunc] and
//     [SigOfEmit] (method.go) — one callable in the form a
//     generator renders: what a body calls each parameter, which
//     recorded-call field each return maps to, which slot carries
//     the error, whether the source's return names survive.
//
// Spell, into [emit] values or into text:
//
//   - Lifters (refconv.go): [FromNode], [ConstraintFromNode],
//     [FieldType], [ElemType], [MapKeyType], [MapValType].
//   - Emit construction (construct.go): [FuncTypeOf],
//     [EmitParams], [EmitReturns], [CallArgs], [DelegateBody],
//     [CaptureAssign], [RecordCall], [RecordFields],
//     [SatisfiesAssertion], [NilOf], [ZeroValueExpr].
//   - List rendering (render.go): [Args], [ParamNames],
//     [CallFields], [Locals], [LocalFields], [Fails], [Reads] —
//     the expression lists a template writes around a signature.
//   - References (refs.go): [QName], [Display], [LocalName],
//     [RefFor], [RefForQualified], [PkgPathOf], [SubjectRef].
//   - Type expressions (typestring.go): [TypeString],
//     [TypeStringQualified] render a source type as text a human
//     reads; [ParseTypeRef] reads a directive value naming one back
//     into a reference. Neither registers an import, which is why
//     the rendered form is for messages and not for source.
//   - Identifiers (idents.go): [SafeIdent], [UniqueIdent],
//     [ReceiverIdent], [PackageName], [IsKeyword],
//     [IsPredeclared], [IsInternal].
//   - Import paths (imports.go): [IsStdlib], [IsValidImportPath],
//     [ExternalTestPackage], [ImportAlias], [PackageClauseFor].
//   - Values (literals.go, values.go): [ZeroLiteral],
//     [ZeroLiteralFor], [SampleValues], [SampleFor],
//     [StructTag], [ParseTag], [Quote], [RawQuote].
//   - Numeric facts (numeric.go): [NumericBounds], [FitsIn],
//     [NextOutOfRange], [FormatVerb], [ParseIntValue].
//   - Generics (generics.go, witness.go): [TypeParamsOf],
//     [TypeParamDecls], [TypeParamNames], [TypeParamRefs],
//     [SelfRef], [Witnesses], [SubstituteTypeParams],
//     [SubstituteSig].
//   - Conventions (conventions.go): [GeneratedHeader],
//     [IsGeneratedSource], [TestFuncName], [ConstructorName],
//     [SentinelName], [Doc], [DeprecatedDoc], [TestFileName].
//
// # How predicates answer
//
// Every type predicate answers as the union: the `go.*` stamp
// where a frontend supplied one, the Go spelling otherwise.
// Neither half suffices alone. A stamp-only answer reads false
// on every graph no Go frontend produced — a fixture, a bridge,
// a synthesised node — and a spelling-only answer cannot see a
// fact the frontend derived from the type checker. The spelling
// half is gated on [node.TypeRef.IsBuiltin], so a qualified type
// cannot match.
//
// # Template surface
//
// [FuncMap] is the canonical bundle the Go backend merges once;
// it cannot grow without bound, because a name in it is a name
// no plugin may contribute. [SigFuncMap], [QueryFuncMap],
// [ConventionFuncMap] and [AllFuncMap] are opt-in and take a
// prefix, so two plugins can both have them.
//
// # The `go.*` metadata vocabulary
//
// Every Go-speaking part of a pipeline shares one set of meta
// keys: the Go frontend stamps most of them, a bridge from another
// source language stamps the render trio, the Go backend reads
// them at its render site, and plugins query them. They are
// declared here because a meta key is interned by name — a
// consumer that cannot import the declaring package re-declares it
// by string and forfeits the compile-time link.
//
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
//   - go.type — string on a [node.TypeRef]: the rendered Go type
//     expression a bridge produced for a declaration written in
//     another source language. The backend's render site reads it
//     first and falls back to the underlying name when no bridge
//     ran.
//   - go.name — string on a [node.Field] or [node.Package]: the
//     Go-idiomatic identifier, or the Go package clause.
//   - go.import — string on a [node.Package]: the Go import path
//     cross-package references resolve through.
//
// # Boundary
//
// The package depends on [node], [emit], [core/meta] and
// [core/naming] — all leaves over the standard library. It
// reaches into neither a frontend (which would couple plugins to
// a source-language choice) nor the Go backend (which would
// violate the depguard rule banning `plugins/` from importing
// `backend/`), and that is what lets every Go-speaking consumer
// import it: the frontend that stamps the vocabulary, the backend
// that reads it, and the plugins between them.
//
// The [core/naming] dependency is deliberate rather than
// incidental. That package converts identifier case and says
// explicitly that reserved words are "not handled here — callers
// layer it on top, typically inside a language-specific frontend
// or backend helper". [SafeIdent] is that layer, and it needs the
// sanitiser underneath it.
//
// Future per-language packages (`lang/rust`, `lang/typescript`)
// mirror this shape with language-flavoured equivalents.
package golang
