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
