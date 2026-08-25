// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// ErrEmptyPattern is returned by [Frontend.Load] when the
// [plugin.FrontendContext.Pattern] is empty or whitespace-only.
// The loader cannot make progress without a concrete pattern to
// hand to [packages.Load]; surfacing a sentinel lets callers
// distinguish the contract violation from real load failures.
var ErrEmptyPattern = errors.New("frontend: empty pattern")

// loadMode is the [packages.LoadMode] every Load call uses. The mode
// requests everything the converter needs to faithfully reconstruct
// declarations: package + module identity, imports, syntax trees,
// fully-resolved type information including type-checker errors,
// and embedded file contents.
// # Race-detector reports from go/types
//
// Requesting NeedTypes makes [packages.Load] type-check in
// dependency-parallel goroutines, and on Go 1.26.5 that trips a data
// race inside go/types itself. `Unalias` memoizes lazily without
// synchronisation — `alias.go` reads `a0.actual` and then writes it —
// so two importers resolving the same alias concurrently race. It
// surfaces through `Context.instanceHash`, which stringifies a generic
// instantiation and unaliases along the way, and it needs generics,
// aliases and enough packages for the window to open: a corpus of ~140
// reports it on most runs, one of ~40 almost never.
//
// Nothing here can fix it. The race is over before Load returns, so
// resolving aliases eagerly afterwards is too late, and there is no
// public knob for the loader's parallelism. It is Go's to fix — the
// memoization wants an atomic or a mutex — and no caller of
// packages.Load can do it on Go's behalf.
//
// GODEBUG=gotypesalias=0 does not help. The setting is still listed in
// internal/godebugs, but go/types no longer reads it: aliases are
// materialised unconditionally, and only a test file and a stale
// comment still mention the name. Measured, not assumed.
//
// What does silence it, for a suite that must run under -race, is
// GOMAXPROCS=1 — the checker's goroutines then interleave rarely
// enough that the window effectively closes. That is a mitigation and
// not a fix: it narrows the race rather than removing it, and it costs
// the parallelism that makes a large load fast.
const loadMode = packages.NeedName |
	packages.NeedFiles |
	packages.NeedImports |
	packages.NeedTypes |
	packages.NeedSyntax |
	packages.NeedTypesInfo |
	packages.NeedTypesSizes |
	packages.NeedModule

// loadPattern is the entry point [Frontend.Load] delegates to. It
// loads every package matching [plugin.FrontendContext.Pattern] via
// [packages.Load], surfaces parse / type errors as positioned
// diagnostics, and dispatches each package to [convertPackage] for
// AST → node conversion.
//
// A nil or empty pattern is rejected with a positioned diagnostic
// and a non-nil error — without a pattern the loader has nothing
// concrete to load and continuing would silently succeed with an
// empty store.
func loadPattern(ctx *plugin.FrontendContext, opts Options) error {
	if strings.TrimSpace(ctx.Pattern) == "" {
		ctx.Diag.For(FrontendName).Errorf(position.Pos{}, "load: empty pattern")
		return ErrEmptyPattern
	}

	cfg := &packages.Config{
		Mode:  loadMode,
		Tests: opts.IncludeTests,
		Dir:   opts.Dir,
		// Supplied so the parse skips ast.Object resolution.
		// x/tools installs a default ParseFile that keeps doing it —
		// its own comment says so — allocating an *ast.Scope per
		// block and an *ast.Object per declared identifier to
		// populate ast.Ident.Obj, a field nothing in this package
		// reads. Every .Obj() call here resolves through go/types,
		// not through the AST.
		//
		// AllErrors is load-bearing: parse diagnostics flow through
		// pkg.Errors into reportPackageErrors. ParseComments is
		// load-bearing for the entire doc and directive model.
		ParseFile: func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
			return parser.ParseFile(fset, filename, src,
				parser.AllErrors|parser.ParseComments|parser.SkipObjectResolution)
		},
	}
	if opts.IgnoreWorkspace {
		// `GOWORK=off` makes packages.Load respect the loaded
		// directory's own go.mod boundary rather than any enclosing
		// go.work. Required when the configured Dir points at a
		// self-contained fixture module that intentionally lives
		// outside the workspace; off by default so in-workspace
		// loading picks up replace directives and cross-module
		// visibility from go.work.
		cfg.Env = append(os.Environ(), "GOWORK=off")
	}
	if tags := strings.TrimSpace(opts.BuildTags); tags != "" {
		cfg.BuildFlags = []string{"-tags=" + tags}
	}

	pkgs, err := packages.Load(cfg, ctx.Pattern)
	if err != nil {
		ctx.Diag.For(FrontendName).Errorf(position.Pos{}, "load %q: %v", ctx.Pattern, err)
		return fmt.Errorf("frontend: load %q: %w", ctx.Pattern, err)
	}

	reportPackageErrors(ctx, pkgs)

	for _, pkg := range pkgs {
		if pkg.PkgPath == "" {
			// Synthetic placeholder packages (e.g. when the pattern
			// matches no real packages) carry no useful information
			// and would only add noise to the store.
			continue
		}
		if err := convertPackageWithCache(ctx, opts, pkg); err != nil {
			ctx.Diag.For(FrontendName).Errorf(position.Pos{}, "convert %q: %v", pkg.PkgPath, err)
		}
	}
	return nil
}

// convertPackageWithCache routes a package through the per-pipeline
// cache before falling back to a fresh AST→node conversion. On a
// cache hit the deserialised [node.Package] is wired and added to
// the store directly; on a miss the converter runs, the result is
// added to the store, and a fresh cache entry is written.
//
// A cache-key computation failure (source file mutated between
// [packages.Load] and the hash pass) skips the cache entirely and
// falls through to a fresh conversion. Write failures surface as
// Warn diagnostics so cache-disk problems are visible without
// blocking the run.
func convertPackageWithCache(ctx *plugin.FrontendContext, opts Options, pkg *packages.Package) error {
	// The framework owns the dance — compose the key, look it up,
	// re-wire and add, or convert and write back — and this supplies
	// the only half that is Go's: which bytes are this package's
	// inputs. The composition fingerprint is folded in there rather
	// than here, so it cannot be left out.
	return plugin.CacheLoad(ctx, FrontendName, FrontendVersion, pkg.PkgPath,
		func() (string, error) { return hashPackageInputs(pkg, opts) },
		func() ([]*node.Package, error) {
			return []*node.Package{buildPackage(ctx, opts, pkg)}, nil
		},
	)
}

// buildPackage runs the AST→node conversion and returns the
// resulting [node.Package] with back-pointers wired but not yet
// added to the store. Separated from the store-write path so the
// cache layer can intercept the result before invoking it.
func buildPackage(ctx *plugin.FrontendContext, opts Options, pkg *packages.Package) *node.Package {
	conv := newConverter(ctx, pkg, opts)
	out := conv.run()
	node.RewireOwners(out)
	return out
}

// reportPackageErrors emits one diagnostic per error attached to the
// loaded packages. Parser, type-checker, and list errors all surface
// here with the position the Go toolchain reported.
func reportPackageErrors(ctx *plugin.FrontendContext, pkgs []*packages.Package) {
	ps := ctx.Diag.For(FrontendName)
	packages.Visit(pkgs, nil, func(pkg *packages.Package) {
		for _, e := range pkg.Errors {
			ps.Errorf(positionFromPackagesError(e), "%s", e.Msg)
		}
	})
}

// positionFromPackagesError converts the colon-delimited "file:line:col"
// position string [packages.Error] carries into a [position.Pos]. The
// conversion is forgiving — malformed position strings yield a zero
// Pos rather than a parse error, because the alternative is hiding
// the diagnostic behind a position-parse failure.
func positionFromPackagesError(e packages.Error) position.Pos {
	// packages.Error.Pos is a "file:line:col" string. We split from
	// the right so file paths containing colons (Windows volume
	// letters, network paths) round-trip correctly.
	rest := e.Pos
	colCutAt := strings.LastIndex(rest, ":")
	if colCutAt < 0 {
		return position.Pos{File: rest}
	}
	colStr := rest[colCutAt+1:]
	rest = rest[:colCutAt]
	lineCutAt := strings.LastIndex(rest, ":")
	if lineCutAt < 0 {
		return position.Pos{File: rest}
	}
	lineStr := rest[lineCutAt+1:]
	file := rest[:lineCutAt]
	// packages.Error.Pos guarantees the colon-split fragments are
	// decimal integers; discarding the strconv error matches the
	// upstream contract.
	line, _ := strconv.Atoi(lineStr) //nolint:errcheck // packages.Error invariant
	col, _ := strconv.Atoi(colStr)   //nolint:errcheck // packages.Error invariant
	return position.At(file, line, col)
}
