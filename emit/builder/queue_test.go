// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"errors"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/kind"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/emit/builder"
	"go.thesmos.sh/eidos/node"
)

// srcStruct returns a source struct carrying a position, which is
// what a queued value's diagnostics point at.
func srcStruct(name string) *node.Struct {
	return &node.Struct{
		Name: name, Package: "example.com/x",
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: "x/user.go", Line: 7}},
	}
}

// queued is a plugin-defined emit value, the shape [builder.Queue]
// exists to route.
type queued struct {
	emit.BaseEmit

	name string
}

func (q *queued) Kind() kind.Kind { return kind.Kind("test." + q.name) }

// recorder is a [builder.Appender] double capturing what a queue
// wrote, so a test asserts on the provenance rather than on the
// store's own bookkeeping.
type recorder struct {
	slots []string
	items []emit.Node
	provs []emit.Provenance
	fail  error
}

func (r *recorder) AppendOriginSlot(
	_ node.Node, slotName string, item emit.Node, prov emit.Provenance,
) error {
	if r.fail != nil {
		return r.fail
	}
	r.slots = append(r.slots, slotName)
	r.items = append(r.items, item)
	r.provs = append(r.provs, prov)
	return nil
}

func TestBase(t *testing.T) {
	t.Parallel()

	t.Run("carries origin, plugin and position together", func(t *testing.T) {
		t.Parallel()
		// A value missing any of the three has failures that name the
		// wrong source line, or no line at all.
		src := srcStruct("User")
		got := builder.Base(builder.For("mockgen"), src)
		if got.OriginNode != src {
			t.Errorf("OriginNode = %v, want the source struct", got.OriginNode)
		}
		if got.SetByName != "mockgen" {
			t.Errorf("SetByName = %q, want mockgen", got.SetByName)
		}
		if got.SourcePos != src.Pos() {
			t.Errorf("SourcePos = %v, want the origin's", got.SourcePos)
		}
	})

	t.Run("a nil origin still carries the plugin identity", func(t *testing.T) {
		t.Parallel()
		// Attribution is the half that survives: an unattributed
		// value is one `explain` cannot trace to a plugin at all.
		got := builder.Base(builder.For("mockgen"), nil)
		if got.SetByName != "mockgen" {
			t.Fatalf("SetByName = %q, want mockgen", got.SetByName)
		}
		if got.OriginNode != nil {
			t.Fatalf("OriginNode = %v, want nil", got.OriginNode)
		}
	})
}

func TestTagged(t *testing.T) {
	t.Parallel()

	t.Run("routes the base to the named output", func(t *testing.T) {
		t.Parallel()
		base := builder.Base(builder.For("gen"), srcStruct("User"))
		if got := builder.Tagged(base, "test"); got.OutputTagName != "test" {
			t.Fatalf("OutputTagName = %q, want test", got.OutputTagName)
		}
	})

	t.Run("leaves the original base untouched", func(t *testing.T) {
		t.Parallel()
		// A plugin building a primary and a companion derives the
		// second from the first; mutating in place would leave the
		// primary pointing at the companion's output.
		base := builder.Base(builder.For("gen"), srcStruct("User"))
		builder.Tagged(base, "test")
		if base.OutputTagName != "" {
			t.Fatalf("OutputTagName = %q, want the original untouched", base.OutputTagName)
		}
	})

	t.Run("preserves the rest of the base", func(t *testing.T) {
		t.Parallel()
		src := srcStruct("User")
		got := builder.Tagged(builder.Base(builder.For("gen"), src), "test")
		if got.OriginNode != src || got.SetByName != "gen" {
			t.Fatalf("Tagged dropped base fields: %+v", got)
		}
	})
}

func TestQueue(t *testing.T) {
	t.Parallel()

	t.Run("queues every value against the origin's slot", func(t *testing.T) {
		t.Parallel()
		// A plugin's outputs are a set that grows; appending them in one
		// call is what keeps a primary and its companion from acquiring
		// separate, divergent copies of this logic.
		rec := &recorder{}
		src := srcStruct("User")
		err := builder.Queue(rec, builder.For("gen"), "top", src,
			&queued{name: "double"}, &queued{name: "suite"})
		if err != nil {
			t.Fatalf("Queue = %v", err)
		}
		if len(rec.items) != 2 {
			t.Fatalf("appended %d values, want 2", len(rec.items))
		}
		if rec.slots[0] != "top" || rec.slots[1] != "top" {
			t.Fatalf("slots = %v, want both top", rec.slots)
		}
	})

	t.Run("the provenance id names the kind and the origin", func(t *testing.T) {
		t.Parallel()
		// What a later plugin targets when it positions its own
		// contribution relative to this one.
		rec := &recorder{}
		err := builder.Queue(rec, builder.For("gen"), "top", srcStruct("User"),
			&queued{name: "double"})
		if err != nil {
			t.Fatalf("Queue = %v", err)
		}
		if rec.provs[0].ID != "test.double.User" {
			t.Fatalf("ID = %q, want test.double.User", rec.provs[0].ID)
		}
		if rec.provs[0].SetBy != "gen" {
			t.Fatalf("SetBy = %q, want gen", rec.provs[0].SetBy)
		}
	})

	t.Run("a nil value is skipped rather than queued", func(t *testing.T) {
		t.Parallel()
		// A projection that declined to build one output should not
		// abort the others.
		rec := &recorder{}
		err := builder.Queue(rec, builder.For("gen"), "top", srcStruct("User"),
			nil, &queued{name: "double"})
		if err != nil {
			t.Fatalf("Queue = %v", err)
		}
		if len(rec.items) != 1 {
			t.Fatalf("appended %d values, want only the non-nil one", len(rec.items))
		}
	})

	t.Run("a nil origin is an error, not a panic", func(t *testing.T) {
		t.Parallel()
		// Reachable from a generator that filtered its work list
		// wrongly; one bad declaration should fail its own emit rather
		// than the run.
		err := builder.Queue(&recorder{}, builder.For("gen"), "top", nil,
			&queued{name: "double"})
		if !errors.Is(err, builder.ErrNilOrigin) {
			t.Fatalf("Queue = %v, want ErrNilOrigin", err)
		}
		err = builder.QueueAs(&recorder{}, builder.For("gen"), "top", nil, "id",
			&queued{name: "double"})
		if !errors.Is(err, builder.ErrNilOrigin) {
			t.Fatalf("QueueAs = %v, want ErrNilOrigin", err)
		}
	})

	t.Run("an append failure names the plugin, kind and origin", func(t *testing.T) {
		t.Parallel()
		// The three facts a reader needs to find the offending call
		// without a stack trace.
		sentinel := errors.New("slot rejected")
		rec := &recorder{fail: sentinel}
		err := builder.Queue(rec, builder.For("gen"), "top", srcStruct("User"),
			&queued{name: "double"})
		if !errors.Is(err, sentinel) {
			t.Fatalf("Queue = %v, want the appender's error wrapped", err)
		}
		for _, want := range []string{"gen", "test.double", "User"} {
			if !strings.Contains(err.Error(), want) {
				t.Fatalf("error %q does not name %q", err, want)
			}
		}
	})

	t.Run("QueueAs identifies by the package rather than the anchor", func(t *testing.T) {
		t.Parallel()
		// Package-scoped output has no declaration of its own. Deriving
		// the id from whichever declaration it anchored on would move
		// the identifier when an unrelated type is renamed.
		rec := &recorder{}
		err := builder.QueueAs(rec, builder.For("gen"), "top", srcStruct("Anchor"),
			"example.com/x", &queued{name: "registry"})
		if err != nil {
			t.Fatalf("QueueAs = %v", err)
		}
		if rec.provs[0].ID != "test.registry.example.com/x" {
			t.Fatalf("ID = %q, want the package-derived id", rec.provs[0].ID)
		}
	})
}
