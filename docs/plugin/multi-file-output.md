# Multi-file plugin output — emitting more than one file per source

A single plugin can declare an **ordered set of outputs**
(optionally tagged) and tag each emit decl with which output it
belongs to. The framework routes decls to the matching file by
suffix; the rest of the routing pipeline (Anchor, the `_test.go`
package shift, `+gen:out` overrides, project / CLI policy, the
cross-package qualifier on `emit.Internal` refs) flows on top
per output. This is how a single plugin emits, for example,
both `<src>_enum.go` (production code) and `<src>_enum_test.go`
(tests) from one source enum.

This guide covers the plugin contract, the per-decl tagging
surface, the precedence pipeline's per-output scoping, the
project-config schema, validation rules, and the conformance
contract. Read [routing.md][1] first — the precedence layers
described there compose with the per-output dispatch described
here.

[1]: routing.md

## TL;DR

| Concept | Surface | Use |
|--------|----------|-----|
| Plugin declares outputs | `Outputs(lang) []plugin.Output` | One entry per rendered file the plugin produces |
| Decl belongs to output | `BaseEmit.OutputTagName` field, read via `OutputTag()` | Set via `pkg.File(tag).<Decl>(...)`; empty = the plugin's primary output |
| Layout looks up suffix | `resolveSuffix` reads `OutputTag()` | Matches against the plugin's declared `Output.Tag` |
| Per-output override | `+gen:out tag=<tag> <path>` / `-o <plugin>:<tag>=<path>` | Scopes routing overrides to one output |
| Per-output config | `plugins[].output.tags.<tag>.<field>` | Project-level routing per output |

Single-file plugins declare one output, set no tags, and behave
identically to the pre-multi-output framework. Multi-file plugins
declare multiple outputs and tag their decls accordingly.

## The contract — `plugin.Output` + `Outputs(lang)`

The `FilenameProvider` capability returns a slice of outputs
keyed by tag. Each `Output` is the plugin's declaration of one
rendered file:

```go
type Output struct {
    // Tag is the stable per-plugin identifier for this output —
    // surfaces in `+gen:out tag=<tag>`, CLI -o flags, project
    // config, and the manifest. Empty for the plugin's primary
    // output (single-file plugins, or the default file in a
    // multi-output plugin).
    Tag string

    // Suffix is the per-source-basename filename suffix the
    // Layout phase appends. Required, non-empty. Composed as
    // <source-basename><Suffix> for alongside-source routing.
    Suffix string
}

type FilenameProvider interface {
    Plugin
    // Outputs returns the set of rendered files this plugin
    // produces in the given backend language. The pipeline
    // calls Outputs after WithPluginOptions applies: once per
    // generator at Build time for shape validation, and once
    // per generator at the start of every Layout phase. Nothing
    // is memoised in between, so the slice must be stable for a
    // given (plugin, language, options) triple.
    //
    // Returning nil or an empty slice signals the plugin has no
    // routable output in the requested language — the same
    // meaning as a non-implementer of FilenameProvider. A plugin
    // that ships templates for one language and stays silent for
    // others returns its slice for the supported language and
    // nil for the rest.
    Outputs(lang string) []Output
}
```

The slice is ordered, deterministic, and part of the plugin's
contract. External tools (CLI, config, manifest) reference
outputs by tag. The slice does not enter the plugin's cache key
— that key is composed from the plugin's version, its read set,
its resolved layout policy, and the run-wide scope flags.

## Per-decl tagging — `BaseEmit.OutputTagName` + `pkg.File(tag)`

Every emit decl carries an `OutputTagName string` field on
`BaseEmit`, read through the `OutputTag()` accessor. Empty tag
means "the plugin's primary output" (the Output declaring an
empty Tag, which validation pins at index 0 — see the validation
rules below). Non-empty tag must match one of the plugin's
declared `Output.Tag` values.

The recommended way to set the tag is the `PackageBuilder.File(tag)`
sub-context:

```go
// Default output — decls land in the plugin's primary file
// (single-file plugin behaviour, OutputTag stays empty).
pkg.Struct("Status", func(sb *builder.StructBuilder) { ... })

// Secondary output — decls built through the sub-context get
// OutputTag = "test" stamped automatically.
pkg.File("test").Function("TestStatusString_RoundTrip", func(fb *builder.FunctionBuilder) { ... })
```

`pkg.File(tag)` is memoised per tag on the root PackageBuilder.
Repeated calls with the same tag return the same sub-context;
a plugin building N decls under `pkg.File("test")` in a loop
reuses one sub-context, not N. `pkg.File("")` is the identity
form — it returns the receiver unchanged, so plugin code that
programmatically computes tags from options can write
`pkg.File(maybeEmpty).<Decl>(...)` without special-casing the
default-output case.

`File` returns a `*PackageBuilder` decorated with the tag — the
full decl-spawning surface (Struct, Interface, Function, Method,
Enum, Alias, Variable, Constant, …) is available on the
sub-context. Nested `pkg.File("a").File("b")` overwrites: the
second call returns a sub-context tagged `"b"`, not `"a.b"`.
Nesting is not a supported pattern — express each logical
sub-file as a single `pkg.File(<tag>)` call directly off the
root `pkg`.

The sub-context shares the root PackageBuilder's underlying
`emit.Package`, its Anchor default origin, and its error sink —
`Err` and `Build` on a sub-context report the root's accumulated
state. Only the stamped tag differs.

Each decl belongs to exactly one output. The tag it carries is a
single string. A plugin that needs the same logical content
in two files (a helper function appearing in both production and
test outputs, say) emits the decl twice — once tagged for each
output. The framework never deduplicates decls across outputs.

## Default-tag semantics

The framework's default-tag rule keeps single-file plugin code
unchanged:

- A plugin declaring one output with an empty `Tag` produces
  decls with empty `OutputTag` — visually and structurally
  identical to today's single-file plugins.
- A plugin declaring multiple outputs declares its primary one
  with an empty `Tag` at index 0. Decls built without a
  `pkg.File(...)` sub-context land in that primary output.

A plugin can also declare every output with a non-empty tag and
require explicit tagging on every decl. In that mode, a decl
reaching Layout with empty `OutputTag` is a hard error — the
framework refuses to silently route it to `outputs[0]` because
the plugin's "no default output exists" intent was explicit.
See the validation rules table below for the diagnostic.

## Routing precedence — per-output scoping

The precedence layers from [routing.md][1] apply per output. A
decl tagged `test` flows through the same pipeline (framework
default → project layout → directive → CLI) as any other decl;
the layers consult per-output keys where they exist. The
`_test.go → <pkg>_test` package shift runs last, after every
override layer, per resolved Target — each tagged output computes
its own Target, and the shift fires independently when that
output's resolved filename ends in `_test.go`. Which layer
supplied the package is deliberately ignored; the only escape is
a package that already ends in `_test`, which the shift leaves
untouched rather than doubling.

### `+gen:out` with `tag=` scope

A `+gen:out` directive on a source applies to a specific output
when scoped with `tag=`:

```go
//+gen:out tag=test testkit/
//+gen:enum
type Status int
```

- Production-output decls land in `store/searcher_enum.go` (the
  default).
- Test-output decls (tag `test`) land in `store/testkit/searcher_enum_test.go`.

The override applies only to decls carrying the named tag. A
`tag=` value no decl carries — a typo, or an output the `enum`
plugin does not declare — matches nothing and the directive is a
silent no-op: every decl routes by its declared suffix and no
diagnostic fires. The unknown-tag rule in the validation table
governs the other direction, a decl whose own tag names no
declared output.

When two plugins emitting against the same origin each declare a
`test` tag in their own Outputs, an unscoped `+gen:out tag=test
<path>` applies to every such plugin — `tag=` without `plugin=`
expresses a cross-cutting intent ("route every plugin's test
output here"), narrowing the standalone directive's existing
apply-to-every-plugin reach by output instead of by plugin.
Reach for `plugin=<name> tag=test` to scope strictly to one
plugin's test output (form covered in the intersection paragraph
below).

`tag=` accepts a `pkg=` companion to pin the rendered package
clause on the targeted output independently:

```go
//+gen:out tag=test pkg=storetest testkit/
//+gen:enum
type Status int
```

- Production-output decls keep the source package (`store`).
- Test-output decls land in
  `store/testkit/searcher_enum_test.go` and render under
  `package storetest_test` — `pkg=` supplies the package name,
  and the `_test.go` shift then appends `_test` to it. Write
  `pkg=storetest_test` to state the rendered clause verbatim.

`tag=`, `pkg=`, and `plugin=` are keyword arguments — order is
irrelevant and the positional path may appear anywhere among
them. `plugin=` and `tag=` compose as an intersection: an
override scoped `plugin=mock tag=test` applies only to the
`test` output of the `mock` plugin — not to any other plugin's
`test` output and not to the `mock` plugin's primary output.

An unscoped `+gen:out` that pins a **filename** against a
multi-output plugin is rejected at Layout time with a teaching
diagnostic — uniform application would silently collapse
`_test.go` and `_main.go` into one file, which Go's per-file
test-classification rule then misreads. A directory-only path
(`testkit/`) is permitted unscoped: the per-output suffixes keep
the filenames distinct inside the shared directory.

Unscoped `+gen:out` on a single-output plugin continues to work
as today (no ambiguity — the plugin has one output, which IS the
default).

### Per-directive `tag=` on emitter-owned directives

Form 3 from [routing.md][1] — `out=` and `pkg=` keys on an
emitter's own directive — recognises `tag=` for per-output
scoping:

```go
//+gen:mock tag=test out=tests/ pkg=mocktest
type Store interface { ... }
```

The override applies only to the `mock` plugin's `test` output;
the primary output keeps the framework default. All three keys
are scoped to the directive's owning plugin — the pipeline
records a form-3 spec as if it carried `plugin=<owner>`, so a
`+gen:mock out=…` never moves a companion plugin's output.
`tag=` narrows that scope one step further, to one of the
owner's declared outputs. A companion that genuinely must follow
another plugin's routing says so with the standalone
`+gen:out plugin=<name>` form.

Form 3 without `tag=` follows the same "no implicit
multi-output collapse" rule as form 2: an unscoped `out=` that
would force two outputs to share a filename is rejected at
Layout time with the same diagnostic. Directory-only overrides
stay safe — the per-output suffixes keep filenames distinct
within the shared directory.

### CLI `-o <plugin>=<path>` / `-o <plugin>:<tag>=<path>`

CLI flag syntax mirrors the directive's scope:

- `-o mock=mocks/handlers.go` — overrides every output the `mock`
  plugin declares. Backward-compatible with the existing CLI
  form, and unambiguous for a single-output plugin; against a
  multi-output plugin it pins one filename for all of them,
  which the Layout phase does not reject the way it rejects the
  directive equivalent.
- `-o mock:test=tests/handlers.go` — overrides the `mock` plugin's
  `test` output specifically, and wins over the plugin-only form
  for that output.

One `=` separator between key and value; `:` lives inside the key
to disambiguate plugin+tag from path-with-colon.

## Project-config schema

Per-plugin routing lives on the plugin's own entry under
`plugins:`, in its `output:` block; per-output routing nests one
level deeper under `tags:`. The top-level `output:` block is the
project-wide default and carries routing fields only — it is not
keyed by plugin, and `tags:` there is ignored because tag values
are plugin-scoped.

```yaml
output:
  # Project-wide default for every plugin.
  layout: alongside-source

plugins:
  # Single-output plugin (no tags): one routing block.
  - name: builder
    output:
      layout: centralised
      package: builders
      dir: internal/builders

  # Multi-output plugin: per-tag routing via `tags:`.
  - name: mock
    output:
      layout: alongside-source      # applies to every output
      tags:
        test:
          package: storetest        # applies to `test` only
```

A field left empty inside `tags:` inherits from the surrounding
per-plugin block, which in turn inherits from the project block.
`dir:` takes effect only under `centralised` layout; under
`alongside-source` the directory comes from the origin and the
field is ignored.

The `tags:` sub-namespace avoids collisions between tag names and
field names — a plugin shipping a `dir` tag and the `dir` field
coexist cleanly.

## Validation rules

The framework rejects malformed `Outputs` slices at Build time,
and malformed per-decl tags at Layout time. The first four
diagnostics wrap `pipeline.ErrInvalidOutputs` and prefix the
message below with `pipeline: invalid plugin Outputs: <plugin>:`;
the last two wrap their own sentinel and name the offending decl.

| Rule | Diagnostic |
|------|-----------|
| Output with empty `Suffix` | `outputs[<i>]: Suffix is required` |
| Duplicate `Tag` values | `outputs declare tag "<tag>" at indices <i> and <j>` |
| More than one Output with empty Tag | `<n> outputs declare an empty Tag; at most one is permitted (the plugin's primary output)` |
| Output with empty Tag exists but is not at index 0 | `outputs[<i>]: empty-Tag output must be declared at index 0` |
| Decl tagged with an output the plugin never declared | `pipeline.ErrUnknownOutputTag` — `pipeline: decl carries unknown OutputTag for its plugin's declared outputs: <kind> "<qname>" emitted by "<plugin>" has OutputTag "<tag>"; declared tags: [<declared>]` |
| Untagged decl on a plugin declaring no empty-Tag output | `pipeline.ErrNoDefaultOutput` — `pipeline: decl carries empty OutputTag but plugin declares no default output: <kind> "<qname>" emitted by "<plugin>"; declared tags: [<declared>]` |

The last two rules are Layout-time because they are
data-dependent — the framework cannot tell statically which decls
a generator will emit. A decl that trips either one is dropped
from the run with an Error diagnostic; the rest of the run
continues.

The framework enforces every rule in this table on its own,
whether or not the plugin runs `plugintest.RunSuite`. The
conformance suite is the earlier signal for authors who run it
during development: it checks the declared slice's shape for
every language it probes, so a violation surfaces in the plugin's
own tests rather than at a consumer's Build.

## Manifest reporting

The per-target manifest entry's plugin attribution is
polymorphic. An untagged attribution marshals as the bare plugin
name, exactly as it did before multi-output support landed; a
tagged one marshals as an object carrying `name` and
`output_tag`. Unmarshal accepts either form, and one `plugins`
array may hold both. The same enum source producing both files
records side-by-side entries — the primary is byte-identical to a
pre-multi-output manifest, and the secondary carries the tag
explicitly (run-level fields and hashes abridged):

```json
{
  "version": 2,
  "outputs": [
    {
      "target": { "dir": "store", "filename": "searcher_enum.go", "package": "store" },
      "plugins": ["enum"],
      "hash": "sha256:…",
      "pipeline_id": "…"
    },
    {
      "target": { "dir": "store", "filename": "searcher_enum_test.go", "package": "store_test" },
      "plugins": [{"name": "enum", "output_tag": "test"}],
      "hash": "sha256:…",
      "pipeline_id": "…"
    }
  ]
}
```

### Cross-tool naming convention

Tags are a **plugin-scoped namespace**, not a global one — two
plugins may each declare a `test` tag without collision. Tooling
that surfaces tags must preserve scope when the surface is
human-readable; the CLI `-o <plugin>:<tag>=<path>` form
establishes `<plugin>:<tag>` as the canonical rendering, and
explain commands, diagnostics, log lines, and any other free-text
surface should follow the same shape (`enum:test`, `mock:test`,
…) so a reader of a multi-plugin pipeline can tell whose `test`
output a message refers to. The manifest JSON itself is exempt:
the structural pairing of `"name"` and `"output_tag"` under the
same plugin entry already scopes the tag unambiguously, and
structured consumers should read both fields together rather
than reconstructing the `<plugin>:<tag>` string.

## Migration from `FilenameSuffix`

`FilenameSuffix(lang) string` is removed in favour of
`Outputs(lang) []Output`. The single-file migration is one entry
returning the existing suffix:

```go
// Before
func (*Plugin) FilenameSuffix(lang string) string {
    if lang == "golang" {
        return "_mock.go"
    }
    return ""
}

// After
func (*Plugin) Outputs(lang string) []plugin.Output {
    if lang != "golang" {
        return nil
    }
    return []plugin.Output{{Suffix: "_mock.go"}}
}
```

Four reference plugins declared a suffix and migrated identically
— `mockgen`, `registrygen`, `repogen`, and the since-removed
`buildergen`. Test fixtures did the same. No behaviour change for
any existing single-output plugin.

## Multi-output example — the enum stringer pattern

A single enum plugin emitting both production code and tests:

```go
func (*Plugin) Outputs(lang string) []plugin.Output {
    if lang != "golang" {
        return nil
    }
    return []plugin.Output{
        {Suffix: "_enum.go"},          // primary, empty tag
        {Tag: "test", Suffix: "_enum_test.go"},
    }
}

func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
    for _, e := range ctx.Reader.Enums().Slice() {
        if !e.HasPositiveDirective(DirectiveName) {
            continue
        }
        pkg := builder.For(Name).Anchor(e)
        p.emitProduction(pkg, e)
        p.emitTests(pkg.File("test"), e)
        out, err := pkg.Build()
        if err != nil {
            return err
        }
        if err := ctx.Store.Emit().AddPackage(out); err != nil {
            return err
        }
    }
    return nil
}
```

`emitProduction` builds decls through `pkg` directly — they land
in the primary output (`<src>_enum.go`). `emitTests` builds
through `pkg.File("test")` — those decls carry the tag `test`
and land in `<src>_enum_test.go`. The framework's `_test.go →
<pkg>_test` shift applies to the test file's resolved package
clause automatically.

The two files share the same source anchor, the same directory,
and (until any directive override) the same project layout
policy. Per-output routing overrides target individual tags
without affecting the other.

## When to reach for siblings instead

Multi-output and sibling plugins both produce more than one file
from the same source — the distinction is **shared lifecycle**.
Use multi-output when the files are tightly coupled and must
move together:

- They share the same source anchor and the same project routing
  policy.
- A user enabling the plugin enables every output; disabling
  the plugin disables every output.
- Per-output overrides are the rare case, not the common one.
- The files' contents conceptually represent one feature
  (production code + the tests that pin its contract; a builder and its compile-time interface assertion).

Reach for sibling plugins when the files have independent
lifecycles or compose into different consumer pipelines:

- A user may want one without the other (mock generation without
  recording overlays; a builder without the JSON-schema export
  it pairs with).
- They run in different priority buckets, plug into different
  generator phases, or react to different directives.
- Their output volumes / cadences differ enough that bundling
  them would confuse users about which plugin produced which
  file.

The mock plugin family is the canonical sibling pattern — `mock`,
`mocktest`, and `mockrecord` each own a distinct concern and a
distinct user opt-in even though all three share the same source
interface anchor. The enum plugin's production code + tests pair
is the canonical multi-output pattern — splitting them into two
plugins would force users to opt into both for the tests to be
useful, with no scenario where one without the other makes sense.

## Composition with weaver plugins

Weaver-style plugins — pure slot-contributing cross-cutters that
decorate decls owned by other plugins (auditweaver, debugweaver,
recording / fault-injection / tracing) — compose with multi-output
hosts without API change:

- **Slot contributions on a host decl** (Prebody, Postbody,
  FieldsSlot, MethodsSlot, …) travel with the host. The weaver
  appends a `Stmt` / `Field` / `Method` to the host's slot; the
  contribution renders inside the host decl's rendered text, so
  it lands in whichever file the host's tag routed the host to.
  Routing never reads the contribution's own tag. The weaver is
  not required to implement `Outputs`; pure-weaver plugins keep
  the "no `FilenameProvider`" signal that already exists today.

  A weaver iterating decls across a multi-output host therefore
  contributes to every output the host emits — its contributions
  flow with each decl's routing. A debug-tracer decorating the
  enum plugin's `String` method (production output) and its
  `TestStringRoundTrip` function (test output) lands one prebody
  in `<src>_enum.go` and another in `<src>_enum_test.go` from a
  single iteration, with no per-output coordination. Weavers that
  want to scope to one output filter on the host's
  `OutputTag()` (or on a per-host meta key) in the iteration
  body — every emit decl exposes the accessor through `BaseEmit`.
- **Origin-anchored slot contributions** (file-level slots
  appended via `EmitView.AppendOriginSlot` against a source node)
  compose through the weaver's *own* `Outputs` slice — the
  framework routes the contribution to the weaver's primary
  output by default. `AppendOriginSlot` takes the item, not a
  builder sub-context, so a weaver targeting one of its own
  outputs stamps `OutputTagName` on the item's `BaseEmit` before
  queueing it — the form the in-tree `enum` plugin uses for its
  `test`-tagged contribution.
- **Weavers that emit routable decls** (rare; the framework
  recommends splitting into a generator + a weaver instead)
  implement `Outputs(lang)` like any other generator.

Cross-plugin output reach — a weaver targeting another plugin's
output namespace (e.g. adding a helper function to the `enum`
plugin's `test` output from outside `enum`) is intentionally not
supported by this spec. `OutputTag` values are plugin-scoped;
a contribution lives in either the host decl's file (via slot
inheritance) or the contributing plugin's own file (via its own
`Outputs`). Cross-plugin file-sharing patterns — when they
emerge — need explicit cross-plugin coordination (published meta
keys, shared anchors) outside the routing surface.

## Composition with templates

Multi-file output is **orthogonal** to the [templates surface][tmpl]:
templates control *how* an emit decl renders into text, outputs
control *where* that text lands. A plugin shipping templates for
custom emit kinds (e.g. an `enum.stringer` kind) gains nothing or
loses nothing from declaring multiple outputs — the template
renders the kind's text; the framework's per-output routing
deposits the rendered text into the right file based on
`OutputTag`.

Two patterns plugin authors use to interleave templates with
multi-output emission:

[tmpl]: templates.md

### Single template, branches on `OutputTag`

The decl carries `OutputTag` through to the render context. A
single `.tmpl` file can branch on it for kinds whose rendered
shape varies per output:

```gotemplate
{{- define "enum.stringer" -}}
{{- if eq .OutputTag "test" -}}
{{- /* test-side rendering: helpers, table-driven cases */ -}}
{{- else -}}
{{- /* production rendering: name const, index var, String method */ -}}
{{- end -}}
{{- end -}}
```

### Distinct emit kinds per output

Plugin authors who prefer separate render shapes give the
production-side and test-side decls **different emit kinds** and
ship one template per kind. The backend's existing
`Kind()`-based dispatch routes each decl to its own template
without any per-tag knowledge — `OutputTag` decides which file
the rendered text lands in; `Kind()` decides what the rendered
text looks like.

```go
type stringer struct{ emit.BaseEmit; ... }
func (*stringer) Kind() kind.Kind { return "enum.stringer" }

type stringerTest struct{ emit.BaseEmit; ... }
func (*stringerTest) Kind() kind.Kind { return "enum.stringer.test" }
```

The plugin ships `enum.stringer.tmpl` and `enum.stringer.test.tmpl`
via `sdk.TemplateProvider`; the backend resolves each through the
funcmap's `render` entry as it would any plugin-defined kind. The
two outputs read cleanly as two kinds, with no `OutputTag`
branches in either template.

The framework imposes no contract on which pattern a plugin
picks. Branch-on-`OutputTag` keeps one template and one kind;
distinct kinds keep templates focused at the cost of two kind
registrations. Both compose cleanly with the per-decl tagging
surface — neither requires backend changes.

## Conformance contract

`plugintest.RunSuite` carries three static checks on
`Outputs(lang)`, run against every language the suite probes:

- **`FilenameProvider returns a well-formed Outputs slice`** — the
  same shape rules the pipeline enforces at Build.
- **`FilenameProvider returns stable Outputs per language`** —
  two consecutive calls with the same argument return equal
  slices.
- **`declaration accessors return slices the caller may keep`** —
  two calls must not share one backing array. The pipeline stores
  what `Outputs` hands it, so a plugin returning its own field
  has its declaration rewritten by any consumer that sorts or
  filters in place. Return a copy.

`plugintest.RunGeneratorSuite` adds two per-fixture checks that
need the generator's actual emissions:

- **`emitted output tags are declared`** — every tag the
  generator stamps on an emit value matches an `Output` the same
  generator declares. An undeclared tag does not fail loudly at
  Layout; it routes somewhere other than the file the tag names.
- **`output-package dispatch tolerates partial routing`** — every
  `emit.OutputPackageSetter` the generator produced survives
  being handed an empty map, a map of foreign tags, and a map
  carrying only the primary tag with no derivable path. Layout
  calls `SetOutputPackages` at most once per value, with only the
  tags that actually routed, so an implementor that indexes the
  map and uses the result unchecked emits a reference to the
  empty package.

Plugin authors run the suites against every backend language they
contribute to; they catch output-shape regressions before the
plugin reaches a real pipeline.
