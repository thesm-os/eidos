// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import (
	"go.thesmos.sh/eidos/emit"
	emitbuilder "go.thesmos.sh/eidos/emit/builder"
)

// BaseEmit is the canonical [emit.BaseEmit] every plugin
// embeds in its plugin-defined emit values. Re-exported here
// so plugin authors only need a single `sdk` import for the
// common emit-value plumbing.
type BaseEmit = emit.BaseEmit

// EmitNode is the [emit.Node] interface plugin-defined emit
// values satisfy. The conventional `var _ EmitNode = (*MyValue)(nil)`
// compile-time assertion uses this alias instead of reaching
// into the emit package directly.
type EmitNode = emit.Node

// EmitTarget is the [emit.Target] descriptor the routing
// layer composes for each emit value. Plugins typically
// construct a zero value (`sdk.EmitTarget{}`) and let the
// Layout phase fill in [emit.Target.Dir] / [emit.Target.Filename]
// / [emit.Target.Package] / [emit.Target.ImportPath] from
// the contribution's origin + project / per-plugin / CLI
// overrides.
type EmitTarget = emit.Target

// OutputPackageSetter is the optional interface an emit value
// implements to receive the resolved package path of each of its
// plugin's other outputs, keyed by output tag.
//
// A generator cannot know its own sibling output's package during
// Generate — the Layout phase decides it — so a value that names a
// symbol in the plugin's other file implements this and rebuilds
// the reference when Layout calls back:
//
//	func (t *Tests) SetOutputPackages(byTag map[string]string) {
//	    if path := byTag[""]; path != "" {
//	        t.SubjectRef = sdk.NewExternal(path, t.TypeName)
//	    }
//	}
//
// Layout calls it at most once per value and may pass a partial
// map: a run that recorded routing errors reaches dispatch with
// some tags missing, and the primary tag can be present-but-empty.
// Index defensively.
//
// Re-exported because it belongs to the same family as the
// optional plugin capabilities above — an interface the framework
// asserts against a plugin's own type — and because the house
// convention is a compile-time assertion, which cannot be written
// without naming the interface:
//
//	var _ sdk.OutputPackageSetter = (*Tests)(nil)
type OutputPackageSetter = emit.OutputPackageSetter

// Slot is a named, kind-checked, provenance-tracked region on an
// emit value — the mechanism by which one plugin contributes into
// another's output without owning it.
//
// Plugins append through the [emit/builder] helpers rather than
// touching a Slot directly, so the contribution carries the
// provenance the backend orders and attributes by. The type is
// re-exported for the other half of the contract: a plugin
// *defining* a slot for others to fill hands one out.
type Slot = emit.Slot

// NewSlot re-exports [emit.NewSlot] — the constructor for a
// plugin-defined slot. An empty element kind accepts any kind,
// which is what a host wants when contributors bring their own
// emit kinds; a non-empty one rejects anything else at append
// time.
//
//nolint:gochecknoglobals // alias re-export of a stable factory.
var NewSlot = emit.NewSlot

// SlotHost is the interface an emit value implements to expose its
// named slots. Implemented by a plugin whose emit kind hosts
// contributions from other plugins; consumed by the backend when
// it renders them.
type SlotHost = emit.SlotHost

// Ref is the [emit.Ref] interface every type-side reference
// satisfies — [emit.BuiltinRef], [emit.ExternalRef], the
// composite shapes. Used as the parameter / return type for
// helpers that hand templates type-ref expressions to render
// through `renderType`.
type Ref = emit.Ref

// Expr is the [emit.Expr] type every value-side / callable
// reference is wrapped in. Used as the parameter / return
// type for helpers that hand templates callable expressions
// to render through `renderExpr`.
type Expr = emit.Expr

// NewExternal re-exports [emit.NewExternal] — the factory
// for fully-qualified package + name expressions the Go
// backend's `renderExpr` registers the import for
// automatically.
//
//nolint:gochecknoglobals // alias re-export of a stable factory.
var NewExternal = emit.NewExternal

// Provenance is the per-contribution provenance context
// returned by [NewProvenance]. Plugins call its `SetBy()` /
// `Provenance(id…)` methods to thread `set-by` /
// per-contribution-ID metadata onto each appended slot
// contribution.
type Provenance = emitbuilder.Context

// NewProvenance returns a fresh per-plugin provenance
// builder bound to setBy (the plugin's stable identifier)
// and the supplied default [EmitTarget]. Plugins typically
// pass the zero target — the Layout phase composes the real
// target from the contribution's origin.
//
//nolint:gochecknoglobals // alias re-export of a stable factory.
var NewProvenance = emitbuilder.For
