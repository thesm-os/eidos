// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
)

// TestCollectContributors_SlotItems covers the walk into slot items.
//
// A plugin-defined emit kind has no store bucket, so the backend
// never receives one directly — it arrives as an item in some host's
// slot. Reading only the enclosing slot's provenance attributes
// whoever appended the node and stops there, which left a plugin
// contributing into that node's own slots out of the header of a
// file it substantially wrote.
func TestCollectContributors_SlotItems(t *testing.T) {
	t.Parallel()

	// hostWithInner builds the reported shape: an outer host whose
	// slot holds an inner host, the two appends attributed to
	// different plugins.
	hostWithInner := func() (*emit.Struct, *emit.Struct) {
		inner := &emit.Struct{Name: "Contract", Package: "x"}
		outer := &emit.Struct{Name: "Outer", Package: "x"}
		if err := outer.Slot("body").Append(inner, emit.Provenance{SetBy: "suite"}); err != nil {
			t.Fatalf("append inner: %v", err)
		}
		return outer, inner
	}

	t.Run("a contributor into a slot item's own slot is attributed", func(t *testing.T) {
		t.Parallel()
		outer, inner := hostWithInner()
		row := &emit.Struct{Name: "Row", Package: "x"}
		if err := inner.Slot("rows").Append(row, emit.Provenance{SetBy: "model"}); err != nil {
			t.Fatalf("append row: %v", err)
		}
		got := map[string]bool{}
		collectContributors(outer, got)
		if !got["model"] {
			t.Fatalf("model contributed into the item's own slot and is absent; got %v", got)
		}
		if !got["suite"] {
			t.Fatalf("the appender of the item should still be attributed; got %v", got)
		}
	})

	t.Run("a slot item's own SetBy is attributed", func(t *testing.T) {
		t.Parallel()
		outer, inner := hostWithInner()
		inner.SetByName = "model"
		got := map[string]bool{}
		collectContributors(outer, got)
		if !got["model"] {
			t.Fatalf("the item's own SetBy is absent; got %v", got)
		}
	})

	t.Run("a cycle through a slot item terminates", func(t *testing.T) {
		t.Parallel()
		// A slot's Owner points back at its host, and nothing stops a
		// plugin appending a node that reaches one. The method-only
		// walk this replaced could not cycle; the item walk can.
		outer, inner := hostWithInner()
		if err := inner.Slot("back").Append(outer, emit.Provenance{SetBy: "loop"}); err != nil {
			t.Fatalf("append cycle: %v", err)
		}
		done := make(chan map[string]bool, 1)
		go func() {
			got := map[string]bool{}
			collectContributors(outer, got)
			done <- got
		}()
		select {
		case got := <-done:
			if !got["loop"] {
				t.Fatalf("cycle walked but the contributor was lost; got %v", got)
			}
		case <-t.Context().Done():
			t.Fatal("collectContributors did not terminate on a cyclic slot graph")
		}
	})
}
