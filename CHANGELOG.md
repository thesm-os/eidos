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

- **A param declares its own kind: `Params` is now `[]shape.Param`.** The
  parallel `SiblingParams` and `SiblingVars` lists are gone, and every key
  carries a `Kind` saying what its value names — `KindOpaque`, `KindCallable`,
  `KindVar`, or the new `KindMember`.

  The lists had to be kept in agreement by hand and nothing checked they were:
  a key named as a sibling but omitted from `Params` resolved correctly and was
  invisible to every `Validate` hook, because those read `Params` alone. One
  declaration per key removes the class.

  `KindOpaque` is the zero value, so a literal param is `{Key: "limit"}`. Every
  shipped contract and mixin is migrated; a shape package declaring its own
  params updates the literal and drops the sibling lists.

### Added

- **A resolver scope for members of a role's answered type.** `KindMember`
  resolves a param naming a method on the handle a role's callable returns —
  `watcher next=Next stop=Stop`, where both are declared on the subscription
  `Watch` answers rather than on the interface `Watch` belongs to. Neither
  existing scope could reach them.

  When the run did not load the answered type's declaration the param stamps
  unvalidated rather than reporting, since the resolver has nothing to check
  against. That is the one place a diagnostic's presence depends on the run's
  patterns, and silence there is not a pass.

### Added

- **`partition` names the parameter it isolates on.** `read=` said what observes
  a partition and nothing said which parameter carries one, so a generated check
  had to guess between position and parameter name. Neither is safe: a callable
  taking `(ctx, partition, key, value)` offers two strings of the same type, and
  a check varying both writes to two different keys — passing against an
  implementation that ignores partitions entirely. That check cannot fail, which
  is worse than generating none.

  `partition.ParamAxis` (`axis=`) names the parameter to vary while the rest are
  held fixed. It is deliberately not a sibling param: a parameter has no
  qualified name, so asking the resolver to look one up in scope reports every
  correct axis as not found. It is validated instead — `partition.Mixin()` now
  carries a `Validate` hook, the first use of a `shape.Mixin` field that had
  never been set.

  The hook checks both halves. An axis naming no parameter of the annotated
  callable is reported, and so is one the `read=` partner does not also declare:
  a check carries one partition value across a write and a read, so the pair has
  to spell it identically. Requiring that turns host-to-partner name matching
  from a generator's guess into a checked invariant — and with the pair pinned,
  hold-versus-vary follows from the signatures, since a parameter the reader
  also takes is identity and one only the writer takes is payload.

  The partner check reaches the sibling list through the host's owner, so it
  applies to methods. A free function records a package path rather than a node,
  so its pair goes unchecked rather than falsely reported, and a `read=` that
  named nothing in scope is left to the resolver's own diagnostic rather than
  reported twice.

  Absence stays legal: the bare form is still a classification, and whether a
  check is worth emitting without an axis belongs to the generator.

- **An `answeringwriter` shape — a write that answers the stored state.**
  `(ctx, V) (V, error)` had no classification: `writer` requires no non-error
  result, and the form fell to `reader`, which recorded the written value as a
  key. A caller ordering writes by a store-assigned stamp needs the difference,
  since a `(ctx, V) error` writer answers nothing and the stamp dies with the
  call.

  Drawn by a package-qualified parameter type equalling the first result type.
  Both halves are load-bearing: identity alone takes every coincidentally-typed
  read with it — a string-keyed string cache, a codec's transformers, a
  classifier — and the qualifier separates them, since a predeclared type
  carries no package while an answered stored state is a declared type. It runs
  above `reader` and below `pointerreader`, which returns a bare pointer with
  no error and cannot collide.

  Named for what it does rather than `upserter`, which is already a contract
  here: one is a signature, the other a writer-and-reader protocol.

- **`if-absent` and `orderafter` can name the error their refusal reports.**
  `if-absent conflict=` and `orderafter unready=`. Both claims are refusals, and
  without the sentinel a check asserts only that some error came back — which a
  store refusing every write passes, and which an implementation failing early
  on a nil map passes as ordering enforcement. Resolved through the var scope,
  like the sentinels added beside them.

- **`run` reports outputs a previous run no longer produces.** The pipeline
  still never deletes — a run cannot tell "no longer generated" from "not
  generated by *this* invocation", which is why removal is `eidos prune`
  deliberately — but it now says so rather than leaving the discovery to a build
  failure naming a stale generated file. Scoped to packages the run actually
  loaded, so a narrow `run ./sub/...` does not report every other package as
  orphaned.

- **The four session-guarantee mixins take `version=`.** `monotonicreads`,
  `monotonicwrites`, `readyourwrites` and `writesfollowreads` are defined
  against an ordering oracle — a logical clock, a row version, the global write
  order — and a law replaying a trace has to read it off each operation.
  Nothing in a signature says which member carries it, and two candidates of
  the same type are ordinary. Opaque like `cas version=`, since a member of the
  read or written type is neither a callable in scope nor a package-level var.

- **Five classifications can name the bound their claim turns on.** `bounded
  min=` beside its existing `limit=`, which is a ceiling and said nothing about
  the floor; `windowed window=`, `lease timeout=`, and a `ttl duration=`. A bound
  nobody declared is a number the generator invents, and a law enforcing an
  invented bound fails implementations that are correct against the one their
  author meant.

- **Four laws that had no classification at all now have one.**
  `readyourwrites`, `noduplicates`, `pointintime` and `scheduled schedule=
  fired=`.

  The first two complete sets whose other members already shipped —
  `readyourwrites` is the fourth session guarantee beside `monotonicreads`,
  `monotonicwrites` and `writesfollowreads`, and `noduplicates` the fourth
  stream claim beside `stableorder`, `permutation` and `overmatch`. It is
  distinct from the existing `readafterwrite`, which names a write partner and
  makes a per-method claim rather than one checked across a client's trace.

  `ttl` is the first classification using all three reference kinds at once:
  `put=`/`read=` through the callable scope, `notfound=` through the package's
  vars, and `duration=` left verbatim.

- **Seven more classifications can reach the second method their law calls.**
  Following the relational-mixin work, another set named a relationship without
  naming the method on the other side of it — and a law that cannot call the
  method never exercises the behaviour, so it reports every implementation as
  correct.

  `leakfree open= close=`, `tamperevident tamper= verify=`, `windowed incr=
  count=`, `eventually settle= sync=`, `streamreflectsmutations delete=` beside
  its existing `mutate=`, and a `redeliver` role on `publisher` — without which
  at-least-once and exactly-once cannot be told apart, since they differ only in
  what a subscriber sees after a redelivery.

- **A `poisonable` mixin naming what induces a sticky failure.** The
  `poisonaccessor` shape recognises `func() error` and that is all a signature
  can say; which operation breaks the subject is a fact about the type.
  `Detector` carries no params and `+gen:shape` accepts only `key=`/`value=`, so
  this lands as a mixin on the detected shape — `poisonable induce=Poison` —
  rather than by giving detectors a parameter mechanism, which would blur the
  line between what is detected and what is declared.

- **Seven classifications can name the sentinel their law compares against, and
  the resolver can qualify a package-level var.** Eight laws select on a
  classification carrying an `error` field, and none could say which error — so
  the field stayed nil, `errors.Is(err, nil)` is false for everything a correct
  implementation returns, and the law failed every subject including the right
  ones. Not a weaker check: no check.

  `cursor sentinel=`, `lease held=`, `transaction notfound=`, `tx closed=`,
  `cas mismatch=`, `deleteremoves sentinel=` and `lifecycleafterclose sentinel=`
  each name theirs. The key is named for the law field it binds, so the binding
  stays a table lookup.

  A sentinel is a package-level var, which neither existing declaration
  resolves: `Roles` and `Mixin.SiblingParams` search callables, and `Params` are
  left verbatim. `shape.Contract.SiblingVars` and `shape.Mixin.SiblingVars` are
  the third kind — resolved against the package, reported when they name
  nothing, and rewritten to a qualified name. Widening the callable declarations
  to also accept vars was the alternative and would have cost the property that
  makes either useful: a key declared as a callable reference would silently
  accept a var of the same name.

  Vars resolve against the package for a method host as much as a function one,
  since a sentinel is declared beside the type rather than on it. Absence stays
  legal, as with every other partner: the bare form is still a classification.

- **Twenty mixins covering the properties a signature cannot reveal.**
  `associative`, `causal`, `commutative`, `conservative`, `defaultonerror`,
  `injectionsafe`, `leakfree`, `monotonicreads`, `monotonicwrites`, `overmatch`,
  `permutation`, `snapshotisolation`, `stableorder`, `sticky`, `tamperevident`,
  `timeaware`, `total`, `windowed`, `writesfollowreads` and `xsssafe`.

  Each sits on a structure eidos already detects — an `aggregator`, a
  `streamreader`, a `reader`, a `lifecycle` — and states the promise that
  structure cannot. A `(T, T) T` signature makes associativity askable, never
  true, so these are declared rather than inferred.

  Three carry a param: `conservative field=`, `sticky key=` and `total domain=`.

- **`publisher` gains a `mode` param** naming the delivery guarantee —
  `at-least-once`, `at-most-once` or `exactly-once`. Absent means unstated
  rather than defaulted, because the three imply different assertions and
  choosing one for a publisher that did not say produces a check that fails on
  correct code.

- **Two contracts for pairs the vocabulary could not express: `codec` and
  `chain`.** Both are shapes eidos already detects structurally — a `pure` or
  `mutator` encoding, a `writer` plus a `streamreader` — with no way to say which
  callable completes the pair, so the property they exist to state had nowhere to
  live.

  `codec` declares `forward` and `inverse`, with a required inverse: a forward
  naming none states a round-trip property nothing can check. Its `fidelity`
  param picks the law — `exact` for `inverse(forward(x)) == x`, `lossy` for
  `forward(inverse(forward(x))) == forward(x)`, which is the strongest true claim
  for an encoding that normalises or discards input.

  `chain` declares `append`, `replay` and an optional `verify`. The replay is
  required, because append-only is a claim about history and there is otherwise
  no way to read it. `verify` names an explicit integrity check where an
  implementation offers one; a chain reporting corruption through a poison
  accessor is checkable without it, so requiring it would rule out the commoner
  spelling.

- **`if-match` can name its predicate as a callable.** The contract carried one
  spelling — `pred=`, an opaque expression the resolver never inspects — so a
  consumer naming a method through it got no qualification, no diagnostic when
  the method did not exist, and nothing back-stamped on the predicate to find
  the writer from. `ifmatch.RoleMatch` (`match=Match`) is that callable as a
  partner role, resolved like any other.

  `ParamPred` is unchanged and stays opaque, because it genuinely is:
  `pred=Version==Expected` names no callable, and handing it to the resolver
  reports every correct directive as unresolved. The two are separate keys for
  that reason — a value that gets qualified and one that must be left verbatim
  cannot share a key without the resolver guessing which it received.

  Additive: every existing `pred=` directive stays valid.

- **`mixintest.RunWithValidator`** drives the umbrella, resolver and validator
  over a package and returns the diagnostics, for a mixin carrying a `Validate`
  hook. The internal test helper had no way to reach the validation pass.

## plugins/v1.14.0 — 2026-08-11

### Added

- **A relational shape mixin can name its second callable.** Eight of the eleven
  mixins whose assertion spans two callables had no way to say which second
  callable they mean, so a generator could act on three of them. Stamping is
  permissive, so `sideeffect observe=Observed` reached the meta key — but only a
  declared sibling param makes the resolver rewrite a bare name into a qualified
  one, and without that a consumer holds `"Observed"` with no package and no
  owner.

  `sideeffect`, `partition`, `hooks`, `sample`, `deleteremoves`,
  `streamreflectsmutations` and `lifecycleafterclose` each gain an exported
  param constant with `Params` and `SiblingParams` composed from it —
  `ParamObserve`, `ParamRead`, `ParamRegister`, `ParamBuilder`, `ParamMutate`,
  `ParamClose`. The key names the partner's role, and mixins wanting the same
  role share one: `partition` and `deleteremoves` both take `read`.

  The partner is optional. The bare form still classifies the callable, and a
  consumer wanting only the classification writes it. A partner that names
  nothing in scope was already reported by the resolver against the host's
  position; declaring the param is what turns that diagnostic on.

### Fixed

- **`readafterwrite` participates in the sibling resolution its documentation
  describes.** It exported `ParamWrite` and composed `Params` from it while
  omitting `SiblingParams`, so `write=Save` stamped as a bare name and the
  rewrite never ran — against a package that names it as the worked example of
  that field in three places, including a `Validate` invariant ("every
  readafterwrite's write partner resolves to a known callable") that could not
  hold.

## v1.14.0 — 2026-08-11

### Added

- **`node.Embed` carries the method set of an interface the run did not
  load.** An interface embedding `io.Closer` or `fmt.Stringer` could not be
  generated for at all: the method-set walk resolves an embed only through a
  loaded declaration, so it reported `ReasonUnresolved` and every correct
  consumer emitted nothing, because a projection missing a method describes a
  type that does not satisfy what it claims to. The frontend already
  type-checked the embed and discarded the result. `Embed.Resolved` is that
  projection, as an `*Interface` rather than a method slice so its methods have
  a real declaring owner — `PkgPathOf` walks up to it, and a loaded embed's
  methods are owned by the interface that declares them, so anything else would
  make the same source produce different origins depending on what the run
  loaded.

  A loaded declaration always wins: `MethodSet` consults its resolver first and
  reads the projection only on a miss. `ReasonUnresolved` still fires for an
  embed with neither. The projection is not indexed, so `Interfaces()` returns
  what the patterns asked for.

  `MethodSet` called with a nil resolver now completes an embed carrying a
  projection, where before a nil resolver meant every embed was unresolved. An
  embed holding its own method set needs nothing looked up.

- **`lang/golang.Sample` reports why it derived nothing.** Twelve refusal sites
  answered with the same empty `Sample`, and only one of them was a fact about
  the type. Passing a nil `Resolver` is legal, so every named-type field derived
  nothing and a generator emitted a test asserting nothing about them —
  compiling, passing, and indistinguishable from a builtin that genuinely has no
  literal. `Sample.Refusal` carries a `SampleRefusal`, and
  `SampleRefusal.Incomplete` separates an input a caller could fix from a
  settled fact. Composites report their element's reason rather than their own.
  `ZeroRefFor` reports through the same field.

- **`lang/golang.SampleRefFor` handles slices and maps.** Both fell through to
  the named-type bail, so a field of `[]string` or `map[string]int` derived no
  sample. The map pair differs in the key, because `map[K]struct{}` is how Go
  spells a set and a value-side difference renders two identical literals for
  it.

- **`sdk` re-exports the sentinels its own surface raises.** The façade carried
  `Options`, `Schema`, `BindOptions`, `Bag`, `Key` and the stock parsers, and
  none of the errors those paths return — so a plugin branching on a decode
  failure or a bad directive value imported `core/opt` and `core/meta`, the
  imports the façade exists to remove. All ten now re-export, with the producers
  that make them reachable: `ReflectOptions` reports a malformed `eidos:` tag as
  an error where `BindOptions` panics, and `LookupKey` / `ParseAuthority` /
  `AnyKey` / `Observer` complete the metadata registry.

- **`sdk/golang.Language`.** A plugin implementing `Outputs`, `Templates` or
  `TemplateFuncs` itself had to import `lang/golang` beside `sdk/golang` to name
  the identifier it dispatches on. The failure the constant prevents is silent:
  a plugin returning nothing for an unrecognised language emits no output and
  reports no error.

- **Architecture decision records.** `docs/adr/0002`–`0006` record the five
  choices `README.md` stated as conclusions — static plugin imports, metadata as
  the extension mechanism, slot composition, per-owner template ownership, and
  one backend per run — each with the alternatives it beat and what it costs.
  `README.md` keeps a summary pointing at the tree.

### Fixed

- **`eidostest/storefixture.GoSource` keeps the imports an initialiser
  references.** The projection rebuilds the import set from rendered type
  expressions, and an initialiser is opaque text the type walk cannot see, so a
  fixture declaring `errors` and initialising a sentinel with `errors.New`
  projected over an empty import block — the one outcome the projection
  documents it will never produce. `Constant.Value` carried the same defect.

- **`eidostest/plugintest` runs the template-resolve check for every plugin,
  against the funcmap a backend actually provides.** The check existed and
  nothing called it, and its seed modelled the reserved names only — so a
  template calling `camel` or `metaStr` failed its own conformance suite against
  output the backend renders correctly. `ReservedTemplateFuncs` exposes the
  assembled set the assertion takes.

- **`lang/golang.ImportAlias` and `PackageClauseFor` drop unreachable guards.**
  Both defended against conditions `naming.Identifier` documents it never
  produces — empty output, and a leading digit — and `PackageClauseFor`'s
  comment asserted the opposite of its dependency's contract.

## v1.13.0 — 2026-08-10

### Added

- **`eidostest/storefixture` gains the three enum hooks a fixture could not
  spell.** `EnumBuilder.Variant` takes an optional callback and a new
  `VariantBuilder` carries `Pos` / `Docs` / `Directive` — the last is why it
  exists, because a per-variant text override is the highest-precedence layer of
  the rule deciding what a variant renders as, and it is authored on the variant.
  Until now that layer was reachable only through a real frontend, where every
  other layer was already covered. `EnumBuilder.Method` declares a method on the
  enum's type, the hook `StructBuilder` and `InterfaceBuilder` already carried;
  `Builder.GoSource` already projected one, so no fixture had reached that arm.
  `Builder.PackageName` sets the declared name without touching the import path
  or retargeting any declaration's file, for the case Go allows and
  `Builder.Package` cannot express — `example.com/api/v2` declaring `package api`.

  The existing two-argument `Variant(name, value)` form is unchanged; the
  callback is variadic.

- **The shape mixins name their KV parameters.** Each of the seven mixins
  carrying parameters now exports a constant — `bounded.ParamLimit`,
  `orderafter.ParamFn`, `readafterwrite.ParamWrite`, `scope.ParamName`,
  `timeout.ParamDuration`, `validates.ParamFn`, `wrappedvia.ParamFn` — with
  `Params` composed from it. A consumer reaching for one otherwise writes
  `Params[0]`, which is a position rather than a key: reordering the list, or
  adding a parameter ahead of it, silently changes what every such call site
  reads. The mixins' own tests spelled the key as a literal and now use the
  constant.

- **`pipeline.Builder.WithPlugins` registers each plugin under every role it
  implements** — the dispatch the CLI performs on a consumer's flat plugin slice,
  and until now the only place it existed. A binary assembling its own plugin set
  had to reproduce the four type assertions to answer the one question that
  matters about that set — does it build — or depend on `eidos/cli` from a package
  that needs nothing else from it. `eidostest/pipelinetest.Builder` mirrors it,
  and `golangtest.DriverOf` now delegates to it rather than keeping the second
  copy of that rule this repo had.

  Registering under *every* role is the behaviour: a plugin that annotates and
  generates satisfies `plugin.Generator` on its own, so registering only that half
  type-checks and leaves the annotator silently dead. Do not follow `WithPlugins`
  with a role-typed setter for the same plugin — that registers it twice within
  one role, which `Build` rejects.

- **`plugintest.RunSetSuite` asserts a whole plugin set, not one plugin.** Every
  suite this package shipped was single-plugin, and every property they check is
  one a plugin can satisfy while the set it ships in does not: two plugins with
  one name, two declaring the same directive schema, two providing one capability
  in one bucket. The checks are the pipeline's own — the set is registered on a
  real builder, with stubs filling only the frontend and backend roles it does not
  claim, so a generator-only bundle is testable without a binary. An unprovided
  `Requires` is deliberately *not* a fault: the pipeline documents that it ignores
  one, and asserting closure would fail sets that run correctly.

- **`plugintest.AssertTemplateFuncsResolve` catches a template calling a function
  nobody registers.** `assertFuncMapsBind` asked whether a registered *name* is a
  legal identifier and the parse check deliberately stubs every unresolved call,
  so neither asked the question that actually fails a render. Such a template
  parses, ships, and fails midway through `Render` in the consumer's build. The
  alternative every generator reached for is a hand-maintained list of names,
  which drifts in the safe direction; this reads the call sites from the parser
  itself, so it cannot fall behind the templates.

- **`store.PendingOfType` / `PendingByOrigin`, re-exported through `sdk`**, filter
  queued origin-slot contributions by emit kind. Seven callers in this workspace
  wrote the same type switch over `PendingOriginSlots`, each also paying for that
  accessor's full copy of the pending list before discarding most of what it
  copied; the sequence form makes no copy at all. These are declared rather than
  aliased in `sdk` because Go has no alias form for a generic function.

- **`lang/golang.SequenceOf` answers a method's range-over-func return in one
  call.** The four existing iterator accessors each return nil for a
  non-sequence, so every generator offering a `Yields` helper wrote the same nil
  guard around them — and the guard is load-bearing, because `FromNode`
  propagates nil rather than refusing it, so a template that skipped the branch
  rendered a nil ref and failed at the backend naming the file rather than the
  method. `Sequence`'s zero value reads as "not a sequence", and the sole-return
  rule is stated where it belongs: a method returning `(iter.Seq[V], error)` is
  not one a helper can generate against, because the helper would have to invent
  a value for the error before it could iterate.

- **`lang/golang.SentinelSubject` decomposes `Err<Subject>`** — the third side of
  a pair that was two. `SentinelName` composed and `IsSentinelName` matched;
  nothing read a subject back out, so a caller wrote
  `strings.TrimPrefix(name, "Err")`, which turns `Errors` into `ors` and `Err`
  into the empty string. Both are valid identifiers, so both compile, and the
  emitted message names a subject the author never wrote. Gated on
  `IsSentinelName`, so the three symbols cannot disagree about a name between
  them. Reachable from a template as `<prefix>sentinelSubject`.

- **`lang/golang.IsWellFormedLiteral` + `ErrMalformedLiteral`** validate text a
  generator stamps into source as a value expression. Deliberately shallow: it
  refuses what would produce a file the toolchain cannot *parse* — an empty
  value, an unterminated string, raw string or rune literal — and passes named
  constants, conversions and qualified identifiers to the consumer's compiler,
  which can resolve them. The failure it prevents is the one with no
  attribution: an unbalanced quote fails as a syntax error somewhere else in a
  file the author never wrote.

- **`lang/golang.ForeignVariants` reports the packages declaring constants of an
  enum's type outside its own.** A fact about eidos's own frontend that a
  consumer otherwise has to know: constants coalesce into an enum only within
  one package, so `const Extra cfg.Status = 3` declared elsewhere never reaches
  `Variants`. It is legal Go, and every generated answer about the set is then
  confidently false — `IsValid` rejects a declared value and `String` falls to
  the numeric fallback for a variant that has a name.

- **`lang/golang` resolves a source-level qualifier against the file that wrote
  the import.** `QualifierOf`, `ImportForQualifier`, `FileOf` and
  `ResolveQualified` answer what `RefForQualified` cannot: that one reads
  everything before the last dot as an import path, which is right for a
  directive value an author wrote and wrong for text read out of source, where
  `pb.Event` means whatever `pb` was bound to *in that file*. Resolved the old
  way it produces an `ExternalRef` whose package is `pb`, which the backend
  rejects at render, naming neither the value nor the code that mangled it.

  `QualifierOf` splits on the first dot where `RefForQualified` splits on the
  last — a Go qualifier is one identifier and cannot contain a dot, while an
  import path can. `FileOf` is the step between what a generator holds (a
  declaration) and what resolution takes (a file): `node.Package.FileByName`
  keys on a basename and `position.Pos` carries a path, so a lookup composed at
  the call site is one `path.Base` from always missing, silently.

- **`eidostest/storefixture` can build the three shapes a Go frontend produces
  and the fixture could not spell.** Each was a hole in the harness rather than
  a missing convenience: a test written against a shape no run produces asserts
  on nothing, and passes.

  `Chan`, `RecvChan` and `SendChan` build a channel the way the Go frontend
  records one — a named `go`.`chan` ref with the element on `TypeArgs[0]` and
  `go.isChannel` / `go.chanDir` / `go.chanElem` stamped beside it. Getting it
  half right is worse than not expressing it: the structure without the stamp
  renders as `go.chan[T]`, and the stamp without the structure renders as an
  error naming a plugin that did not build the ref.

  `Bound(raw, embeds…)` carries `node.Constraint.Raw` as well as `Embedded`.
  `Constraint` populates only the latter, which no frontend does, and anything
  reading `Raw` as authoritative — including every derivation over Go's type-set
  form, which `Embedded` cannot express at all — sees a constraint stating no
  bound. `Builder.GoSource` now prints a raw-only constraint verbatim rather
  than as `any`, which compiled and admitted every type the author excluded.

  `Builder.File` declares a source file with its own import block, and
  `FileBuilder.ImportAs` / `Builder.ImportAs` record an explicit local name. Go
  scopes a qualifier to the file that wrote the import, so anything resolving
  one reads a `node.File`; `Builder.Import` populated only the package-level
  union, which has no aliases and no per-file scope.

- **`lang/golang.EnumFallback` pairs an enum's out-of-set conversion with the
  verb that prints it.** A generated `String` converts a value outside the
  declared set and formats the result, and the two have to agree — but nothing
  related them, so both generators in this workspace derived them separately and
  both got it wrong. `int(v)` truncates a set declared over `float64` to its
  integer part, prints as `Ratio(0)`, and neither the compiler nor `go vet`
  objects; for a set declared over another package's string type it does not
  compile at all, and the numeric verb beside it prints the wrong thing either
  way. `EnumFallback` returns the conversion as an `emit.Ref` rather than a
  name, which is what makes a cross-package underlying type render qualified and
  register its import — text cannot ask for one.

  The reference `enum` generator now reads it, so its emitted `String` is
  correct for float-backed and cross-package sets; output for an `int`-backed
  enum is byte-identical. Its `API` emit value gains `FallbackConv` and
  `FallbackVerb`, and its private `underlyingName` copy of
  `lang/golang.EnumUnderlying` is gone.

## v1.12.0 — 2026-08-10

### Breaking

- **`lang/golang.MethodSet` is removed.** It walked an interface's embeds a
  second time, beside `node.MethodSet`, with its own cycle guard and its own
  vocabulary for a failed embed — and the type-set workaround had diverged
  between the two, each catching a different half. Interface embedding has no
  shadowing and no depth rule, which is every rule the promotion code around it
  exists for, so the Go-side walk added nothing.

  Migration: `node.MethodSet(i, resolve)`, or `ctx.Reader.MethodSet(i)` for a
  caller holding the store. Its `[]InterfaceMethod` becomes
  `MethodSetResult.Entries`, carrying the same method and attribution.

### Added

- **`lang/golang`: the enum vocabulary answers for float-backed sets** (#1).
  `EnumValues` parsed every variant through `ParseIntValue` and returned false
  for the whole set on the first non-integer, so `OutOfRangeValue` returned
  nothing and the generated out-of-range probe was silently dropped — leaving a
  suite that checks every declared variant is valid and never checks that an
  undeclared one is rejected, which is the half that catches a missing
  `default:`. `EnumFloatValues`, `OutOfRangeFloat` and `ParseFloatValue` answer
  for those sets; `OutOfRangeLiteral` answers for either kind as source text,
  taking the integer reading where both parse.

  The rest of the library was already float-aware — `FormatVerb` returns `%g`
  for these types — so one half of the numeric vocabulary answered and the
  other did not, with nothing marking the difference.

- **`lang/golang`: four more funcmap bundles** (#4). `AllFuncMap` reached 37 of
  163 exported functions, so a generator needing the enum vocabulary, the shape
  matchers, the embed walks or the witness helpers could reach none of them from
  a template and grew a Go adapter per call. `EnumFuncMap`, `ShapeFuncMap`,
  `EmbedFuncMap` and `GenericsFuncMap` join the union.

  Where a signature does not suit `text/template` it travels in the shape that
  does, and the choice between them is the point: an incomplete embed walk is
  an `error` and aborts the render, because rendering a partial field set emits
  a builder short a setter. An absent zero variant is an empty value the
  template tests with `{{ if }}`, because a typed-iota set starting at one is
  the ordinary case rather than a failure.

- **`sdk.NewSink`, `sdk.NewStore` and `sdk.NewStoreReader`** (#5). The
  diagnostic and store *types* were aliased and the constructors that make them
  were not, so a test assembling a phase context by hand imported `core/diag`
  and `store` for one call apiece — leaving the façade in exactly the case it
  was written for.

- **`store.Reader.Resolve`, `PackageAt` and `FileAt`.** `lang/golang` hangs nine
  functions off a `Resolver` port it declares and cannot implement —
  `UnderlyingOf`, `ComparableDeep`, `SampleFor`, `ZeroLiteralFor`, `FieldSet`,
  `PromotedFields`, `PromotedMethods`, `ExportedFieldSet`, `EmbedsType`. Nothing
  in the tree supplied one, so a plugin holding a real graph wrote the adapter
  before it could call any of them. Pass `ctx.Reader`; `sdk/golang` asserts the
  connection, because it is the only place both packages are in scope.

  `PackageAt` and `FileAt` hit the indexes those buckets already carry, where
  filtering the full list turns a per-declaration lookup into a quadratic scan.

- **`lang/golang.SampleRefFor`, `ZeroRefFor` and `Sample`.** The string-returning
  forms composed a type with `QName` — the import path joined to the name — and
  returned `example.com/cfg.Weekday(42)`, which is not Go and registers no
  import. A spelling depends on the file the value lands in, so it travels as a
  reference. The array and struct arms both types were missing are there too.

  `SampleFor` and `ZeroLiteralFor` keep their signatures and now answer only for
  types a string can spell, which is the honest half of what they claimed.

- **`core/directive.Last`** — last-wins lookup for a repeatable directive
  carrying a value. First-wins is right for a flag and wrong for
  `+gen:default limit=10` followed by `limit=50`, which emitted 10 against a
  source that says 50, with no diagnostic because both lines are well-formed.

- **`lang/golang.IsSentinelName`**, the matcher `SentinelName`'s own docs referred
  to. A generator composing a name with one rule and finding it with another
  emits variables its own detector cannot see.

- **`plugins/annotator/shape.Plugin.Annotators` and `shape/full`.** A complete
  shape registration is three plugins; registering the umbrella alone still
  stamps, so the output looks right while every `Contract.Required` declaration
  and `Mixin.Validate` hook goes unenforced. What is lost is diagnostics, which
  is the thing whose absence looks like success.

- **`eidostest/golangtest.DriverOf` and `RenderOf`** take more than one fixture
  package, for a generator whose subject is what happens *between* packages.
  `Source.AssertContains` / `AssertNotContains` reach what no scope can — a
  comment explaining why a check was omitted belongs to no declaration's body.
  `Generated.WithRequire` builds against a real dependency.

- **`eidostest/plugintest.Annotate`, `Generate` and `GenerateWithReader`** drive
  one plugin against a store. The conformance suites answer "is this
  well-formed"; a plugin's own tests mostly ask "what did it do to this store",
  and answering it meant building a phase context by hand.
  `storefixture.Builder.Directive` attaches one to the package itself, which
  every sub-builder could already do.

### Changed

- **`lang/golang.MethodSet` is deleted; `PromotedMethods` walks through
  `node.MethodSet`.** Two walkers over one graph, each with its own cycle
  guard, its own duplicate rule and its own vocabulary for a failed embed — and
  the type-set workaround had diverged between them, each catching a different
  half. The Go-side walk added nothing: interface embedding has no shadowing
  and no depth rule, which is every rule the surrounding promotion code exists
  for. It had no callers outside this package.

### Fixed

- **`eidostest/golangtest.Driver` dropped the annotator half of a dual-role
  plugin** (#7). It registered every plugin it was given as a generator only. A
  plugin that annotates and generates satisfies `plugin.Generator` on its own,
  so it type-checked and the parameter looked honoured — while the pipeline
  iterated an annotator list the plugin was absent from, the generator then read
  metadata nothing had stamped, and the output came out short with no
  diagnostic. Every role a plugin implements is now registered, which is what
  the CLI does, so a test agrees with a real run by default.

- **`node.MethodSet` reported a type-set term as a failed embed.** An
  interface's `Embeds` list holds two unrelated things — types whose methods it
  takes on, and terms constraining its type set — and the walk asked the
  resolver to tell them apart. A resolver cannot: it answers *not loaded* for
  both a term it was never meant to see and an embed the run genuinely missed.
  So `interface{ ~[]byte }` came back as "not loaded by this run", and because
  `EmbedName` renders a slice as the empty string, stubgen reported it as
  `embeds "" which not loaded by this run`.

  The composite half is now decided from the shape, before the resolver, by the
  new `node.TypeRef.MayDenoteInterface`. A slice, map, func, array, pointer,
  anonymous struct or type parameter cannot name an interface in any language
  this model describes, so it is skipped rather than reported — it never
  claimed to contribute methods, so it cannot have failed to.

  A *named* term stays ambiguous and still reports: `int`, `error` and an
  unloaded `MyReader` are one shape here, and pretending otherwise trades a
  wrong diagnostic for a missing method.

- **`node.IsConstraint` classified `interface{ error }` as a generic
  constraint.** It keyed on `IsBuiltin`, which is true for `int` and equally
  true for `error` and `any` — the two builtins that *are* interfaces. It now
  uses the same shape predicate, so an interface embedding `error` reads as
  what it is.

  The cost is stated rather than hidden: `interface{ int | int64 }` no longer
  reads as a constraint structurally, because that shape is indistinguishable
  from `interface{ error }` without type information. A Go pipeline asks
  `lang/golang.IsConstraintInterface`, which reads the stamp the Go frontend
  already sets; `node.IsConstraint` is the fallback for a graph no Go frontend
  produced, which is what its documentation already claimed it was.

- **`lang/golang.FromNode` panicked on the nils this package manufactures.**
  Eight accessors answer nil for *not applicable* — `PointerElem` on a
  non-pointer, `IteratorElem` on a method returning no sequence, `MapKey` on a
  slice — and four lifts (`ElemType`, `MapKeyType`, `MapValType`, `FieldType`)
  read a field that may be absent. Every crossing between them was a nil
  dereference inside a generator, with no position attached.

  `FromNode(nil)` now returns nil. The frontend's non-nil guarantee covers refs
  a frontend *parsed*, not absence this package spells itself. A caller that
  branches on absence gets the nil it is testing for; one that does not gets
  `ErrUnsupportedRef` from the backend's render site, naming the file and the
  type it could not spell.

- **`reference/stubgen` and `reference/mockgen` declined nothing for a generic
  constraint.** An interface annotated `+gen:stub` that is `interface{ int |
  int64 }` was walked as a method-set contract: the resolver was asked for
  `int`, missed, and the plugin reported an embed the run did not load — telling
  the author to widen a run for a declaration that has no method set to double.
  Both now decline it through `lang/golang.IsConstraintInterface`, which reads
  the stamp the Go frontend sets, and say so.

  `mockgen` also stopped emitting an empty package when every interface in a
  source package is declined. Adding it anyway rendered a file carrying nothing
  but the generated-by header, which reads as a generator that ran and failed
  rather than one that had nothing to do.

- **`lang/golang.IsByteSlice` keyed on the literal element name**, so `[]byte`
  and `[]uint8` — one type, recorded as the author wrote it — produced two
  different builder APIs. `IsByteSliceAny` now delegates to it.

- **`plugins/generator/builder`: `defaults=3.14` emitted a reference to the
  symbol `14` in the package `3`.** The check tested for a leading or trailing
  dot; both halves are now validated as an import path and an identifier.

## v1.11.0 — 2026-08-10

### Breaking

- **`reference/stubgen`: recorded-call field names follow the framework's
  rule.** The error slot is `Err` and a lone value slot is `Result`; several
  value slots are `Result0`, `Result1`, … numbered across the value slots only.
  `StoreListCall{Result0 []string; Result1 error}` becomes `{Err error; Result
  []string}`.

  The plugin carried its own numbering, which counted every slot — so adding an
  error return to a source method renumbered the value fields beside it, and a
  consumer's assertions moved for a reason unrelated to what they assert. A test
  naming `.Result0` has to update once; it then stops moving.

- **`reference/registrygen`: slot entries carry a composed provenance id.** It
  spelled its own, `registry.<name>`, where every sibling carries
  `<kind>.<name>`. The id is what a later plugin targets to position its own
  contribution, so a plugin that had matched the old spelling no longer will.

### Added

- **`sdk` is now the whole surface a plugin names.** A plugin had to know where
  each part of the framework lived: `node` for the source model, `emit` and
  `emit/builder` for output, `core/meta` to state a fact, `core/diag` to report
  one, `core/position` to point at a line, `store` to read the graph, `plugin`
  for the phase contexts. All of it is re-exported — 195 aliases across eight
  files, source model unprefixed and emit carrying the `Emit` prefix, because
  the two fail silently when confused: an emit value built against a source
  shape never renders, and a source query against an emit shape never matches.

  Aliases throughout, so nothing that compiled against the old spelling stops.

- **`sdk/golang.FuncPrefix` folds a plugin name into a template-function
  prefix.** `text/template` accepts only identifier-shaped function names and
  panics inside `Funcs` on anything else, so a plugin named `debug-weaver` took
  the whole run down. The name is a user-visible identity that provenance and
  directive scoping key on, so it is the prefix that bends.

- **`sdk/golang.BuiltinTemplates` declares a plugin that ships none.** A plugin
  emitting only standard decls has no kind of its own for a template to resolve,
  and the backend's missing-template diagnostic would otherwise point at the one
  case that is deliberate.

- **`eidostest/storefixture.Builder.GoSource` projects a fixture into the Go
  source it describes.** Asserting that generated output compiles needs the
  hand-written package it references; supplying that separately makes the
  fixture that drove the run and the source it stands for two things that can
  disagree — silently, since a stale support file still compiles.

- **`eidostest/golangtest.Render` and `Driver` drive a fixture to its files in
  one call.** Both take the backend rather than constructing one, which keeps
  the package out of `backend/golang`'s module graph. `AssertDoesNotSatisfy`
  states what a shape detector is really claiming: not that the canonical shape
  passes, but that every near miss fails.

- **`eidostest/plugintest.AssertRenderStmt` and `AssertExternalCall`.** A
  contributor's tests read a slot entry's kind back, which `sdk` withholds on
  grounds true of a generator and false of a test of one.

- **`lang/golang.WithReceiverFromType` and `ParamIdentsFor`.** The receiver
  identifier is the type name's initial made unique against the parameter
  identifiers, an ordering a caller cannot resolve alone — one generator
  projected its signature twice to get there. `ParamIdentsFor` takes declared
  names, for a generator that lowered away its `node.Param` before it needed
  identifiers and was reaching past the uniqueness pass.

### Fixed

- **`lang/golang`: a variadic method matched a standard-library shape.** A
  frontend records a variadic parameter as its *element* type with `Variadic`
  set, so `Write(p ...[]byte)` arrives carrying exactly the `[]byte` that
  `io.Writer` wants, and every shape in `sigshape.go` read the type alone and
  answered yes — reporting a method that cannot satisfy the interface.
  `SameSignature`, in the same package, compared the flag correctly. Every shape
  there is a fixed-arity stdlib contract and none admits a `...T`, so the guard
  is on arity rather than per-shape.

- **`reference/validategen` never qualified a subject type.** `pkgPathOf`
  asserted on a `PkgPath()` method no node kind implements — every node spells
  the field `Package` — so it always answered empty, and a validator routed into
  its own package named a subject type that was not in scope there.

- **`reference/mockgen`: a parameter could shadow the receiver.** A source
  interface declaring `Do(m string)` emitted `func (m *FooMock) Do(m string)`,
  where `m.DoFunc` resolves against a string. No fixture whose parameters avoid
  the receiver letter ever reached it.

### Changed

- **Seven reference plugins dropped private copies of rules the framework
  answers**, including four rewrites of the option-or-default accessor the
  schema tag already carries, `stubgen`'s whole signature projection, and
  `shapewriter`'s restatement of `io.Writer`'s signature. `auditweaver` builds
  its contribution from the emit constructors and drops its template;
  `debugweaver` keeps the custom-kind route, so the pair documents the choice
  rather than implying one answer.

- **The reference plugins' generated output is now compiled and run.**
  `plugins/` may not import a backend, so the assertion lives in
  `cmd/eidos-reference`, which already registers every plugin and backend and is
  depended on by nothing.

## v1.10.0 — 2026-08-07

At v1.9.0 `lang/golang` was three files — `doc.go`, `golang.go`, `refconv.go` —
exporting fourteen symbols, every one of which is still present and unchanged.
Everything this release adds to that package is therefore new, not altered:
upgrading from v1.9.0 requires no change to code that compiled against it.

Some of those additions changed shape more than once before release. Those
migrations matter only to a consumer who tracked `main` between tags, and are
collected under *Migrating from an intermediate commit* at the end rather than
listed as breaking changes nobody upgrading from a release can hit.

### Breaking

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

- **`eidostest/golangtest` asserts on the Go a generator produced.** A set of
  composable helpers, not a conformance suite — what a generator should emit is
  that generator's business, and the one universal claim ("it must compile")
  still needs the hand-written package the output references, which only the
  fixture author has.

  Three layers, each usable alone:

  - **Is it valid Go?** `AssertCompiles`, `AssertVets`, and
    `AssertSatisfies(type, iface)`, which compiles `var _ Iface =
    (*Type)(nil)` beside the output. That last one catches the failures a
    generated double has that still compile — a dropped variadic marker
    declares `Print(args string)` where the interface wants `Print(args
    ...string)`, and a method promoted through an embed can go missing
    entirely. And `AssertTestsPass`, which compiles *and runs* a generated
    `_test.go`: a generator whose output is a test suite has that suite as its
    real contract, and asserting that a `t.Run` of some name exists passes just
    as well when the check is empty.
  - **Does it declare what I meant?** `AssertType`, `AssertFunc`,
    `AssertMethod(...).Signature(...)`, `AssertField`, `AssertEmbeds`,
    `AssertOrder`, `AssertPointerReceiver`, `AssertDoc`, plus `AssertSubtest` /
    `AssertNoSubtest` / `AssertParallel` for generated test suites. They exist
    for their failure messages as much as their subject matter: a missing
    substring says a substring is missing, while `AssertMethod` says the method
    exists with a different signature. They are also immune to gofmt's column
    alignment, which a substring spelling a struct field is not.
  - **What do consumers depend on?** `API()` and `AssertAPIGolden` render the
    exported surface — sorted, comment-free, bodies dropped — so a golden over
    it changes only when a consumer's code would have to. `AssertGolden`
    normalises the header's `Command:` line, which every generator keeping a
    byte golden had already written privately and identically.

  Plus `AssertImportsOnly` (a generator's imports are API: one added breaks
  every consumer whose module lacks it), `AssertGeneratedHeader` (the exact
  `DO NOT EDIT` line tooling keys on), `AssertDocumented`, `AssertFormatted`,
  and `WithGoVersion` (a template emitting a later release's syntax raises
  every consumer's Go floor). `InFunc` / `InMethod` narrow a substring check to
  one declaration, so "the method that cannot fail has no fault arm" is a
  question rather than an occurrence count over the whole file.

  Toolchain assertions shell out to `go` — seconds, not milliseconds. Build the
  module once per fixture and let the structural assertions carry the
  fine-grained work; a `Generated` caches its built module across assertions.

- **`backend/golang` reports every unrenderable emit kind before rendering
  starts.** `ErrTemplateMissing` already fired from the render site, but by then
  the run is midway through a target: it names one kind, on one file, and stops.
  A plugin that shipped no template at all — a misspelled `define`, a tree
  rooted one directory too high, a kind renamed on one side only — surfaced as a
  single confusing failure about whichever declaration sorted first, with every
  other affected kind invisible until that one was fixed and the run repeated.
  The whole set is now reported at once, each entry naming the contributing
  plugin and what to check. Only plugin-defined kinds are checked: the core
  `emit.` namespace renders its expressions and statements through dedicated
  funcmap helpers that have no template and never will.

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

### Migrating from an intermediate commit

Nothing here affects an upgrade from v1.9.0 — every symbol below arrived after
that tag. These are the shape changes between untagged commits on `main`, kept
because anyone who pinned one of them needs them.

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

- **`lang/golang`: `ComparableDeep` reports which type it could not reach.** The
  second result changes from a `known bool` to `[]UnresolvedType`, and the
  `EmbedProblem` vocabulary is renamed `ResolveProblem` — shared now by the
  embed walks and the type walk, because the reasons are the same three facts
  about the run whatever is being resolved. Its constants lose their `Embed`
  prefix: `EmbedNotLoaded` becomes `NotLoaded`, `EmbedNoResolver` becomes
  `NoResolver`, `EmbedTooDeep` becomes `TooDeep`, and `EmbedGeneric` becomes
  `GenericEmbed`.

  A caller that could only say "comparability is undetermined" left the author
  to work out which of a struct's twelve fields was the problem. Every
  unreachable field is now collected rather than only the first, because the
  author has to make all of them reachable and reporting one per run turns that
  into as many runs as there are fields.

  Migration: `if !known` becomes `if len(problems) != 0`.

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
