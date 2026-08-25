// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"go/ast"
	"regexp"
)

// generatedHeaderPattern matches the canonical "this file was
// generated" marker every gofmt-clean code generator stamps as its
// header — the same shape `go/build.IsGenerated` recognises. The
// pattern allows arbitrary text between "Code generated" and "DO
// NOT EDIT." so generator names with spaces or version suffixes
// pass through.
//
//nolint:gochecknoglobals // compiled-once pattern, read-only.
var generatedHeaderPattern = regexp.MustCompile(`^//\s*Code generated .* DO NOT EDIT\.$`)

// IsGeneratedFile reports whether f carries the canonical
// `// Code generated ... DO NOT EDIT.` marker as one of its
// leading comments. The marker may sit anywhere in the file's
// pre-package-clause comment groups so the function scans every
// comment group whose end precedes the `package` keyword (Go's own
// `go/build.IsGenerated` follows the same rule).
//
// Exported because callers outside the converter (the [Frontend]
// pipeline harness or future external tooling) may want to apply
// the same predicate without re-implementing the regex.
func IsGeneratedFile(f *ast.File) bool {
	if f == nil {
		return false
	}
	pkgPos := f.Package
	for _, g := range f.Comments {
		if g == nil {
			continue
		}
		// Only consider comment groups that close before the
		// `package` clause — generator-marker comments sit in the
		// file preamble, never inside the body.
		if pkgPos.IsValid() && g.End() >= pkgPos {
			break
		}
		for _, c := range g.List {
			if generatedHeaderPattern.MatchString(c.Text) {
				return true
			}
		}
	}
	return false
}
