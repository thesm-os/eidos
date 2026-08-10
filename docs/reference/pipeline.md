# Pipeline API reference

Constructing a pipeline is the embedder's job, not a plugin's. A
plugin declares itself and is registered; this page is for the binary
that assembles them.

For the command-line surface over an already-assembled pipeline, see
[cli.md](cli.md).

## Composing a pipeline

```go
pipeline.New().
    WithFrontend(p plugin.Frontend).
    WithAnnotator(p plugin.Annotator).
    WithGenerator(p plugin.Generator).
    WithBackend(p plugin.Backend).
    WithSink(s sink.Sink).
    WithCache(c cache.Cache).
    WithDiag(s *diag.Sink).
    WithDirective(schemas ...directive.Schema).
    WithDirectivePrefix(prefix string).
    WithParallel(phases ...pipeline.Phase).
    WithPluginOptions(name string, kv map[string]string).
    WithManifestPath(path string).
    WithDryRun(dryRun bool).                                // runs every phase; skips the manifest write
    WithBrand(brand string).                                // tool identifier in header, footer, manifest
    WithPipelineID(id string).                              // manifest attribution across pipelines
    WithCommand(cmd string).                                // literal text of the "Command:" header line
    WithSourceRoot(root string).                            // prefix stripped from "Source:" header paths
    WithVerbose(v bool).
    WithOutputLayout(layout string).                        // alongside-source | centralised
    WithOutputPackage(name string).                         // pins Target.Package for every decl in scope
    WithOutputDir(dir string).                              // centralised-layout output directory
    WithOutputFilename(filename string).                    // pins Target.Filename for every decl in scope
    WithPluginOutputFilename(plugin, tag, path string).     // that pin, scoped to one plugin output
    WithProjectOutput(layout, pkg, dir string).             // project-level layout / package / dir policy
    WithPluginOutput(name, layout, pkg, dir string).        // per-plugin override of that policy
    WithPluginTagOutput(name, tag, layout, pkg, dir string). // per-(plugin, tag) refinement of it
    WithTargetSymbol(name string).                          // scope filter; matches Name or QName suffix .Name
    Build()           // (*Pipeline, error)
```

`Build` returns sentinel errors callers compare with `errors.Is`:
`ErrNoFrontend`, `ErrNoBackend`, `ErrMultipleBackends`,
`ErrDuplicatePlugin`, `ErrDuplicateProvider`, `ErrCycle`,
`ErrInvalidOptions`, `ErrDuplicateDirective`, `ErrIncompatibleEmitVersion`,
`ErrInvalidDirectivePrefix`, `ErrTemplateFuncCollision`,
`ErrInvalidOutputs`. The full list lives in `pipeline/errors.go`.

`Pipeline.Run(ctx, patterns...)` runs the configured pipeline and
returns `ErrRunHadErrors` when any plugin emitted an Error diagnostic,
or `ErrNoSink` without running a phase when no sink was configured.
`Pipeline.DryRun(ctx)` returns the resolved `*Plan` without executing.

Four sentinels belong to the Layout phase, which cannot fail a
`Build`: `ErrMissingFilenameProvider`, `ErrUnknownOutputTag`,
`ErrNoDefaultOutput`, and `ErrUnscopedMultiOutputOverride`. Layout
reports each one twice — as a positioned Error diagnostic naming the
offending decl and plugin, and as a retained wrapped error that `Run`
joins onto `ErrRunHadErrors` in its return value. So a host classifies
a routing failure with `errors.Is` on the `Run` error rather than
substring-matching the diagnostic text.

## Sinks and caches

```go
sink.NewDisk(root string) *Disk           // writes files under root/
sink.NewMemory() *Memory                  // in-memory map; for tests
sink.NewMulti(sinks ...Sink) *Multi       // fan-out
sink.NewStdout(w io.Writer) *Stdout       // single stream
```

```go
cache.NewDisk(root string) *Disk          // persistent
cache.NewNone() *None                     // disabled
```

### Frontend / backend implementations

- `frontend/golang.New()` — Go AST → node graph; populates `go.*`
  metadata keys (`go.iterValueType`, `go.elementType`, …) consumed
  by downstream annotators.
- `backend/golang.New()` — renders the emit graph to gofmt-clean Go
  source through a template-driven pipeline. Contract documented in
  [`../backend/golang.md`](../backend/golang.md).
