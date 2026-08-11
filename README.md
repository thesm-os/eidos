# eidos

[![CI](https://github.com/thesmos-ai/eidos/actions/workflows/ci.yml/badge.svg)](https://github.com/thesmos-ai/eidos/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/go.thesmos.sh/eidos.svg)](https://pkg.go.dev/go.thesmos.sh/eidos)
[![Go Report Card](https://goreportcard.com/badge/go.thesmos.sh/eidos)](https://goreportcard.com/report/go.thesmos.sh/eidos)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/github/go-mod/go-version/thesmos-ai/eidos)](go.mod)

A plugin-driven code-generation library built around typed metadata, a
queryable intermediate representation, and composable injection slots.
Output is byte-deterministic, `gofmt`-clean, and cache-friendly.

```
Source files → Frontend → Annotators → Generators → Backend → Sink
                          (stamp meta) (emit fragments) (render)
```

## What it is

`eidos` is a Go library for assembling code generators. Hosts embed it
and compose a pipeline from plugins:

- a **Frontend** parses source into a language-agnostic node graph
  (the source-side IR);
- **Annotators** detect patterns and stamp typed metadata onto nodes;
- a directive-override step lets users override any of that from
  source comments (`+gen:`, `-gen:`);
- **Generators** read the model and emit code fragments on a parallel
  output-side IR — including fragments that target named *slots* on
  other generators' output;
- a **Backend** runs the fragments through templates, resolves
  imports, formats, and writes through a configurable sink.

The supported target today is Go → Go. The pipeline, plugin contracts,
and slot model are language-agnostic; additional frontends and backends
slot in through the same plugin interfaces.

## Quality contracts

These are guarantees the library provides, tested in CI:

- **Determinism.** The same inputs produce byte-identical output
  across runs and machines. Topological ties break alphabetically;
  every output carries a SHA-256 provenance hash over its body bytes;
  the header and footer carry no run-dependent fields.
- **Composability.** Cross-cutting concerns stack into typed slots on
  generated code in deterministic order — capability-topological
  across plugins, append-sequence within a plugin. Generators don't
  become god-objects.
- **Provenance.** Every metadata entry, slot contribution, and emit
  entity carries `(setBy, authority, sourcePos)` provenance. The
  authority ladder (plugin < directive < manual) resolves conflicting
  writes deterministically.
- **Parallel safety.** Annotators with disjoint `Provides` may run
  concurrently; the Store and emit graph are race-detector clean and
  enforce mutability windows per phase. The Go backend renders its
  targets concurrently, one worker per CPU: each target gets its own
  template clone and its own `ImportSet`, and what the workers share
  they only read. Sink writes and diagnostics are replayed in target
  order afterwards, so concurrency changes nothing observable —
  output stays byte-identical run to run.
- **Caching.** The Go and protobuf frontends key their parsed node
  graph on the frontend version plus a content hash over every input
  file and the configured options, and skip re-conversion on a hit.
  For annotators, generators, and the backend the pipeline records a
  fingerprint only: a key over the plugin name, its declared version,
  its read-set hash, its resolved layout policy, and the run's scope
  filters. Nothing reads that key to skip work — it answers "did this
  plugin run against these inputs" for tooling.
- **Panic isolation.** A plugin that panics produces an `Error`
  diagnostic with a stack trace; subsequent plugins still run; the
  pipeline returns a structured error rather than a raw panic.
- **Layering.** `depguard` rules in `.golangci.yml` enforce the
  package layering: `node/` and `emit/` are language-agnostic and
  forbidden from importing language-specific stdlib (`go/ast`,
  `go/format`, `text/template`); `core/*` cannot import any specific
  frontend or backend; frontends and backends cannot import each
  other.

## Installation

```bash
go get go.thesmos.sh/eidos
```

Requires Go 1.26 or later.

## Minimal example

A working pipeline rendering one greeting struct per source struct,
alongside each source file:

```go
package main

import (
    "context"
    "fmt"

    bgolang "go.thesmos.sh/eidos/backend/golang"
    "go.thesmos.sh/eidos/eidostest/pipelinetest"
    "go.thesmos.sh/eidos/pipeline"
    "go.thesmos.sh/eidos/sdk"
    sdkgo "go.thesmos.sh/eidos/sdk/golang"
    "go.thesmos.sh/eidos/sink"
)

// helloGenerator emits one `<Source>Greeting` struct per source
// struct. The embedded base declares the output suffix, so the
// routing layer composes `<src-basename>_hello.go`; setting Origin
// on each emitted decl is what lets Layout resolve its directory,
// package and import path from the source.
type helloGenerator struct{ *sdkgo.Base }

func newHello() *helloGenerator {
    return &helloGenerator{Base: sdkgo.NewPlugin("hellogen").
        Outputs(sdk.Output{Suffix: "_hello.go"}).
        Build()}
}

func (g *helloGenerator) Generate(ctx *sdk.GeneratorContext) error {
    c := sdk.NewProvenance(g.Name())
    for _, src := range ctx.Reader.Structs().Slice() {
        pkg, err := c.Package(src.Package, src.Package).
            Struct(src.Name+"Greeting", func(s *sdk.StructBuilder) {
                s.Origin(src)
                s.Field("Message", sdk.Builtin("string"), nil)
            }).
            Build()
        if err != nil {
            return err
        }
        if err := ctx.Store.Emit().AddPackage(pkg); err != nil {
            return err
        }
    }
    return nil
}

func main() {
    src := &sdk.Package{Name: "x", Path: "x"}
    src.Structs = []*sdk.Struct{{
        Name:     "User",
        Package:  src.Path,
        BaseNode: sdk.BaseNode{SourcePos: sdk.Pos{File: "user.go", Line: 1}},
    }}
    p, err := pipeline.New().
        WithFrontend(pipelinetest.FromNodes(src)).
        WithGenerator(newHello()).
        WithBackend(bgolang.New()).
        WithSink(sink.NewDisk("./out")).
        Build()
    if err != nil {
        fmt.Println(err)
        return
    }
    if err := p.Run(context.Background()); err != nil {
        fmt.Println(err)
    }
}
```

The rendered output lands at `./out/user_hello.go`. Layout
composes the filename from the source basename (`user`) and the
plugin's declared suffix (`_hello.go`); package and directory come
from the source struct's package.

For real Go-source input, swap `pipelinetest.FromNodes(...)` for
`frontend/golang.New()` and pass package patterns to `p.Run`:

```go
p.Run(ctx, "./...")
```

Proto-source pipelines register the proto3 frontend at
`frontend/protobuf` plus the `protogo` bridge annotator at
`bridge/protogo`; the bridge stamps Go-namespaced translation
meta on proto-derived nodes so the existing Go backend renders
compilable Go without learning anything proto-specific:

```go
pipeline.New().
    WithFrontend(protobuf.New()).
    WithAnnotator(protogo.New()).
    WithBackend(backend_golang.New()).
    Build()
```

The bridge-annotator pattern generalises to any source-language
→ target-language pair: a future `protorust` / `prototypescript`
follows the same shape (annotator stamps target-language meta;
the matching target backend reads it). The framework stays
language-neutral; per-pair translation lives in the bridge
plugin alongside the frontend. See `frontend/protobuf/doc.go`
and `bridge/protogo/doc.go` for the per-package contract.

## Public API surface

Moved into the documentation tree so each audience has one home:

- **Composing a pipeline**, sinks, caches, and frontend/backend
  wiring — [docs/reference/pipeline.md](docs/reference/pipeline.md)
- **Plugin role interfaces** —
  [docs/plugin/quickstart.md](docs/plugin/quickstart.md)
- **Building emit graphs** —
  [docs/plugin/recipes.md](docs/plugin/recipes.md)
- **Cross-cutting slot contributions** —
  [docs/plugin/composition.md](docs/plugin/composition.md)

## Determinism and provenance

See [docs/explanation/determinism.md](docs/explanation/determinism.md).

## Test harnesses

See [docs/plugin/conformance.md](docs/plugin/conformance.md) for the
conformance suites, and
[docs/README.md](docs/README.md) for the rest of the tree.

## Project layout

```
node/                 source-side IR (language-agnostic)
emit/                 output-side IR (language-agnostic)
emit/builder/         fluent decl + slot-contribution API plugins use
store/                two-view (Source / Emit) store with mutability windows
writer/               file-builder primitives: ImportSet, Header, Footer
sink/                 Sink interface + Disk / Memory / Multi / Stdout
cache/                Cache interface + Disk / None
manifest/             written-output manifest for prune support
priority/             capability-topo sort helpers
plugin/               role interfaces (Frontend, Annotator, Generator, Backend)
                      plus optional capabilities (Versioned, OptionsProvider,
                      CapabilityProvider, TemplateProvider, …)
pipeline/             Builder + Pipeline orchestration

sdk/                  plugin-author façade; re-exports the contract-shaped
                      surface of plugin / priority / core so a plugin's
                      import block stays compact

core/                 language-agnostic foundation primitives:
  core/contract/        interfaces both node.* and emit.* graphs implement
  core/diag/            diagnostics (Info / Warn / Error) with positions
  core/directive/       +gen: / -gen: parsing, schemas, registry
  core/kind/            Kind discriminator shared by node / emit / directives
  core/meta/            typed metadata keys, authority levels, provenance
  core/naming/          case conversion (Pascal / Camel / Snake / Screaming / Title)
  core/opt/             typed plugin-options primitives
  core/position/        Pos / Range
  core/srcfile/         <src-basename>_<suffix> output-filename convention

lang/golang/          Go-language conventions shared by any plugin that
                      emits Go, keeping plugin cores language-agnostic

frontend/golang/      Go AST → node graph + go.* metadata
frontend/protobuf/    proto3 descriptors → node graph
backend/golang/       Go renderer: templates, funcmap, ImportSet, gofmt
bridge/protogo/       proto→Go bridge annotator: stamps Go-namespaced
                      translation meta so the Go backend stays proto-agnostic

plugins/              production plugin set (own module):
  plugins/annotator/shape/  callable-signature classification — detectors,
                            contracts, and mixins over the +gen:shape contract
  plugins/generator/        builder / enum / sentinel generators

reference/            reference plugins kept runnable as worked examples:
                      repogen, mockgen, registrygen, auditweaver,
                      debugweaver, shapewriter
cli/                  command kernels (run / plan / explain / check / prune /
                      version); no flag parsing, no CLI framework
cmd/eidos-reference/  demonstration binary wiring the in-tree plugin ensemble

docaudit/             package-doc vs implemented meta-key audit

eidostest/            test harnesses for downstream authors:
  eidostest/pipelinetest/  generic pipeline harness, golden-file diffing
  eidostest/storefixture/  typed source-graph builders
  eidostest/plugintest/    plugin-author conformance suite
  eidostest/frontendtest/  frontend-author harness (language-neutral)
  eidostest/backendtest/   backend-author emit-injection harness
  eidostest/acceptancetest/ in-tree black-box harness over the reference binary

docs/
  docs/backend/golang.md    Go-backend contract reference (template set,
                            funcmap, envelope, sentinels)
  docs/frontend/golang.md   Go-frontend contract reference
  docs/plugin/              plugin-authoring guide (quickstart, recipes,
                            composition, routing, templates, conformance)
```

Layering is enforced by `depguard` in `.golangci.yml`.

## Design decisions

Five choices shape everything else: plugins are compiled in rather than
loaded, they exchange facts through typed metadata rather than calls,
shared output composes through slots rather than inheritance, templates
are owned by backends and plugins rather than the framework, and a run
targets one backend.

Each is recorded with the alternatives it beat and what it costs in
[`docs/adr/`](docs/adr/README.md).

## Contributing

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for development workflow,
linting, and test-coverage expectations. Security issues:
[`SECURITY.md`](SECURITY.md).

## License

MIT. Copyright Thesmos B.V. See [`LICENSE`](LICENSE).
