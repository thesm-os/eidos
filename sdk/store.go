// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk

import "go.thesmos.sh/eidos/store"

// The store, re-exported as the types a plugin names — not as a
// way to make one.
//
// A plugin never constructs a store, a reader, or a read set: the
// pipeline hands it both on the phase context, and a plugin that
// built its own would query a graph nothing else can see and
// contribute reads no cache key ever hashes. So the constructors
// stay behind the [store] import, along with the indices and the
// read-set plumbing that only the pipeline and the cache layer
// have any business touching.
//
// What is here is the vocabulary a plugin needs to factor its own
// code: the handle types on the context, the two views, and the
// shapes their accessors return.

// Store is the shared in-memory graph, carried as the Store field
// on every phase context. Named by a plugin that factors a helper
// taking the store rather than the whole context.
//
// Prefer querying through [StoreReader]: a read through the store
// directly bypasses read tracking, so the plugin's cache key does
// not record it and a change to what it read will not invalidate
// its output.
type Store = store.Store

// StoreReader is the per-plugin read-tracking handle, carried as
// the Reader field on the annotator, generator, and backend
// contexts. Every terminal query through it records what was read,
// which is what lets the cache tell that a plugin stopped
// depending on something.
//
// Named with the Store prefix because a bare `Reader` in a Go
// facade reads as an io.Reader.
type StoreReader = store.Reader

// NodeView is the source side of the [Store] — frozen by the time
// any annotator or generator runs, so its shape is safe to hold
// across a phase.
type NodeView = store.NodeView

// EmitView is the output side of the [Store]. Mutable during the
// generator phase and frozen for the backend; a generator appends
// here and a later generator reads what earlier ones produced.
type EmitView = store.EmitView

// Query is the deferred, typed query a [StoreReader] accessor
// returns. Predicates accumulate through Where and nothing is
// iterated until a terminal call, so a helper may take a Query and
// narrow it without paying for a pass.
type Query[T any] = store.Query[T]

// Bucket is the insertion-ordered, qualified-name-keyed collection
// a [NodeView] or [EmitView] per-kind accessor returns. Iteration
// order is the frontend's insertion order, which is what makes a
// run's output byte-stable.
type Bucket[T any] = store.Bucket[T]

// PendingOriginSlot is one slot contribution that named an origin
// no generator has produced output for yet — the mechanism by
// which a cross-cutting generator contributes into a host it does
// not know has run.
//
// A host generator drains these from the emit view during its own
// Generate. Ignoring them is the silent failure the type exists to
// prevent: the contributing plugin reports success, and its
// contribution never reaches a file.
type PendingOriginSlot = store.PendingOriginSlot

// The failure modes the store returns to a plugin. Every one is
// reachable from a call a plugin makes — adding a package,
// appending into a slot — and they need telling apart: a duplicate
// name is usually the plugin emitting twice for one declaration,
// while a frozen view is the plugin writing in the wrong phase.
// Collapsing them into "add failed" leaves the author guessing.
var (
	// ErrDuplicateEntity is returned when an entry is already
	// recorded.
	ErrDuplicateEntity = store.ErrDuplicateEntity

	// ErrDuplicateQName is the [ErrDuplicateEntity] case where the
	// collision is on the qualified name. A generator emitting one
	// entity per declaration hits this when two declarations
	// project onto the same name — which is a naming bug in the
	// generator, not a duplicate call.
	ErrDuplicateQName = store.ErrDuplicateQName

	// ErrFrozen is returned by a write to a view whose phase has
	// closed. Reaching it means the write is in the wrong phase,
	// not that the value was wrong.
	ErrFrozen = store.ErrFrozen

	// ErrNilEntry is returned when an add presents nil.
	ErrNilEntry = store.ErrNilEntry

	// ErrUnknownSlotName is returned when an append names a slot
	// the host does not define — the usual failure when a
	// cross-cutting plugin and its host disagree about a slot's
	// name, which nothing else catches because the append is the
	// only place the two meet.
	ErrUnknownSlotName = store.ErrUnknownSlotName
)
