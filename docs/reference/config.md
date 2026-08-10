# Configuration reference

The config file is `.<brand>.yaml` — `.eidos.yaml` for the reference
binary. It is discovered by walking up from the working directory, or
named explicitly with `-config`.

Everything in it is optional. A project with no config file runs every
registered plugin over `./...` and writes alongside the source.

```yaml
version: 1

sources:
  - frontend: golang
    patterns: ["./..."]

plugins:
  - name: enum
    options:
      strip_prefix: true

output:
  layout: alongside-source

cache:
  enabled: true
```

## `version`

Schema version. Currently `1`. A file declaring a version the binary
does not know is rejected rather than partially interpreted.

## `sources`

Which frontend loads what.

| Key | Type | Meaning |
|---|---|---|
| `frontend` | string | The frontend plugin's name — `golang`, `protobuf`. |
| `patterns` | list | Source patterns for that frontend. Go patterns for `golang`; file globs for `protobuf`. |

Omitted entirely, the run uses `./...` against every registered
frontend. Positional arguments on the command line override
`patterns`.

## `plugins`

Per-plugin enablement, options and routing.

| Key | Type | Meaning |
|---|---|---|
| `name` | string | The plugin's `Name()`. |
| `enabled` | bool | Explicit off switch. Absent means enabled. |
| `options` | map | Passed to the plugin's typed options. Keys are the plugin's own. |
| `output` | block | Per-plugin routing override — same shape as top-level `output`. |

A plugin listed with no keys beyond `name` is a no-op: registration is
what enables a plugin, and this block only *modifies* it.

An unknown option key is a configuration error, not a warning. A typo
in a key that silently did nothing is the failure mode this rejects.

## `output`

Where rendered files land. Also valid per-plugin, and per output tag.

| Key | Type | Meaning |
|---|---|---|
| `layout` | string | `alongside-source` (default) or `centralised`. |
| `package` | string | Package name for generated files. Required under `centralised` unless a directive supplies one. |
| `dir` | string | Output directory. Only meaningful under `centralised`. |
| `tags` | map | Per-output-tag overrides, keyed by tag, each the same shape. |

`alongside-source` writes next to the file that declared the source
type. `centralised` collects everything under `dir`.

`tags` is for multi-output plugins — a generator emitting both a type
and its tests declares two outputs, and this routes them separately:

```yaml
plugins:
  - name: stub
    output:
      tags:
        test:
          dir: internal/testing
```

Routing resolves through a precedence ladder — framework default,
plugin suffix, project config, per-plugin config, source directive,
CLI flag — with later winning. [plugin/routing.md](../plugin/routing.md)
documents the whole ladder; `eidos explain` reports which layer
supplied each field for a given file.

## `cache`

| Key | Type | Meaning |
|---|---|---|
| `enabled` | bool | Absent means enabled. |
| `dir` | string | Cache location. Defaults to a brand-named directory. |

The cache keys on each plugin's captured reads plus the plugin
fingerprint. A plugin that changed behaviour without bumping its
`Version()` will serve stale output from a warm cache — which is why
`Versioned` exists and why `-no-cache` is the first thing to try when
output looks wrong.

## `manifest`

| Key | Type | Meaning |
|---|---|---|
| `path` | string | Where the run records what it produced. |

The manifest is what makes `prune` and `check` possible: it records
every file the run claimed, its hash, and the routing decision behind
it. Deleting it does not break a run — it breaks the ability to detect
what a *later* run stopped producing.

## `directives`

| Key | Type | Meaning |
|---|---|---|
| `prefix` | string | The directive prefix. Defaults to `gen`. |

With the default, annotations read `//+gen:enum`. A tool branding
itself `testkit` sets `prefix: testkit` and its users write
`//testkit:enum`.

Change this and every existing annotation in the codebase stops being
recognised — silently, because an unrecognised comment is just a
comment. See [directives.md](directives.md).

## `parallel`

A list of plugin names permitted to run concurrently within their
priority bucket. A plugin only qualifies if it reads no emit graph;
see `NodesOnly` in [plugin/conformance.md](../plugin/conformance.md).

## `envelope`

Customises the generated-file header and footer.

| Key | Type | Meaning |
|---|---|---|
| `header_prefix` | list | Lines before the generated marker. |
| `header_suffix` | list | Lines after it. |
| `footer_suffix` | list | Lines after the provenance trailer. |
| `sources_override` | list | Replaces the recorded source list. |

The generated marker line itself is not customisable: `prune` matches
on it before deleting anything, and tooling across the ecosystem keys
on its canonical form.

## `verbose`

Boolean. Equivalent to passing `-verbose`.

## See also

- [CLI reference](cli.md)
- [Directive reference](directives.md)
