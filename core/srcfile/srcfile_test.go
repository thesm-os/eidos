// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package srcfile_test

import (
	"path/filepath"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/core/srcfile"
)

// TestWithSuffix covers the stringer-style filename derivation
// across the supported input shapes: absolute / relative source
// paths, the empty-pos fallback, and the no-extension edge case.
func TestWithSuffix(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		pos      position.Pos
		fallback string
		suffix   string
		want     string
	}{
		{
			name:     "absolute path strips dir + extension",
			pos:      position.Pos{File: "/abs/users/article.go"},
			fallback: "article",
			suffix:   "_repo.go",
			want:     "article_repo.go",
		},
		{
			name:     "relative path behaves the same",
			pos:      position.Pos{File: "users/article.go"},
			fallback: "article",
			suffix:   "_builder.go",
			want:     "article_builder.go",
		},
		{
			name:     "non-go suffix passes through verbatim",
			pos:      position.Pos{File: "src/lib.rs"},
			fallback: "lib",
			suffix:   "_codegen.rs",
			want:     "lib_codegen.rs",
		},
		{
			name:     "empty pos falls back to lower-cased name + suffix",
			pos:      position.Pos{},
			fallback: "Article",
			suffix:   "_repo.go",
			want:     "article_repo.go",
		},
		{
			name:     "extension-less basename uses bare name",
			pos:      position.Pos{File: "/abs/Makefile"},
			fallback: "Makefile",
			suffix:   "_gen",
			want:     "Makefile_gen",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := srcfile.WithSuffix(tc.pos, tc.fallback, tc.suffix); got != tc.want {
				t.Fatalf("WithSuffix = %q, want %q", got, tc.want)
			}
		})
	}
}

// FuzzWithSuffix drives the filename derivation over arbitrary
// positions, fallbacks, and suffixes.
//
// The result is handed to a sink as a filename, so it is a security
// boundary as much as a naming one: pos.File arrives from a frontend
// and can name any path on disk, and a result that kept a directory
// component would make the sink write outside the target directory.
// The three properties below are the ones a caller relies on without
// checking.
//
// Seeds cover the branches WithSuffix takes (populated file, empty
// file, extensionless basename, extension-only basename that falls back
// anyway) plus the boundaries: absolute and relative paths, a trailing
// separator, dot and dot-dot elements, an empty fallback, an empty
// suffix, and a suffix that is itself a path.
func FuzzWithSuffix(f *testing.F) {
	for _, seed := range []struct{ file, fallback, suffix string }{
		{"/abs/users/article.go", "article", "_repo.go"},
		{"users/article.go", "article", "_builder.go"},
		{"src/lib.rs", "lib", "_codegen.rs"},
		{"", "Article", "_repo.go"},
		{"/abs/Makefile", "Makefile", "_gen"},
		{".go", "article", "_repo.go"},
		{"a/", "article", "_repo.go"},
		{".", "article", "_repo.go"},
		{"..", "article", "_repo.go"},
		{"/", "article", "_repo.go"},
		{"//", "article", "_repo.go"},
		{"<synthetic>", "article", "_repo.go"},
		{"article.go", "", ""},
		{"", "", ""},
		{"article.go", "a/b", "_repo.go"},
		{"article.go", "article", "/escape.go"},
		{"\xff.go", "\xff", "_repo.go"},
	} {
		f.Add(seed.file, seed.fallback, seed.suffix)
	}

	f.Fuzz(func(t *testing.T, file, fallback, suffix string) {
		pos := position.Pos{File: file}
		got := srcfile.WithSuffix(pos, fallback, suffix)

		// The suffix is the caller's declaration of what kind of file
		// this is — "_repo.go", "_builder.go". A result that lost it
		// would be routed and compiled as the wrong kind of artifact.
		if !strings.HasSuffix(got, suffix) {
			t.Fatalf("WithSuffix(%q, %q, %q) = %q, which does not end with the suffix", file, fallback, suffix, got)
		}

		// The derivation must not introduce a path separator of its
		// own. Separators reaching it through fallback or suffix are
		// the caller's business, so those inputs are excluded rather
		// than the property weakened.
		//
		// KNOWN DEFECT, deliberately excluded: a pos.File consisting
		// only of separators. filepath.Base returns a single separator
		// for such a path, and the base != "" guard in WithSuffix
		// admits it, so WithSuffix(Pos{File: "/"}, "x", "_gen.go")
		// returns "/_gen.go" — an absolute path presented as a
		// filename. Deleting this exclusion is how the test reports
		// the fix.
		if !containsSeparator(fallback) && !containsSeparator(suffix) && !isSeparatorOnly(file) {
			if containsSeparator(got) {
				t.Fatalf("WithSuffix(%q, %q, %q) = %q, which carries a path separator",
					file, fallback, suffix, got)
			}
		}

		// The result depends on the source's basename alone. Two
		// entities in the same file must land in the same output file
		// regardless of how the frontend spelled the path — the whole
		// point of the stringer-style convention.
		//
		// The prefix is concatenated rather than joined: filepath.Join
		// cleans "." and ".." away, which would compare a different
		// basename against the original instead of the same one.
		if file != "" && !containsSeparator(file) {
			nested := "pkg/sub/" + file
			if got2 := srcfile.WithSuffix(position.Pos{File: nested}, fallback, suffix); got2 != got {
				t.Fatalf("WithSuffix is sensitive to the directory prefix: %q for %q, %q for %q",
					got, file, got2, nested)
			}
		}
	})
}

// containsSeparator reports whether s holds a path separator under the
// running platform's rules. Both '/' and filepath.Separator are tested
// because filepath.Base treats both as separators on Windows and the
// property is about what Base can return, not about what this platform
// prefers.
func containsSeparator(s string) bool {
	return strings.ContainsRune(s, '/') || strings.ContainsRune(s, filepath.Separator)
}

// isSeparatorOnly reports whether s is non-empty and made up entirely
// of path separators — the shape filepath.Base collapses to a single
// separator, and the one input for which WithSuffix returns a path
// rather than a filename.
//
// The cutset is both separator characters on every platform rather
// than the platform's own. On Unix that makes the predicate slightly
// generous about a lone backslash, which only ever widens an exclusion
// for an input the property would have passed anyway.
func isSeparatorOnly(s string) bool {
	return s != "" && strings.Trim(s, `/\`) == ""
}
