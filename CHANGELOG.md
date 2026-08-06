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

### Added

- **`eidostest/plugintest`: `Violation`, `BrokenPlugin`, `Violations`, and
  `LyingNodesOnlyGenerator`.** Plugins that deliberately break exactly one
  framework contract. Running `RunSuite` against one and watching it fail is the
  cheapest way to confirm a conformance harness is wired up at all — the failure
  mode this guards against is a suite that is never actually invoked.

- **`eidostest/plugintest.ConformanceLanguage`.** The backend language every
  capability lookup in the suite is driven with. Exported because it is part of
  the contract: a plugin keyed on any other spelling answers none of the suite's
  probes.

- Fuzz targets across `core/directive`, `core/naming`, `core/srcfile`, `writer`,
  `store`, `pipeline`, `cli` and `frontend/golang`; benchmarks including scaling
  cases across the hot paths; and executable examples for the `eidostest`
  harnesses and `emit/builder`.

### Fixed

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
