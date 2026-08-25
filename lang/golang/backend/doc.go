// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package backend is eidos' Go-target backend. It renders [emit] graphs to
// gofmt-clean, byte-deterministic Go source through a template-driven
// pipeline with first-class plugin extension.
//
// # Quick start
//
// Register a backend instance on a pipeline builder:
//
//	pipeline.New().
//	    WithFrontend(golang.New()).            // (frontend package)
//	    WithBackend(golang.New()).
//	    Build()
//
// The backend identifies itself with [Name] (`backend.golang`) and [Language]
// (`golang`). Plugin authors use [Language] when implementing
// [plugin.TemplateProvider.Templates] to target this backend.
//
// # Rendering pipeline
//
// [Backend.Render] groups emit entities by their [emit.Target] and produces one
// file per non-empty Target through ctx.Sink. Each Target goes through:
//
//  1. Template execution against a per-Target template clone — `imp` calls
//     accumulate into a fresh [writer.ImportSet] so import tracking is
//     isolated per file.
//  2. One AST walk over the assembled body. Imports the body never
//     references are dropped; qualifiers no import binds surface as one Warn
//     each, after subtracting the package-scope names every target rendering
//     into the same package declares. The surviving imports are written as a
//     path-sorted block, grouped standard-library first.
//  3. [go/format.Source] for canonical Go formatting. Failure attaches an
//     Error diagnostic and falls back to the unformatted body so the user can
//     debug.
//  4. Header (item 1) + body (items 2–8) + footer (item 9) composition. The
//     footer carries the SHA-256 provenance hash over the body bytes actually
//     written. The header carries no timestamp so two runs over the same
//     emit graph produce byte-identical output, header and footer included.
//  5. Sink write. Write failures propagate as a wrapped error from Render —
//     they indicate I/O faults rather than content defects.
//
// Empty Targets (no decls and every slot empty after the pre-render pass) are
// filtered before render — they never enter the sink or the manifest. Prior
// artifacts at the same path are reclaimed by `eidos prune`.
//
// # Templates
//
// The backend ships eight kind-templates, one per renderable top-level emit
// kind:
//
//	emit.struct    emit.interface  emit.function
//	emit.method    emit.enum       emit.alias
//	emit.variable  emit.constant
//
// Template names match [emit.Node.Kind] verbatim; the `render` funcmap entry
// routes dispatch by `Node.Kind()`. [emit.File] is composed in Go (header,
// imports, slot ordering, footer) and never goes through `render`. The
// `fragment.` template-name prefix is reserved for future shared partials;
// plugin-defined templates using it are rejected at parse time with
// [ErrReservedTemplatePrefix]. Plugin-defined emit kinds follow the same
// convention — a plugin that exposes Kind `sagagen.saga` ships a template
// defining `sagagen.saga`.
//
// # Funcmap
//
// The canonical funcmap is computed at Build time and cached via
// [sync.OnceValue]. Reserved entries — dispatch helpers
// (`render`, `renderType`, `renderStmt`, `renderExpr`), canonical render
// helpers (`renderParams`, `renderReturns`, `renderReceiver`, slot-composition
// `render<Host><Slot>`), collision handlers (`imp`, `slot`), and metadata
// (`provenance`) — cannot be overridden; the override pass rejects them with
// [ErrReservedFuncName]. Overrideable leaf utilities (case conversion, string
// operations) sit alongside via [plugin.TemplateProvider.TemplateFuncs].
//
// Plugin contributions go through a two-pass merge per Render call:
//
//  1. Extensions ([plugin.TemplateProvider.TemplateFuncs]) — collected across
//     ctx.Plugins. Cross-plugin name collisions or collisions with any core
//     canonical entry fail Build with [ErrTemplateFuncCollision].
//  2. Overrides ([plugin.TemplateProvider.TemplateOverrides]) — applied across
//     ctx.Ordered in capability topological order. Reserved-name violations
//     fail with [ErrReservedFuncName]; each successful override emits an Info
//     diagnostic naming the winning plugin and the previous owner.
//
// # Slot composition
//
// Multiple generators contribute to the same file via two complementary
// mechanisms:
//
//   - Free-floating decls share an [emit.Target]; the backend renders them
//     at layout item 6 in capability-topo + `QName()` order. Duplicates
//     produce [ErrDuplicateEntity] naming both contributors.
//   - Explicit [emit.File] slots — `Top()`, `Init()`, `Bottom()` — accumulate
//     decls and statements across plugins. The backend renders Top above
//     free-floating decls (item 5), merges Init contributions into a single
//     `func init() { … }` body (item 7), and renders Bottom below (item 8).
//
// Per-host slots on structured kinds (`fields`, `methods`, `embeds`,
// `variants`, `prebody`, `postbody`, `params`, `tags`) merge similarly:
// typed direct content first, then slot contributions re-grouped by plugin
// topo. Named-entry slots (Methods, Fields, Variants) enforce
// [ErrDuplicateEntity] on cross-plugin name collisions; unnamed entries
// (Stmts in `prebody`, etc.) render in append order without duplicate check.
//
// # Headers, footers, provenance
//
// The header carries the canonical `DO NOT EDIT` marker plus optional
// `Source:`, `Plugins:`, and `Command:` lines. Sources are aggregated from
// emit-entity origins (union-then-sort); plugins that don't thread source
// provenance produce a synthetic `unknown` marker. The footer carries a
// two-line tail: a brand-prefixed end-of-content marker followed by a
// brand-prefixed `// <brand>:provenance <hash>` line carrying the SHA-256
// of the body bytes. Splitting them keeps each statement scannable on its
// own and lets tools grep `^// <brand>:provenance ` to recover the hash
// without parsing a composite line.
//
// Library embedders extend the envelope via [plugin.BackendContext] fields:
// `HeaderPrefix` / `HeaderSuffix` (additional lines before / after the
// standard block), `FooterSuffix` (extra trailing lines), `Brand` (default
// `eidos`), `Command`, and `SourcesOverride`.
//
// # Determinism
//
// The output contract is byte-identical across runs for identical input.
// Slot ordering is capability-topo + append-sequence; free-floating decls
// sort by capability-topo + QName(); headers carry no timestamp; the
// provenance hash is over the body bytes actually written. The `format.Source`
// pass is itself deterministic per input, so the success-vs-failure path is a
// property of the input — hash equality across runs is preserved regardless of
// which path the format pass took.
//
// Determinism now extends to the import block, which it did not while the
// goimports resolve pass ran: an unresolved reference sent that pass into the
// developer's module cache to choose an import, so the same emit graph produced
// different bytes on different machines. Nothing in the render path consults
// anything outside the emit graph.
//
// # Sentinel errors
//
// Build-time invariant violations and template-execution errors surface via
// exported sentinels callers compare with [errors.Is]:
//
//   - [ErrTemplateMissing] — no template registered for an entity's
//     [emit.Node.Kind].
//   - [ErrUnsupportedRef] — `renderType` called on a [emit.Ref] kind the
//     current funcmap can't render, or an unsupported [emit.TypeRef] target.
//   - [ErrUnsupportedExpr] — `renderExpr` called on an [emit.ExprKind] or
//     [emit.LiteralKind] variant the current funcmap can't render.
//   - [ErrUnsupportedStmt] — `renderStmt` called on an [emit.StmtKind]
//     variant the current funcmap can't render.
//   - [ErrMixedNamedParams] — a parameter list mixes named and unnamed
//     entries (forbidden by Go's grammar).
//   - [ErrDuplicateEntity] — a slot received two contributions producing
//     the same [emit.Node] `QName()`.
//   - [ErrNilHost] — a slot-composition funcmap helper received a nil host
//     entity.
//   - [ErrEmptyTarget] — a Target produced no renderable content after the
//     pre-render pass; the loop continues to the next Target.
//   - [ErrTemplateFuncCollision] — Pass 1 of the funcmap merge: two plugin
//     extensions collide, or an extension collides with a core canonical
//     entry.
//   - [ErrReservedFuncName] — Pass 2 of the funcmap merge: a plugin tried
//     to override a reserved canonical entry.
//   - [ErrTemplateNameCollision] — two plugins both define a template
//     under the same name.
//   - [ErrReservedTemplatePrefix] — a plugin defines a template under a
//     reserved name prefix (currently `fragment.`).
//   - [ErrSlotHostUnsupported] — the `slot` funcmap helper was invoked
//     against a value that doesn't implement [emit.SlotHost].
//
// # Concurrency
//
// A Backend instance is safe for concurrent use; [Backend.Render] holds no
// state across calls. The parent template tree is parsed once at construction
// and never mutated thereafter — every per-Target render clones it, binds
// per-Target funcmap closures, and accumulates per-Target ImportSet state in
// isolation. Plugin templates merge into the per-Render clone, not the
// parent, so concurrent Render calls remain race-free.
//
// Render dispatches those per-Target renders across one worker per CPU.
// Isolation is what makes that safe, and it was the design from the start;
// this only puts it to work. Scaling is close to linear — a 1000-target
// workspace renders in roughly a quarter of the sequential time on four
// cores.
//
// Determinism is preserved by separating rendering from emission. Workers
// return finalised bytes and a private diagnostic buffer; the caller then
// walks results in ByTarget().Keys() order, replays each buffer into
// ctx.Diag, and writes to the sink. So neither sink ordering nor diagnostic
// ordering depends on which worker finished first — which matters because
// `-diag-format json` makes the diagnostic sequence observable, and a Stdout
// sink makes the write sequence observable.
//
// The cost is memory: every target's rendered output is held until the write
// pass. For a code generator that is bounded by the size of what it emits,
// and the sequential loop already held one file at a time.
package backend
