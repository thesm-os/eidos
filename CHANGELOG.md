# Changelog

Notable changes to eidos, newest first.

The project versions each module in the `go.work` set independently, so a
release tag names one module (`eidostest/v1.5.0`) while the entry below groups
everything that shipped together. Entries call out which module a change lands
in, because upgrading one does not pull the others.

This file records what a consumer of the published API needs to know. Internal
refactors, test additions that change no contract, and documentation fixes are
omitted unless they change what a caller can rely on. One line per change — the
reasoning lives in the docblock of whatever the line names.

## Unreleased

### Breaking

- **`SourceRules.TypeParams` and `TypeArgs` take the parameter list, not a struct.** Five node kinds carry one and the rendering is identical for all five, so taking a struct answered for one and left every generator over interfaces reaching past the rules into a language package.
- **The signature projection moved to `emit` as `SigInfo`, `SigParam` and `SigReturn`.** `golang.Sig`, `Param` and `Return` are aliases, so callers are unaffected — except `Sig.ReceiverIdent()`, which is now the field beside `Name` and `Params` rather than the type's one guarded accessor.
- **`golang.SubstituteSig` takes the type parameters to bind against.** Go has no syntax for a method-level type parameter, so binding against the signature's own list was a guaranteed no-op for every interface method — the one case a generated double needs. Pass `s.TypeParams` for the old behaviour; pass the owner's to rewrite a method at its interface's witnesses.

### Breaking

- **`ids.Param` embeds `shape.Param`.** A positional `ids.Param{owner, key}` literal no longer compiles; `Key`, `Kind`, `Role` and `Required` promote, so the accessors answer a key's whole declaration rather than its spelling.

### Added

- **`lang/golang/sdk` forwards `SampleRefFor`, `BindTypeArgs` and `IsExported`, aliases `Source`, and `sdk` re-exports `ExprKind` with every variant** ([#75](https://github.com/thesm-os/eidos/issues/75)): the façade carried three of the five steps from a struct to a composed literal and produced the consumer of a `Sample` but not its producer, and a test asserting what a generator built had to import `emit` to name the kind.
- **`if-match` accepts `field=`, naming the member of the written value the predicate judges** ([#77](https://github.com/thesm-os/eidos/issues/77)). `shape.KindValueField`, the mirror of `cas`'s `version=`: the law's witness is two values that differ, and on a keyed record varying the wrong member writes a different record instead of a rejected one, so the check went quietly vacuous. Optional.
- **`golang.SamplePart`** ([#74](https://github.com/thesm-os/eidos/issues/74)) renders one sample as an element of a composite literal the caller builds — the position where a typed sample drops its conversion and a composite one is rebuilt around its Ref, a rule one consumer had hand-copied brace-trimming included — re-exported from `lang/golang/sdk`, since the `sdk.Sample` it takes arrives through the neutral `TypeRules.SamplesOf`.
- **`pointintime` accepts `write=` and `eventually` accepts `observe=`** ([#73](https://github.com/thesm-os/eidos/issues/73)). Both state a law whose witness needs a second callable and neither could name one — `pointintime` declared no parameters at all, and `eventually`'s `settle=` / `sync=` both describe reaching quiescence rather than where to look afterwards. Optional, so a subject already carrying either keeps its classification.
- **`ttl` accepts `lifetime=`, naming the member of the stored value that carries its own expiry** ([#72](https://github.com/thesm-os/eidos/issues/72)). `shape.KindValueField`, mutually exclusive with `duration=` — a lifetime is fixed by the directive or carried by the value, and a store with per-entry expiry could previously only misdescribe itself or classify nothing.
- **Every contract role is a named constant, re-exported as `ids.Contract<Name>Role<Role>`** — 23 of the 26 contract packages spelled their role vocabulary as bare strings, and two of those named part of it, which is worse: a consumer finding `cursor.RoleOpen` has every reason to expect `RoleNext`. `ids.ContractRoles` enumerates them and a test pins that every registered role has one.
- **`shape/ids` answers the catalog by name** — `ContractOf`, `MixinOf`, `DetectorOf` return the registered spec, and `ContractParam` / `MixinParam` one key's declaration; `ContractParams` and `MixinParams` now derive from the registered catalog rather than restating it, so what the package answers is what the validator enforces.
- **`sdk.SigRules`** is an optional rules interface answering `SigOf` and `IsConstraint`, so a generator that doubles a contract asks its declared rules for a signature rather than wrapping its own Source. Found by assertion, like `EnumRules` and `ErrorRules` ([#62](https://github.com/thesm-os/eidos/issues/62)).
- **`sdk.DeclarationRules`, `sdk.TypeRules` and `sdk.NamingRules`** are the three halves `SourceRules` is composed from, so a helper reading only one states that in its signature without importing `plugin`.
- **`lang/golang/sdk` forwards the Go questions no neutral interface answers** — `QName`, `LocalName`, `Deref`, `ElemType`, `FromNode`, `FuncSignature`, `IsContext` / `IsString` / `IsInteger` / `IsFloat` / `Nilable`, `ComparableDeep`, `SequenceOf`, `SentinelSubject`, `NamedReturnsUsable`, with the `ResolveProblem` and `Iterator` vocabularies their answers are read through.
- **`shape.Mixin`, `Contract` and `Detector` can be marked `Documentary`** ([#69](https://github.com/thesm-os/eidos/issues/69)), with `ids.Documentary` reading it across all three families: a classification that carries information rather than an invariant, so a consumer reporting coverage tells a gap a rule could close from a silence that is owed. `errors`, `scope` and `deprecated` are marked.
- **`sdk.LanguageReporter`** ([#68](https://github.com/thesm-os/eidos/issues/68)) warns once per language a plugin cannot read, replacing the copy each of eidos's six generators and annotators carried. The zero value is usable; the caller supplies the clause naming what it did not produce.
- **`shape.KindValueField` and `shape.KindParam`** ([#67](https://github.com/thesm-os/eidos/issues/67)): a param naming a field of the host's value type resolves against the answered value — or the written one for a host answering nothing, pointer-stripped, promotion honoured, the rule stated on the kind's docblock so consumers mirror it rather than re-derive it — and a param naming a parameter of the host validates against its signature. Eight keys retyped: `version=` on `cas`, `causal` and the four session guarantees is a value field and **now stamps qualified**; `sticky key=`, `partition axis=` and `scope axis=` are params, with `scope`'s Validate hook deleted and `partition`'s trimmed to the pair check only the hook can make. `conservative field=` stays opaque until its intent is pinned.
- **`shape.Param` can be declared `Required`** ([#66](https://github.com/thesm-os/eidos/issues/66)), for both mixins and contracts: the validator reports a host whose folded stamps hold no value for the key, scoped through `ParamsForRole` on the contract side, with an empty value counting as absent and a declaration split across lines judged whole. `orderafter` requires both its params and `cursor`'s producer arm requires `next=` through the field, replacing its hand-rolled hook — **an incomplete `orderafter` directive now goes red on upgrade**.
- **`gofixture.InterfaceBuilder.TypeSet`** ([#65](https://github.com/thesm-os/eidos/issues/65)) declares a constraint interface — the terms as embeds plus the marker the frontend derives from the union, which the model cannot carry structurally and a fixture previously stamped by reaching past the façade for the raw key.
- **`golang.StructOf` and `golang.MemberField`** ([#64](https://github.com/thesm-os/eidos/issues/64)) resolve a type reference to the struct declaring it and find an exported member by name with promotion honoured — the two steps a generator aiming emitted code at a member re-derived for itself. Both re-exported from `lang/golang/sdk`.
- **`golang.ReportMethodSet` and `golang.Consequence`** report every embed that contributed nothing to a resolved method set and say whether the result is usable, through a narrow `Reporter` port `ctx.Diag` already satisfies.
- **`golang.UnexportedName`** lowers an identifier's leading rune, completing the pair `ExportedName` opened. Rune-aware, and distinct from `naming.Camel`, which converts the whole identifier and so does not round-trip.
- **`+gen:witness T=int`** names the concrete type a generic parameter is instantiated at, so a declaration whose constraint no language can reason about still gets checks.
- **`emit.ExprIndexList`** carries a multi-argument instantiation; two nested `ExprIndex` values spell `Pair[K][V]`, which is a different expression.
- **`SourceRules.SubstituteParams` and `SubstituteRef`** answer what a type looks like at its witnesses, in the source and projected forms respectively.
- **`SourceRules.LiteralFor`** renders text as a literal of a member's type, for a value read from somewhere that already consumed the language's quoting.
- **`lang/golang/sdk` re-exports the assertion dialect's entry names**, so a plugin replacing the dialect need not import `lang/golang` beside the façade.
- **`needsDiffHelper`** lets a replaced dialect drop the generated file's comparison helper and the imports it alone needed.
- **`assertDeepEqual` joins the Go assertion dialect** — the structural counterpart to `assertEqual`, taking its comparison callee as an argument so the import registers.
- **`shape/detectors.All` is checked against the directory tree**, like its `contracts` and `mixins` siblings; a detector package missing from the list shipped and never ran.
- **`lang/typescript`** is the TypeScript language adapter: the conventions package with the `ts.*` metadata vocabulary, a tree-sitter frontend, a canonical-template backend, and a plugin sdk whose `Support` / `Builtin` / `Reads` mirror Go's.
- **`typescript.Source` answers the optional rule sets** — `EnumRules`, `SigRules` and `ErrorRules` — so a generator that asks by assertion works over TypeScript declarations rather than generating its degraded form.
- **`lang/typescript/typescripttest` and `tsfixture`** are the TypeScript counterparts to `golangtest` / `gofixture`: a store fixture with a `TSSource` projection, structural assertions over generated output, `tsc`-backed type-check and satisfaction assertions, and a `node --test` gate for generated suites.
- **`EIDOS_TYPESCRIPT_TOOLCHAIN` turns `typescripttest`'s tsc and Node skips into failures**, so a job that installed the toolchain reports a broken install rather than quietly checking nothing.
- **`node.Interface` and `emit.Interface` carry a field list** (ADR-0008): a TypeScript interface declares properties alongside methods, with `FieldByName` / `FieldsWith` on both sides and a `FieldsSlot` on emit. Go frontends never populate it.
- **`emit/builder.InterfaceBuilder.Field`** declares a property on an emit interface — the model, walk and slots carried fields while the fluent builder could not spell one.
- **The TypeScript backend renders the declaration-level `ts.*` vocabulary**: visibility, `static`, `abstract`, accessors, optional methods, overload signatures (in place of the derived one), index and construct signatures, `const enum`, type-parameter defaults, and initialisers on variables and constants — an initialised binding drops `declare`.
- **The reference binary reads TypeScript**: the frontend joined `defaultPlugins`; a TypeScript-targeting binary swaps the backend for the TypeScript one, which `TestTypeScriptTargetE2E` demonstrates.

### Changed

- **The Go backend builds its static funcmap once rather than per output file** — 22% off render time and 36% off the bytes it allocates, measured on `BenchmarkBackend_Render`.
- **TypeScript classes render as `export declare class`** — the backend renders no bodies, and a bodiless method in a plain class is TS2391; `async` is no longer spelled on methods, since it is illegal on a declaration and the Promise return type is the contract.
- **A TypeScript frontend registered beside other frontends treats a tree with no TypeScript as silence** rather than an error, matching the protobuf frontend; `ErrNoMatch` still reaches direct loader callers.
- **Generated code no longer carries the generator's reasoning.** It moved into the templates; four comments stay in the sentinel checks, each explaining a check that is deliberately absent.
- **Exported generated API carries docblocks that say what the signature does not** — what a second `Build` returns, what a replacing setter discards, what an entry setter does with a key already present.
- **`SourceRules` no longer answers `WitnessArgs`.** It rendered the instantiation as text, which registered no import for a witness naming another package; compose from `Witnesses` instead.

### Fixed

- **The Go backend no longer reports a used import as unused when a declaration shares the package name** ([#76](https://github.com/thesm-os/eidos/issues/76)). A qualifier in a type position — a struct field's type, a parameter's, a result's — is proof the import is live, since no local can stand there; `func Commit(ctx context.Context, tx tx.Tx)` warned on every run.
- **The `plugins` depguard rule denies a frontend or backend again.** It named `eidos/frontend` and `eidos/backend`, paths that stopped existing at ADR-0007, so the rule barring a plugin from importing a backend matched nothing; now spelled per language like its `frontends` / `backends` siblings.
- **`golang.ComparableDeep` answers for curated standard-library types** ([#71](https://github.com/thesm-os/eidos/issues/71)). The resolver never holds the standard library, so a struct with a `time.Duration` field came back undetermined and every comparison it was party to was refused; `time.Duration`, `time.Time`, `strings.Builder` and `bytes.Buffer` now answer, on the same curated terms `stdlibSamples` already uses.
- **A directive denying keys still takes `out=` / `pkg=` / `tag=`.** The routing widening reached `AllowedKeys` and not `DenyKeys`, so a generator declaring no options of its own had the framework's overrides reported as "accepts no keys" and honoured by the router anyway.
- **A string default declared in a struct tag renders as a string** ([#59](https://github.com/thesm-os/eidos/issues/59)). Go's tag grammar eats one layer of quoting, so `default:"localhost"` was stamped verbatim and named an identifier nobody declared.
- **A tag default may still name a symbol** ([#60](https://github.com/thesm-os/eidos/issues/60)). `LiteralFor` takes the declaring file: a qualifier the import block binds stays a reference, as does a full import path whose symbol is exported, and everything else on a textual member is quoted.
- **A directive value can name a stdlib package** ([#58](https://github.com/thesm-os/eidos/issues/58)). The two notations were told apart by a slash before the last dot, which no single-segment path has; the import block decides now, with the path form as the fallback.
- **A generic declaration's builder gets checks with bodies in them** ([#57](https://github.com/thesm-os/eidos/issues/57)). The file was emitted for having witnesses and every subtest then dropped for having type parameters, so it compiled, passed, and asserted nothing.
- **The builder generator's checks compile for a member of any type** ([#54](https://github.com/thesm-os/eidos/issues/54)). Comparison goes through `github.com/google/go-cmp` against a sample bound at the member's type — **a project running this generator takes a go-cmp dependency**.
- **`shape` and `defaults` read a package nothing marked, like every other plugin already did.** Both dispatched on the marker alone, so a synthesised graph, a bridge or a fixture had its stamps silently withheld while `sample`, `witness`, `builder`, `enum` and `sentinel` read it — `SourceOf` falls back to the sole registered language, and `shape`, which declares none because it renders nothing, now falls back to the one language its detectors answer for. A package marked with a language neither covers is still skipped.
- **`gofixture` marks the packages it builds as Go** ([#63](https://github.com/thesm-os/eidos/issues/63)). Nothing stamped `MetaFrontend`, so every plugin dispatching per package skipped a fixture without a diagnostic and its assertions passed having looked at an empty sink; thirteen call sites re-added the marker by hand. `Builder.Language` names another frontend and `Builder.Unmarked` suppresses it for a test whose subject is the skip.
- **The witness annotator stamps interfaces and aliases, not just structs** ([#61](https://github.com/thesm-os/eidos/issues/61)). Its schema admitted all three, so `+gen:witness` on a generic interface validated, stamped nothing, reported nothing, and left every check withheld — which is every witnessable declaration a generator that doubles a contract sees.
- **`stubgen` and `mockgen` no longer emit a double short an embedded method.** Both reported the unresolved embed and generated anyway; the pipeline carries every phase to completion, so the backend rendered the short file to disk with a non-zero exit as the only sign.
- **The Go assertion dialect emits source that parses whatever it is given.** `assertEqual` composed into an `if` header, so an operand carrying a composite literal produced a file the toolchain rejected before compiling it.

## v1.15.1 — 2026-08-25

### Breaking

- **The Go fixture and Go assertions move to `lang/golang`.** `eidostest/storefixture` becomes `lang/golang/golangtest/gofixture` and `eidostest/golangtest` becomes `lang/golang/golangtest`, breaking a module cycle that resolved to an untagged version for third-party consumers.
- **Language support is grouped by language, one module apiece.** `frontend/golang`, `backend/golang` and `sdk/golang` move under `lang/golang/{frontend,backend,sdk}`, `frontend/protobuf` under `lang/protobuf/frontend`; the old module paths keep their tags and receive nothing further. See ADR-0007.
- **A plugin declares what it emits per language.** `sdk.NewPlugin(name)` and `sdk.Base` replace the `sdkgo` constructors; templates, outputs and funcmaps move into a `sdk.LanguageSupport` bundle registered with `For(lang, …)`.
- **Template helpers register under the names they are declared with.** The per-plugin funcmap prefix, `FuncPrefix` and the `prefix` parameter on every `lang/golang` funcmap constructor are gone.
- **A param declares its own kind: `Params` is now `[]shape.Param`.** `SiblingParams` and `SiblingVars` are gone and each key carries a `Kind` — `KindOpaque` (the zero), `KindCallable`, `KindVar` or `KindMember`.

### Added

- **A `shape/ids` package re-exporting every catalog name** — 22 detector, 26 contract and 58 mixin names as untyped constants, pinned against the three aggregators by test.
- **A `deleter` detector.** `Delete(ctx, key) error` and `Put(ctx, v) error` are one signature, so a removal classified as a `writer` and derived a write-then-read-back check asserting the reverse of correct behaviour.
- **A `closer` shape** — the bare `Close() error` teardown, which fell to `poisonaccessor` and reddened under a read-purity law. Name-gated, deliberately.
- **An `answeringwriter` shape** — `(ctx, V) (V, error)`, which had no classification and fell to `reader`, recording the written value as a key.
- **A `serializable` mixin, distinct from `snapshotisolation`**, which permits write skew by definition and so cannot state the stronger claim.
- **An `accumulates` mixin** ([#44](https://github.com/thesm-os/eidos/issues/44)) — the second position on the effect axis beside `idempotent`.
- **An `indexed by=Len` mixin** naming what sizes a callable's integer parameters, so `Less(i, j int)` is not handed 42 against a five-element slice.
- **A `poisonable induce=` mixin** naming the operation that induces a sticky failure, which no signature can reveal.
- **A `notfound sentinel=` mixin** ([#41](https://github.com/thesm-os/eidos/issues/41)) — the plain reader's own miss sentinel, previously spellable only by misusing `ttl`.
- **Twenty mixins covering properties a signature cannot reveal** — `associative`, `causal`, `commutative`, `conservative`, `defaultonerror`, `injectionsafe`, `leakfree`, `monotonicreads`, `monotonicwrites`, `overmatch`, `permutation`, `snapshotisolation`, `stableorder`, `sticky`, `tamperevident`, `timeaware`, `total`, `windowed`, `writesfollowreads`, `xsssafe`.
- **Four laws that had no classification at all now have one** — `readyourwrites`, `noduplicates`, `pointintime` and `scheduled schedule= fired=`.
- **Two contracts for pairs the vocabulary could not express: `codec` and `chain`**, each declaring the callable that completes the round trip.
- **`partition axis=`** names the parameter to vary while the rest are held fixed; without it a check varying two same-typed strings passes against an implementation ignoring partitions entirely.
- **Five relational mixins can name the callable a check observes through** — `atomic`, `crdtmerge`, `serializable`, `snapshotisolation` and `injectionsafe` gain `read=`, and `crdtmerge` also `write=`.
- **Seven more classifications can reach the second method their law calls** — `leakfree`, `tamperevident`, `windowed`, `eventually`, `streamreflectsmutations`, and a `redeliver` role on `publisher`.
- **Seven classifications can name the sentinel their law compares against**, and `SiblingVars` resolves a package-level var; without it the law compared against nil and failed every subject.
- **Five classifications can name the bound their claim turns on** — `bounded min=`, `windowed window=`, `lease timeout=`, `ttl duration=`.
- **The four session-guarantee mixins take `version=`**, naming the field carrying the ordering a replayed trace reads.
- **`causal` takes `version=`**, the same ordering stamp.
- **`attempts=` on `retrysucceeds` and `axis=` on `scope`** ([#43](https://github.com/thesm-os/eidos/issues/43)); `scope`'s axis is validated documentation, since varying it alone writes twice without observing either scope ([#46](https://github.com/thesm-os/eidos/issues/46)).
- **`if-absent conflict=` and `orderafter unready=`** name the error their refusal reports, without which the check asserts only that some error came back.
- **`if-match match=`** names the predicate as a resolvable callable; the opaque `pred=` is unchanged.
- **`publisher mode=`** names the delivery guarantee — `at-least-once`, `at-most-once` or `exactly-once`.
- **A `Role` on `shape.Param`**, so one key can mean two things across a contract's arms, unblocking the `open` producer arm on `cursor`.
- **A `KindMember` resolver scope** for a param naming a method on the handle a role's callable returns.
- **`Params` on `shape.ContractMember`**, mirroring `MixinAttachment.Params`, so a `Validate` hook need not re-walk.
- **An optional `stats` role on `pool`** and an optional `reader` role on `batch-writer` ([#45](https://github.com/thesm-os/eidos/issues/45)), each naming the observation their claim is checked through.
- **`DenyKeys()` on a directive schema**, for a directive whose contract is that it takes no arguments.
- **A `renderSample` template function on the Go backend** ([#50](https://github.com/thesm-os/eidos/issues/50)), replacing the four-arm dispatch every consumer hand-wrote.
- **`Sample` carries expressions, so func and channel parameters sample** ([#47](https://github.com/thesm-os/eidos/issues/47)) — a func as the no-op literal, a channel as `make(chan T)`.
- **`lang/golang` samples `time.Duration` without a resolver** ([#42](https://github.com/thesm-os/eidos/issues/42)) from a curated table, since the standard library is never loaded.
- **`run` reports outputs a previous run no longer produces**, scoped to packages the run loaded. Removal is still `prune`, deliberately.
- **The two catalog aggregators are pinned against the tree**, so a mixin or contract package left out of `All()` fails rather than shipping absent from every pipeline.
- **`mixintest.RunWithValidator`** drives the umbrella, resolver and validator over a package for a mixin carrying a `Validate` hook.

### Fixed

- **The package doc renders into one file per package, not every file** ([#53](https://github.com/thesm-os/eidos/issues/53)) — 131 packages in one consumer's corpus declared a package comment they do not own.
- **The `Plugins:` header names a plugin that only contributes into another node's slots** ([#52](https://github.com/thesm-os/eidos/issues/52)); the walk stopped at whoever appended a node and never asked what it carried.
- **The stale-output warning and `prune` both check the disk before claiming a file is there** ([#51](https://github.com/thesm-os/eidos/issues/51)); absent entries report `already gone:` and carry their own count.
- **The manifest converges instead of accumulating claims** ([#51](https://github.com/thesm-os/eidos/issues/51)) — an in-scope entry with no file behind it is dropped, gated on verified absence.
- **Struct-form inners compose only where Go permits type elision** ([#49](https://github.com/thesm-os/eidos/issues/49)); a struct-typed field now spells the inner's type and registers its import.
- **Composite samples no longer interpolate an expression-form inner as empty text** ([#48](https://github.com/thesm-os/eidos/issues/48)), which rendered `{CreatedAt: }` for a struct whose first samplable field is a `time.Time`.
- **The two core directives declare the keys they read**, so a misspelled `plugn=` on `+gen:out` is reported rather than routing nothing silently.
- **A mistyped mixin parameter key is reported instead of stamped** into a namespace nobody reads, un-arming exactly one check while the run stays green.
- **A mistyped contract name is reported instead of silently skipped**, which dropped a whole law family with the callable still listed as classified.
- **`run` reports orphaned test outputs**, whose `<pkg>_test` import path no source package declares, so the lookup missed for the whole class.

## plugins/v1.14.0 — 2026-08-11

### Added

- **A relational shape mixin can name its second callable.** Seven mixins gain an exported param constant, so a bare name is rewritten into a qualified one instead of reaching the consumer with no package and no owner.

### Fixed

- **`readafterwrite` participates in the sibling resolution its documentation describes.** It exported `ParamWrite` and omitted `SiblingParams`, so the rewrite never ran and a documented `Validate` invariant could not hold.

## v1.14.0 — 2026-08-11

### Added

- **`node.Embed` carries the method set of an interface the run did not load**, so an interface embedding `io.Closer` can be generated for; a loaded declaration always wins.
- **`lang/golang.Sample` reports why it derived nothing.** Twelve refusal sites answered with one empty `Sample`, and only one was a fact about the type.
- **`lang/golang.SampleRefFor` handles slices and maps**, both of which fell through to the named-type bail.
- **`sdk` re-exports the ten sentinels its own surface raises**, plus `ReflectOptions`, `LookupKey`, `ParseAuthority`, `AnyKey` and `Observer`.
- **`sdk/golang.Language`**, so a plugin implementing `Outputs` or `Templates` itself need not import `lang/golang` to name what it dispatches on.
- **Architecture decision records** `docs/adr/0002`–`0006`, recording five choices `README.md` stated as conclusions.

### Fixed

- **`eidostest/storefixture.GoSource` keeps the imports an initialiser references**; an initialiser is opaque text the type walk cannot see, so a sentinel using `errors.New` projected over an empty import block.
- **`eidostest/plugintest` runs the template-resolve check for every plugin**, against the funcmap a backend actually provides rather than the reserved names alone.
- **`lang/golang.ImportAlias` and `PackageClauseFor` drop unreachable guards** against conditions `naming.Identifier` documents it never produces.

## v1.13.0 — 2026-08-10

### Added

- **`eidostest/storefixture` gains the three enum hooks a fixture could not spell** — `EnumBuilder.Variant`'s callback with `VariantBuilder`, `EnumBuilder.Method`, and `Builder.PackageName`.
- **`eidostest/storefixture` can build the three shapes a Go frontend produces and the fixture could not spell** — channels the way the frontend records them, `Bound(raw, …)` carrying `Constraint.Raw`, and `Builder.File` with a per-file import block.
- **The shape mixins name their KV parameters** as exported constants, so a consumer reaches for a key rather than `Params[0]`.
- **`pipeline.Builder.WithPlugins` registers each plugin under every role it implements** — the dispatch the CLI performed and nothing else did.
- **`plugintest.RunSetSuite` asserts a whole plugin set, not one plugin** — two plugins with one name, two declaring one directive schema, two providing one capability.
- **`plugintest.AssertTemplateFuncsResolve`** catches a template calling a function nobody registers, reading the call sites from the parser rather than a hand-maintained list.
- **`store.PendingOfType` / `PendingByOrigin`**, re-exported through `sdk`, filter queued origin-slot contributions by emit kind without copying the pending list.
- **`lang/golang.SequenceOf`** answers a method's range-over-func return in one call, replacing the nil guard every generator wrote around the four existing accessors.
- **`lang/golang.SentinelSubject`** decomposes `Err<Subject>`; the hand-rolled `TrimPrefix` turned `Errors` into `ors`.
- **`lang/golang.IsWellFormedLiteral` + `ErrMalformedLiteral`** validate text stamped into source as a value expression — deliberately shallow, refusing only what the toolchain cannot parse.
- **`lang/golang.ForeignVariants`** reports the packages declaring constants of an enum's type outside its own, which never reach `Variants` and make every generated answer about the set confidently false.
- **`lang/golang` resolves a source-level qualifier against the file that wrote the import** — `QualifierOf`, `ImportForQualifier`, `FileOf`, `ResolveQualified`. `RefForQualified` splits on the last dot, which is wrong for text read out of source.
- **`lang/golang.EnumFallback`** pairs an enum's out-of-set conversion with the verb that prints it; derived separately, both generators in this workspace got it wrong.

## v1.12.0 — 2026-08-10

### Breaking

- **`lang/golang.MethodSet` is removed** — a second walk beside `node.MethodSet` whose type-set workaround had diverged. Migrate to `node.MethodSet(i, resolve)` or `ctx.Reader.MethodSet(i)`.

### Added

- **The enum vocabulary answers for float-backed sets** ([#1](https://github.com/thesm-os/eidos/issues/1)) — `EnumFloatValues`, `OutOfRangeFloat`, `ParseFloatValue`, `OutOfRangeLiteral`; the out-of-range probe was silently dropped for those sets.
- **Four more `lang/golang` funcmap bundles** ([#4](https://github.com/thesm-os/eidos/issues/4)) — `EnumFuncMap`, `ShapeFuncMap`, `EmbedFuncMap`, `GenericsFuncMap`. `AllFuncMap` reached 37 of 163 exported functions.
- **`sdk.NewSink`, `sdk.NewStore` and `sdk.NewStoreReader`** ([#5](https://github.com/thesm-os/eidos/issues/5)), the constructors for types the façade already aliased.
- **`store.Reader.Resolve`, `PackageAt` and `FileAt`** — nothing in the tree supplied the `Resolver` port `lang/golang` hangs nine functions off.
- **`lang/golang.SampleRefFor`, `ZeroRefFor` and `Sample`.** The string forms returned `example.com/cfg.Weekday(42)`, which is not Go and registers no import.
- **`core/directive.Last`** — last-wins lookup, right for `+gen:default limit=10` followed by `limit=50`, which emitted 10 with no diagnostic.
- **`lang/golang.IsSentinelName`**, the matcher `SentinelName`'s own docs referred to.
- **`plugins/annotator/shape.Plugin.Annotators` and `shape/full`.** Registering the umbrella alone still stamps, so the output looks right while every `Required` and `Validate` goes unenforced.
- **`eidostest/golangtest.DriverOf` and `RenderOf`** take more than one fixture package, plus `Source.AssertContains` and `Generated.WithRequire`.
- **`eidostest/plugintest.Annotate`, `Generate` and `GenerateWithReader`** drive one plugin against a store, and `storefixture.Builder.Directive` attaches one to the package itself.

### Changed

- **`PromotedMethods` walks through `node.MethodSet`**, removing the second cycle guard and duplicate rule.

### Fixed

- **`eidostest/golangtest.Driver` dropped the annotator half of a dual-role plugin** ([#7](https://github.com/thesm-os/eidos/issues/7)); it type-checked, and the generator then read metadata nothing had stamped.
- **`node.MethodSet` reported a type-set term as a failed embed.** The composite half is decided from the shape now, before the resolver, by `node.TypeRef.MayDenoteInterface`.
- **`node.IsConstraint` classified `interface{ error }` as a generic constraint**, keying on `IsBuiltin`, which is true for the two builtins that are interfaces.
- **`lang/golang.FromNode` panicked on the nils this package manufactures.** `FromNode(nil)` returns nil; a caller that does not branch gets `ErrUnsupportedRef` naming the file.
- **`reference/stubgen` and `reference/mockgen` declined nothing for a generic constraint**, telling the author to widen a run for a declaration with no method set.
- **`lang/golang.IsByteSlice` keyed on the literal element name**, so `[]byte` and `[]uint8` produced two different builder APIs.
- **`plugins/generator/builder`: `defaults=3.14` emitted a reference to the symbol `14` in the package `3`.** Both halves are validated now.

## v1.11.0 — 2026-08-10

### Breaking

- **`reference/stubgen`: recorded-call field names follow the framework's rule** — `Err` for the error slot, `Result` for a lone value, `Result0`/`Result1` numbered across value slots only.
- **`reference/registrygen`: slot entries carry a composed `<kind>.<name>` provenance id**, so a plugin matching the old `registry.<name>` spelling no longer will.

### Added

- **`sdk` is now the whole surface a plugin names** — 195 aliases across eight files, source model unprefixed and emit carrying the `Emit` prefix, because the two fail silently when confused.
- **`sdk/golang.FuncPrefix`** folds a plugin name into a template-function prefix; `text/template` panics inside `Funcs` on a non-identifier, so a plugin named `debug-weaver` took the run down.
- **`sdk/golang.BuiltinTemplates`** declares a plugin that ships none, so the missing-template diagnostic does not fire on the deliberate case.
- **`eidostest/storefixture.Builder.GoSource`** projects a fixture into the Go source it describes, so the fixture and its support file cannot disagree.
- **`eidostest/golangtest.Render` and `Driver`** drive a fixture to its files in one call, plus `AssertDoesNotSatisfy` for what a shape detector really claims.
- **`eidostest/plugintest.AssertRenderStmt` and `AssertExternalCall`**, which read a slot entry's kind back.
- **`lang/golang.WithReceiverFromType` and `ParamIdentsFor`** — the receiver identifier is the type name's initial made unique against the parameter identifiers, an ordering a caller cannot resolve alone.

### Changed

- **Seven reference plugins dropped private copies of rules the framework answers**, including four rewrites of the option-or-default accessor and `stubgen`'s whole signature projection.
- **The reference plugins' generated output is now compiled and run**, from `cmd/eidos-reference`, since `plugins/` may not import a backend.

### Fixed

- **`lang/golang`: a variadic method matched a standard-library shape.** A frontend records the element type with `Variadic` set, so `Write(p ...[]byte)` arrived carrying exactly the `[]byte` `io.Writer` wants.
- **`reference/validategen` never qualified a subject type.** `pkgPathOf` asserted on a `PkgPath()` method no node kind implements.
- **`reference/mockgen`: a parameter could shadow the receiver**, emitting `func (m *FooMock) Do(m string)`.

## v1.10.0 — 2026-08-07

At v1.9.0 `lang/golang` was three files exporting fourteen symbols, every one of
which is still present and unchanged — so upgrading from v1.9.0 requires no
change to code that compiled against it. Shape changes between untagged commits
are collected under *Migrating from an intermediate commit*.

### Breaking

- **`eidostest/plugintest`: `RunSuite` gained checks that existing plugins can fail** — fresh-slice declaration accessors, templates that parse and claim no reserved name, a truthful `NodesOnly`, and two `Outputs` checks that previously probed languages no plugin keys on.
- **`lang/golang`'s type-parameter helpers are keyed on the parameters, not the declaration** — `TypeParamsOf`, `IsGeneric`, `TypeParamDecls`, `TypeParamNames`, `TypeParamRefs`, `SelfRef`; the struct-shaped entry points remain and delegate.
- **`lang/golang` gained the Go identifier rules `core/naming` defers to it** — `IsKeyword`, `IsPredeclared`, `SafeIdent`, `UniqueIdent`, `ReceiverIdent`, `PackageName`, `IsInternal`; a field called `type` previously emitted code that did not compile.
- **`lang/golang` gained typed readers for the whole `go.*` vocabulary** — 22 keys were declared with no accessor, so every consumer decided independently what an absent stamp meant.
- **`bridge/protogo.GoFieldName` is deprecated** in favour of `naming.Pascal`, which it duplicated.
- **`emit/builder.For` no longer takes a target.** No production caller ever passed a non-zero value; drop the second argument.
- **`store.EmitView` gained `AppendOrigin` and `AppendOriginAs`, and `emit` gained `PrimaryPackage`** — the four lines every generator ends with, written once.
- **`node`: interface method sets resolve through embeds** via `node.MethodSet` and `store.Reader.MethodSet`. An embed contributing nothing is reported, not dropped. Also `node.Declares`, `MethodByName`, `PointerReceiver`, `FieldOfType`, `EmbedName`, `LocalName`, `IsExportedName`, `IsConstraint`, `store.Reader.Implementers`.
- **`reference/stubgen` and `reference/mockgen` now double embedded methods**, so the generated double satisfies the interface it doubles.
- **`pipeline.Build` rejects a plugin implementing part of a multi-method optional capability**, returning `ErrPartialCapability`; declaring none of them is unaffected.
- **`plugin`: `TemplateProvider` and `CapabilityProvider` are composed from single-method interfaces**, so the detection can distinguish "declared none" from "declared some". Adds `plugin.Capabilities()` and `plugin.Gaps(p)`.
- **`plugintest`: the `CapabilityProvider` check is renamed `optional capabilities are implemented in full or not at all`** and now covers `TemplateProvider`.
- **`plugin.FrontendContext` gained a `Fingerprint` field.** Additive, but a frontend caching a node graph **must** fold it into the cache key.
- **`plugintest.RunFrontendSuite` fails a fixture the frontend rejects**, drives both determinism passes against a warm `cache.Disk`, and gains `FrontendFixture.ExpectsEmpty` and `Fingerprint`.
- **`plugintest`'s generator, annotator and frontend suites read the diagnostic sink they hand the plugin**, so a plugin reporting an Error on every input no longer passes every subtest. Adds `AllowsPositionlessDiagnostics`.

### Added

- **`eidostest/golangtest` asserts on the Go a generator produced** — three composable layers: is it valid Go (`AssertCompiles`, `AssertVets`, `AssertSatisfies`, `AssertTestsPass`), does it declare what I meant (`AssertType`, `AssertMethod(...).Signature(...)`, `AssertSubtest`, …), and what do consumers depend on (`API`, `AssertAPIGolden`, `AssertImportsOnly`).
- **`backend/golang` reports every unrenderable emit kind before rendering starts**, rather than one kind on one file midway through a target.
- **`sdk/golang` is the plugin base every Go-generating plugin embeds.** `NewGenerator` takes the three things a generator cannot omit and chains the rest; the set it replaces had sixteen copies of the language dispatch, two testing against the wrong constant.
- **`lang/golang.MethodSet` flattens an interface's method set through its embeds**, embedded methods first, depth-first in source order.
- **`emit/builder.Queue` and `QueueAs`** append a plugin's emit values to an origin's slot through a new `Appender` port.
- **`sdk` re-exports the emit-queueing helpers** — `EmitBase`, `EmitBaseTagged`, `QueueEmit`, `QueueEmitAs`, `PrimaryPackage`, `EmitAppender`.
- **`lang/golang` is now the complete Go vocabulary a generator needs** — signature queries, well-known method shapes, the callable projection, emit construction, list rendering, references, import paths, numeric facts, values, instantiation, conventions, template bundles, embedding, interface satisfaction, enums and type expressions, each previously answered privately and differently by two or more consumers.
- **Every shipped plugin now declares `plugin.Versioned`.** Seventeen were missing it, contributing `name@""`, so changing their behaviour could never invalidate a warm cache.
- **`emit.StmtRender` and `NewRenderStmt`**, so a plugin can contribute into a statement slot while rendering through its own template.

### Changed

- **`lang/golang` gained the zero-value and struct-tag spellings.** `ZeroLiteral` reports whether a zero is derivable at all — a partial private copy answered `nil` for the widths it omitted, rendering `Code: nil` for an `int8`.
- **`lang/golang` gained the signature rules three generators had each written a slice of** — `ParamIdent`, `ParamIdents`, `ErrorSlot`, `ReturnsError`, `NamedReturnsUsable`.
- **`eidostest/plugintest` ships plugins that deliberately break one contract each** — `Violation`, `BrokenPlugin`, `Violations`, `LyingNodesOnlyGenerator`, `ErroringGenerator`, `ErroringAnnotator`, `ErroringFrontend` — so a harness that is never invoked is detectable.
- **`eidostest/plugintest.ConformanceLanguage`** is exported, because a plugin keyed on any other spelling answers none of the suite's probes.
- **Fuzz targets, scaling benchmarks and executable examples** across `core/directive`, `core/naming`, `core/srcfile`, `writer`, `store`, `pipeline`, `cli` and `frontend/golang`.
- **The `bench:` gate is removed from `.ergon.yaml`**; it thresholded against a baseline file that was never created. `make test-bench` still runs them.

### Fixed

- **`plugins/generator/sentinel` recognised no custom error type at all.** The signature predicates read a return's binding name after the model gained named return slots, and every `Error() string` in the wild is anonymous.
- **`plugins/generator/sentinel` wrote `nil` into typed struct fields.** Fields whose only derivable zero is nil are dropped from the rendered checks rather than emitted into a file that does not compile.
- **`frontend/protobuf`: a cold run and a cached run produced different graphs.** `convertFiles` never wired the owner back-pointers the cache-hit path rewires explicitly.
- **`emit`: a reserved slot's element kind depended on which accessor reached it first.** It is a property of the slot name now; a foreign kind fails at `Append` with `ErrSlotElementType`.
- **`pipeline`: a `+gen:out` directive could write outside the source tree.** `..` segments survived the strip and cancelled components of the origin's own directory.
- **`writer`: `ImportSet.Imp` returned the same-package elision sentinel for a foreign package** whose path picked up a trailing slash, producing a reference to an undefined bare symbol.
- **`writer`: `ImportSet.Imp` was quadratic in paths sharing a last segment** — 1000 colliding imports cost 31ms and 1.27M allocations, now 256µs and 5.5K.
- **`frontend/golang`, `frontend/protobuf`: a warm cache served a graph parsed by an older frontend, indefinitely.** Keys now carry the composition fingerprint and the module version; upgrading re-parses, deliberately.
- **`.gitignore` discarded every fuzz reproducer.** A bare `fuzz/` rule matched `testdata/fuzz/`, so a crasher was dropped before it could be committed.

### Migrating from an intermediate commit

- **`lang/golang` predicates answer as a union** — the `go.*` stamp where a frontend supplied one, the Go spelling otherwise, so `IsError` works on a graph no Go frontend produced.
- **The embed walks report which embeds failed, not whether any did.** `[]UnresolvedEmbed` replaces the `bool`; `if !complete` becomes `if len(problems) != 0`. A generic embed is now refused rather than promoted, and `PromotedMethods` applies Go's promotion rules in full.
- **`ComparableDeep` reports which type it could not reach.** `[]UnresolvedType` replaces `known bool`, and `EmbedProblem` is renamed `ResolveProblem` with its constants losing the `Embed` prefix.

### Known limitations

- **`core/naming` case conversion is idempotent over ASCII letters and separators, not universally.** Every style reaches a fixed point by the second application; fixing it would change generated identifiers.
- **Branch coverage cannot be measured for nine of the ten modules.** gobco copies a package to a temporary directory, where the relative `replace` does not resolve.
- **The pipeline-level cache is write-only.** Frontends implement real skip-on-hit; the per-plugin key is a fingerprint for tooling and is never read to skip work.
