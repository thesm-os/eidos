// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// oneStruct returns a store holding a single struct, which is enough
// for an annotator to have something to stamp and a generator
// something to read.
func oneStruct(t *testing.T) *store.Store {
	t.Helper()
	s := store.New()
	err := s.Nodes().AddPackage(&node.Package{
		Name: "x", Path: "example.com/x",
		Structs: []*node.Struct{{Name: "User", Package: "example.com/x"}},
	})
	if err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	return s
}

// warnOnce reports one warning, so a test can assert that the sink a
// driver returns is the one the plugin actually wrote to.
type warnOnce struct{ msg string }

func (*warnOnce) Name() string { return "warn-once" }

func (w *warnOnce) Generate(ctx *plugin.GeneratorContext) error {
	ctx.Diag.Warnf(position.Pos{}, "%s", w.msg)
	return nil
}

// TestDrivers pins the targeted drivers a plugin's own tests use.
func TestDrivers(t *testing.T) {
	t.Parallel()

	t.Run("Annotate runs the plugin against the store", func(t *testing.T) {
		t.Parallel()
		s := oneStruct(t)
		plugintest.Annotate(t, &taggingAnnotator{name: "tagger"}, s)

		got, ok := s.Nodes().Structs().ByQName("example.com/x.User")
		if !ok {
			t.Fatalf("the struct went missing")
		}
		if got.Meta() == nil {
			t.Errorf("Annotate did not reach the plugin: nothing was stamped")
		}
	})

	t.Run("Generate returns the diagnostics the plugin wrote", func(t *testing.T) {
		t.Parallel()
		// The reason to return the sink rather than swallow it: the
		// diagnostics are most of what a generator's tests assert on.
		d := plugintest.Generate(t, &warnOnce{msg: "sample warning"}, oneStruct(t))
		if len(d.Diagnostics()) != 1 {
			t.Fatalf("Diagnostics = %d, want the one the plugin wrote", len(d.Diagnostics()))
		}
		if !strings.Contains(d.Diagnostics()[0].Message, "sample warning") {
			t.Errorf("message = %q, want the plugin's", d.Diagnostics()[0].Message)
		}
	})

	t.Run("a panicking plugin fails the test rather than the binary", func(t *testing.T) {
		t.Parallel()
		// The difference between one red test and a run with no
		// results at all, which is what a driver without the recover
		// gives you.
		fake := newFakeTIn(t)
		s := oneStruct(t)
		panicked := captureFatal(func() {
			plugintest.Annotate(fake, &panickingAnnotator{name: "boom"}, s)
		})
		if !panicked {
			t.Fatalf("a panicking annotator must fail the test")
		}
		if !strings.Contains(strings.Join(fake.fatals, "\n"), "boom") {
			t.Errorf("failure must name the plugin; got %q", fake.fatals)
		}
	})
}
