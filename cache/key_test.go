// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cache_test

import (
	"testing"

	"go.thesmos.sh/eidos/cache"
)

func TestNewKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		parts []string
		want  string
	}{
		{"single part", []string{"plugin"}, "plugin"},
		{"multiple parts join with colons", []string{"plugin", "foo", "v1"}, "plugin:foo:v1"},
		{"empty parts are dropped", []string{"plugin", "", "v1"}, "plugin:v1"},
		{"all-empty input yields empty key", []string{"", ""}, ""},
		{"zero parts yields empty key", nil, ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := cache.NewKey(tc.parts...); got != tc.want {
				t.Fatalf("NewKey(%v) = %q, want %q", tc.parts, got, tc.want)
			}
		})
	}
}

func TestNewKeyDoesNotMutateCallerSlice(t *testing.T) {
	t.Parallel()

	// Regression: NewKey previously filtered in place via parts[:0],
	// which wrote through to the caller's backing array whenever the
	// call site expanded a slice. Only the empty-part path filtered,
	// so the corruption was invisible until a caller reused the slice.
	parts := []string{"plugin", "", "v1"}
	original := append([]string(nil), parts...)

	if got, want := cache.NewKey(parts...), "plugin:v1"; got != want {
		t.Fatalf("NewKey(%v) = %q, want %q", original, got, want)
	}

	for i := range original {
		if parts[i] != original[i] {
			t.Fatalf("NewKey mutated caller slice: got %q, want %q", parts, original)
		}
	}
}

func TestHashBytes(t *testing.T) {
	t.Parallel()

	t.Run("returns the SHA-256 hex digest of the input", func(t *testing.T) {
		t.Parallel()
		// SHA-256("") = e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855
		got := cache.HashBytes(nil)
		want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
		if got != want {
			t.Fatalf("empty hash = %q, want %q", got, want)
		}
	})

	t.Run("identical input produces identical hashes", func(t *testing.T) {
		t.Parallel()
		a := cache.HashBytes([]byte("payload"))
		b := cache.HashBytes([]byte("payload"))
		if a != b {
			t.Fatalf("identical inputs produced different hashes: %q vs %q", a, b)
		}
	})

	t.Run("different inputs produce different hashes", func(t *testing.T) {
		t.Parallel()
		a := cache.HashBytes([]byte("alpha"))
		b := cache.HashBytes([]byte("beta"))
		if a == b {
			t.Fatalf("different inputs collided on %q", a)
		}
	})
}

func TestHashStrings(t *testing.T) {
	t.Parallel()

	t.Run("order-insensitive: differently-ordered inputs hash identically", func(t *testing.T) {
		t.Parallel()
		a := cache.HashStrings([]string{"x", "y", "z"})
		b := cache.HashStrings([]string{"z", "x", "y"})
		if a != b {
			t.Fatalf("order should not affect HashStrings: %q vs %q", a, b)
		}
	})

	t.Run("does not mutate the caller's slice", func(t *testing.T) {
		t.Parallel()
		input := []string{"c", "a", "b"}
		cache.HashStrings(input)
		want := []string{"c", "a", "b"}
		for i, v := range input {
			if v != want[i] {
				t.Fatalf("HashStrings should not mutate input; got %v", input)
			}
		}
	})

	t.Run("different inputs produce different hashes", func(t *testing.T) {
		t.Parallel()
		a := cache.HashStrings([]string{"x"})
		b := cache.HashStrings([]string{"y"})
		if a == b {
			t.Fatalf("different inputs collided")
		}
	})
}
