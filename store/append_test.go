// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store_test

import (
	"errors"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/store"
)

// srcStructFor returns a source struct carrying a position, which
// is what a queued value's diagnostics point at.
func srcStructFor(name string) *node.Struct {
	return &node.Struct{
		Name: name, Package: "example.com/x",
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/user.go", Line: 7}},
	}
}

// emitStructFor returns a queueable emit value.
func emitStructFor(name string) *emit.Struct {
	return &emit.Struct{Name: name, Package: "x"}
}

func TestAppend(t *testing.T) {
	t.Parallel()

	t.Run("queues a value against the origin's slot", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		src := srcStructFor("User")
		if err := s.Emit().AppendOrigin("gen", "top", src, emitStructFor("UserMock")); err != nil {
			t.Fatalf("Append: %v", err)
		}
		if got := s.Emit().PendingOriginSlots(); len(got) != 1 {
			t.Fatalf("queued %d contributions, want 1", len(got))
		}
	})

	t.Run("queues every value in one call", func(t *testing.T) {
		t.Parallel()
		// Appending a set in one call is what keeps a primary and its
		// companion from acquiring divergent copies of this logic.
		s := store.New()
		err := s.Emit().AppendOrigin("gen", "top", srcStructFor("User"),
			emitStructFor("A"), emitStructFor("B"))
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
		if got := s.Emit().PendingOriginSlots(); len(got) != 2 {
			t.Fatalf("queued %d contributions, want 2", len(got))
		}
	})

	t.Run("stamps provenance naming the plugin", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		_ = s.Emit().AppendOrigin("mockgen", "top", srcStructFor("User"), emitStructFor("UserMock"))
		got := s.Emit().PendingOriginSlots()[0]
		if got.Prov.SetBy != "mockgen" {
			t.Fatalf("SetBy = %q, want mockgen", got.Prov.SetBy)
		}
	})

	t.Run("derives the provenance id from kind and origin", func(t *testing.T) {
		t.Parallel()
		// The id is what a later plugin targets when positioning its
		// own contribution relative to this one.
		s := store.New()
		_ = s.Emit().AppendOrigin("gen", "top", srcStructFor("User"), emitStructFor("UserMock"))
		id := s.Emit().PendingOriginSlots()[0].Prov.ID
		if !strings.HasSuffix(id, ".User") {
			t.Fatalf("Provenance.ID = %q, want it to name the origin", id)
		}
		if !strings.Contains(id, "struct") {
			t.Fatalf("Provenance.ID = %q, want it to name the kind", id)
		}
	})

	t.Run("skips a nil value without failing the batch", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		err := s.Emit().AppendOrigin("gen", "top", srcStructFor("User"),
			emitStructFor("A"), nil, emitStructFor("B"))
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
		if got := s.Emit().PendingOriginSlots(); len(got) != 2 {
			t.Fatalf("queued %d, want the two non-nil values", len(got))
		}
	})

	t.Run("surfaces a rejected append naming the slot", func(t *testing.T) {
		t.Parallel()
		// An empty slot name is a plugin bug, and the error has to say
		// which append failed — a bare store error names neither the
		// value nor the slot.
		s := store.New()
		err := s.Emit().AppendOrigin("gen", "", srcStructFor("User"), emitStructFor("A"))
		if err == nil {
			t.Fatalf("empty slot name accepted")
		}
		if !errors.Is(err, store.ErrUnknownSlotName) {
			t.Fatalf("error = %v, want ErrUnknownSlotName", err)
		}
	})

	t.Run("stops at the first rejected value", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		err := s.Emit().AppendOrigin("gen", "top", nil, emitStructFor("A"))
		if err == nil {
			t.Fatalf("nil origin accepted")
		}
	})
}

func TestAppendAs(t *testing.T) {
	t.Parallel()

	t.Run("identifies the contribution by the supplied name", func(t *testing.T) {
		t.Parallel()
		// Package-scoped output is anchored on whichever declaration
		// the package offered but is about the package. Deriving the
		// id from the anchor would move a plugin's target identifier
		// when an unrelated type is renamed.
		s := store.New()
		err := s.Emit().AppendOriginAs("gen", "top", "users", srcStructFor("Anchor"), emitStructFor("Checks"))
		if err != nil {
			t.Fatalf("AppendAs: %v", err)
		}
		id := s.Emit().PendingOriginSlots()[0].Prov.ID
		if !strings.HasSuffix(id, ".users") {
			t.Fatalf("Provenance.ID = %q, want it to name the package", id)
		}
	})

	t.Run("still anchors on the supplied origin", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		anchor := srcStructFor("Anchor")
		_ = s.Emit().AppendOriginAs("gen", "top", "users", anchor, emitStructFor("Checks"))
		if got := s.Emit().PendingOriginSlots()[0].Origin; got != anchor {
			t.Fatalf("Origin = %v, want the anchor", got)
		}
	})
}

func TestAppend_UnnamedOrigin(t *testing.T) {
	t.Parallel()

	t.Run("falls back to the kind when the origin has no name", func(t *testing.T) {
		t.Parallel()
		// A bare `<kind>.` anchor is one no later plugin can target
		// unambiguously once a second nameless origin appears.
		s := store.New()
		err := s.Emit().AppendOrigin("gen", "top",
			&node.File{Name: "x.go"}, emitStructFor("Gen"))
		if err != nil {
			t.Fatalf("Append: %v", err)
		}
		id := s.Emit().PendingOriginSlots()[0].Prov.ID
		if strings.HasSuffix(id, ".") {
			t.Fatalf("Provenance.ID = %q, want a non-empty anchor", id)
		}
	})
}
