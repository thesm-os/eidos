// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shapewriter_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"go.thesmos.sh/eidos/docaudit"
)

// TestDocAuditCoversEveryMetaKey pins that every meta key the
// writer-shape annotator constructs from a literal string is
// documented in the package's doc.go. The plugin's whole output is
// metadata other generators branch on, so a key added in code and
// omitted from the catalog is an undocumented public contract; this
// audit fails the build instead of letting it ship.
func TestDocAuditCoversEveryMetaKey(t *testing.T) {
	t.Parallel()
	docaudit.AssertEveryMetaKeyDocumented(t, packageDir(t))
}

// packageDir returns the absolute path of the directory the test
// file lives in.
func packageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	return filepath.Dir(file)
}
