// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/golang/frontend"
)

// TestIsCgoFile drives the pure-string cgo-file predicate the
// frontend uses inside [filterSyntaxFiles]. The predicate is the
// only black-box-testable surface: go/packages drops files whose
// basename begins with `_` before they ever reach [packages.Load]
// output, so reproducing the live filter behaviour from a fixture
// test would require synthesising a real cgo build — out of scope
// for a unit test.
func TestIsCgoFile(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		path string
		want bool
	}{
		{"empty", "", false},
		{"canonical synthetic", "/tmp/build/_cgo_gotypes.go", true},
		{"any _cgo_ prefix", "/tmp/build/_cgo_helper.go", true},
		{"cgo-gcc-prolog substring", "/tmp/build/cgo-gcc-prolog/aux.go", true},
		{"plain go file", "/tmp/pkg/user.go", false},
		{"go file with underscore but no cgo prefix", "/tmp/pkg/_internal.go", false},
		{"path without separators", "_cgo_gotypes.go", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := frontend.IsCgoFile(tc.path); got != tc.want {
				t.Fatalf("IsCgoFile(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

// TestCgoFilter validates the [Options.SkipCgoFiles] dispatch
// through the public Load surface. Real cgo packages are out of
// scope for a fixture test; this case asserts the default-off
// disposition does not break non-cgo source loading.
func TestCgoFilter(t *testing.T) {
	t.Parallel()
	t.Run("default skip-cgo true loads non-cgo packages normally", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{}\n",
		})
		if pkg.StructByName("S") == nil {
			t.Fatalf("S missing — non-cgo source should always load")
		}
	})
}
