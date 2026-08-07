// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package builder_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/position"
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
