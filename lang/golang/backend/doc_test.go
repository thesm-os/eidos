// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/golang/backend"
	"go.thesmos.sh/eidos/plugin"
)

// TestPackageContract documents the package's primary contract:
// the [Backend] type implements [plugin.Backend] and exposes
// stable identifiers [Name] and [Language] for pipeline
// registration and template-provider language scoping.
func TestPackageContract(t *testing.T) {
	t.Parallel()

	t.Run("Backend satisfies plugin.Backend", func(t *testing.T) {
		t.Parallel()
		var _ plugin.Backend = backend.New()
	})

	t.Run("Name and Language constants are non-empty", func(t *testing.T) {
		t.Parallel()
		if backend.Name == "" {
			t.Fatalf("Name constant must not be empty")
		}
		if backend.Language == "" {
			t.Fatalf("Language constant must not be empty")
		}
	})
}
