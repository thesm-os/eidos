// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package multiargwriter_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"go.thesmos.sh/eidos/docaudit"
)

// TestDocAuditCoversEveryMetaKey pins that every meta key this
// detector constructs from a literal string is mentioned in the
// package's doc.go. A new key landing in code without a
// corresponding doc.go entry fails this audit.
func TestDocAuditCoversEveryMetaKey(t *testing.T) {
	t.Parallel()
	docaudit.AssertEveryMetaKeyDocumented(t, packageDirForDoc(t))
}

// packageDirForDoc returns the absolute path of the directory
// the test file lives in.
func packageDirForDoc(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	return filepath.Dir(file)
}
