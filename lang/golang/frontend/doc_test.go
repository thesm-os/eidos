// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package doc_test exists so the package's doc-only file participates
// in the project standard of one source file → one test file. The
// import below keeps the dependency edge live and prevents the file
// from being mistaken for unused.
package frontend_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"go.thesmos.sh/eidos/docaudit"
	"go.thesmos.sh/eidos/lang/golang/frontend"
)

// TestPackageDoc is the doc.go pair: the file declares no symbols of
// its own beyond the package documentation; the pair asserts the
// package's identifier surface stays reachable from a _test importer.
func TestPackageDoc(t *testing.T) {
	t.Parallel()
	t.Run("package symbols are reachable", func(t *testing.T) {
		t.Parallel()
		if frontend.FrontendName == "" {
			t.Fatalf("FrontendName must not be empty")
		}
		if frontend.FrontendVersion == "" {
			t.Fatalf("FrontendVersion must not be empty")
		}
	})
}

// TestDocAuditCoversEveryMetaKey pins that every meta key the Go
// frontend constructs from a literal string is mentioned in the
// package's doc.go, so a new key cannot land in code without an
// entry in the doc.go catalog.
//
// The per-struct-tag keys are the one gap the audit cannot close:
// [frontend.MetaTagPrefix] is concatenated with a tag name read at
// runtime, so the [meta.EnsureKey] call in stamp_helpers.go carries
// no literal for the audit to extract. Those keys are documented by
// namespace in the catalog instead, and that entry is unenforced —
// review is the only thing holding it to the code.
func TestDocAuditCoversEveryMetaKey(t *testing.T) {
	t.Parallel()
	docaudit.AssertEveryMetaKeyDocumented(t, packageSourceDir(t))
}

// packageSourceDir returns the absolute path of the directory the
// test file lives in.
//
// Resolved from the compiled-in file path rather than the process
// working directory: sibling tests in this package pivot [os.Chdir]
// into fixture trees while they load sources, so any caller that
// walks the package's own sources by relative path races that pivot
// and fails on a file it can see but not open.
func packageSourceDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

// langGolangSourceDir returns the absolute path of the lang/golang
// package source, where the shared `go.*` meta vocabulary is
// declared.
//
// Resolved from this file's own location rather than by import
// path, for the same reason as [packageSourceDir]: sibling tests
// pivot the working directory while loading fixtures.
func langGolangSourceDir(t *testing.T) string {
	t.Helper()
	// The shared vocabulary is this package's parent: the frontend
	// lives under the language it speaks.
	return filepath.Join(packageSourceDir(t), "..")
}
