// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

import (
	"go.thesmos.sh/eidos/core/kind"
)

// File is one output file in the emit tree. Backends group emit
// declarations by their [Target] and produce one File per group;
// File carries the per-file import block and per-file slots
// ("top", "bottom", "init") for cross-cutting content that has no
// natural host on a specific declaration.
//
// Per-file metadata (build tags, copyright headers, file-level doc
// comments) lives in [BaseEmit.DocLines] and through the standard
// slots — backends render the slots alongside the file's grouped
// declarations in slot order.
//
// File does not enumerate the declarations it owns: declarations
// route to files via [Target] equality, and the owning [Package]
// holds the declaration slices directly. This avoids dual-source
// truth for "which declarations belong to this file".
type File struct {
	BaseEmit

	// Name is the file basename inside the output directory
	// (e.g. "user_gen.go"). Matches [Target.Filename] on the
	// declarations routed to this file.
	Name string `json:"name"`

	// Package is the package name the file declares
	// (Go's `package foo`). Matches [Target.Package] on the
	// declarations routed here.
	Package string `json:"package,omitempty"`

	// Dir is the file's output directory relative to the project
	// root. Matches [Target.Dir] on the declarations routed here.
	Dir string `json:"dir,omitempty"`

	// ImportPath is the canonical import path of the package this
	// file declares. Matches [Target.ImportPath] on the
	// declarations routed here; the backend forwards it to the
	// per-file [writer.ImportSet] for same-package import elision.
	ImportPath string `json:"import_path,omitempty"`

	// Imports is the file's import block. Backends commonly derive
	// imports from [ExternalRef] usage at render time via the `imp`
	// template func; explicit Imports let generators force-include
	// packages for side effects or to declare aliases up front.
	Imports []*Import `json:"imports,omitempty"`

	// Owner is the [Package] this file belongs to.
	//
	// Owner is excluded from JSON encoding to break the host →
	// child cycle. Deserialized graphs re-wire Owner via
	// [RewireOwners].
	Owner *Package `json:"-"`

	slotMap
}

// Kind returns [KindFile].
func (*File) Kind() kind.Kind { return KindFile }

// Target returns the [Target] value that declarations route through
// to land in this file. Composed from [File.Dir], [File.Name],
// [File.Package], and [File.ImportPath].
func (f *File) Target() Target {
	return Target{
		Dir:        f.Dir,
		Filename:   f.Name,
		Package:    f.Package,
		ImportPath: f.ImportPath,
	}
}

// Path returns the file path under the project root —
// "Dir/Name" with normalised slash separators. Returns "" when
// either component is empty.
func (f *File) Path() string {
	if f.Dir == "" || f.Name == "" {
		return ""
	}
	return f.Dir + "/" + f.Name
}

// Top returns the "top" slot for content rendered above the file's
// declarations (file header comments, build tags, top-level
// directives). Element kind is unconstrained — top content is
// intentionally heterogeneous.
func (f *File) Top() *Slot { return f.Slot("top") }

// Bottom returns the "bottom" slot for content rendered after the
// file's declarations.
func (f *File) Bottom() *Slot { return f.Slot("bottom") }

// Init returns the "init" slot for items rendered inside an
// `init()` function in this file. The slot accepts [emit.Stmt]
// values for typed statements and plugin-defined emit kinds whose
// template renders a single-statement form (e.g. a registry's
// `Register(...)` call). The slot itself carries no [Slot.ElemKind]
// constraint so plugin-defined kinds reach the renderer.
func (f *File) Init() *Slot { return f.Slot("init") }

// ImportsSlot returns the "imports" slot for cross-cutting import
// injection. Distinct from [File.Imports]: the typed field is for
// the owning generator's direct content; the slot accepts contributions
// from other generators.
func (f *File) ImportsSlot() *Slot { return f.Slot(slotImports) }

// Slot returns the named slot, creating it lazily without an
// element-kind constraint. Used for custom slot names that
// plugin-defined emit kinds declare.
func (f *File) Slot(name string) *Slot { return f.slot(f, name, fileSlotKind(name)) }

// ImportByPath returns the import with the given path, or nil when
// absent. Empty input returns nil.
func (f *File) ImportByPath(path string) *Import {
	if path == "" {
		return nil
	}
	for _, imp := range f.Imports {
		if imp.Path == path {
			return imp
		}
	}
	return nil
}

// ImportByAlias returns the import with the given local alias, or
// nil when absent. Searches the explicit [Import.Alias] field;
// path-derived default aliases are not matched here. Empty input
// returns nil.
func (f *File) ImportByAlias(alias string) *Import {
	if alias == "" {
		return nil
	}
	for _, imp := range f.Imports {
		if imp.Alias == alias {
			return imp
		}
	}
	return nil
}

// fileSlotKind returns the element kind the named slot carries on a
// File, or "" for a name this kind does not reserve.
//
// The kind is a property of the NAME, not of which accessor created
// the slot. Without this, File.Slot(name) created an unconstrained
// slot while the typed accessor created a constrained one, and since
// creation is lookup-or-create the surviving constraint was decided by
// whichever plugin ran first — so two contributors to one host got
// different validation depending on registration order, and the
// permissive path was reachable by accident.
func fileSlotKind(name string) kind.Kind {
	switch name {
	case slotImports:
		return KindImport
	default:
		return ""
	}
}
