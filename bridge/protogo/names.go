// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package protogo

import (
	"strings"

	"go.thesmos.sh/eidos/core/naming"
)

// GoFieldName returns the Go-idiomatic PascalCase identifier for a
// proto-style snake_case name (`user_id` → `UserID`).
//
// Deprecated: use [naming.Pascal], which applies the same rule with
// the same initialism set. This package carried a private copy of
// both; the two agreed on every input, which is the drift that had
// not happened yet rather than a difference worth keeping. Removed
// no earlier than the next minor release.
func GoFieldName(name string) string { return naming.Pascal(name) }

// GoPackageName returns the Go package clause derived from a
// proto package qualifier or from a `go_package` option value.
// The input forms it handles:
//
//   - `pkg/path;name`     → "name" (explicit semicolon-suffix)
//   - `pkg/path`          → last `/`-separated segment
//   - `dotted.qualifier`  → last dotted segment
//   - bare identifier     → unchanged
//
// The result is the identifier the Go backend emits in the
// `package <X>` clause. Empty input produces empty output;
// callers fall back to the proto package qualifier when
// GoPackageName returns empty.
func GoPackageName(value string) string {
	if value == "" {
		return ""
	}
	if _, after, ok := strings.Cut(value, ";"); ok {
		return after
	}
	if i := strings.LastIndexByte(value, '/'); i >= 0 {
		return value[i+1:]
	}
	if i := strings.LastIndexByte(value, '.'); i >= 0 {
		return value[i+1:]
	}
	return value
}

// GoImportPath returns the Go import path from a
// `go_package` option value. The semicolon-suffix form
// (`pkg/path;name`) trims the trailing identifier; the bare
// path form returns unchanged. Empty input produces empty output.
func GoImportPath(value string) string {
	if value == "" {
		return ""
	}
	if before, _, ok := strings.Cut(value, ";"); ok {
		return before
	}
	return value
}
