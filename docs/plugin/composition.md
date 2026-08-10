# Multi-generator composition

The pattern most plugin authors reach for once they understand
the basics: **several generators, each shipping its own
templates, all contributing into the same host emit decl via
slots**. This is how cross-cutting concerns compose without
plugins needing to know about each other.

This guide walks through a realistic example — generating a
production HTTP handler from a single annotated source struct,
where seven generators collaborate to produce the final output.
No full plugin code; just the orchestration shape, the slot
semantics, and the rendered result.

## The scenario

A user writes one annotated struct:

```go
package api

// +gen:handler
type CreateUser struct {
    Email string `validate:"required,email"`
    Name  string `validate:"required"`
}

// The user implements the business logic; everything else is
// generated.
func (h *CreateUserHandler) handle(
    ctx context.Context, req *CreateUser,
) (*CreatedResponse, error) {
    // domain logic
}
```

The expected output: a fully-wired HTTP handler with request
decoding, validation, authentication, metrics, distributed
tracing, error recovery, and audit logging — all composed from
seven small plugins that each own one concern.

## The ensemble

| Plugin           | Priority bucket          | Provides       | Contributes to                                |
|------------------|--------------------------|----------------|-----------------------------------------------|
| `handlergen`     | `GeneratorFoundation`    | `http.handler` | Emits `<Type>Handler` struct + `ServeHTTP` method scaffold |
| `validategen`    | `GeneratorComposition`   | `http.validate`| Emits `Validate(*<Type>) error` + appends call to `ServeHTTP.prebody` |
| `authgen`        | `GeneratorCrossCutting`  | `http.auth`    | Appends auth-check to `ServeHTTP.prebody` (before validation) |
| `metricgen`      | `GeneratorCrossCutting`  | `http.metric`  | Appends counter to `prebody`, latency observe to `postbody` |
| `tracegen`       | `GeneratorCrossCutting`  | `http.trace`   | Appends span-start to `prebody`, deferred span-end to `postbody` |
| `errorgen`       | `GeneratorCrossCutting`  | `http.error`   | Wraps `ServeHTTP.postbody` with recover-and-respond |
| `auditgen`       | `GeneratorFinalize`      | —              | Appends audit-log call after the response is written |

The priority buckets enforce phase ordering:

```
GeneratorFoundation   →  handlergen
        ↓
GeneratorComposition  →  validategen
        ↓
GeneratorCrossCutting →  authgen, metricgen, tracegen, errorgen
                         (parallel within bucket, capability-topo
                          for cross-references)
        ↓
GeneratorFinalize     →  auditgen
```

By the time `auditgen` runs, every prior plugin has appended
its contribution; auditgen sees the assembled emit graph and
slots its own contribution in last.

## How each plugin contributes

### `handlergen` — the foundation

Emits the host decl that everyone else extends. Owns the
`ServeHTTP` method's structural skeleton; cross-cutting plugins
contribute into its slots.

handlergen ships **no template at all**. It emits an ordinary
`emit.Struct` carrying an ordinary `emit.Method`, and the
backend's core `emit.method` template renders it:

```go
serve := &emit.Method{
    Name:         "ServeHTTP",
    ReceiverName: "h",
    Receiver:     sdk.Ptr(sdk.Internal(handler)),
    Params: []*emit.Param{
        {Name: "w", Type: sdk.External("net/http", "ResponseWriter")},
        {Name: "r", Type: sdk.Ptr(sdk.External("net/http", "Request"))},
    },
    Body: []*sdk.Stmt{
        sdk.NewRawStmt("// body owned by handlergen"),
        sdk.NewRawStmt("h.serve(w, r)"),
    },
}
```

The composition happens inside the core template, which calls
`renderMethodBody`. That single helper renders **the prebody
slot, then the method's typed `Body`, then the postbody slot**.
handlergen does not know what will land in the slots, and does
not have to arrange for them to be rendered — writing a custom
template here would be work the framework already does.

This is the **programmatic** half of the composition story:
a plugin whose output is core emit kinds, rendered by templates
it does not own. The **template** half is `middlewaregen` and
its contributors below, where each plugin declares its own emit
kind and ships the template that renders it. Reach for the
programmatic form when the output is a plain Go declaration;
reach for the template form when the shape is yours and a
contributor needs to render inside it.

### `validategen` — composition

Reads `validate:"..."` struct tags, emits a free-standing
`Validate(*CreateUser) error` function, and prepends a call to
the host method's `prebody` slot.

```go
// validategen.Generate (excerpt)
func (*Plugin) Generate(ctx *sdk.GeneratorContext) error {
    c := sdk.NewProvenance(Name)
    for _, pending := range ctx.Store.Emit().PendingOriginSlots() {
        host, ok := pending.Item.(*handlergen.Handler)
        if !ok {
            continue
        }
        origin := host.Origin()

        // The emitted Validate function, in validategen's own file.
        v := &Validator{
            BaseEmit: sdk.BaseEmit{OriginNode: origin, SetByName: c.SetBy()},
            FuncName: "Validate" + host.Source,
        }
        if err := ctx.Store.Emit().AppendOriginSlot(
            origin, "top", v, c.Provenance(Name+".validator."+host.Source),
        ); err != nil {
            return err
        }

        // The call to it, into the slot the host hands out.
        call := sdk.NewExprStmt(sdk.NewCall(
            sdk.NewIdent(v.FuncName),
            sdk.NewAddr(sdk.NewIdent("req")),
        ))
        if err := host.Prebody().Append(
            call, c.Provenance(Name+".call."+host.Source),
        ); err != nil {
            return err
        }
    }
    return nil
}
```

**The type assertion is the guard, and it is load-bearing.**
`FileFor` is lookup-*or-create*, so a contributor that ran where its
host did not would create the file alone — a fragment of declarations
hanging off types nothing declared. `Requires` does not prevent that:
an unsatisfied requirement is ignored silently. Nor does checking the
source directive, which tells you what was *asked for* rather than
what was *produced*. Matching on the host's own emit value is the only
check that holds.

It also hands you the host's projection — its method set, its names —
so you read them off the host rather than re-deriving them from
source. Two derivations of one thing are two chances to disagree, and
the disagreement surfaces as generated code referencing a symbol the
host never emitted.

Plus a separate emit decl — the generated `Validate` function —
which uses validategen's own template:

```go
// validategen template (validategen.validatefn.tmpl, abbreviated)
{{- define "validategen.validatefn" -}}
func Validate(req *{{ .Type }}) error {
{{ range .Rules }}
    if {{ renderExpr .Condition }} {
        return fmt.Errorf({{ renderExpr .Message }})
    }
{{ end }}
    return nil
}
{{- end -}}
```

### `authgen` — first cross-cutter

Prepends auth-check statements to `prebody`. Runs before
validategen's contribution in source order because authgen
declares `Provides: "http.auth"` and validategen declares
`Requires: "http.auth"` — the capability topo orders authgen
first within the cross-cutting bucket.

```go
// authgen.Generate (excerpt)
authStmt := sdk.NewIf(
    sdk.NewCall(
        sdk.External("myco/auth", "RequireToken"),
        sdk.NewMethodCall(sdk.NewIdent("r"), "Context"),
        sdk.NewIdent("r"),
    ),
    []*sdk.Stmt{
        sdk.NewExprStmt(sdk.NewMethodCall(
            sdk.NewIdent("h"), "respondError",
            sdk.NewIdent("w"), sdk.NewIdent("err"),
        )),
        sdk.NewReturn(),
    },
)
host.Prebody().Prepend(authStmt, c.Provenance("http.auth.check"))
```

`Prepend` (versus `Append`) puts the check at the start of the
prebody, before validation — auth is the canonical "fail first"
concern.

### `metricgen`, `tracegen` — paired cross-cutters

Both contribute to **prebody** AND **postbody**, demonstrating
that one plugin can append to multiple slots on the same host.
tracegen ships a small template for the span-start expression so
its rendered call composes with the runtime tracer correctly:

```go
// tracegen template (tracegen.spanstart.tmpl, abbreviated)
{{- define "tracegen.spanstart" -}}
ctx, span := {{ renderExpr .Tracer }}.Start(
    {{ renderExpr .Ctx }}, {{ renderExpr .OpName }})
{{- end -}}
```

And a sibling template for the deferred end:

```go
// tracegen template (tracegen.spanend.tmpl, abbreviated)
{{- define "tracegen.spanend" -}}
defer span.End()
{{- end -}}
```

The plugin emits two custom emit kinds (`SpanStart`, `SpanEnd`)
and appends one to each slot. The backend's `renderMethodPrebody`
helper dispatches to the matching template by kind, so multiple
plugin-defined emit kinds in the same slot render correctly.

### `errorgen` — wrapping the postbody

Appends a deferred recover-and-respond wrapper to `postbody`.
Order matters: errorgen runs LAST in the cross-cutting bucket
(declares `Requires: "http.auth", "http.metric", "http.trace"`)
so its `defer recover()` registers before any of the other
deferred statements unwind on panic.

```go
// errorgen template (errorgen.recover.tmpl, abbreviated)
{{- define "errorgen.recover" -}}
if rec := recover(); rec != nil {
    h.respondError(w, {{ renderExpr .ErrorExpr }})
}
{{- end -}}
```

The contribution lands in `postbody` because Go's `defer`
semantics put recover at the end of the function lexically but
run it on every return. handlergen's template renders postbody
inside the function body, after the response write — exactly
where the recover dispatch needs to live.

### `auditgen` — finaliser

Appends a final audit-log statement to `postbody`. Runs in
`GeneratorFinalize` so it sees every cross-cutting plugin's
contribution and slots in after them — the audit log always
runs last in the response path.

```go
// auditgen template (auditgen.auditcall.tmpl, abbreviated)
{{- define "auditgen.auditcall" -}}
{{ renderExpr .AuditFunc }}({{ renderExpr .Ctx }},
    {{ renderExpr .Operation }}, {{ renderExpr .Subject }})
{{- end -}}
```

## The rendered output

After every plugin runs, the backend renders the file. The
template-dispatch funcmap walks each emit decl, finds the
matching template by `Kind()`, and inlines slot contributions
in append order:

```go
// Code generated by eidos. DO NOT EDIT.
// Sources: api/create_user.go
// Plugins: handlergen v1.0.0, validategen v1.0.0, authgen v1.0.0,
//          metricgen v1.0.0, tracegen v1.0.0, errorgen v1.0.0,
//          auditgen v1.0.0

package api

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "myco/audit"
    "myco/auth"
    "myco/metrics"
    "myco/tracing"
)

func (h *CreateUserHandler) ServeHTTP(
    w http.ResponseWriter, r *http.Request,
) {
    // — prebody contributions (in append order) —

    // [authgen] (prepended — runs first)
    if err := auth.RequireToken(r.Context(), r); err != nil {
        h.respondError(w, err)
        return
    }

    // [tracegen] span start
    ctx, span := tracing.Tracer.Start(r.Context(), "CreateUser")

    // [metricgen] counter
    start := time.Now()
    metrics.RequestsReceived.WithLabelValues("CreateUser").Inc()

    // [validategen] validate call
    if err := Validate(&req); err != nil {
        h.respondError(w, err)
        return
    }

    // — body owned by handlergen —
    var req CreateUser
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        h.respondError(w, err)
        return
    }
    resp, err := h.handle(ctx, &req)
    if err != nil {
        h.respondError(w, err)
        return
    }
    h.respondJSON(w, resp)

    // — postbody contributions (in append order) —

    // [tracegen] deferred span end
    defer span.End()

    // [metricgen] latency observe
    metrics.RequestLatency.WithLabelValues("CreateUser").
        Observe(time.Since(start).Seconds())

    // [errorgen] recover-and-respond
    if rec := recover(); rec != nil {
        h.respondError(w, fmt.Errorf("panic: %v", rec))
    }

    // [auditgen] audit log
    audit.Log(ctx, "CreateUser", req.Email)
}

// — emitted by validategen —
func Validate(req *CreateUser) error {
    if req.Email == "" {
        return fmt.Errorf("email is required")
    }
    if !strings.Contains(req.Email, "@") {
        return fmt.Errorf("email must be valid")
    }
    if req.Name == "" {
        return fmt.Errorf("name is required")
    }
    return nil
}
```

Note what the plugin author writing each plugin did NOT do:

- **No plugin imported another.** authgen has no idea
  validategen exists; both contribute through the host's
  `prebody` slot.
- **No plugin coordinated ordering directly.** The capability
  topo (`Provides` / `Requires` declarations) and the priority
  buckets did the ordering.
- **No plugin built import paths by hand.** Every
  `sdk.External("myco/audit", "Log")` produces an expression
  the backend resolves into the file's import set
  automatically.
- **No plugin rendered Go syntax directly.** Each template
  uses `renderExpr` / `renderStmt` to delegate to the canonical
  funcmap; the cross-cutting weavers don't need to know how
  `if`, `defer`, or composite literals format.

## The data-flow shape

```
+-----------------+  +gen:handler
| source CreateUser| ──────────────────┐
+-----------------+                    │
                                       ▼
       ┌──────────────────────────────────────────────┐
       │ Foundation phase                             │
       │  • handlergen reads source struct            │
       │  • emits: CreateUserHandler struct +         │
       │           ServeHTTP method (skeleton)        │
       └────────────────────┬─────────────────────────┘
                            ▼
       ┌──────────────────────────────────────────────┐
       │ Composition phase                            │
       │  • validategen emits Validate(req) fn        │
       │  • validategen.Prebody.Append → ServeHTTP    │
       └────────────────────┬─────────────────────────┘
                            ▼
       ┌──────────────────────────────────────────────┐
       │ Cross-cutting phase  (capability-topo order) │
       │                                              │
       │  authgen     → ServeHTTP.Prebody  (prepend)  │
       │  tracegen    → ServeHTTP.Prebody  (append)   │
       │                ServeHTTP.Postbody (append)   │
       │  metricgen   → ServeHTTP.Prebody  (append)   │
       │                ServeHTTP.Postbody (append)   │
       │  errorgen    → ServeHTTP.Postbody (append)   │
       └────────────────────┬─────────────────────────┘
                            ▼
       ┌──────────────────────────────────────────────┐
       │ Finalize phase                               │
       │  • auditgen → ServeHTTP.Postbody (append)    │
       └────────────────────┬─────────────────────────┘
                            ▼
       ┌──────────────────────────────────────────────┐
       │ Backend render                               │
       │  • dispatch each emit decl by Kind to its    │
       │    plugin's template                         │
       │  • renderMethodPrebody/Postbody inlines      │
       │    contributions in append order             │
       │  • resolve imports, gofmt, stamp header      │
       └────────────────────┬─────────────────────────┘
                            ▼
                  api/create_user_handler.go
```

## Why this works

**Slots decouple ownership from contribution.** handlergen owns
the host method's scaffold; six other plugins contribute into
it without knowing each other exists. The slot abstraction
inverts the usual "decorator pattern" coordination problem —
each plugin appends; the host renders.

**Priority buckets enforce phase semantics.** Foundation must
run before composition; composition before cross-cutting;
cross-cutting before finalize. The bucket boundaries are hard;
within a bucket, capability topo orders dependent plugins
correctly.

**Templates compose through the funcmap, not through
inheritance.** Every plugin's template is independent; the
backend's `renderMethodPrebody` helper composes contributions
at render time. Adding an eighth plugin (say, a rate-limit
weaver) doesn't touch any of the existing seven — it just
appends into the same slot.

**Provenance tracks who did what.** Every slot appendage
carries a `Provenance` record naming the contributing plugin
and the entity that produced it. When a generated file looks
suspicious, the manifest tells you which plugin contributed
each statement.

**The user wrote one annotated struct.** Eight files of
generated code; zero coordination between plugins; full audit
trail in the manifest. That's the composition story eidos is
designed for.

## Custom slots — plugins depending on plugins

> Runnable as `reference/middlewaregen` (the host) with
> `reference/authgen`, `reference/metricgen` and
> `reference/tracegen` (the contributors). The end-to-end
> assertions live in `reference/middlewaregen/composition_test.go`.

Standard slots (`prebody`, `postbody`, `methods`, `fields`, …)
live on the core emit kinds (`Method`, `Struct`, `Function`)
that any plugin can use without coordination. The richer
composition pattern is when **one plugin defines a custom emit
kind with a custom slot, and other plugins contribute into
that slot** — making the contributors explicitly depend on the
slot-owning plugin.

Extend the HTTP-handler example: add a `middlewaregen` plugin
that emits a `MiddlewareStack` emit kind exposing a `chain`
slot. The cross-cutters (authgen, metricgen, tracegen) now
contribute middleware entries into `chain` instead of inlining
their concerns into `prebody`. The handler's runtime composes
the chain at startup; the prebody slot stays focused on
per-request concerns.

### The new plugin

```
Plugin: middlewaregen
Priority: GeneratorFoundation (runs alongside handlergen)
Provides: "http.middleware"
Emits:    *MiddlewareStack per +gen:handler source
Slot:     "chain" on each MiddlewareStack
```

`MiddlewareStack` is a plugin-defined emit kind:

```go
package middlewaregen

import (
)

const Kind sdk.Kind = "middlewaregen.stack"

type MiddlewareStack struct {
    sdk.BaseEmit

    HandlerType string  // e.g. "CreateUserHandler"
    Target      sdk.EmitTarget

    // The custom slot for cross-plugin contributions.
    chain *sdk.Slot
}

func (s *MiddlewareStack) Kind() sdk.Kind { return Kind }

// Chain returns the custom slot other plugins contribute into.
// The framework's Slot machinery handles append-order
// determinism, provenance tracking, and template-time access.
func (s *MiddlewareStack) Chain() *sdk.Slot {
    if s.chain == nil {
        s.chain = sdk.NewSlot("chain", sdk.EmitKindExpr)
    }
    return s.chain
}

// Slot satisfies sdk.SlotHost so the generic `slot` template
// helper can reach the chain by name.
func (s *MiddlewareStack) Slot(name string) *sdk.Slot {
    if name == "chain" {
        return s.Chain()
    }
    return sdk.NewSlot(name, "")
}

var _ sdk.SlotHost = (*MiddlewareStack)(nil)
```

The slot's **element kind** is part of the contract. Declaring
`sdk.EmitKindExpr` makes the framework reject a malformed append
(a `*Statement` into an expression slot) at the append call,
naming the contributing plugin, instead of producing broken
output at render.

Leave it empty when contributors bring their own emit kinds, as
the runnable version of this example does — the content is then
heterogeneous by design and no single kind describes it. The
trade is where the error surfaces: an open slot accepts anything
and fails at render with a missing template naming a *kind*,
rather than at append naming a *plugin*.

### The template

```
{{- define "middlewaregen.stack" -}}
// middleware is the request-time chain assembled at handler init.
func (h *{{ .HandlerType }}) middleware(handler http.Handler) http.Handler {
{{- range (slot . "chain").Items }}
    handler = {{ render . }}(handler)
{{- end }}
    return handler
}
{{- end -}}
```

The new helper here is `slot . "chain"` — the generic accessor
that returns the named slot. Note the argument order: the host
comes first, the slot name second. The helper returns a
`*sdk.Slot`, which a template cannot range over directly, so
the iteration is over `.Items`.

Each item is rendered with `render`, which dispatches on the
item's `Kind()` to whichever template owns it. That is what lets
a contributor ship its own emit kind and its own template rather
than handing over a bare expression — the host's template never
learns what a contribution looks like. A slot whose entries are
uniformly expressions can use `renderExpr` instead and declare
`sdk.EmitKindExpr` as its element kind, trading that flexibility
for a kind check at append time.

Two constraints that bite the moment a second contributor
appears, both learned by building this:

- **Template *filenames* must be unique across every plugin in
  the pipeline**, not just `define` names. Templates are
  registered under their base filename as well, so three
  contributors each shipping `entry.tmpl` collide. Name the file
  for the plugin: `authgen.entry.tmpl`.
- **Contributors must return `nil` from `TemplateFuncs`** unless
  they have a helper of their own. The shared Go helpers are
  already merged into the backend's overrideable funcmap, so
  returning them re-registers existing names — a Build-time
  `ErrTemplateFuncCollision`, which means two plugins that both
  did it could never appear in one pipeline.

### Contributor plugins

The three cross-cutters now declare a dependency on
middlewaregen via `Requires`, and contribute expressions
naming their middleware wrappers into the `chain` slot:

```go
// authgen
func (*Plugin) Provides() []string { return []string{"http.auth"} }
func (*Plugin) Requires() []string { return []string{"http.middleware"} }

func (p *Plugin) Generate(ctx *sdk.GeneratorContext) error {
    for _, stack := range ctx.Reader.EmitOfKind(middlewaregen.Kind).Slice() {
        wrapCall := sdk.NewExternal("myco/middleware", "WrapAuth")
        stack.(*middlewaregen.MiddlewareStack).Chain().Append(
            wrapCall,
            c.Provenance("http.auth.middleware"),
        )
    }
    return nil
}
```

```go
// metricgen
func (*Plugin) Provides() []string { return []string{"http.metric"} }
func (*Plugin) Requires() []string {
    return []string{"http.middleware", "http.auth"}
}
// Contributes sdk.NewExternal("myco/middleware", "WrapMetrics") to chain
```

```go
// tracegen
func (*Plugin) Provides() []string { return []string{"http.trace"} }
func (*Plugin) Requires() []string {
    return []string{"http.middleware", "http.metric"}
}
// Contributes sdk.NewExternal("myco/middleware", "WrapTrace") to chain
```

Two things to notice:

1. **Each contributor declares `http.middleware` in
   `Requires`.** The capability topo sees the dependency and
   schedules every contributor strictly after middlewaregen has
   emitted the `MiddlewareStack`. Without that declaration, a
   contributor might run first, find no `MiddlewareStack` to
   append to, and silently no-op.

2. **Contributors also require each other.** `metricgen
   Requires: ["http.middleware", "http.auth"]` says "I depend
   on middlewaregen AND I want to land after authgen." Without
   the inter-cross-cutter dependency, append order within the
   slot is determined by registration order — non-deterministic
   across builds when registration order varies.

### The rendered output

The custom slot composes into the middleware chain function:

```go
// — emitted by middlewaregen, populated by auth/metric/trace —
func (h *CreateUserHandler) middleware(handler http.Handler) http.Handler {
    handler = middleware.WrapAuth(handler)
    handler = middleware.WrapMetrics(handler)
    handler = middleware.WrapTrace(handler)
    return handler
}
```

The three cross-cutters no longer need to contribute into
`ServeHTTP.prebody`; the middleware function does it once at
handler construction time. handlergen's `ServeHTTP` then
delegates through the chain:

```go
func (h *CreateUserHandler) ServeHTTP(
    w http.ResponseWriter, r *http.Request,
) {
    h.middleware(h.dispatch).ServeHTTP(w, r)
}

func (h *CreateUserHandler) dispatch(
    w http.ResponseWriter, r *http.Request,
) {
    // — handlergen's per-request body —
}
```

### The dependency graph

```
                       middlewaregen
                       (owns MiddlewareStack
                        + "chain" slot)
                            ▲ ▲ ▲
                            │ │ │
                Requires    │ │ │ Requires
            ┌───────────────┘ │ └───────────────┐
            │                 │                 │
       authgen          metricgen          tracegen
       (Provides:        (Provides:        (Provides:
        http.auth)       http.metric)      http.trace)
            ▲                 ▲
            │                 │
            └──── Requires ───┘
                              ▲
                              │ Requires
                              │
                          tracegen

Within MiddlewareStack.chain (capability-topo order):
   authgen    →  middleware.WrapAuth
   metricgen  →  middleware.WrapMetrics
   tracegen   →  middleware.WrapTrace
```

The framework's resolver topologically sorts the contributors:
authgen has no peer dependency, so it lands first. metricgen
declares `Requires: ["http.middleware", "http.auth"]`, so it
runs after authgen. tracegen declares
`Requires: ["http.middleware", "http.metric"]`, so it runs
after metricgen. Append order within the slot reflects the
contributor order, producing deterministic chain composition.

### Failure modes the framework catches

**Missing dependency.** If authgen forgets to declare
`Requires: "http.middleware"`, the resolver may schedule
authgen before middlewaregen. authgen's
`ctx.Reader.EmitOfKind(...)` query returns an empty slice;
authgen silently no-ops. The framework surfaces this as an
Info diagnostic naming the unresolved capability — visible in
verbose mode but not a build failure.

**Cyclic dependency.** If authgen declares `Requires:
"http.metric"` and metricgen declares `Requires: "http.auth"`,
the capability resolver detects the cycle and fails Build with
a positioned diagnostic naming both plugins.

**Slot kind mismatch.** A plugin trying to
`Chain().Append(stmt)` (a `*sdk.Stmt`) when the slot was
declared `sdk.EmitKindExpr` is rejected at append time with
`emit.ErrSlotKindMismatch`. The error pinpoints the offending
plugin (via the Provenance argument) so the user knows which
plugin needs fixing.

**Duplicate contribution.** Slot semantics treat appends as
ordered; appending the same contribution twice produces two
chain entries. The conformance suite's determinism check
catches non-deterministic contribution behaviour but does not
de-duplicate identical-but-distinct contributions — that's a
deliberate choice (the same wrapper could legitimately appear
twice).

### Why custom slots over standard ones

Standard slots are the right tool when contributors don't need
to coordinate with each other or with a specific upstream.
Cross-cutting weavers stamping debug-logs / audit-trails into
every method's `prebody` are the canonical case.

Custom slots are the right tool when:

- **A specific upstream plugin owns the host concept.** Only
  middlewaregen knows what a middleware stack is; only it can
  define the slot's contract.
- **The slot's element kind is non-trivial.** Standard slots
  accept any statement; a custom slot can declare "this slot
  takes only `*sdk.Expr` values" or "this slot takes only the
  plugin-defined `Middleware` kind", catching wrong appends at
  append time.
- **Plugin ordering matters within the slot.** Capability
  topology orders contributors deterministically; standard
  slots fall back to registration order when no `Requires` is
  declared.
- **The template's slot semantics differ from "render in a
  block".** The middleware-chain template wraps each
  contribution in `handler = X(handler)`; standard prebody
  contributions render directly as statements.

In a mature plugin ecosystem, custom slots are how foundation
plugins expose their composition surface. handlergen could
expose `request-validators`, `response-encoders`,
`error-mappers` as named slots on its `HandlerSpec` emit kind;
each becomes a coordination point for ecosystem plugins to
plug into without any direct coupling between contributors.

## Where to take this

- **`reference/auditweaver`** and **`reference/debugweaver`**
  are the simplest real cross-cutting weavers in-tree — read
  them next.
- **[templates.md](templates.md)** covers the template surface
  in depth: the kind/template name contract, the funcmap, the
  extension / override mechanism.
- **[conformance.md](conformance.md)** describes the per-plugin
  test suites; every plugin in a multi-generator pipeline still
  runs its conformance suite in isolation.
- For a slimmer real example, `reference/registrygen` ships a
  plugin-defined emit kind + template + cross-cutting slot
  contribution in ~280 lines.
