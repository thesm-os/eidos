// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package storefixture

import (
	"fmt"
	"strings"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/store"
)

// defaultPackageName is the package name applied to a [Builder] that
// never calls [Builder.Package].
const defaultPackageName = "test"

// defaultPackagePath is the import path applied to a [Builder] that
// never calls [Builder.Package].
const defaultPackagePath = "example.com/test"

// anonymousDeclStem is the filename stem [declFile] falls back to
// for a declaration with no name. An unnamed declaration is fixture
// misuse, but it must not reintroduce the empty basename this
// synthetic position exists to prevent.
const anonymousDeclStem = "decl"

// declFile returns the synthetic source file a declaration is
// stamped with — `<pkg>/<lowercased-decl>.go`.
//
// # Why the fixture positions anything at all
//
// The Layout phase composes an output filename as
// `<origin-basename><plugin-suffix>`, where the basename comes from
// the origin's [position.Pos] File. A positionless origin therefore
// routes to the bare suffix, and every suffix declared in this repo
// starts with an underscore — a basename go/packages discards before
// packages.Load ever sees the file. The generated file lands on
// disk, is valid Go, and does not exist as far as the toolchain is
// concerned, with no diagnostic at any severity. Seeding a
// production-shaped position keeps fixture-driven pipelines routing
// to files the toolchain can see.
//
// The directory component carries the package name so the composed
// emit target has a non-empty Dir; a target with an empty Dir
// renders a blank path in every harness failure message.
func declFile(pkgName, declName string) string {
	stem := strings.ToLower(declName)
	if stem == "" {
		stem = anonymousDeclStem
	}
	if pkgName == "" {
		return stem + ".go"
	}
	return pkgName + "/" + stem + ".go"
}

// retargetPos rewrites p to the synthetic file for declName under
// newPkg, but only when p still holds the synthetic file computed
// under oldPkg. A position the caller set through a sub-builder's
// Pos is left alone — the explicit value always wins.
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
// stores and merge in the test, or graduate to a [pipelinetest.Pipeline]
// driven by a synthetic frontend.
//
// Each declaration entry point ([Builder.Struct], [Builder.Interface],
// etc.) takes a configuration callback that receives the matching
// sub-builder. The callback runs synchronously; the Builder's state is
// updated before the entry point returns.
//
// A Builder is not safe for concurrent use. Tests typically construct
// a Builder in setup, configure it, call [Builder.Build], and let it
// fall out of scope.
type Builder struct {
	pkg *node.Package
}

// New returns a Builder seeded with an empty package whose Name is
// "test" and whose Path is "example.com/test". Call [Builder.Package]
// to override either value.
func New() *Builder {
	return &Builder{pkg: &node.Package{
		Name: defaultPackageName,
		Path: defaultPackagePath,
	}}
}

// Package overrides the package name and import path on the
// accumulating [node.Package]. Calling Package after declarations
// have already been added is allowed and rewrites the in-progress
// package's identity — existing decls' [node.Struct.Package],
// [node.Function.Package], and equivalents are rewritten so qualified
// names stay coherent with the new path.
//
// The synthetic source file each declaration carries names the
// package too, so the same pass retargets it — otherwise a package
// renamed after its declarations were added would route its
// generated output into the old package's directory. A position set
// explicitly through a sub-builder's Pos is never rewritten; see
// [StructBuilder.Pos].
func (b *Builder) Package(name, path string) *Builder {
	old := b.pkg.Name
	b.pkg.Name = name
	b.pkg.Path = path
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

// Import records an import path on the package's deduped import set.
// Imports are rarely meaningful in unit-test fixtures but the option
// is here so test cases that inspect the import view of a frontend's
// output have a way to seed entries.
func (b *Builder) Import(path string) *Builder {
	b.pkg.Imports = append(b.pkg.Imports, &node.Import{Path: path, Owner: b.pkg})
	return b
}

// Directive attaches d to the package's own directive list.
//
// Every sub-builder has had one and the package had not, so a fixture
// for a package-scoped plugin — one reading `+gen:x` off the package
// rather than off a declaration — had to append to
// [Builder.PackageNode]'s DirectiveList by hand. That is the only
// reason such a test touched the node graph directly, and it put the
// fixture's one interesting fact outside the fixture.
//
// Distinct from a directive on a declaration: a plugin keyed to the
// package applies to everything in it, which is what makes it
// package-scoped rather than a shorthand for repeating the directive.
func (b *Builder) Directive(d *directive.Directive) *Builder {
	b.pkg.DirectiveList = append(b.pkg.DirectiveList, d)
	return b
}

// Struct declares a struct in the accumulating package. When fn is
// non-nil it runs against a fresh [StructBuilder] before Struct
// returns, allowing the caller to populate fields, methods, embeds,
// directives, and docs.
//
// Duplicate struct names within the same package cause [Builder.Build]
// to fail with [store.ErrDuplicateQName]; the duplicate is not detected
// at Struct call time so callers may shadow earlier names intentionally
// in pathological tests.
func (b *Builder) Struct(name string, fn func(*StructBuilder)) *Builder {
	file := declFile(b.pkg.Name, name)
	s := &node.Struct{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: file}},
		Name:     name,
		Package:  b.pkg.Path,
	}
	sb := &StructBuilder{s: s, pkgPath: b.pkg.Path, file: file}
	if fn != nil {
		fn(sb)
	}
	b.pkg.Structs = append(b.pkg.Structs, s)
	return b
}

// Interface declares an interface in the accumulating package.
func (b *Builder) Interface(name string, fn func(*InterfaceBuilder)) *Builder {
	file := declFile(b.pkg.Name, name)
	i := &node.Interface{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: file}},
		Name:     name,
		Package:  b.pkg.Path,
	}
	ib := &InterfaceBuilder{i: i, pkgPath: b.pkg.Path, file: file}
	if fn != nil {
		fn(ib)
	}
	b.pkg.Interfaces = append(b.pkg.Interfaces, i)
	return b
}

// Function declares a standalone (non-method) function.
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

// Variable declares a package-level variable.
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

// Constant declares a package-level constant that is not part of an
// idiomatic enum group.
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

// Enum declares an enum (a group of typed constants sharing an
// underlying type) in the accumulating package.
func (b *Builder) Enum(name string, fn func(*EnumBuilder)) *Builder {
	e := &node.Enum{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: declFile(b.pkg.Name, name)}},
		Name:     name,
		Package:  b.pkg.Path,
	}
	eb := &EnumBuilder{e: e}
	if fn != nil {
		fn(eb)
	}
	b.pkg.Enums = append(b.pkg.Enums, e)
	return b
}

// Alias declares a type alias or type definition. Use
// [AliasBuilder.True] to mark the declaration as an alias
// (`type X = Y`) rather than a definition (`type X Y`).
func (b *Builder) Alias(name string, fn func(*AliasBuilder)) *Builder {
	a := &node.Alias{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: declFile(b.pkg.Name, name)}},
		Name:     name,
		Package:  b.pkg.Path,
	}
	ab := &AliasBuilder{a: a}
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
// construction or to assert against the raw shape.
func (b *Builder) PackageNode() *node.Package { return b.pkg }

// Build returns a fresh [store.Store] populated with the accumulated
// package. The builder is reusable: each call constructs an
// independent store, so consecutive calls return distinct stores
// containing the same configured package.
//
// Build panics on any state the underlying [store.NodeView.AddPackage]
// rejects — duplicate qualified names, nil entries. Such states reflect
// builder misuse rather than test data, and a panic at construction is
// easier to debug than a returned error swallowed silently. Tests
// catch the panic through the standard testing.T flow.
func (b *Builder) Build() *store.Store {
	s := store.New()
	if err := s.Nodes().AddPackage(b.pkg); err != nil {
		// Test-only fixture; misuse-on-construction surfaces as a panic.
		panic(fmt.Errorf("storefixture: build failed: %w", err)) //nolint:forbidigo
	}
	return s
}
