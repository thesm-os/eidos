// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tsfixture

import (
	"fmt"
	"strings"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/store"
)

// markerAuthority is the provenance recorded beside the frontend
// marker [Builder.Build] stamps. A fixture is not a frontend, and a
// reader auditing where a stamp came from should be able to tell.
const markerAuthority = "tsfixture"

// The identity applied to a [Builder] that never calls
// [Builder.Package].
//
// A TypeScript frontend names a package after the directory it read,
// so the two are a directory and its basename rather than a module
// path and a package clause. A fixture spelling them any other way
// would route its output somewhere a real run never would.
const (
	defaultPackageName = "test"
	defaultPackagePath = "src/test"
)

// anonymousDeclStem is the filename stem [declFile] falls back to for
// a declaration with no name. An unnamed declaration is fixture
// misuse, but it must not reintroduce the empty basename this
// synthetic position exists to prevent.
const anonymousDeclStem = "decl"

// declFile returns the synthetic source file a declaration is stamped
// with — `<pkg>/<lowercased-decl>.ts`.
//
// The Layout phase composes an output filename as
// `<origin-basename><plugin-suffix>`, where the basename comes from
// the origin's [position.Pos] File. A positionless origin routes to
// the bare suffix — `.gen.ts`, a dotfile — which lands on disk and is
// loaded by nothing, with no diagnostic at any severity. Seeding a
// production-shaped position keeps fixture-driven pipelines routing
// to files a bundler can see.
//
// The directory component carries the package name so the composed
// emit target has a non-empty Dir; a target with an empty Dir renders
// a blank path in every harness failure message.
func declFile(pkgName, declName string) string {
	stem := strings.ToLower(declName)
	if stem == "" {
		stem = anonymousDeclStem
	}
	if pkgName == "" {
		return stem + ".ts"
	}
	return pkgName + "/" + stem + ".ts"
}

// retargetPos rewrites p to the synthetic file for declName under
// newPkg, but only when p still holds the synthetic file computed
// under oldPkg. A position the caller set through a sub-builder's Pos
// is left alone — the explicit value always wins.
func retargetPos(p *position.Pos, oldPkg, newPkg, declName string) {
	if p.File == declFile(oldPkg, declName) {
		p.File = declFile(newPkg, declName)
	}
}

// Builder accumulates declarations into a single [node.Package] and
// turns the package into a populated [store.Store] on demand.
//
// All declarations the Builder accepts are added to one package; the
// fixture deliberately does not support multi-package construction —
// unit tests that need cross-package fixtures should build separate
// stores and merge in the test, or graduate to a
// [pipelinetest.Pipeline] driven by a synthetic frontend.
//
// Each declaration entry point ([Builder.Interface], [Builder.Class],
// etc.) takes a configuration callback that receives the matching
// sub-builder. The callback runs synchronously; the Builder's state is
// updated before the entry point returns.
//
// A Builder is not safe for concurrent use. Tests typically construct
// a Builder in setup, configure it, call [Builder.Build], and let it
// fall out of scope.
type Builder struct {
	pkg *node.Package

	// lang is the frontend marker [Builder.Build] stamps, and unmarked
	// suppresses it. Two fields rather than one sentinel, because
	// "stamp nothing" is a deliberate choice a test makes and the empty
	// string is what an unset field holds.
	lang     string
	unmarked bool
}

// New returns a Builder seeded with an empty package whose Name is
// "test" and whose Path is "src/test". Call [Builder.Package] to
// override either value.
func New() *Builder {
	return &Builder{lang: typescript.Language, pkg: &node.Package{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: defaultPackagePath}},
		Name:     defaultPackageName,
		Path:     defaultPackagePath,
	}}
}

// Package overrides the package name and directory on the
// accumulating [node.Package]. Calling Package after declarations
// have already been added is allowed and rewrites the in-progress
// package's identity — existing declarations' Package fields are
// rewritten so qualified names stay coherent with the new path.
//
// The synthetic source file each declaration carries names the
// package too, so the same pass retargets it — otherwise a package
// renamed after its declarations were added would route its generated
// output into the old directory. A position set explicitly through a
// sub-builder's Pos is never rewritten; see [ClassBuilder.Pos].
//
// Reach for [Builder.PackageName] where the directory and the
// declared name deliberately differ, which retargets nothing.
func (b *Builder) Package(name, path string) *Builder {
	old := b.pkg.Name
	b.pkg.Name = name
	b.pkg.Path = path
	b.pkg.SourcePos = position.Pos{File: path}

	for _, s := range b.pkg.Structs {
		s.Package = path
		retargetPos(&s.SourcePos, old, name, s.Name)
		for _, f := range s.Fields {
			retargetPos(&f.SourcePos, old, name, s.Name)
		}
		for _, m := range s.Methods {
			retargetPos(&m.SourcePos, old, name, s.Name)
		}
	}
	for _, i := range b.pkg.Interfaces {
		i.Package = path
		retargetPos(&i.SourcePos, old, name, i.Name)
		for _, f := range i.Fields {
			retargetPos(&f.SourcePos, old, name, i.Name)
		}
		for _, m := range i.Methods {
			retargetPos(&m.SourcePos, old, name, i.Name)
		}
	}
	for _, f := range b.pkg.Functions {
		f.Package = path
		retargetPos(&f.SourcePos, old, name, f.Name)
	}
	for _, v := range b.pkg.Variables {
		v.Package = path
		retargetPos(&v.SourcePos, old, name, v.Name)
	}
	for _, c := range b.pkg.Constants {
		c.Package = path
		retargetPos(&c.SourcePos, old, name, c.Name)
	}
	for _, e := range b.pkg.Enums {
		e.Package = path
		retargetPos(&e.SourcePos, old, name, e.Name)
	}
	for _, a := range b.pkg.Aliases {
		a.Package = path
		retargetPos(&a.SourcePos, old, name, a.Name)
	}
	return b
}

// PackageName sets the package's declared name without touching its
// directory or retargeting any declaration's synthetic source file.
//
// A TypeScript frontend derives the name from the directory, so the
// two normally agree. They stop agreeing the moment a test drives a
// graph some other frontend built — a bridge naming a package after
// its proto file while its declarations sit in a directory named
// something else — and a generator composing a specifier from one
// while a file references the other is exactly the bug a fixture
// wants to reproduce.
func (b *Builder) PackageName(name string) *Builder {
	b.pkg.Name = name
	return b
}

// Import records a module specifier on the package's deduped import
// set.
//
// The package-level union, not a file's import block. Anything
// resolving a name reads a [node.File], because TypeScript scopes an
// imported binding to the module that wrote the import — declare the
// file with [Builder.File] when that is what the test is about.
func (b *Builder) Import(path string) *Builder {
	return b.ImportAs("", path)
}

// ImportAs records an import under an explicit local name —
// TypeScript's `import { User as U } from './user'`, or the namespace
// form `import * as pb from './gen'`.
//
// Package-scoped, like [Builder.Import], and subject to the same
// caveat: the union is a view over every file's imports and no
// binding resolves against it, because two modules may bind one
// specifier to different names.
func (b *Builder) ImportAs(alias, path string) *Builder {
	b.pkg.Imports = append(b.pkg.Imports, &node.Import{
		BaseNode: node.BaseNode{SourcePos: b.pkg.Pos()},
		Path:     path,
		Alias:    alias,
	})
	return b
}

// Directive attaches d to the package's directive list.
func (b *Builder) Directive(d *directive.Directive) *Builder {
	b.pkg.DirectiveList = append(b.pkg.DirectiveList, d)
	return b
}

// Interface declares a TypeScript interface in the accumulating
// package. When fn is non-nil it runs against a fresh
// [InterfaceBuilder] before Interface returns, allowing the caller to
// populate properties, methods, heritage, directives and docs.
//
// Duplicate names within the same package cause [Builder.Build] to
// fail with [store.ErrDuplicateQName]; the duplicate is not detected
// at Interface call time so callers may shadow earlier names
// intentionally in pathological tests.
func (b *Builder) Interface(name string, fn func(*InterfaceBuilder)) *Builder {
	file := declFile(b.pkg.Name, name)
	i := &node.Interface{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: file}},
		Name:     name,
		Package:  b.pkg.Path,
	}
	ib := &InterfaceBuilder{i: i, file: file}
	if fn != nil {
		fn(ib)
	}
	b.pkg.Interfaces = append(b.pkg.Interfaces, i)
	return b
}

// Class declares a TypeScript class, which the neutral model records
// as a [node.Struct].
//
// Named for what the source says rather than for the node it builds:
// a fixture author writes the declaration they mean to describe, and
// `Struct("User", …)` describes nothing anyone would write in a `.ts`
// file. [ClassBuilder.Node] returns the [node.Struct] for a test
// asserting on the model directly.
func (b *Builder) Class(name string, fn func(*ClassBuilder)) *Builder {
	file := declFile(b.pkg.Name, name)
	s := &node.Struct{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: file}},
		Name:     name,
		Package:  b.pkg.Path,
	}
	cb := &ClassBuilder{s: s, file: file}
	if fn != nil {
		fn(cb)
	}
	b.pkg.Structs = append(b.pkg.Structs, s)
	return b
}

// Function declares a module-level function.
func (b *Builder) Function(name string, fn func(*FunctionBuilder)) *Builder {
	f := &node.Function{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: declFile(b.pkg.Name, name)}},
		Name:     name,
		Package:  b.pkg.Path,
	}
	fb := &FunctionBuilder{f: f}
	if fn != nil {
		fn(fb)
	}
	b.pkg.Functions = append(b.pkg.Functions, f)
	return b
}

// Variable declares a module-level `let` or `var` binding.
func (b *Builder) Variable(name string, fn func(*VariableBuilder)) *Builder {
	v := &node.Variable{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: declFile(b.pkg.Name, name)}},
		Name:     name,
		Package:  b.pkg.Path,
	}
	vb := &VariableBuilder{v: v}
	if fn != nil {
		fn(vb)
	}
	b.pkg.Variables = append(b.pkg.Variables, v)
	return b
}

// Constant declares a module-level `const` binding.
func (b *Builder) Constant(name string, fn func(*ConstantBuilder)) *Builder {
	c := &node.Constant{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: declFile(b.pkg.Name, name)}},
		Name:     name,
		Package:  b.pkg.Path,
	}
	cb := &ConstantBuilder{c: c}
	if fn != nil {
		fn(cb)
	}
	b.pkg.Constants = append(b.pkg.Constants, c)
	return b
}

// Enum declares a TypeScript enum.
//
// Unlike Go's, it is a declaration in its own right rather than a
// group of constants a frontend recognised — so a variant carries the
// value the source wrote and nothing infers one. Leave a variant's
// value unset for a member that took the implicit counter; see
// [EnumBuilder.Variant].
func (b *Builder) Enum(name string, fn func(*EnumBuilder)) *Builder {
	file := declFile(b.pkg.Name, name)
	e := &node.Enum{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: file}},
		Name:     name,
		Package:  b.pkg.Path,
	}
	eb := &EnumBuilder{e: e, file: file}
	if fn != nil {
		fn(eb)
	}
	b.pkg.Enums = append(b.pkg.Enums, e)
	return b
}

// Alias declares a type alias — TypeScript's `type X = Y`.
//
// Always a true alias. TypeScript has no counterpart to Go's type
// *definition*, which creates a distinct type sharing a
// representation; `type X = Y` introduces a name for Y and nothing
// more. [AliasBuilder] therefore has no True method, and
// [node.Alias.IsAlias] is set on every declaration this builds.
func (b *Builder) Alias(name string, fn func(*AliasBuilder)) *Builder {
	file := declFile(b.pkg.Name, name)
	a := &node.Alias{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: file}},
		Name:     name,
		Package:  b.pkg.Path,
		IsAlias:  true,
	}
	ab := &AliasBuilder{a: a, file: file}
	if fn != nil {
		fn(ab)
	}
	b.pkg.Aliases = append(b.pkg.Aliases, a)
	return b
}

// PackageNode returns the [node.Package] accumulated so far. The
// returned pointer aliases the Builder's internal storage — callers
// that mutate it will affect subsequent [Builder.Build] calls. Use
// this accessor to set typed metadata on individual nodes after
// construction, or to assert against the raw shape.
func (b *Builder) PackageNode() *node.Package { return b.pkg }

// Build returns a fresh [store.Store] populated with the accumulated
// package. The builder is reusable: each call constructs an
// independent store, so consecutive calls return distinct stores
// containing the same configured package.
//
// Build panics on any state the underlying [store.NodeView.AddPackage]
// rejects — duplicate qualified names, nil entries. Such states reflect
// builder misuse rather than test data, and a panic at construction is
// easier to debug than a returned error swallowed silently.
func (b *Builder) Build() *store.Store {
	if !b.unmarked && b.lang != "" {
		node.MetaFrontend.Set(b.pkg.EnsureMeta(), b.lang, markerAuthority)
	}
	s := store.New()
	if err := s.Nodes().AddPackage(b.pkg); err != nil {
		// Test-only fixture; misuse-on-construction surfaces as a panic.
		panic(fmt.Errorf("tsfixture: build failed: %w", err)) //nolint:forbidigo
	}
	return s
}

// Language overrides the frontend marker stamped on the built package.
//
// For a test whose subject is a graph some other frontend produced —
// a bridge is the case — where the declarations are still most easily
// spelled with this builder but the package must not claim to have
// been parsed from TypeScript.
func (b *Builder) Language(name string) *Builder {
	b.lang = name
	return b
}

// Unmarked suppresses the frontend marker, producing the graph a
// package nothing claimed would have.
//
// For a test whose subject IS that path: every plugin dispatching per
// package treats an unmarked one as not its business and skips it
// without a diagnostic, deliberately, so that fixtures and bridges do
// not drown a run in warnings.
func (b *Builder) Unmarked() *Builder {
	b.unmarked = true
	return b
}
