// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipelinetest_test

import (
	"testing"

	"go.thesmos.sh/eidos/eidostest/internal/nodefixture"
	"go.thesmos.sh/eidos/eidostest/pipelinetest"
	"go.thesmos.sh/eidos/store"
)

func TestFromNodes(t *testing.T) {
	t.Parallel()

	t.Run("returns a frontend whose Name identifies the synthetic source", func(t *testing.T) {
		t.Parallel()
		fe := pipelinetest.FromNodes()
		if fe.Name() == "" {
			t.Fatalf("FromNodes frontend must have a non-empty Name")
		}
	})

	t.Run("Load adds every supplied package on the first call", func(t *testing.T) {
		t.Parallel()
		pkg := nodefixture.PackageIn("users", "example.com/users", "User")

		fe := pipelinetest.FromNodes(pkg)
		s := store.New()
		if err := fe.Load(loadCtx(s, "")); err != nil {
			t.Fatalf("Load returned error: %v", err)
		}
		if _, ok := s.Nodes().Structs().ByQName("example.com/users.User"); !ok {
			t.Fatalf("package not added to store")
		}
	})

	t.Run("Load is idempotent across repeated invocations", func(t *testing.T) {
		t.Parallel()
		pkg := nodefixture.Package("S")
		fe := pipelinetest.FromNodes(pkg)
		s := store.New()

		if err := fe.Load(loadCtx(s, "first")); err != nil {
			t.Fatalf("first Load returned error: %v", err)
		}
		if err := fe.Load(loadCtx(s, "second")); err != nil {
			t.Fatalf("second Load returned error: %v", err)
		}
		if got := s.Nodes().Structs().Len(); got != 1 {
			t.Fatalf("Load should add exactly once across calls; got %d structs", got)
		}
	})

	t.Run("Load propagates AddPackage errors", func(t *testing.T) {
		t.Parallel()
		pkg := nodefixture.Package("Dup")
		fe := pipelinetest.FromNodes(pkg, pkg)
		s := store.New()

		if err := fe.Load(loadCtx(s, "")); err == nil {
			t.Fatalf("expected error from duplicate-qname Load; got nil")
		}
	})

	t.Run("accepts multiple packages", func(t *testing.T) {
		t.Parallel()
		a := nodefixture.PackageIn("a", "example.com/a", "A")
		b := nodefixture.PackageIn("b", "example.com/b", "B")

		fe := pipelinetest.FromNodes(a, b)
		s := store.New()
		if err := fe.Load(loadCtx(s, "")); err != nil {
			t.Fatalf("Load returned error: %v", err)
		}
		if _, ok := s.Nodes().Structs().ByQName("example.com/a.A"); !ok {
			t.Fatalf("package a not loaded")
		}
		if _, ok := s.Nodes().Structs().ByQName("example.com/b.B"); !ok {
			t.Fatalf("package b not loaded")
		}
	})
}
