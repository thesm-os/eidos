// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bufbuild/protocompile"
	"github.com/bufbuild/protocompile/reporter"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// errEditionSource is the sentinel returned by [scanSyntax] when a
// proto source declares `edition = "..."`. The frontend supports
// `syntax = "proto3";` only; editions sources surface a positioned
// `diag.Error` and are skipped. Compared with [errors.Is].
//
//nolint:gochecknoglobals // sentinel error per the framework convention.
var errEditionSource = errors.New("frontend: edition source detected")

// loadPattern is the per-pattern Load entry point invoked from
// [Frontend.Load]. The resolved descriptor set composes into the
// per-plugin cache key and the source-mapping converter walks it
// into [store.Store.Nodes] entries. An empty Pattern is a no-op
// so plugin-shape tests that don't supply a proto source root
// drive the frontend without hitting the compiler.
func loadPattern(ctx *plugin.FrontendContext, opts Options) error {
	if ctx == nil || ctx.Pattern == "" {
		return nil
	}
	ps := ctx.Diag.For(FrontendName)
	root := resolveRoot(opts.Dir)
	files, err := discoverProtoFiles(root, ctx.Pattern)
	if err != nil {
		ps.Errorf(position.Pos{}, "frontend: discover %s in %s: %v", ctx.Pattern, root, err)
		return nil
	}
	entries := filterUnsupportedSyntax(ps, root, files)
	if len(entries) == 0 {
		return nil
	}
	compiler := newCompiler(ps, opts, root)
	resolved, err := compiler.Compile(context.Background(), entries...)
	switch {
	case err == nil:
		// Compile succeeded; the resolved descriptor set is
		// fully populated and safe to hash for the cache key.
	case errors.Is(err, reporter.ErrInvalidSource):
		// Parse / resolution errors already flowed through the
		// reporter callbacks attached to Handler; the returned
		// sentinel is the terminal indicator that the run hit at
		// least one such error, with the authoritative
		// per-position diagnostics already recorded on ps. The
		// returned descriptor slice is partial — entries for files
		// that failed to parse are nil — so the cache step is
		// skipped (there is no coherent descriptor set to hash).
		return nil
	default:
		// Any other error (context cancellation, internal panic
		// wrapped as error, resolver failure outside the reporter
		// path) needs a surface so consumers don't miss it.
		ps.Errorf(position.Pos{}, "frontend: compile %s: %v", root, err)
		return nil
	}
	// The framework owns the caching — composing the key, looking it
	// up, re-wiring what it finds, adding to the store and writing
	// back — and folds in the composition fingerprint where a caller
	// cannot leave it out. What is proto's is the hash of the resolved
	// descriptor set and the conversion itself.
	//
	// One unit per pattern rather than one per package: a proto run
	// resolves its whole import closure together, and a descriptor set
	// is what any part of it is keyed by.
	descriptors := make([]protoreflect.FileDescriptor, 0, len(resolved))
	for _, f := range resolved {
		descriptors = append(descriptors, f)
	}
	return plugin.CacheLoad(ctx, FrontendName, FrontendVersion, ctx.Pattern,
		func() (string, error) { return hashInputs(ps, opts, descriptors), nil },
		func() ([]*node.Package, error) { return convertFiles(ctx, ps, descriptors), nil },
	)
}

// resolveRoot returns the directory the resolver searches for
// proto sources. Falls back to the process working directory when
// the configured [Options.Dir] is empty (the documented
// zero-config behaviour).
func resolveRoot(configured string) string {
	if configured != "" {
		return configured
	}
	if cwd, err := os.Getwd(); err == nil {
		return cwd
	}
	return "."
}

// discoverProtoFiles walks root and returns every `.proto` file
// reachable per pattern. Three pattern forms are supported:
//
//   - `<dir>/...` — Go-style recursive glob. Walks `root/<dir>/`
//     (or root itself when dir is empty / `.`) for every
//     `*.proto`. Includes `./...`, `./sub/...`, `sub/...`,
//     and similar.
//   - `<dir>` — a directory path (no `/...` suffix, no `.proto`
//     extension). Walks `root/<dir>/` non-recursively at the
//     top level — equivalent to `<dir>/*.proto`.
//   - `<file.proto>` — a single literal file path relative to
//     root. Passed through verbatim.
//
// Walks that find no proto files return the empty slice without
// error — a Go-style pattern naming a directory that holds no
// proto sources is a legitimate "no input for the protobuf
// frontend on this pattern" signal, not a failure. Loaders for
// other source languages handle those patterns instead.
//
// Returned paths are root-relative so protocompile's resolver
// finds them via the configured [SourceResolver.ImportPaths]. The
// slice is sorted alphabetically for deterministic compile order.
func discoverProtoFiles(root, pattern string) ([]string, error) {
	// Literal `.proto` file path → trust the caller.
	if filepath.Ext(pattern) == ".proto" {
		return []string{pattern}, nil
	}
	scanDir, recursive := scanTargetForPattern(pattern)
	walkRoot := filepath.Join(root, scanDir)
	info, err := os.Stat(walkRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			// Pattern named a directory that doesn't exist under
			// root — legitimate "no proto input for this pattern"
			// when the pattern targets a non-proto source tree.
			return nil, nil
		}
		return nil, fmt.Errorf("frontend: stat %s: %w", walkRoot, err)
	}
	if !info.IsDir() {
		// Resolved to a file but not a `.proto` — also a legitimate
		// "no proto input" signal.
		return nil, nil
	}
	var out []string
	walkErr := filepath.WalkDir(walkRoot, func(path string, d fs.DirEntry, werr error) error {
		if werr != nil {
			return fmt.Errorf("walk %s: %w", path, werr)
		}
		if d.IsDir() {
			if !recursive && path != walkRoot {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".proto" {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return fmt.Errorf("rel %s: %w", path, relErr)
		}
		out = append(out, filepath.ToSlash(rel))
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("frontend: walk %s: %w", walkRoot, walkErr)
	}
	sort.Strings(out)
	return out, nil
}

// scanTargetForPattern splits a Go-style source pattern into the
// directory to walk and whether the walk should recurse.
//
// Pattern shapes:
//
//   - `./...` / `...` → root, recursive
//   - `<dir>/...` / `./<dir>/...` → <dir>, recursive
//   - `<dir>` / `./<dir>` → <dir>, top-level only
func scanTargetForPattern(pattern string) (dir string, recursive bool) {
	cleaned := strings.TrimPrefix(pattern, "./")
	if cleaned == "..." {
		return ".", true
	}
	if stripped, ok := strings.CutSuffix(cleaned, "/..."); ok {
		if stripped == "" {
			return ".", true
		}
		return stripped, true
	}
	if cleaned == "" {
		return ".", false
	}
	return cleaned, false
}

// filterUnsupportedSyntax pre-scans each file for the proto3-only
// guard. Files whose first declaration is `edition = "..."`
// surface a positioned `diag.Error` and are dropped from the
// compile set; remaining files continue to the protocompile
// compile pass. proto2 sources reach protocompile and surface as
// resolution errors via the standard reporter callback —
// protocompile handles proto2's syntax declaration on the
// resolver path; the editions guard here is the explicit
// fast-path because editions is the proto3-incompatible case
// users most commonly write.
func filterUnsupportedSyntax(ps *diag.PluginSink, root string, files []string) []string {
	out := make([]string, 0, len(files))
	for _, rel := range files {
		abs := filepath.Join(root, rel)
		switch err := scanSyntax(abs); {
		case errors.Is(err, errEditionSource):
			ps.Errorf(
				position.Pos{File: abs, Line: 1, Column: 1},
				"frontend: %s declares an edition source (`edition = ...`); only proto3 is supported, file skipped",
				rel,
			)
		default:
			out = append(out, rel)
		}
	}
	return out
}

// scanSyntax reads the first non-blank, non-comment line of path
// and returns [errEditionSource] when it matches the `edition =
// "..."` syntax declaration the proto editions migration path
// introduces. Returns nil for files where the syntax declaration
// is `syntax = "proto3";`, missing, proto2, or anything else;
// protocompile rejects unsupported syntaxes through its own
// resolver path so the scanner only handles the editions case
// that needs the explicit guard.
//
// The match is identifier-bounded: a line starting with
// `edition` followed by whitespace or `=` triggers the guard;
// `edition_foo` or any other identifier starting with the
// substring does not. The scanner reads at most the first few
// non-blank lines so it stays cheap on large source files.
// Errors opening or reading the file return nil — the subsequent
// protocompile pass surfaces a positioned diagnostic against the
// same file, so a scanner-side error suppression here avoids
// duplicate messages.
func scanSyntax(path string) error {
	f, err := os.Open(path) //nolint:gosec // path is from the configured search root.
	if err != nil {
		return nil
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") {
			continue
		}
		if isEditionDecl(line) {
			return errEditionSource
		}
		// Any other first-non-comment line means the file isn't an
		// edition source; let protocompile handle it.
		return nil
	}
	return nil
}

// isEditionDecl reports whether line is a proto `edition`
// declaration — `edition` followed by whitespace or `=`. Tight
// match so `edition_foo` and similar identifiers starting with
// the substring don't fire the editions guard.
func isEditionDecl(line string) bool {
	const keyword = "edition"
	if !strings.HasPrefix(line, keyword) {
		return false
	}
	rest := line[len(keyword):]
	if rest == "" {
		return true
	}
	next := rest[0]
	return next == ' ' || next == '\t' || next == '='
}

// newCompiler constructs the protocompile.Compiler configured for
// the supplied options. The reporter callbacks funnel errors and
// warnings through the frontend's per-plugin diagnostic sink so
// every diagnostic is attributed to the protobuf frontend's name
// with positions resolved through protocompile.
func newCompiler(ps *diag.PluginSink, opts Options, root string) *protocompile.Compiler {
	extra := importPathList(opts.ImportPaths)
	roots := make([]string, 0, 1+len(extra))
	roots = append(roots, root)
	roots = append(roots, extra...)
	resolver := protocompile.Resolver(&protocompile.SourceResolver{ImportPaths: roots})
	if opts.IncludeWellKnown {
		resolver = protocompile.WithStandardImports(resolver)
	}
	rep := reporter.NewReporter(
		func(err reporter.ErrorWithPos) error {
			ps.Errorf(posFromReporter(err), "frontend: %s", err.Unwrap())
			// Returning nil here keeps protocompile compiling the
			// remaining files rather than aborting on the first
			// error — the framework's run-to-completion contract
			// demands the per-file diagnostics surface together so
			// users see every parse error in one run.
			return nil
		},
		func(warn reporter.ErrorWithPos) {
			ps.Warnf(posFromReporter(warn), "frontend: %s", warn.Unwrap())
		},
	)
	return &protocompile.Compiler{
		Resolver:       resolver,
		Reporter:       rep,
		SourceInfoMode: protocompile.SourceInfoStandard,
	}
}

// posFromReporter translates protocompile's per-error source
// position into the framework's [position.Pos] form. protocompile
// reports file + line + column tuples; the framework's `Offset`
// field stays zero because protocompile does not surface byte
// offsets on its source-location API.
func posFromReporter(err reporter.ErrorWithPos) position.Pos {
	p := err.GetPosition()
	return position.Pos{File: p.Filename, Line: p.Line, Column: p.Col}
}
