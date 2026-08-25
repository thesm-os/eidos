// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"bufio"
	"bytes"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// sourceExtensions are the file extensions the frontend parses.
//
// `.mts` and `.cts` are the explicit ES-module and CommonJS forms;
// they carry the same syntax as `.ts` and differ only in how the
// runtime resolves them, so they take the same grammar.
var sourceExtensions = []string{".ts", ".tsx", ".mts", ".cts"} //nolint:gochecknoglobals // immutable lookup table

// includeFile reports whether a candidate path should be parsed.
func includeFile(path string, opts Options) bool {
	if !hasSourceExtension(path) {
		return false
	}
	if opts.SkipNodeModules && hasSegment(path, "node_modules") {
		return false
	}
	if !opts.IncludeDeclarations && isDeclarationFile(path) {
		return false
	}
	if !opts.IncludeTests && isTestFile(path) {
		return false
	}
	return true
}

// hasSourceExtension reports whether path ends in a TypeScript source
// extension.
func hasSourceExtension(path string) bool {
	ext := filepath.Ext(path)
	for _, e := range sourceExtensions {
		if strings.EqualFold(ext, e) {
			return true
		}
	}
	return false
}

// hasSegment reports whether path contains the given directory
// segment.
//
// Segment-wise rather than a substring search: a directory legitimately
// named `my_node_modules_helper` contains the substring and is not the
// thing being excluded.
func hasSegment(path, segment string) bool {
	return slices.Contains(strings.Split(filepath.ToSlash(path), "/"), segment)
}

// isDeclarationFile reports a `.d.ts` file (or its `.d.mts` /
// `.d.cts` variants) — types only, no implementation.
func isDeclarationFile(path string) bool {
	base := filepath.Base(path)
	for _, e := range sourceExtensions {
		if strings.HasSuffix(base, ".d"+e) {
			return true
		}
	}
	return false
}

// isTestFile reports a file named by either of the two conventions
// the ecosystem settled on — `x.test.ts` and `x.spec.ts`.
//
// A `__tests__` directory is deliberately not matched: it holds files
// that are themselves named by one of these conventions, so the name
// check already covers them, and a project using the directory for
// fixtures would lose those too.
func isTestFile(path string) bool {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	return strings.HasSuffix(stem, ".test") || strings.HasSuffix(stem, ".spec")
}

// generatedMarker matches the canonical generated-file header, the
// same shape Go's own tooling recognises.
var generatedMarker = regexp.MustCompile(
	`^//\s*Code generated .* DO NOT EDIT\.$`,
) //nolint:gochecknoglobals // compiled once, immutable

// isGenerated reports whether src carries the generated-file marker
// in its leading comment block.
//
// Only the first lines are examined, and the scan stops at the first
// line that is neither a comment nor blank. The marker means "this
// file was produced by a tool"; a line matching it halfway down a
// file is a string or a quoted example, not a header.
func isGenerated(src []byte) bool {
	const maxHeaderLines = 32

	scanner := bufio.NewScanner(bytes.NewReader(src))
	for line := 0; line < maxHeaderLines && scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		switch {
		case text == "":
			continue
		case generatedMarker.MatchString(text):
			return true
		case strings.HasPrefix(text, "//"), strings.HasPrefix(text, "/*"),
			strings.HasPrefix(text, "*"), strings.HasPrefix(text, "*/"):
			continue
		default:
			return false
		}
	}
	return false
}
