// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/lang/protobuf/frontend"
)

// TestMetaKeyNames pins the canonical key names the frontend stamps.
// The names are programmer-facing identifiers — cache keys, the
// `explain` output, cross-frontend audit assertions all pivot on
// them. A typo here would silently shadow the expected name without
// a build error.
//
// Table-driven because every row asks the same Key → expected name
// question.
func TestMetaKeyNames(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		key  string
		want string
	}{
		{"MetaFrontend", frontend.MetaFrontend.Name(), "frontend"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if tc.key != tc.want {
				t.Fatalf("%s name = %q, want %q", tc.name, tc.key, tc.want)
			}
		})
	}
}

// TestMetaOptionPrefix pins the documented namespace under which
// proto custom-option entries are stamped. The prefix is the public
// surface plugins consuming proto-derived nodes string-prefix-scan
// against; changing it would silently break every such consumer.
func TestMetaOptionPrefix(t *testing.T) {
	t.Parallel()
	t.Run("uses the documented proto.option. namespace", func(t *testing.T) {
		t.Parallel()
		if frontend.MetaOptionPrefix != "proto.option." {
			t.Fatalf("MetaOptionPrefix = %q, want %q", frontend.MetaOptionPrefix, "proto.option.")
		}
	})
}

// TestOptionMetaKey verifies the dynamic option-key helper composes
// the documented `proto.option.<full-name>` form and registers the
// resulting key through [meta.EnsureKey] (idempotent — repeat calls
// return the same singleton).
func TestOptionMetaKey(t *testing.T) {
	t.Parallel()

	t.Run("composes the documented namespaced name", func(t *testing.T) {
		t.Parallel()
		k := frontend.OptionMetaKey("eidos.protobuf.testdata.simple.example", meta.StringParser)
		want := "proto.option.eidos.protobuf.testdata.simple.example"
		if k.Name() != want {
			t.Fatalf("OptionMetaKey name = %q, want %q", k.Name(), want)
		}
		if !strings.HasPrefix(k.Name(), frontend.MetaOptionPrefix) {
			t.Fatalf("OptionMetaKey name %q lacks prefix %q", k.Name(), frontend.MetaOptionPrefix)
		}
	})

	t.Run("returns the same key for repeat calls", func(t *testing.T) {
		t.Parallel()
		first := frontend.OptionMetaKey("eidos.protobuf.testdata.simple.repeat", meta.IntParser)
		second := frontend.OptionMetaKey("eidos.protobuf.testdata.simple.repeat", meta.IntParser)
		if first.Name() != second.Name() {
			t.Fatalf("OptionMetaKey idempotence: first %q, second %q", first.Name(), second.Name())
		}
	})
}
