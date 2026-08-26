// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tsfixture

import (
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// FileBuilder configures a [node.File] within a [Builder]'s
// accumulating package — a module and, mainly, its import block.
//
// # Why a file exists at all when declarations do not live on one
//
// [node.Package] holds declarations as flat slices regardless of which
// file declared them, which is the view generators want. Imports are
// the exception: TypeScript scopes an imported binding to the module
// that wrote the import, so `User` means whatever was imported *there*
// and means nothing one file over. Anything resolving a name reads a
// [node.File]; a fixture that cannot build one cannot exercise it, and
// [Builder.Import] populates only the package-level deduplicated
// union, which has no local names and no per-module scope.
//
// # This does not place declarations
//
// A declaration's file comes from its [position.Pos], which the
// builder synthesises as `<pkg>/<lowercased-name>.ts`. Declaring a
// file here does not move anything into it and nothing checks that the
// two agree — pin a declaration's position with the sub-builder's Pos
// when a test needs them to.
type FileBuilder struct {
	file *node.File
}

// Node returns the underlying [node.File].
func (b *FileBuilder) Node() *node.File { return b.file }

// Import records an import with no local rename — the `User` in
// `import { User } from './user'`.
//
// Also appended to the package's deduplicated union, because that is
// what a frontend produces and a consumer reading
// [node.Package.Imports] would otherwise see a package importing
// nothing while its modules import plenty.
func (b *FileBuilder) Import(path string) *FileBuilder {
	return b.ImportAs("", path)
}

// ImportAs records an import under an explicit local name —
// TypeScript's `import { User as U } from './user'`, or the namespace
// form `import * as pb from './gen'`.
//
// The local name is the whole reason module-scoped imports are worth
// modelling. Without one a binding is derivable from the specifier's
// last segment, which is what every consumer assumed before there was
// a way to write a fixture that disagrees; with one, the derived
// answer is wrong and nothing says so.
func (b *FileBuilder) ImportAs(alias, path string) *FileBuilder {
	imp := &node.Import{
		BaseNode: node.BaseNode{SourcePos: b.file.Pos()},
		Path:     path,
		Alias:    alias,
		Owner:    b.file,
	}
	typescript.MetaModuleSpecifier.Set(imp.EnsureMeta(), path, markerAuthority)
	b.file.Imports = append(b.file.Imports, imp)
	return b
}

// ImportType records a type-only import — `import type { User } from
// './user'`.
//
// Worth telling apart from the value form: a type-only import is
// erased at compile time, so a generated file that emitted it for a
// value it actually constructs fails at run time with the binding
// undefined, and the compiler says nothing.
func (b *FileBuilder) ImportType(path string) *FileBuilder {
	b.ImportAs("", path)
	imp := b.file.Imports[len(b.file.Imports)-1]
	typescript.MetaTypeOnly.Set(imp.EnsureMeta(), true, markerAuthority)
	return b
}

// ReExport records a re-export — `export { User } from './user'` —
// naming the specifier and the names it forwards.
//
// Modelled as an import because that is what the frontend produces:
// the module names a specifier and pulls bindings from it, and whether
// they are then re-exposed is what [typescript.MetaReExport]
// distinguishes. Pass no names for the star form, `export * from`.
func (b *FileBuilder) ReExport(path string, names ...string) *FileBuilder {
	b.ImportAs("", path)
	imp := b.file.Imports[len(b.file.Imports)-1]
	typescript.MetaReExport.Set(imp.EnsureMeta(), true, markerAuthority)
	if len(names) > 0 {
		typescript.MetaReExportNames.Set(imp.EnsureMeta(), names, markerAuthority)
	}
	return b
}

// Docs appends file-level doc-comment lines.
func (b *FileBuilder) Docs(lines ...string) *FileBuilder {
	b.file.DocLines = append(b.file.DocLines, lines...)
	return b
}

// Directive attaches d to the file's directive list.
func (b *FileBuilder) Directive(d *directive.Directive) *FileBuilder {
	b.file.DirectiveList = append(b.file.DirectiveList, d)
	return b
}

// File declares a module on the accumulating package. When fn is
// non-nil it runs against a fresh [FileBuilder] before File returns.
//
// Naming a file that already exists returns a builder over the
// existing one rather than shadowing it, so a fixture assembled across
// several calls accumulates imports into one module — which is what a
// second `File("user.ts", …)` reads as, and the alternative is two
// files of one name, a shape no filesystem produces.
func (b *Builder) File(name string, fn func(*FileBuilder)) *Builder {
	fb := &FileBuilder{file: b.fileNamed(name)}
	if fn != nil {
		fn(fb)
	}
	b.syncPackageImports()
	return b
}

// fileNamed returns the package's file of that name, creating it on
// first mention. Path is composed the way the builder composes a
// declaration's synthetic position, so a fixture pinning a declaration
// into this file with Pos and one declaring the file here agree on
// what its path is.
func (b *Builder) fileNamed(name string) *node.File {
	if f := b.pkg.FileByName(name); f != nil {
		return f
	}
	path := name
	if b.pkg.Name != "" {
		path = b.pkg.Name + "/" + name
	}
	f := &node.File{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: path, Line: 1, Column: 1}},
		Name:     name,
		Path:     path,
		Owner:    b.pkg,
	}
	b.pkg.Files = append(b.pkg.Files, f)
	return f
}

// syncPackageImports rebuilds [node.Package.Imports] as the
// deduplicated union of every file's imports plus any recorded
// directly on the package.
//
// Rebuilt rather than appended to, because a module's import block is
// mutable through its [FileBuilder] after the file was declared, and a
// union computed once would then describe an earlier state. Dedup is
// by specifier and local name together: two modules importing one
// specifier under different names are two distinct facts, and
// collapsing them loses the one a resolver needs.
func (b *Builder) syncPackageImports() {
	seen := make(map[[2]string]struct{}, len(b.pkg.Imports))
	union := make([]*node.Import, 0, len(b.pkg.Imports))
	add := func(imp *node.Import) {
		key := [2]string{imp.Path, imp.Alias}
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		union = append(union, &node.Import{
			BaseNode: node.BaseNode{SourcePos: imp.Pos()},
			Path:     imp.Path,
			Alias:    imp.Alias,
			Owner:    b.pkg,
		})
	}
	for _, imp := range b.pkg.Imports {
		add(imp)
	}
	for _, f := range b.pkg.Files {
		for _, imp := range f.Imports {
			add(imp)
		}
	}
	b.pkg.Imports = union
}
