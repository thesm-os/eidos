// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"path/filepath"
	"sync"

	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// FrontendName is the [plugin.Plugin.Name] this frontend reports.
//
// Consumers reference it when supplying options, when reading the
// frontend's diagnostics out of the sink, and when filtering
// store-side nodes by the `frontend = "typescript"` marker stamped on
// every produced [node.Package]. It is also the `setBy` attribution
// on every `ts.*` stamp, which is what `eidos explain` prints.
const FrontendName = "typescript"

// Frontend is the TypeScript frontend. The zero value is unusable;
// construct via [New].
//
// Safe for concurrent use. The pipeline may dispatch [Frontend.Load]
// from several goroutines, one per pattern; options are read under
// the mutex and the tree-sitter parsers come from a pool, since a
// parser is not itself concurrency-safe.
type Frontend struct {
	mu   sync.Mutex
	opts Options
}

// New returns a TypeScript frontend ready for registration on a
// pipeline builder. The defaults in [defaultOptions] hold until the
// pipeline calls [Frontend.SetOptions].
func New() *Frontend {
	return &Frontend{opts: defaultOptions()}
}

// Name returns [FrontendName].
func (*Frontend) Name() string { return FrontendName }

// Version returns [FrontendVersion].
func (*Frontend) Version() string { return FrontendVersion }

// EmitVersions reports the emit major versions this frontend is
// compatible with.
func (*Frontend) EmitVersions() []string {
	out := make([]string, len(supportedEmitVersions))
	copy(out, supportedEmitVersions)
	return out
}

// OptionsSchema returns the reflected schema describing the
// frontend's configurable options.
func (*Frontend) OptionsSchema() opt.Schema { return optionsSchema }

// SetOptions decodes supplied options into the frontend's own.
// Called by the pipeline at Build time.
func (f *Frontend) SetOptions(o opt.Options) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return o.Decode(&f.opts) //nolint:wrapcheck // opt.Options.Decode already names the plugin
}

// snapshotOptions returns a copy of the current options, so a Load
// running concurrently with SetOptions sees a stable view.
func (f *Frontend) snapshotOptions() Options {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.opts
}

// Load implements [plugin.Frontend].
//
// The pattern is expanded to a file set, each file is parsed and
// converted, and the results are grouped into one [node.Package] per
// directory. Per-file problems attach to ctx.Diag and the run
// continues; only a pattern that resolves to nothing is fatal, since
// that is a mistake in the invocation rather than in the source.
func (f *Frontend) Load(ctx *plugin.FrontendContext) error {
	opts := f.snapshotOptions()
	ps := ctx.Diag.For(FrontendName)

	files, err := expandPattern(ctx.Pattern, opts)
	if err != nil {
		return err
	}

	for dir, paths := range groupByDir(files) {
		if err := loadPackage(ctx, dir, paths, opts); err != nil {
			ps.Errorf(pkgPos(dir), "load %s: %v", dir, err)
		}
	}
	return nil
}

// loadPackage converts one directory's files into a package,
// consulting the cache first.
//
// The framework owns the caching: it composes the key — folding in the
// composition fingerprint, which is the contract's one MUST — looks it
// up, re-wires an entry it finds, and writes back what this converts.
// What is TypeScript's is the hash of the directory's inputs and the
// conversion itself.
func loadPackage(
	ctx *plugin.FrontendContext, dir string, paths []string, opts Options,
) error {
	return plugin.CacheLoad(ctx, FrontendName, FrontendVersion, dir,
		func() (string, error) { return hashInputs(paths, opts) },
		func() ([]*node.Package, error) { return convertDir(ctx, dir, paths, opts) },
	)
}

// convertDir parses one directory's files into a package.
//
// An empty result rather than a nil package for a directory nothing
// converted: the caller adds whatever comes back, and a directory of
// files this frontend cannot read is an ordinary outcome rather than a
// failure of the run.
func convertDir(
	ctx *plugin.FrontendContext, dir string, paths []string, opts Options,
) ([]*node.Package, error) {
	pkg := &node.Package{
		BaseNode: node.BaseNode{SourcePos: pkgPos(dir)},
		Name:     filepath.Base(dir),
		Path:     dir,
	}
	MetaFrontend.SetAt(
		pkg.EnsureMeta(), FrontendName, meta.AuthorityPlugin, FrontendName, pkgPos(dir),
	)

	converted := false
	for _, path := range paths {
		if convertFile(ctx, pkg, path, opts) {
			converted = true
		}
	}
	if !converted {
		return nil, nil
	}
	dedupePackageImports(pkg)
	return []*node.Package{pkg}, nil
}

// convertFile parses one file and appends its declarations to pkg.
// Reports whether anything was converted.
func convertFile(
	ctx *plugin.FrontendContext, pkg *node.Package, path string, opts Options,
) bool {
	ps := ctx.Diag.For(FrontendName)

	src, err := readSource(path)
	if err != nil {
		ps.Errorf(pkgPos(path), "%v", err)
		return false
	}
	if opts.SkipGeneratedFiles && isGenerated(src) {
		return false
	}

	parsedFile, err := parseFile(path, src)
	if err != nil {
		ps.Errorf(pkgPos(path), "%v", err)
		return false
	}
	defer parsedFile.close()

	c := newConv(parsedFile, pkg.Path, ps, ctx.Parser)
	c.reportSyntaxErrors()

	file := &node.File{
		BaseNode: node.BaseNode{SourcePos: parsedFile.posAt(nil)},
		Name:     filepath.Base(path),
		Path:     path,
		Owner:    pkg,
	}

	for _, decl := range c.declarations(parsedFile.root()) {
		attach(pkg, file, decl)
	}
	pkg.Files = append(pkg.Files, file)
	return true
}

// pkgPos returns a position naming a path and nothing else — the
// shape [position.Pos] documents as "the file is known even if no
// line is", which reads better in a diagnostic than the zero value.
func pkgPos(path string) position.Pos {
	return position.Pos{File: path}
}

// attach files one converted declaration into the package's typed
// buckets and records the import on its owning file.
func attach(pkg *node.Package, file *node.File, decl node.Node) {
	switch d := decl.(type) {
	case *node.Struct:
		pkg.Structs = append(pkg.Structs, d)
	case *node.Interface:
		pkg.Interfaces = append(pkg.Interfaces, d)
	case *node.Alias:
		pkg.Aliases = append(pkg.Aliases, d)
	case *node.Enum:
		pkg.Enums = append(pkg.Enums, d)
	case *node.Function:
		pkg.Functions = append(pkg.Functions, d)
	case *node.Variable:
		pkg.Variables = append(pkg.Variables, d)
	case *node.Constant:
		pkg.Constants = append(pkg.Constants, d)
	case *node.Import:
		d.Owner = file
		file.Imports = append(file.Imports, d)
		// Appended unconditionally; [dedupePackageImports] compacts
		// the result once every file has contributed. A per-append
		// scan would be quadratic in the number of imports a
		// directory holds.
		pkg.Imports = append(pkg.Imports, d)
	}
}

// dedupePackageImports reduces pkg.Imports to one entry per module
// specifier, keeping the first occurrence.
//
// [node.Package.Imports] is documented as the deduplicated union over
// the package's files, and [node.Package.ImportByPath] answers with a
// single entry per path — so without this a consumer counting a
// package's dependencies counts a module once per file that imports
// it. Per-file imports on [node.File.Imports] are left alone: those
// are the declarations as written, and each file genuinely carries
// its own.
//
// First occurrence rather than last, so the surviving entry is the
// one earliest in the package's sorted file order and the result is
// stable across runs.
func dedupePackageImports(pkg *node.Package) {
	if len(pkg.Imports) < 2 {
		return
	}
	seen := make(map[string]struct{}, len(pkg.Imports))
	out := pkg.Imports[:0]
	for _, imp := range pkg.Imports {
		if _, dup := seen[imp.Path]; dup {
			continue
		}
		seen[imp.Path] = struct{}{}
		out = append(out, imp)
	}
	pkg.Imports = out
}

// groupByDir buckets file paths by their containing directory.
//
// One [node.Package] per directory, not per file. TypeScript's unit
// of modularity is the file, so a package per file would be the
// closer translation — and would produce a package per module, which
// is unusable: routing, imports and the store's own grouping all
// assume a package holds related declarations. A directory is what
// people already organise around, and it maps onto the output
// directory a backend writes to.
func groupByDir(paths []string) map[string][]string {
	out := map[string][]string{}
	for _, p := range paths {
		dir := filepath.Dir(p)
		out[dir] = append(out[dir], p)
	}
	return out
}

// compile-time confirmation that the frontend satisfies the role and
// the capability interfaces the pipeline probes for.
var (
	_ plugin.Frontend        = (*Frontend)(nil)
	_ plugin.Versioned       = (*Frontend)(nil)
	_ plugin.EmitVersioned   = (*Frontend)(nil)
	_ plugin.OptionsProvider = (*Frontend)(nil)
)
