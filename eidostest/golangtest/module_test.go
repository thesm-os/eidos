// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golangtest_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/eidostest/golangtest"
)

// usesRuntime is a generated file importing the generator's runtime
// library — the shape [golangtest.Generated.WithRequire] exists for.
const usesRuntime = `package svc

import "example.com/runtimelib"

// Mark is what a generated file would emit against its runtime.
var Mark = runtimelib.Marker()
`

// TestWithRequire pins the middle ground between a stdlib-only module
// and a copied one: generated output importing the generator's own
// runtime library.
func TestWithRequire(t *testing.T) {
	t.Parallel()

	t.Run("generated code compiles against the required module", func(t *testing.T) {
		t.Parallel()
		// The claim that matters: the import resolves. Without the
		// require+replace pair this fails as "no required module
		// provides package", which reads as a bug in the generator
		// rather than as a gap in the test module.
		golangtest.Of(golangtest.File{Path: "svc/svc.gen.go", Src: []byte(usesRuntime)}).
			WithModulePath("example.com/gen").
			WithRequire("example.com/runtimelib", "testdata/runtimelib").
			AssertCompiles(t)
	})

	t.Run("without the requirement the same file does not compile", func(t *testing.T) {
		t.Parallel()
		// The control. Without it the subtest above passes whether or
		// not WithRequire did anything, since a build that resolves
		// nothing and a build that resolves everything both come back
		// green if the assertion is not actually reaching the compiler.
		s := probe(t)
		golangtest.Of(golangtest.File{Path: "svc/svc.gen.go", Src: []byte(usesRuntime)}).
			WithModulePath("example.com/gen").
			AssertCompiles(s)

		if !s.failed {
			t.Fatalf("an unresolvable import must fail the build; " +
				"if it does not, the sibling subtest proves nothing")
		}
	})

	t.Run("the go directive rises to the requirement's floor", func(t *testing.T) {
		t.Parallel()
		// A dependency declaring a later floor than the test module is
		// rejected with a message about the *dependency's* go.mod,
		// which reads as the dependency being broken rather than as the
		// test module being behind it. A deliberately ancient setting
		// makes the requirement's own directive unambiguously higher.
		golangtest.Of(golangtest.File{Path: "svc/svc.gen.go", Src: []byte(usesRuntime)}).
			WithModulePath("example.com/gen").
			WithGoVersion("1.16").
			WithRequire("example.com/runtimelib", "testdata/runtimelib").
			AssertCompiles(t)
	})

	t.Run("a replace target that is not a module fails with the reason", func(t *testing.T) {
		t.Parallel()
		// Otherwise the `go` command reports it against a generated
		// go.mod, naming neither the assertion nor the argument.
		s := probe(t)
		golangtest.Of(golangtest.File{Path: "svc/svc.gen.go", Src: []byte("package svc\n")}).
			WithModulePath("example.com/gen").
			WithRequire("example.com/absent", "testdata/does-not-exist").
			AssertCompiles(s)

		if !s.failed {
			t.Fatalf("a replace target with no go.mod must fail the test")
		}
		if !strings.Contains(s.msg, "go.mod") {
			t.Errorf("failure must name the missing go.mod; got %q", s.msg)
		}
	})

	t.Run("InModule and WithRequire together are refused", func(t *testing.T) {
		t.Parallel()
		// Composing them would mean rewriting a go.mod this package did
		// not write; either silent outcome builds and only one is what
		// the caller asked for.
		s := probe(t)
		golangtest.Of(golangtest.File{Path: "svc/svc.gen.go", Src: []byte("package svc\n")}).
			InModule("testdata/consumer").
			WithRequire("example.com/runtimelib", "testdata/runtimelib").
			AssertCompiles(s)

		if !s.failed {
			t.Fatalf("InModule + WithRequire must be refused rather than silently resolved")
		}
	})
}
