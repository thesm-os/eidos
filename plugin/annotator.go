// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugin

import (
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/store"
)

// Annotator is the role for plugins that read existing nodes and
// stamp metadata. Annotators run in the annotator phase, ordered
// across priority buckets and topo-sorted within a bucket. They
// must not add or remove nodes; the source-side store is
// structurally frozen between the frontend phase and the generator
// phase.
//
// Annotate receives an [AnnotatorContext] supplying the store and
// the diagnostic sink. The plugin reads the store through a
// [store.Reader] (the pipeline supplies one in M3 onwards) and
// stamps metadata via the [meta.Bag] on each node it visits.
type Annotator interface {
	Plugin

	// Annotate runs the plugin's annotation pass: read through
	// ctx.Reader, write through each node's EnsureMeta. Per-node
	// issues attach to ctx.Diag; fatal failures return a non-nil
	// error.
	Annotate(ctx *AnnotatorContext) error
}

// AnnotatorContext is the per-run context handed to every
// [Annotator]. The pipeline constructs one per annotator and
// supplies the shared store and diagnostic sink along with a
// per-plugin [store.Reader] for read-tracking.
type AnnotatorContext struct {
	// Store is the shared in-memory database.
	//
	// Present for the operations Reader does not cover — chiefly
	// the emit side, and store handles a plugin threads into its
	// own helpers. Reads that decide what the plugin stamps belong
	// on Reader; see that field.
	Store *store.Store

	// Reader is the per-plugin read-tracking handle, and the one
	// annotators query through.
	//
	// Captured reads compose the plugin's cache key, so a read that
	// bypasses Reader is a read the cache cannot invalidate on: the
	// source changes, the fingerprint does not, and the run serves
	// output that is stale but indistinguishable from current.
	// Nothing reports it.
	Reader *store.Reader

	// Diag is the diagnostic sink shared with every plugin in the
	// run. Per-node issues attach here.
	Diag *diag.Sink
}
