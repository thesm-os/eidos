// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugin

import "go.thesmos.sh/eidos/node"

// BeforeNodesHook is the optional pre-iteration hook. When an
// annotator value passed to [Walk] implements this interface, the
// dispatcher invokes BeforeNodes once before any per-kind hook
// fires. Use it for visitor state initialisation that depends on
// the live [AnnotatorContext] rather than the plugin's struct
// fields.
type BeforeNodesHook interface {
	BeforeNodes(ctx *AnnotatorContext)
}

// AfterNodesHook is the optional post-iteration hook. When an
// annotator value passed to [Walk] implements this interface, the
// dispatcher invokes AfterNodes once after every per-kind hook
// has fired. Use it for aggregate diagnostics or final stamping
// that requires every node to have been visited first.
type AfterNodesHook interface {
	AfterNodes(ctx *AnnotatorContext)
}

// StructHook is the optional per-struct hook. [Walk] invokes
// OnStruct once per [node.Struct] in the store's Structs bucket,
// in stable insertion order.
type StructHook interface {
	OnStruct(ctx *AnnotatorContext, s *node.Struct)
}

// InterfaceHook is the optional per-interface hook. [Walk] invokes
// OnInterface once per [node.Interface] in the store's Interfaces
// bucket, in stable insertion order.
type InterfaceHook interface {
	OnInterface(ctx *AnnotatorContext, i *node.Interface)
}

// MethodHook is the optional per-method hook. [Walk] invokes
// OnMethod once per [node.Method] in the store's Methods bucket
// (which spans every struct- and interface-declared method), in
// stable insertion order. Methods on interfaces carry a nil
// [node.Method.Receiver]; hook implementations that care about
// the receiver shape must handle the absence explicitly.
type MethodHook interface {
	OnMethod(ctx *AnnotatorContext, m *node.Method)
}

// FunctionHook is the optional per-free-function hook. [Walk]
// invokes OnFunction once per [node.Function] in the store's
// Functions bucket, in stable insertion order. The hook fires
// only for standalone functions; methods reach their own
// [MethodHook] entry point.
type FunctionHook interface {
	OnFunction(ctx *AnnotatorContext, f *node.Function)
}

// Walk drives target's hook methods over every node in the
// annotator context's store. It is the canonical annotation-walk
// helper the spec's "Visitor helper" pattern refers to —
// annotators implement only the hooks they care about, and Walk
// introspects target for those hooks via interface assertions
// before dispatching.
//
// Iteration order:
//
//  1. [BeforeNodesHook.BeforeNodes] (if implemented).
//  2. Every [node.Struct] in stable insertion order via
//     [StructHook.OnStruct] (if implemented).
//  3. Every [node.Interface] in stable insertion order via
//     [InterfaceHook.OnInterface] (if implemented).
//  4. Every [node.Method] in stable insertion order via
//     [MethodHook.OnMethod] (if implemented). The Methods bucket
//     spans both struct- and interface-declared methods; method
//     hooks see every callable bound to a receiver.
//  5. Every [node.Function] in stable insertion order via
//     [FunctionHook.OnFunction] (if implemented). Free functions
//     only; methods reach their hook through step 4.
//  6. [AfterNodesHook.AfterNodes] (if implemented).
//
// Walk returns nil. Per-node failures attach to ctx.Diag rather
// than aborting the iteration — the spec's "completion regardless
// of error" contract for annotators. A non-nil error is reserved
// for catastrophic failures (none today; the signature carries
// the slot so future extensions can surface fatal cases).
func Walk(ctx *AnnotatorContext, target any) error {
	if h, ok := target.(BeforeNodesHook); ok {
		h.BeforeNodes(ctx)
	}
	if h, ok := target.(StructHook); ok {
		for s := range ctx.Store.Nodes().Structs().All() {
			h.OnStruct(ctx, s)
		}
	}
	if h, ok := target.(InterfaceHook); ok {
		for i := range ctx.Store.Nodes().Interfaces().All() {
			h.OnInterface(ctx, i)
		}
	}
	if h, ok := target.(MethodHook); ok {
		for m := range ctx.Store.Nodes().Methods().All() {
			h.OnMethod(ctx, m)
		}
	}
	if h, ok := target.(FunctionHook); ok {
		for f := range ctx.Store.Nodes().Functions().All() {
			h.OnFunction(ctx, f)
		}
	}
	if h, ok := target.(AfterNodesHook); ok {
		h.AfterNodes(ctx)
	}
	return nil
}
