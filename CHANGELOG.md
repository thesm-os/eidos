# Changelog

Notable changes to eidos, newest first.

The project versions each module in the `go.work` set independently, so a
release tag names one module (`eidostest/v1.5.0`) while the entry below groups
everything that shipped together. Entries call out which module a change lands
in, because upgrading one does not pull the others.

This file records what a consumer of the published API needs to know. Internal
refactors, test additions that change no contract, and documentation fixes are
omitted unless they change what a caller can rely on.

## Unreleased

### Breaking

- **`lang/golang`: the embed walks report which embeds failed, not whether any
  did.** `FieldSet`, `PromotedFields`, `ExportedFieldSet` and `PromotedMethods`
  return `[]UnresolvedEmbed` in place of their `bool` second result. The bool
  said an answer was partial and erased which embed was missing and why, so a
  caller could report only that something was wrong. Each entry now names the
  embedding declaration, the embed as the author spelled it, its source
  position, and one of `EmbedNoResolver`, `EmbedNotLoaded`, `EmbedGeneric` or
  `EmbedTooDeep`.

  Severity stays the caller's: a generator that must not emit a partial double
  treats `EmbedNotLoaded` as an error, and one filling a documentation table
  treats the same thing as a footnote.

  Migration: `if !complete` becomes `if len(problems) != 0`.

  Two behaviour changes ride along. A **generic embed is now refused rather
  than promoted** — `Base[T]`'s members are typed in that declaration's type
  parameters, so copying them across produced output naming identifiers not in
  scope; the embed itself stays reachable by its own name and the refusal is
  reported as `EmbedGeneric`. And `PromotedMethods` **applies Go's promotion
  rules in full**: it now recurses instead of stopping at the first level, two
  methods reachable at equal depth cancel rather than the first one winning,
  and an embedded interface contributes its *flattened* set — so a struct
  embedding `io.ReadCloser` has `Read` and `Close`, where before it had
  neither, because `ReadCloser` declares no methods of its own.

  `PromotedMethod` gained `Depth` and `Path` and replaced the `Through` field
  with a `Through()` method returning the first hop, so the same fact has one
  home. `Selector()` renders the explicit path a call needs when promotion
  cancelled.

- **`eidostest/plugintest`: `RunSuite` gained checks that existing plugins can
  fail.** This is the intended effect — each new check catches a defect class
  that was previously silent — but a plugin that passed before may go red on
  upgrade without its own code changing.

  | New check | What it catches | Likely to fire when |
  |---|---|---|
  | `declaration accessors return slices the caller may keep` | `Provides` / `Requires` / `Directives` / `Outputs` returning the plugin's own slice, so a consumer that sorts or filters it in place rewrites your declaration for every later caller | the accessor returns a stored field directly rather than a fresh literal or `slices.Clone` |
  | `TemplateProvider ships templates that parse and claim no reserved name` | a malformed template, which previously surfaced midway through `Render`; and a template claiming the backend's reserved `fragment.` prefix | a typo in a `.tmpl`, or a `define` name starting `fragment.` |
  | `NodesOnly declaration is truthful` (generator suite) | a generator declaring `NodesOnly() == true` that reads the emit graph anyway — a live data race, because the pipeline dispatches its whole bucket concurrently on the strength of that declaration | the generator reads `ctx.Reader.Emit*()` or `ctx.Store.Emit()` while declaring `NodesOnly` |

  **Two Outputs checks now run for the first time.** `FilenameProvider returns
  stable Outputs per language` and `FilenameProvider returns a well-formed
  Outputs slice` previously probed the languages `go`, `rust`, `ts`, `py` and
  `""`. Every real plugin keys `Outputs` on `golang`, so both checks iterated an
  empty slice and validated nothing. They now probe
  `plugintest.ConformanceLanguage`. If your plugin declares a malformed Outputs
  slice — an empty `Suffix`, a duplicate `Tag`, two empty tags, or the
  empty-tag output somewhere other than index 0 — you will see it now.

  Migration: for the fresh-slice check, return `slices.Clone(...)` from the
  accessor. For the others, the failure message names the specific rule.

- **`lang/golang`'s type-parameter helpers are keyed on the parameters, not the
  declaration.** Five node kinds carry a type-parameter list — `Struct`,
  `Interface`, `Function`, `Method`, `Alias` — and the rendering is identical
  for all five, but `TypeParams`, `TypeArgs` and `SelfType` took only
  `*node.Struct`. A generator working over interfaces could not reuse them and
  wrote its own; one downstream consumer ended up with three copies.

  New: `TypeParamsOf`, `IsGeneric`, `TypeParamDecls`, `TypeParamNames`,
  `TypeParamRefs`, `SelfRef`. The three struct-shaped entry points remain and
  now delegate, so there is one implementation rather than two that can
  diverge.

- **`lang/golang` gained the Go identifier rules `core/naming` defers to it.**
  `naming.Identifier` sanitises runes and states that reserved words are "not
  handled here — callers layer it on top, typically inside a language-specific
  helper". Nothing did, so a generator deriving a parameter name from a source
  field called `type` or `len` emitted code that did not compile.

  `IsKeyword`, `IsPredeclared`, `SafeIdent`, `UniqueIdent`, `ReceiverIdent`,
  `PackageName`, `IsInternal`. `PackageName` handles the major-version suffix —
  `example.com/foo/v2` is package `foo`, and taking the trailing segment yields
  a clause that compiles and names the wrong thing everywhere.

- **`bridge/protogo.GoFieldName` is deprecated in favour of `naming.Pascal`.**
  The bridge carried a private `commonInitialisms` table and a hand-rolled
  PascalCase converter that duplicated `naming.CommonInitialisms` and
  `naming.Pascal`; the two agreed on every input tested. Removed no earlier
  than the next minor release.

- **`lang/golang` gained typed readers for the whole `go.*` vocabulary.**
  Twenty-two keys were declared with no accessor, so every consumer wrote the
  value-plus-ok dance itself, decided independently what an absent stamp meant,
  and — where the dance was awkward — re-derived the fact structurally instead.
  A downstream generator matched `iter.Seq` by package and name while eidos
  stamped `go.isIterSeq` on the same function, and wrote its own `IsError`
  beside the stamped `go.isError`.

  `IsError`, `IsContext`, `IsStringer`, `IsComparable`, `IsInterface`,
  `EmbedsInterface`, `IsEmptyInterface`, `IsConstraintInterface`,
  `ReceiverIsPointer`, `UnderlyingKind`, `IotaValue`, `IsChannel`, `ChanDir`,
  `ChanElem`, `IsBidirectionalChan`, `IteratorOf`, `IterKeyType`,
  `IterValueType`, `ConstraintTerms`, `GoType`, `GoName`, `GoImport`, `Tag`,
  `Tags`. Absent reads as false or empty throughout; a nil node reads as
  unstamped rather than panicking.

  Also `IsByte` (both spellings — the frontend records whichever the author
  wrote), `IsEmptyStruct`, `IsVariadic`, and `Instantiation` — the third of
  Go's three type-parameter spellings, completing `TypeParams` (declaration)
  and `TypeArgs` (use). `MetaTagPrefix` moved from `frontend/golang` with the
  rest of the vocabulary and is aliased there as `Deprecated`.

- **`emit/builder.For` no longer takes a target.** The variadic `emit.Target`
  argument was documented as a test-fixture escape hatch, and no production
  caller ever passed a non-zero value — all twenty call sites across this
  repository and the reference plugins passed `emit.Target{}`. Tests that
  pre-stamp a target use `Context.WithTarget`, which is unchanged.

  Migration: drop the second argument. `sdk.NewProvenance(Name)` likewise.

- **`store.EmitView` gained `AppendOrigin` and `AppendOriginAs`, and `emit`
  gained `PrimaryPackage`.** Every generator ends the same way — build one
  value per output, stamp it, queue it against its origin so Layout can route
  it — and each writing that out again is how the copies drift.

  `PrimaryPackage` folds the two ways
  `emit.OutputPackageSetter.SetOutputPackages` says "no answer": the primary
  tag may be absent from a partial map, or present but empty because
  centralised routing could not derive an import path. Both mean the same to a
  caller, and folding them stops each implementor reasoning it out again.

  `emit/builder` keeps `Base` and `Tagged`, which need no store.

- **`node`: interface method sets resolve through embeds.** `node.MethodSet`
  walks an interface's embedded interfaces transitively;
  `store.Reader.MethodSet` supplies a resolver over the loaded graph.

  Reading `node.Interface.Methods` alone reads what the source typed, not what
  the interface has, and the difference is invisible until the generated code
  fails to compile: a double missing an embedded method does not satisfy the
  interface it doubles, and the compiler reports that against the generated file
  rather than against the run that produced it. Every generator reading
  interface methods should move to `MethodSet`.

  An embed that contributes nothing is reported rather than dropped, classified
  as `ReasonUnresolved` (this run did not load it — legitimate for a narrow
  run), `ReasonNonInterface`, `ReasonCyclic`, or `ReasonGeneric` (a
  parameterised embed, refused because the model carries no way to substitute
  its type arguments through the embedded signatures). `MethodSetResult.OK`
  reports whether the set is complete; a generator emitting a type that must
  satisfy the interface checks it.

  Also new: `node.Declares`, `node.MethodByName`, `node.PointerReceiver`,
  `node.FieldOfType`, `node.EmbedName`, `node.LocalName`,
  `node.IsExportedName`, `node.IsConstraint`, and `store.Reader.Implementers`.

  `EmbedName` is worth singling out: an embed by pointer carries its name on the
  pointee, so reading the reference's own name yields the empty string and the
  field is silently dropped from anything derived from it.

- **`reference/stubgen` and `reference/mockgen` now double embedded methods.**
  Both read `Interface.Methods` directly, so a stub or mock for an interface
  embedding another was missing the embedded methods and did not satisfy the
  interface it doubled — a compile error reported against the generated file
  rather than the run. `stubgen` additionally rejected an interface composed
  purely of embeds as "declares no methods".

  Generated output changes for any interface with embeds: the double gains the
  methods it was missing. An embed this run could not resolve is now reported as
  a positioned diagnostic naming the embed and why, rather than silently
  producing a short double.

- **`pipeline.Build` now rejects a plugin that implements part of a
  multi-method optional capability.** A Go interface assertion is
  all-or-nothing, so a plugin declaring two of `plugin.TemplateProvider`'s three
  methods satisfied neither the interface nor any consumer's check for it: the
  backend's assertion failed, the plugin's templates were never parsed and its
  funcmap entries never registered, and the rendered output came out short with
  nothing reported. `plugintest` made the same assertion and *skipped* its
  template checks, so the run stayed green on both sides.

  `Build` returns `pipeline.ErrPartialCapability` naming the plugin, the methods
  it declared, and the methods it did not. The same detection covers
  `plugin.CapabilityProvider`, where the consequence is a declared `Priority`
  the pipeline silently ignores.

  Declaring **none** of a capability's methods is unaffected — opting out of
  templates or of ordering stays free. No plugin in this repository is rejected
  by the new check.

  Migration: add the missing method. For `TemplateProvider` that is almost
  always `TemplateOverrides(lang string) template.FuncMap` returning `nil`,
  which is the method a plugin overriding nothing has no reason to write.

- **`plugin`: `TemplateProvider` and `CapabilityProvider` are now composed from
  single-method interfaces.** `TemplateSource`, `TemplateFuncSource`,
  `TemplateOverrideSource`, `PrioritySource`, `ProvidesSource` and
  `RequiresSource` are new exported interfaces. Both composites are unchanged,
  so every current implementer still satisfies them; the halves exist so the
  detection can probe them individually and distinguish "declared none of them"
  from "declared some of them".

  `plugin.Capabilities()` and `plugin.Gaps(p)` are new: the description of each
  multi-method capability, and the partial-implementation detection over it.
  They live beside the interfaces they describe so a method added to one cannot
  leave the probe behind, and both `Build` and the conformance suite consume the
  same detection rather than holding copies that could disagree.

- **`eidostest/plugintest`: the check
  `CapabilityProvider is implemented in full or not at all` is renamed
  `optional capabilities are implemented in full or not at all` and now covers
  `TemplateProvider` too.** Check names appear in adopters' test output, so the
  rename is visible even though the contract only widened. A plugin declaring
  part of `TemplateProvider` previously reached every template check's
  skip-absent-capability path and reported green.

  `ViolationPartialTemplateProvider` is a new exported `Violation` for driving
  the widened check.

- **`plugin.FrontendContext` gained a `Fingerprint` field.** Additive, so
  existing frontends compile unchanged. Frontends that cache a parsed node
  graph **must** fold it into their cache key — see Fixed, below, for why.

- **`eidostest/plugintest`: `RunFrontendSuite` gained two per-fixture checks and
  now fails a fixture the frontend rejects.** Previously the suite applied a
  fixture's `Options` before building the context and discarded the rejection,
  so a frontend whose schema declared a required field the fixture omitted
  passed every subtest with `Load` invoked zero times.

  | Change | What you see |
  |---|---|
  | a rejected fixture is now fatal | `FrontendFixture "<name>": the frontend rejected the fixture's Options: …; populate them so Load is actually driven` |
  | `fixture=<N>/Load populates the store` | `Load recorded no nodes for fixture "<name>" …` — set the new `FrontendFixture.ExpectsEmpty` when an empty node graph is the outcome the fixture pins |
  | `fixture=<N>/Load re-parses when the composition fingerprint changes` | `… Load read back cache entries written under composition fingerprint …; plugin.FrontendContext.Fingerprint MUST be folded into the cache key` |

  `fixture=<N>/Load is deterministic across two runs` now drives both passes
  against one `cache.Disk` rooted in `t.TempDir()` instead of a `cache.None`, so
  the second pass runs warm. A frontend whose cache round-trip loses a node — an
  owner back-pointer the deserialised graph rewires and the parsed one does not —
  fails there for the first time. The empty-pattern probe now inherits the first
  fixture's `Options`, so a required-option frontend reaches `Load` at all.

  `FrontendFixture` gained `ExpectsEmpty bool` and `Fingerprint string`. Both are
  additive with a working zero value; the suite picks both fingerprints, so
  leaving the field empty still exercises the changed-composition pass.

- **`eidostest/plugintest`: the generator, annotator and frontend suites now
  read the diagnostic sink they hand the plugin.** All three built a collecting
  sink, passed it in, and dropped it on return, so a plugin reporting an
  Error-severity diagnostic on every input cleared 7/7 generator, 4/4 annotator
  and 3/3 frontend subtests. The backend suite has always failed that shape. The
  escape was never silent — `Pipeline.Run` returns `ErrRunHadErrors` after any
  phase that recorded one, so every user run exits non-zero — but the suite
  certified a plugin nobody could use.

  | New subtest | What it catches |
  |---|---|
  | `<Role> produces no Error-severity diagnostics` (per fixture) | an Error or Internal diagnostic on an input the fixture declares the plugin handles |
  | `Generate` / `Annotate` `on empty store produces no Error-severity diagnostics` | a plugin that complains when there is nothing to do, so every project whose patterns expand to no matches exits non-zero |
  | `<Role> diagnostics carry a source position` (per fixture) | a diagnostic built with a zero `position.Pos`, which the text formatter renders as a dash where the file and line belong |

  The frontend's `Load on empty pattern does not panic` probe is deliberately
  exempt from both: rejecting an empty pattern loudly is the conforming
  behaviour, not a defect.

  Migration: fix the plugin, or narrow the fixture to the input it genuinely
  covers — a fixture is a declaration that the plugin handles that input, and a
  negative-path input belongs in a direct test that asserts the diagnostic. For
  the position check only, `GeneratorFixture`, `AnnotatorFixture` and
  `FrontendFixture` gained `AllowsPositionlessDiagnostics bool`, for run- and
  configuration-level complaints that genuinely name no source construct. It
  does not waive the no-Error-severity contract.

### Added

- **`sdk/golang` is the plugin base every Go-generating plugin embeds.** A
  plugin declares six things before it generates anything — name, version,
  priority bucket, capabilities, directives, and per-language outputs plus
  templates — of which nine of the ten methods are identical for every Go
  generator apart from the values they return. `NewGenerator(name, templates,
  outputs...)` takes the three a generator cannot omit and chains the rest;
  `NewPlugin(name)` is the same for a plugin shipping no templates. `Build()`
  freezes the declaration into a `*Base` the plugin embeds.

  Written out per plugin, those methods drift. The set this replaces had
  sixteen copies of the language dispatch and two of them tested the language
  marker against a local constant rather than `golang.Language` — a plugin
  that silently emitted nothing, with no diagnostic, because the string did
  not match.

  `Builder` and `Base` are separate types on purpose: a single mutable type
  would leave a plugin's declared outputs and template tree writable for the
  life of the process, from any goroutine holding the plugin. `Build()` panics
  on a malformed declaration — an empty name, outputs with no template tree, an
  output with no suffix, a duplicate output tag — because it runs inside a
  plugin's `New` and so fires at process start on the first test that
  constructs the plugin.

  Note the trade-off before adopting it: a plugin that writes its own
  capability methods stops compiling when the pipeline adds one, and its author
  decides what the new method answers. Embedding `Base` keeps it compiling
  against a default chosen in a file the plugin author does not read. `Base`
  answers the value meaning *not provided* for anything unset — nil, and
  `sdk.DefaultPriority`, which is the bucket a plugin implementing no
  capability already occupies — and `TestBaseSatisfiesExactly` pins the
  provider set so a new interface method fails loudly there instead.

- **`lang/golang.MethodSet` flattens an interface's method set through its
  embeds.** A generator reading `iface.Methods` reads what the source typed. A
  double missing a method inherited through an embed does not satisfy the
  interface it doubles, and a suite missing one asserts about a different
  interface than the one a consumer implements. Embedded methods come first,
  depth-first in source order, then the interface's own, so generated ordering
  is stable as an embed gains a method.

- **`emit/builder.Queue` and `QueueAs` append a plugin's emit values to an
  origin's slot.** The last four lines of every `Generate`, written once:
  stamp each value with provenance naming its kind and the declaration it came
  from, then append. They write through a new `Appender` port that
  `store.EmitView` satisfies structurally, so the builder stays a leaf over the
  emit model. A nil value is skipped; a nil origin is `ErrNilOrigin`.

- **`sdk` now re-exports the emit-queueing helpers.** `EmitBase`,
  `EmitBaseTagged`, `QueueEmit`, `QueueEmitAs`, `PrimaryPackage` and the
  `EmitAppender` port. `Base`, `Tagged` and `PrimaryPackage` already existed in
  `emit/builder` and `emit`; nothing in `sdk` pointed at them, so plugin
  authors reimplemented all three rather than finding them.

- **`lang/golang` is now the complete Go vocabulary a generator needs.** A Go
  generator does four things in order — asks questions of a source
  declaration, projects it into renderable data, spells that data as Go, and
  declares itself to the pipeline. The package answered fragments of the first
  and third; it now answers all three. Every group below was answered
  privately, and differently, by two or more consumers first.

  **Signature queries** (`query.go`) — `Callable`, `ReceiverOf`,
  `IsInterfaceMethod`, `HasContext`, `ContextParam`, `StripContext`,
  `TrailingVariadic`, `StripVariadic`, `ErrorReturn`, `StripError`,
  `StripErrorTypes`, `PointerElem`, `SliceElem`, `ArrayElem`, `MapKey`,
  `MapValue`, `FuncSignature`, `Deref`, `IteratorOfType`, `IteratorElem`,
  `IteratorSecond`, `IteratorYieldsError`, plus the structural predicates
  `IsBool`, `IsString`, `IsInteger`, `IsFloat`, `IsComplex`, `IsNumeric`,
  `IsAny`, `IsBuiltinNamed`, `IsBlank`, `Nilable`, `Keyable`.

  **Well-known method shapes** (`sigshape.go`) — `IsErrorMethod`,
  `IsUnwrapMethod`, `IsIsMethod`, `IsAsMethod`, `IsStringMethod`,
  `IsWriteMethod`, `IsReadMethod`, `IsCloseMethod`, the four codec shapes,
  `ImplementsError`/`Stringer`/`Writer`/`Reader`, `ReturnsOnly`,
  `SignatureMatches`, `IsByteSliceAny`. Matched on the slot's *type*: every
  `Error() string` in the wild is written anonymously, so a classifier reading
  the binding name compiles and matches nothing.

  **The projection** (`method.go`) — `Sig`, `Param`, `Return` with `SigOf`,
  `SigOfFunc` and `SigOfEmit`. One callable in the form a generator renders:
  what a body calls each parameter, which recorded-call field each return maps
  to, which slot carries the error, whether the source's return names survive.
  It accepts both models, because a generator consuming upstream emit output
  needs the same projection over a shape with no source node.

  **Emit construction** (`construct.go`) — `FuncTypeOf`, `FuncTypeFrom`,
  `EmitParams`, `EmitReturns`, `CallArgs`, `FieldCall`, `MethodCall`,
  `DelegateBody`, `CaptureAssign`, `ReturnLocals`, `RecordCall`,
  `RecordFields`, `SatisfiesAssertion`, `NilOf`, `ZeroValueExpr`.

  **List rendering** (`render.go`) — `Args`, `ParamNames`, `Idents`,
  `IdentArgs`, `Blanks`, `CallFields`, `Locals`, `LocalFields`, `IdentFields`,
  `NamedFields`, `Reads`, `Fails`, `ZeroArgs`.

  **References** (`refs.go`) — `QName`, `Display`, `MethodQName`, `LocalName`,
  `RefFor`, `RefForQualified`, `RefsOf`, `ParamRefs`, `ReturnRefs`,
  `PkgPathOf`, `SubjectRef`, `ErrBadSymbol`.

  **Import paths** (`imports.go`) — `IsStdlib`, `IsValidImportPath`,
  `ExternalTestPackage`, `IsExternalTestPackage`, `ImportAlias`,
  `TrimVersionSuffix`, `PackageClauseFor`, `TestPackageSuffix`.

  **Numeric facts** (`numeric.go`) — `NumericBounds`, `FitsIn`, `IsUnsigned`,
  `BitSize`, `NextOutOfRange`, `FormatVerb`, `ParseIntValue`,
  `ParseStringValue`.

  **Values** (`values.go`) — `Resolver`, `SampleValues`, `SampleFor`,
  `ZeroLiteralFor`, `ParseTag`, `TagValue`, `Quote`, `RawQuote`. Zero and
  sample are different questions: a check comparing against a single value
  passes whenever the subject already held it.

  **Instantiation** (`witness.go`) — `WitnessPalette`, `Witnesses`,
  `WitnessNames`, `WitnessUse`, `WitnessBindings`, `AdmitsAnyBasicType`,
  `SubstituteTypeParams`, `SubstituteSig`, and the emit-side type-param
  lifters.

  **Conventions** (`conventions.go`) — `GeneratedHeader`, `IsGeneratedSource`,
  `BuildTag`, `TestFuncName`, `BenchmarkFuncName`, `FuzzFuncName`,
  `ExampleFuncName`, `ConstructorName`, `GetterName`, `SetterName`,
  `WithName`, `SentinelName`, `ParseFuncName`, `Doc`, `DeprecatedDoc`,
  `TestFileName`, `IsTestFileName`.

  **Template bundles** (`funcmap.go`) — `SigFuncMap`, `QueryFuncMap`,
  `ConventionFuncMap`, `AllFuncMap`, all prefixed so two plugins can hold
  them; the canonical `FuncMap` is unchanged. `TemplateZeroLiteral` is the
  installable form of `ZeroLiteral`, since `text/template` panics on a
  `(T, bool)` signature.

  **Embedding** (`embed.go`) — `EmbedIdent`, `EmbedTarget`, `PromotedField`,
  `FieldSet`, `PromotedFields`, `ExportedFieldSet`, `PromotedMethods`,
  `EmbedsType`. Go's promotion rules in full: a declared field shadows a
  promoted one, a shallower promotion shadows a deeper one, and two
  promotions at equal depth cancel. A generator reading `s.Fields` reads what
  the source typed, not what the struct has — `struct{ Base; Name string }`
  has every exported field of Base as well.

  **Interface satisfaction** (`satisfies.go`) — `Satisfies`,
  `SameSignature`, `MissingMethod`, `UnderlyingOf`, `ComparableDeep`,
  `RecommendedReceiver`, `ReceiverIsPointerDecl`. `Satisfies` distinguishes a
  missing method from one of the wrong signature, which are different
  mistakes. `ComparableDeep` is the honest version of `Keyable`: given a
  resolver it sees that a struct holds a slice field, which the
  reference-only answer cannot.

  **Enums** (`enum.go`) — `EnumForm`, `EnumFormOf`, `EnumUnderlying`,
  `VariantText`, `VariantOverride`, `EnumTexts`, `EnumTextLiteral`,
  `DuplicateText`, `ZeroVariant`, `EnumValues`, `OutOfRangeValue`,
  `OutOfRangeText`, `EnumMethods`, `EnumDeclares`, `IsIotaDerived`. The six
  facts every enum generator derives, including the one two implementations
  disagreed on: for a string-valued enum the textual form is the *declared
  value*, not the identifier. Deriving `US` from `US Region = "us-east"`
  discards the only thing the declaration said — a value arriving from JSON
  no longer parses, while still round-tripping against itself, so a check
  testing only the generated pair passes.

  **Type expressions** (`typestring.go`) — `TypeString`,
  `TypeStringQualified`, `ParseTypeRef`, `ErrBadTypeExpr`. Rendering is for
  messages and doc comments: it registers no import, so text put into
  generated source names a package the file never imports. `ParseTypeRef`
  reads `[]*pkg.T` and `map[string]*User` back into a reference, which is
  what a directive value naming a type needs; function types, channels,
  arrays and generic instantiations are refused rather than half-parsed.

  **More well-known shapes** — `IsMarshalBinary`, `IsUnmarshalBinary`,
  `IsGobEncode`, `IsGobDecode`, `IsScanMethod`, `IsValuerMethod`,
  `IsLenMethod`, `IsLessMethod`, `IsSwapMethod`, `ImplementsSorter`,
  `IsEqualMethod`, `IsCompareMethod`, `IsCloneMethod`, `IsResetMethod`,
  `IsValidateMethod`, `IsByteSliceAny`, and `Codecs`, which reports only
  *complete* pairs — a type declaring `MarshalJSON` without its partner does
  not round-trip, and a check asserting that it does fails against code that
  never claimed to.

  **Doc and tag conventions** — `DocSummary`, `DocDeprecation`, `JSONName`,
  `HasTagOption`. `JSONName` distinguishes `json:"-"` from `json:"-,"`, which
  a caller splitting on the comma gets backwards.

### Changed

- **`lang/golang` predicates now answer as a union: the `go.*` stamp where a
  frontend supplied one, the Go spelling otherwise.** `IsError` and
  `IsContext` previously read the stamp alone, which is false on every graph
  no Go frontend produced — a test fixture, a bridge from another source
  language, a generator's own synthesised node. A generator asking which
  return slot carries the error found none.

  The spelling half is gated on `node.TypeRef.IsBuiltin`, so a qualified
  `mypkg.error` cannot match. What remains is a type declared in the package
  under generation and named `error`, which shadows the predeclared identifier;
  every Go classifier in this repository already accepts the same risk.

  Migration: none for a pipeline with a Go frontend. A consumer relying on the
  narrower answer — asserting that an unstamped `error` reads as *not* the
  builtin — now sees it recognised.

- **`lang/golang` gained the zero-value and struct-tag spellings.**
  `ZeroLiteral` returns a type's zero as Go source and reports whether one is
  derivable at all — the second result is the point. A generator naming a field
  in a composite literal needs a value for it, and a partial private copy of
  this table answered `nil` for the widths it omitted, so an `int8` field
  rendered `Code: nil`. Not derivable for a named non-interface type, an array,
  an anonymous struct or a type parameter, because the model records a name and
  the caller may be able to resolve what the model cannot.

  `StructTag` assembles a tag from ordered `TagEntry` pairs, quoted with Go's
  own escaping and without the surrounding backticks — the backend owns those,
  and a caller embedding them produces a literal it cannot nest.

- **`lang/golang` gained the signature rules three generators had each written
  a slice of.** `ParamIdent` and `ParamIdents` name a parameter a generated
  body can reference: the declared name where there is one, `arg<N>` where the
  source left it anonymous, keyword adjustment on top, and uniqueness within
  the list — `Read(arg1 []byte, []byte)` names the first parameter exactly what
  the second falls back to.

  `ErrorSlot` and `ReturnsError` find the error return by asking each slot
  rather than by taking the last one: `(error, string)` is legal Go, and a
  positional rule binds the wrong slot without failing to compile.
  `NamedReturnsUsable` reports whether the source's return names survive onto a
  generated signature — all-or-nothing, because Go requires results to be all
  named or all anonymous and a name colliding with the receiver does not
  compile.

- **`eidostest/plugintest`: `Violation`, `BrokenPlugin`, `Violations`,
  `LyingNodesOnlyGenerator`, `ErroringGenerator`, `ErroringAnnotator` and
  `ErroringFrontend`.** Plugins that deliberately break exactly one framework or
  per-role contract. Running `RunSuite` against one and watching it fail is the
  cheapest way to confirm a conformance harness is wired up at all — the failure
  mode this guards against is a suite that is never actually invoked. The three
  `Erroring*` fixtures report an Error-severity diagnostic on every input and
  otherwise behave, so they defeat the diagnostic checks and nothing else.

- **`eidostest/plugintest.ConformanceLanguage`.** The backend language every
  capability lookup in the suite is driven with. Exported because it is part of
  the contract: a plugin keyed on any other spelling answers none of the suite's
  probes.

- Fuzz targets across `core/directive`, `core/naming`, `core/srcfile`, `writer`,
  `store`, `pipeline`, `cli` and `frontend/golang`; benchmarks including scaling
  cases across the hot paths; and executable examples for the `eidostest`
  harnesses and `emit/builder`.

### Fixed

- **`plugins/generator/sentinel` recognised no custom error type at all.**
  `node.Return` carries a binding name alongside the type and both spell that
  field `Name`, so the three signature predicates read the binding name after
  the model gained named return slots. Every `Error() string` in the wild is
  written anonymously, so the predicate matched nothing: the whole
  custom-error-type half of the plugin went silent — no diagnostic, no output,
  no failing test, because the fixtures only asserted the framework contract.
  `Is` and `Unwrap` detection were dead the same way.

  Consumers of a package declaring an error type now get the checks that half
  was always meant to emit.

- **`plugins/generator/sentinel` wrote `nil` into typed struct fields.** A
  private zero-literal table answered `nil` for every width it omitted, so an
  `int8` field rendered `Code: nil`. The rendered check also used a field's
  sample in two positions Go constrains further than a composite literal does:
  `wantF := <sample>` needs a value with a default type and `target.F != wantF`
  needs a comparable one, and the nil keyword satisfies neither.

  Fields whose only derivable zero is nil — pointers, slices, maps, and named
  types the model cannot resolve — are now dropped from the rendered checks
  rather than emitted into a file that does not compile. A dropped field costs
  one assertion; rendering it cost the whole file, and the failure landed in
  the consumer's build of generated code.

### Added

- **Every shipped plugin now declares `plugin.Versioned`.** The pipeline hashes
  each plugin's `name@version` into the composition fingerprint that frontends
  fold into their cache keys, and the backend's version feeds the per-plugin key
  as well. A plugin declaring no version contributed `name@""`, so changing its
  behaviour could never invalidate a warm cache — the caller got stale output
  with no signal, which is what made `--no-cache` a workaround in the first
  place.

  Seventeen plugins were missing it: every `reference/` plugin except auditgen,
  errorgen, handlergen and validategen; all four under `plugins/`; plus
  `backend/golang` and `bridge/protogo`. All now declare `Version = "1.0.0"`.

  `plugintest`'s `AssertVersionedStability` cannot catch the omission — it
  checks that a declared version is stable, and a plugin that does not implement
  the interface passes it vacuously. `cmd/eidos-reference` gained a guard
  asserting the declaration itself over the exact set the binary registers.

- **`emit`: `StmtRender` and `NewRenderStmt`, so a plugin can contribute into a
  statement slot while rendering through its own template.** The `prebody` /
  `postbody` slots on `Method` and `Function` are constrained to `emit.stmt`,
  which meant a cross-cutting plugin could not append a value of its own emit
  kind there — and therefore could not own how its contribution renders. It had
  to assemble the `emit.Stmt` union in Go, and anything the union does not model
  fell back to `NewRawStmt` text.

  `NewRenderStmt(node)` wraps any `emit.Node` in a statement that reports
  `emit.KindStmt`, satisfying the slot, and defers its spelling to the template
  registered under the wrapped node's `Kind`. Provenance, ordering and import
  collection are unaffected: the wrapped node is walked as part of the graph, so
  its external references still register on the host file.

  The wrapper is deliberately thin rather than a widening of the slot. Backends
  type a statement block as `[]*emit.Stmt` end to end, so a slot item has to be
  a `Stmt`; admitting bare nodes would push a runtime type check into the block
  renderer, which exists precisely to assume its contents are statements.

  Both spellings remain supported and neither is deprecated —
  `docs/plugin/recipes.md` documents the choice. `reference/auditweaver` and
  `reference/debugweaver` now take the template route and are the worked
  examples.

  `backend/golang` reports two ways of building the wrapper wrong, both at
  render time: a nil wrapped node wraps `ErrUnsupportedStmt`, and a wrapped kind
  with no registered template wraps `ErrTemplateMissing`.

### Fixed

- **`frontend/protobuf`: a cold run and a cached run produced different graphs.**
  `convertFiles` never wired the owner back-pointers, while the cache-hit path
  rewires them explicitly because JSON breaks the cycle with `json:"-"` on every
  `Owner`. A freshly-parsed field therefore reported a nil owner and the same
  field read back from the cache reported its struct, so anything resolving a
  declaration's qualified name upward saw one answer on the first run of a
  project and another on the second. Conversion now rewires too.

- **`emit`: a reserved slot's element kind depended on which accessor reached
  it first.** Slot creation is lookup-or-create, and only the typed accessors
  passed a kind: `Method.Prebody()` minted a slot constrained to `emit.stmt`,
  while `Method.Slot("prebody")` minted the same slot unconstrained. Whichever
  ran first won, so two plugins contributing to one method got different
  validation depending on registration order — and the unchecked path was
  reachable by accident, letting a foreign node reach the renderer and fail as
  malformed output instead of at append time with the offending plugin named.

  The element kind is now a property of the slot name, identical through both
  accessors, for every reserved slot: `imports` on `File`; `fields` / `methods`
  / `embeds` on `Struct`; `methods` / `embeds` on `Interface`; `methods` on
  `Alias`; `variants` on `Enum`; and `prebody` / `postbody` / `params` /
  `returns` on `Method` and `Function`. `File`'s `top` / `bottom` / `init`,
  `Field`'s `tags`, every `Package` slot, and all custom names stay
  unconstrained as before.

  This is a defect fix that changes behaviour, so it ships without a
  deprecation cycle. A plugin that reached a reserved slot by string and
  appended a foreign kind was already producing output the backend could not
  render; it now fails at `Append` with `ErrSlotElementType` naming the slot.
  Plugins that want to contribute a node of their own kind should have the host
  declare a custom slot — see `docs/plugin/composition.md`.

- **`pipeline`: a `+gen:out` directive could write outside the source tree.**
  `splitOutDirectivePath` stripped a leading separator but left `..` segments
  intact, and `composeTarget` applies `filepath.Join(originDir, dir)` — which
  cleans, so each surviving `..` cancelled a component of the origin's own
  directory. `+gen:out ../../x.go` escaped the package, and with enough segments
  the module. The path is now cleaned against a synthetic root, which is what
  the docblock always claimed. A trailing separator still means
  "directory, keep the composed filename", so `+gen:out build/` is unaffected.

- **`writer`: `ImportSet.Imp` returned the same-package elision sentinel for a
  foreign package.** `DefaultAlias` took everything after the last `/`, which
  for a trailing-slash path is the empty string — and an empty alias tells the
  backend to emit the symbol unqualified. An `emit.External` path picking up a
  trailing slash from a join or a config value produced a reference to an
  undefined bare symbol. Trailing separators are now trimmed before derivation,
  and a path with no derivable alias is rejected with an error wrapping
  `ErrEmptyPath` rather than silently aliased to `""`.

- **`writer`: `ImportSet.Imp` was quadratic in paths sharing a last segment.**
  Collision resolution restarted its suffix scan at `2` for every path and
  formatted each probe, so 1000 colliding imports cost 31ms and 1.27M
  allocations. It now resumes from the highest suffix already issued. Alias
  assignment is unchanged; 1000 colliding imports cost 256µs and 5.5K
  allocations.

- **`frontend/golang`, `frontend/protobuf`: a warm cache served a node graph
  parsed by an older frontend, indefinitely.** The cache key contained a
  hand-maintained `FrontendVersion` constant that had never been incremented,
  while the frontends' metadata stamping had changed. With unchanged source, an
  eidos upgrade did not invalidate anything, and plugins depending on newer
  metadata silently saw a graph that predated it — `--no-cache` was the only
  workaround. Keys now additionally carry the pipeline composition fingerprint
  (every plugin's name paired with its `Versioned` string) and the module
  version from build info where available.

  Consequence: upgrading eidos, or any plugin that declares a version,
  re-parses. That is deliberate. A plugin that implements no `Versioned`
  contributes an empty version and does not invalidate — declaring a version is
  how a plugin opts into being able to invalidate the cache its output depends
  on.

- **`.gitignore` discarded every fuzz reproducer.** A bare `fuzz/` rule matched
  `testdata/fuzz/`, so a crasher found by `go test -fuzz` was dropped before it
  could be committed and the regression evaporated. `testdata/fuzz/` is now
  un-ignored, mirroring the existing rule for `testdata/rapid/`.

### Changed

- **The `bench:` gate is removed from `.ergon.yaml`**, along with the
  `bench-baseline` and `bench-regression` make targets. It configured a
  regression threshold against a baseline file that was never created, so it
  gated nothing. Benchmarks remain and `make test-bench` still runs them; they
  are diagnostics, not a gate. The scaling cases are the ones worth reading —
  a code generator's risk is superlinear behaviour on a large workspace, not
  throughput.

### Known limitations

- **`core/naming` case conversion is idempotent over ASCII letters and
  separators, not universally.** `Pascal("aA1")` yields `"AA1"`, and applying
  `Pascal` again yields `"Aa1"`; `Pascal("aÉ")` and `ScreamingSnake("ßa")`
  behave similarly. Every style reaches a fixed point by the second
  application. Fixing this would change generated identifiers for existing
  consumers, so it is documented rather than changed.

- **Branch coverage cannot be measured for nine of the ten modules.** gobco
  instruments by copying a package to a temporary directory, and every module
  except the root uses a relative `replace` that does not resolve there.

- **The pipeline-level cache is write-only.** Frontends implement real
  skip-on-hit; the per-plugin key `pipeline` records for annotators, generators
  and the backend is a fingerprint for tooling and is never read to skip work.
