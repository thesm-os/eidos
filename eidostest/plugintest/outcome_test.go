// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest_test

import (
	"errors"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/store"
)

// TestBuildFixtureStoreRecovering pins that a fixture whose own
// construction panics fails one subtest instead of the test binary.
//
// A fixture builder panics deliberately on anything the store
// rejects, so a mistyped fixture arrives as a panic. Unrecovered it
// escaped the subtest and took every sibling subtest — and every
// sibling Test in the package — down with it, which is the worst
// possible failure mode for a suite whose job is to report what is
// wrong with one plugin.
func TestBuildFixtureStoreRecovering(t *testing.T) {
	t.Parallel()

	t.Run("a panicking fixture is reported as an error", func(t *testing.T) {
		t.Parallel()
		s, err := plugintest.BuildFixtureStoreRecovering(func() *store.Store {
			panic("gofixture: build failed: duplicate qualified name")
		})
		if err == nil {
			t.Fatalf("a panicking fixture must surface as an error, got store=%v err=nil", s)
		}
		if !errors.Is(err, plugintest.ErrFixtureBuildPanicked) {
			t.Errorf("error must wrap ErrFixtureBuildPanicked; got %v", err)
		}
		if !strings.Contains(err.Error(), "duplicate qualified name") {
			t.Errorf("the recovered value must reach the message; got %v", err)
		}
		if s != nil {
			t.Errorf("no store may be returned alongside a build failure; got %v", s)
		}
	})

	t.Run("a well-formed fixture is returned unchanged", func(t *testing.T) {
		t.Parallel()
		want := store.New()
		got, err := plugintest.BuildFixtureStoreRecovering(func() *store.Store { return want })
		if err != nil {
			t.Fatalf("a well-formed fixture must not report an error; got %v", err)
		}
		if got != want {
			t.Errorf("the fixture's own store must be returned; got %p want %p", got, want)
		}
	})
}

// TestProbeVerb pins that the suite names the verb that actually
// happened. Reporting a plainly returned error as a panic contradicts
// the role contracts, which reserve the return value for what the
// suite cannot classify — and the obvious way to silence the wrong
// message is `return nil`, which manufactures a no-op plugin.
func TestProbeVerb(t *testing.T) {
	t.Parallel()

	t.Run("a recovered panic is named a panic", func(t *testing.T) {
		t.Parallel()
		if got := plugintest.ProbeVerb(plugintest.ErrProbePanicked); got != "panicked" {
			t.Errorf("ProbeVerb(ErrProbePanicked) = %q, want %q", got, "panicked")
		}
	})

	t.Run("a returned error is named a returned error", func(t *testing.T) {
		t.Parallel()
		if got := plugintest.ProbeVerb(plugintest.ErrProbeReturnedError); got != "returned an error" {
			t.Errorf("ProbeVerb(ErrProbeReturnedError) = %q, want %q", got, "returned an error")
		}
	})
}
