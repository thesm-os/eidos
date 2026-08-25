# eidos documentation

Two audiences, two paths. Pick the one that matches why you are here.

## Using eidos

You have a Go codebase and you want a tool built on eidos to generate
code from it.

| Document | Answers |
|---|---|
| [reference/cli.md](reference/cli.md) | What each command and flag does, and what each exit code means |
| [reference/config.md](reference/config.md) | Every key in `.eidos.yaml` |
| [reference/directives.md](reference/directives.md) | The `+gen:` annotations you write in your own source |
| [reference/pipeline.md](reference/pipeline.md) | Assembling a pipeline in Go — the builder API, sinks, caches |
| [explanation/determinism.md](explanation/determinism.md) | Why two runs over the same input produce byte-identical files |
| [plugin/routing.md](plugin/routing.md) | Where generated files land, and how to move them |

New here? **[tutorials/first-generated-file.md](tutorials/first-generated-file.md)**
walks the whole loop in twenty minutes — annotate a type, generate,
break it on purpose to watch the drift gate catch you, delete an
output cleanly.

| Task | How-to |
|---|---|
| Move generated files somewhere else | [how-to/route-generated-output.md](how-to/route-generated-output.md) |
| Delete output you no longer want | [how-to/remove-stale-output.md](how-to/remove-stale-output.md) |

## Extending eidos

You are writing a plugin — a frontend, annotator, generator or
backend.

Start at **[plugin/README.md](plugin/README.md)**, which sequences the
seven documents in that directory. In brief:

| Document | Answers |
|---|---|
| [plugin/quickstart.md](plugin/quickstart.md) | Write a first plugin, end to end |
| [plugin/recipes.md](plugin/recipes.md) | Patterns for each role, pointing at working reference plugins |
| [plugin/conformance.md](plugin/conformance.md) | Testing against the framework suites |
| [plugin/templates.md](plugin/templates.md) | Shipping templates and funcmaps |
| [plugin/composition.md](plugin/composition.md) | Several generators contributing to one file |
| [plugin/routing.md](plugin/routing.md) | The precedence ladder that decides output paths |
| [plugin/multi-file-output.md](plugin/multi-file-output.md) | Emitting more than one file per source |

## Language layers

Everything eidos knows about a language lives under `lang/<lang>`, one
module apiece. For Go that is four packages sharing a path, and they
are not interchangeable — start with the first row if you are unsure
which one you want.

| Document | Answers |
|---|---|
| [lang/golang/README.md](lang/golang/README.md) | Go conventions every consumer shares — and which of the four packages you may import |
| [lang/golang/frontend.md](lang/golang/frontend.md) | How Go source becomes the node graph, and every `go.*` key it stamps |
| [lang/golang/backend.md](lang/golang/backend.md) | How the emit graph becomes Go files — templates, funcmap, envelope |

`lang/protobuf` is the second language: a frontend and no backend,
because proto is read and never written.

## Elsewhere in the repo

- [README.md](../README.md) — what eidos is, and whether you want it.
- [CONTRIBUTING.md](../CONTRIBUTING.md) — working on eidos itself.
- [CHANGELOG.md](../CHANGELOG.md) — what changed.
- [adr/](adr/README.md) — why the architecture is the way it is.
- [`sdk`](../sdk/doc.go) — the plugin-author façade, including what it
  deliberately does not re-export and why.
