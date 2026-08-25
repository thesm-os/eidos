// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"go/ast"
	"slices"

	"golang.org/x/tools/go/packages"
)

// filterSyntaxFiles returns the subset of pkg.Syntax the converter
// should walk. The default is every loaded syntax file. Two
// filters participate, both on by default:
//
//   - [Options.SkipCgoFiles]: drops cgo-synthesized files via
//     [IsCgoFile].
//   - [Options.SkipGeneratedFiles]: drops files carrying the
//     canonical generator marker via [IsGeneratedFile] so a re-run
//     never re-parses the framework's own previous output as fresh
//     source.
//
// The cgo branch is unreachable in pure-Go fixture tests because
// [golang.org/x/tools/go/packages] hides underscore-prefixed files
// before they appear in pkg.Syntax; the generated-marker branch
// is the user-facing predicate and carries the test coverage.
func filterSyntaxFiles(pkg *packages.Package, opts Options) []*ast.File {
	return slices.DeleteFunc(slices.Clone(pkg.Syntax), func(f *ast.File) bool {
		if opts.SkipCgoFiles && IsCgoFile(pkg.Fset.Position(f.Pos()).Filename) {
			return true
		}
		if opts.SkipGeneratedFiles && IsGeneratedFile(f) {
			return true
		}
		return false
	})
}
