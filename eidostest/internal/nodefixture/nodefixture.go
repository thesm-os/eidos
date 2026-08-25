// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package nodefixture builds the small node graphs eidostest's own
// tests drive their suites against.
//
// The suites in this module are language-neutral, and so is this: a
// package, some structs, a synthetic file each. Nothing here knows
// what Go is. A test needing a declaration with fields, methods,
// generics or a rendered source projection wants
// `lang/golang/golangtest/gofixture` instead — and cannot have it,
// because that package lives in the module this one exists to stay
// independent of. `eidostest` requiring `lang/golang` is the module
// cycle; a test-only import puts the requirement in go.mod just as a
// production one does.
//
// Internal, because it is scaffolding rather than surface. A plugin
// author gets the fixture for their own language; this covers the
// three shapes the conformance suites' own tests need and stops.
package nodefixture

import (
	"fmt"
	"strings"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/store"
)

// DefaultName and DefaultPath identify the package [Package] and
// [Store] build. Exported so a test asserting on a qualified name
// spells it from the same constant the fixture used.
const (
	DefaultName = "test"
	DefaultPath = "example.com/test"
)

// Store returns a store holding one package with the named structs.
//
// Fresh on every call: the determinism checks build two stores and
// compare what a plugin makes of each, so a shared one would compare
// a graph against itself.
//
// Panics on anything [store.NodeView.AddPackage] rejects — a
// duplicate name, a nil entry. That is fixture misuse rather than
// test data, and it surfaces better at construction than as an error
// dropped three frames later.
func Store(structs ...string) *store.Store {
	s := store.New()
	if err := s.Nodes().AddPackage(Package(structs...)); err != nil {
		panic(fmt.Errorf("nodefixture: %w", err)) //nolint:forbidigo
	}
	return s
}

// Package returns the package [Store] wraps, unstored — for the
// frontend tests, which hand nodes to a loader rather than reading
// them out of a store.
func Package(structs ...string) *node.Package {
	return PackageIn(DefaultName, DefaultPath, structs...)
}

// PackageIn is [Package] under an explicit name and import path, for
// the tests that load two packages and check they stayed apart.
func PackageIn(name, path string, structs ...string) *node.Package {
	pkg := &node.Package{Name: name, Path: path}
	for _, s := range structs {
		pkg.Structs = append(pkg.Structs, &node.Struct{
			BaseNode: node.BaseNode{SourcePos: position.Pos{File: declFile(name, s)}},
			Name:     s,
			Package:  path,
		})
	}
	return pkg
}

// declFile is the synthetic source file a declaration is stamped
// with.
//
// Positioned rather than left blank because the Layout phase composes
// an output filename from the origin's file basename, and every
// suffix declared in this repo starts with an underscore. A
// positionless origin therefore routes to a name go/packages
// discards, and the pipeline tests that run a real backend would emit
// files the toolchain cannot see — valid Go, on disk, invisible, with
// no diagnostic at any severity.
func declFile(pkgName, declName string) string {
	return pkgName + "/" + strings.ToLower(declName) + ".go"
}
