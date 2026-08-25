// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package shape is the framework for classifying callable
// signatures (free functions and methods) into named shapes that
// downstream generators consume. The package owns the universal
// contract — the shape triple plus the contract- and mixin-axis
// keys, the `+gen:shape` directive and the umbrella [Plugin] — but
// knows nothing about any individual shape. Each shape (Reader,
// Writer, Lifecycle, …) lives in its own sub-package alongside this
// one and exports a [Detector] the consumer composes in.
//
// Signature primitives are not part of that contract. A detector
// composes its query from the language package its [DetectFunc] is
// registered under — [go.thesmos.sh/eidos/lang/golang] for Go —
// which is where every Go-speaking part of a pipeline already reads
// the same questions from.
//
// # The contract
//
// A shape stamp is three meta keys on a callable's bag:
//
//   - `shape`            — the canonical name (string, e.g. "reader")
//   - `shape.key_type`   — qualified type of the key/input parameter, when applicable
//   - `shape.value_type` — qualified type of the value/output, when applicable
//
// The triple is the universal contract — same across every source
// language. A Go `func(ctx, K) (V, error)` and a hypothetical
// Rust `fn(&self, K) -> Result<V, E>` both surface as
// `shape = "reader"`; downstream consumers branch on the stamp
// without caring about the source frontend.
//
// # Multi-language detection
//
// Detection logic differs per source language. A shape sub-package
// exports a [Detector] whose [Detector.Detect] map carries one
// [DetectFunc] per supported frontend:
//
//	func Detector() shape.Detector {
//	    return shape.Detector{
//	        Name: "reader",
//	        Detect: map[string]shape.DetectFunc{
//	            golang.Language:     detectGolang,
//	            protobuf.Language:   detectProtobuf,
//	        },
//	    }
//	}
//
// The umbrella plugin asks each package what language it was written
// in through [sdk.LanguageOf] and dispatches to the matching
// [DetectFunc]. The answer is the name the producing frontend
// stamped, which is the same namespace a backend answers to — so the
// key is spelled through the language package's own constant rather
// than as a literal, and a detector cannot register under a name no
// frontend produces and then silently never run.
//
// Declarations in a language with no registered entry are skipped
// without stamping — permissive by construction, because a catalog
// covering one language should not fail a run that also parses
// another.
//
// # Composition
//
// The umbrella [Plugin] takes every shape's [Detector] through
// [Plugin.Detectors] and runs them in registration order on every
// callable in the store:
//
//	pipe.WithAnnotator(shape.New().Detectors(
//	    reader.Detector(),
//	    writer.Detector(),
//	    lifecycle.Detector(),
//	))
//
// One plugin owns the `+gen:shape` directive schema, one walks
// the node graph (via the framework's [plugin.Walk] hook
// dispatcher), one applies the override-then-detect cascade.
// There is no separate directive plugin to register and no risk
// of duplicate-directive registration when the consumer wants
// more than one shape.
//
// # User overrides
//
// A `+gen:shape <name>` directive on a callable wins over every
// detector. The umbrella plugin checks the directive list first
// per callable; on a hit, the matching meta keys are stamped and
// the detector cascade is skipped via the "already stamped" guard.
//
// # Contracts
//
// A [Contract] is a protocol spanning several callables — a `tx`
// with its begin/commit/rollback, a `persister` with its writer
// and reader. Membership is authored, never detected: a
// `+gen:contract <name> role=<role> [<k>=<v>]…` directive on each
// participant. Contract stamps are orthogonal to the shape triple;
// a callable routinely carries both.
//
// The umbrella plugin stamps four keys, all attributed to
// `shape.contract`:
//
//   - `shape.contracts` — string list; the contract names this
//     callable belongs to, in the order the directives were
//     written, de-duplicated. This is the entry point: read it
//     first, then read the per-contract keys below for each name.
//     Absent on callables carrying no `+gen:contract` directive.
//   - `shape.contract.<name>.role` — string; which role in
//     `<name>`'s vocabulary this callable fills, verbatim from
//     `role=`. [Contract.Roles] is the accepted vocabulary; a role
//     outside it is stamped anyway and diagnosed by the [Resolver],
//     so consumers must tolerate an unknown value rather than
//     assume validity.
//   - `shape.contract.<name>.partner.<role>` — string; the
//     callable filling `<role>` for this specific participant. The
//     value changes between passes: the umbrella plugin writes the
//     raw name as authored, then the [Resolver] rewrites it to a
//     qualified name. Consumers running after the refinement
//     bucket see qualified names.
//   - `shape.contract.<name>.param.<key>` — string; a directive
//     argument for a key declared in [Contract.Params] (`cas
//     version=Version`, `rate-limit limit=100`). Whether the value
//     is rewritten depends on the key's [ParamKind]: a [KindOpaque]
//     value stays verbatim, while the resolvable kinds are rewritten
//     to qualified names in the refinement bucket exactly as a
//     partner is. A key carrying a [Param.Role] is a param only on
//     directives hosted by that role; elsewhere the same key is read
//     as a partner reference.
//
// The [Resolver] also writes these keys in the reverse direction.
// When it resolves a partner reference, it back-stamps the partner
// with the contract name, its inferred role, and the pointer back
// to the host, attributed to `shape.contract.resolver`. A callable
// can therefore carry contract stamps it never declared itself,
// because a sibling named it.
//
// The `<name>` and `<role>` path components come from the
// registered [Contract] and the authored directive, so these three
// key families are constructed at run time by [ContractRoleKey],
// [ContractPartnerKey] and [ContractParamKey] rather than declared
// as constants. The `shape.contract.` prefix is the stable part.
//
// # Mixins
//
// A [Mixin] is one orthogonal invariant layered on top of whatever
// structural shape a callable already has — atomic, idempotent,
// monotonic. Unlike a contract it binds nothing to anything else;
// it decorates a single callable, authored as
// `+gen:mixin <name>… [<k>=<v>]…`.
//
// Two keys, both attributed to `shape.mixin`:
//
//   - `shape.mixins` — string list; the mixin names decorating
//     this callable, in the order written, de-duplicated. One
//     directive may name several mixins and several directives
//     stack, so this list accumulates across the whole doc
//     comment. Names with no registered [Mixin] are diagnosed and
//     never stamped, so every entry here is a mixin the pipeline
//     can describe.
//   - `shape.mixin.<name>.<param>` — string; one KV parameter of
//     the named mixin. Values are opaque by default; for keys
//     listed in [KindCallable] the [Resolver] rewrites the
//     value from a raw sibling name to a qualified name, the same
//     two-pass treatment contract partners get. Parameters are
//     dropped (with a diagnostic) when a single directive pairs
//     them with more than one mixin name, because the plugin will
//     not guess which mixin owns them.
//
// As with contracts, `<name>` and `<param>` are authored, so the
// per-mixin keys are built on demand by [MixinParamKey] under the
// stable `shape.mixin.` prefix.
//
// # Traversal
//
// [Plugin.Annotate] walks the packages the [sdk.StoreReader] exposes
// and dispatches over every callable each one holds.
//
// Through the Reader rather than the framework's [sdk.Walk] helper,
// which iterates the store directly: reads the Reader captures
// compose the plugin's cache key, and a read that goes around it is
// one the cache cannot invalidate on — the source changes, the
// fingerprint does not, and the next run serves stamps derived from
// declarations that have since moved.
//
// Per package rather than over the store's flat buckets, because the
// language a declaration is read with is a fact about the package
// that produced it; walking packages hands each callable its
// language on the way past.
//
// All four method-carrying declarations are covered. A struct and an
// interface are the obvious two, and Go attaches methods to any
// defined type — so `type Weekday int` carries them on an
// [sdk.Alias], and the same declaration moves them onto an
// [sdk.Enum] the moment a const block coalesces it.
//
// # Where the boundary sits
//
// The shape library handles every concern except the shape's own
// detection logic and name:
//
//   - It owns the `+gen:shape` directive schema and the override
//     pre-stamp pass.
//   - It reaches every callable in every package the run loaded.
//   - It looks up the [DetectFunc] for the language the package was
//     written in.
//   - It applies the already-stamped skip.
//   - It stamps the three meta keys on a positive [Match].
//
// Per-shape packages contribute only the [DetectFunc] bodies and
// the canonical name they stamp. The shape vocabulary is the
// union of registered shape packages; nothing in this library
// enumerates the world.
package shape
