// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk_test

import (
	"errors"
	"testing"

	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// TestStoreAliasesPreserveIdentity pins the store handles as
// aliases.
//
// These are the types a plugin names to factor its own code: the
// context hands it a *store.Store and a *store.Reader, and a
// helper it extracts must accept exactly those. A wrapper would
// make the façade spelling unusable at the only call site that
// matters.
//
// The generic aliases carry the extra risk. `Query[T]` and
// `Bucket[T]` are generic type aliases, a newer language feature
// than the rest of this package relies on; the assignments below
// are what would catch a build where they silently degraded into
// distinct instantiations.
//
//nolint:staticcheck // intentional redundant typing — the redundancy is the test
func TestStoreAliasesPreserveIdentity(t *testing.T) {
	t.Parallel()

	t.Run("handles alias to the store package", func(t *testing.T) {
		t.Parallel()
		var s1 *sdk.Store
		var s2 *store.Store = s1
		_ = s2
		var r1 *sdk.StoreReader
		var r2 *store.Reader = r1
		_ = r2
	})

	t.Run("views alias to the store package", func(t *testing.T) {
		t.Parallel()
		var n1 *sdk.NodeView
		var n2 *store.NodeView = n1
		_ = n2
		var e1 *sdk.EmitView
		var e2 *store.EmitView = e1
		_ = e2
		var p1 sdk.PendingOriginSlot
		var p2 store.PendingOriginSlot = p1
		_ = p2
	})

	t.Run("generic accessors alias to the store package", func(t *testing.T) {
		t.Parallel()
		var q1 *sdk.Query[*node.Struct]
		var q2 *store.Query[*node.Struct] = q1
		_ = q2
		var b1 *sdk.Bucket[*node.Method]
		var b2 *store.Bucket[*node.Method] = b1
		_ = b2
	})
}

// TestStoreAccessorsReturnFacadeTypes proves the aliases line up
// with what the pipeline actually hands a plugin, rather than
// merely with the types the store declares.
//
// The store is constructed here through the underlying package on
// purpose: a plugin has no business making one, so the façade does
// not re-export the constructor, and this test stands in for the
// pipeline that would.
//
//nolint:staticcheck // intentional redundant typing — the redundancy is the test
func TestStoreAccessorsReturnFacadeTypes(t *testing.T) {
	t.Parallel()

	s := store.New()
	reader := store.NewReader(s)

	t.Run("the reader hands back facade-typed queries", func(t *testing.T) {
		t.Parallel()
		var q *sdk.Query[*sdk.Struct] = reader.Structs()
		if q == nil {
			t.Fatal("Reader.Structs returned nil")
		}
		if got := q.Count(); got != 0 {
			t.Fatalf("empty store reported %d structs", got)
		}
	})

	t.Run("the views hand back facade-typed buckets", func(t *testing.T) {
		t.Parallel()
		var nodes *sdk.NodeView = s.Nodes()
		var emits *sdk.EmitView = s.Emit()
		var structs *sdk.Bucket[*sdk.Struct] = nodes.Structs()
		var emitStructs *sdk.Bucket[*sdk.EmitStruct] = emits.Structs()
		if structs.Len() != 0 || emitStructs.Len() != 0 {
			t.Fatalf("empty store reported %d source and %d emit structs",
				structs.Len(), emitStructs.Len())
		}
	})

	t.Run("pending origin slots come back facade-typed", func(t *testing.T) {
		t.Parallel()
		var pending []sdk.PendingOriginSlot = s.Emit().PendingOriginSlots()
		if len(pending) != 0 {
			t.Fatalf("empty store reported %d pending slots", len(pending))
		}
	})
}

// TestStoreSentinelsAreDistinct pins the store's failure modes as
// re-exported and as separate answers.
//
// The pair that matters is duplicate-versus-frozen: they call for
// opposite fixes — a naming bug in the generator against a write
// in the wrong phase — and a façade that collapsed them would
// leave a plugin author unable to tell which they had.
func TestStoreSentinelsAreDistinct(t *testing.T) {
	t.Parallel()

	t.Run("each aliases its store sentinel", func(t *testing.T) {
		t.Parallel()
		pairs := []struct {
			name string
			got  error
			want error
		}{
			{"ErrDuplicateEntity", sdk.ErrDuplicateEntity, store.ErrDuplicateEntity},
			{"ErrDuplicateQName", sdk.ErrDuplicateQName, store.ErrDuplicateQName},
			{"ErrFrozen", sdk.ErrFrozen, store.ErrFrozen},
			{"ErrNilEntry", sdk.ErrNilEntry, store.ErrNilEntry},
			{"ErrUnknownSlotName", sdk.ErrUnknownSlotName, store.ErrUnknownSlotName},
		}
		for _, pair := range pairs {
			if !errors.Is(pair.got, pair.want) {
				t.Errorf("sdk.%s does not match its store sentinel", pair.name)
			}
		}
	})

	t.Run("a qualified-name clash is still a duplicate", func(t *testing.T) {
		t.Parallel()
		// The wrapping is the contract a plugin relies on: catch
		// the general case, then narrow.
		if !errors.Is(sdk.ErrDuplicateQName, sdk.ErrDuplicateEntity) {
			t.Error("ErrDuplicateQName no longer wraps ErrDuplicateEntity")
		}
	})

	t.Run("a frozen view is not a duplicate", func(t *testing.T) {
		t.Parallel()
		if errors.Is(sdk.ErrFrozen, sdk.ErrDuplicateEntity) {
			t.Error("ErrFrozen must not match ErrDuplicateEntity")
		}
	})
}

// TestPendingFilters pins that the façade's two declared functions
// forward rather than reimplement.
//
// Everything else in this package is a type alias, which cannot
// diverge. Go has no alias form for a generic function, so these two
// are declarations — and a declaration is a thing that can drift.
func TestPendingFilters(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T) (*sdk.EmitView, sdk.Node) {
		t.Helper()
		v := store.New().Emit()
		origin := &node.Struct{Name: "User"}
		err := v.AppendOriginSlot(
			origin, "top", &sdk.EmitStruct{Name: "UserStub"}, sdk.EmitProvenance{})
		if err != nil {
			t.Fatalf("AppendOriginSlot: %v", err)
		}
		return v, origin
	}

	t.Run("PendingOfType agrees with the store's own", func(t *testing.T) {
		t.Parallel()
		v, origin := build(t)
		var seen int
		for got, item := range sdk.PendingOfType[*sdk.EmitStruct](v) {
			seen++
			if got != origin || item.Name != "UserStub" {
				t.Fatalf("yielded %+v, %+v", got, item)
			}
		}
		if seen != 1 {
			t.Fatalf("yielded %d contributions, want 1", seen)
		}
	})

	t.Run("PendingByOrigin agrees with the store's own", func(t *testing.T) {
		t.Parallel()
		v, origin := build(t)
		got := sdk.PendingByOrigin[*sdk.EmitStruct](v)
		if len(got) != 1 || got[origin].Name != "UserStub" {
			t.Fatalf("PendingByOrigin = %+v", got)
		}
	})
}
