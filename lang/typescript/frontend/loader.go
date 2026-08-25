// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// ErrEmptyPattern reports a Load call with no pattern to expand.
var ErrEmptyPattern = errors.New("frontend: empty pattern")

// ErrNoMatch reports a pattern that resolved to no readable source
// files.
//
// A pattern matching nothing is almost always a mistake — a typo, or
// a path relative to the wrong directory — and reporting it is what
// separates that from a run that legitimately had nothing to do.
var ErrNoMatch = errors.New("frontend: pattern matched no TypeScript files")

// expandPattern resolves a user-supplied pattern to the source files
// the converter should parse, sorted so a run's order does not depend
// on the filesystem's.
//
// Four forms are accepted, chosen to match what a TypeScript project
// actually looks like plus the recursive form Go users already type:
//
//   - `./src/...`   every file beneath src, recursively
//   - `./src`       every file directly in src
//   - `./src/*.ts`  a glob, expanded by the filesystem
//   - `./src/a.ts`  one file
func expandPattern(pattern string, opts Options) ([]string, error) {
	if strings.TrimSpace(pattern) == "" {
		return nil, ErrEmptyPattern
	}

	root := opts.Dir
	if root == "" {
		wd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("frontend: resolve working directory: %w", err)
		}
		root = wd
	}

	files, err := matchPattern(root, pattern, opts)
	if err != nil {
		return nil, err
	}

	files = slices.DeleteFunc(files, func(path string) bool {
		return !includeFile(path, opts)
	})
	if len(files) == 0 {
		return nil, fmt.Errorf("%w: %s", ErrNoMatch, pattern)
	}

	slices.Sort(files)
	return slices.Compact(files), nil
}

// matchPattern dispatches on the pattern's form and returns candidate
// paths, before per-file filtering.
func matchPattern(root, pattern string, opts Options) ([]string, error) {
	abs := pattern
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(root, pattern)
	}

	if trimmed, recursive := strings.CutSuffix(abs, string(filepath.Separator)+"..."); recursive {
		return walkDir(trimmed, opts)
	}
	if trimmed, recursive := strings.CutSuffix(abs, "/..."); recursive {
		return walkDir(trimmed, opts)
	}

	info, err := os.Stat(abs)
	switch {
	case err == nil && info.IsDir():
		return readDir(abs)
	case err == nil:
		return []string{abs}, nil
	}

	// Not a path that exists — treat it as a glob. A glob that matches
	// nothing returns no error from filepath.Glob, and the empty
	// result surfaces as ErrNoMatch in the caller rather than here,
	// so a typo and an empty directory report the same way.
	matches, globErr := filepath.Glob(abs)
	if globErr != nil {
		return nil, fmt.Errorf("frontend: bad pattern %q: %w", pattern, globErr)
	}
	return matches, nil
}

// walkDir returns every file beneath dir.
//
// Directory pruning happens here rather than in the per-file filter
// because skipping `node_modules` after descending into it has
// already paid the walk: a project's dependency tree is routinely
// tens of thousands of files, and the filter would reject each one
// individually.
func walkDir(dir string, opts Options) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if opts.SkipNodeModules && d.Name() == "node_modules" {
				return fs.SkipDir
			}
			return nil
		}
		out = append(out, path)
		return nil
	})
	switch {
	case errors.Is(err, fs.ErrNotExist):
		// A recursive pattern rooted at a path that is not there is
		// the same user mistake as one that matches nothing, and the
		// caller reports it as [ErrNoMatch]. Returning the walk error
		// instead would give `./absent` and `./absent/...` two
		// different errors for one typo.
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("frontend: walk %s: %w", dir, err)
	}
	return out, nil
}

// readDir returns the files directly inside dir, without descending.
func readDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("frontend: read %s: %w", dir, err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			out = append(out, filepath.Join(dir, e.Name()))
		}
	}
	return out, nil
}

// readSource reads one file's bytes.
func readSource(path string) ([]byte, error) {
	body, err := os.ReadFile(path) //nolint:gosec // path is pattern-resolved, not user input at this layer
	if err != nil {
		return nil, fmt.Errorf("frontend: read %s: %w", path, err)
	}
	return body, nil
}
