// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package protobuf

import (
	"path/filepath"
	"sort"
	"strings"

	"google.golang.org/protobuf/reflect/protoreflect"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// convertFiles walks the resolved descriptor set and produces one
// [node.Package] per distinct proto package qualifier. Multiple
// source files sharing one proto package merge into a single
// Package entry — the merge is deterministic: descriptor iteration
// sorts by file path before any merging or stamping, so the
// produced graph is byte-stable across runs regardless of
// protocompile's internal resolve order.
//
// Each produced Package carries the cross-frontend provenance
// marker (`frontend = "protobuf"`) plus a [node.File] entry per
// contributing proto source with that file's imports recorded;
// the Package's Imports slice is the deduplicated union across
// every contributing file.
func convertFiles(
	ctx *plugin.FrontendContext, ps *diag.PluginSink,
	descriptors []protoreflect.FileDescriptor,
) []*node.Package {
	sorted := sortDescriptors(descriptors)
	pkgs := map[string]*node.Package{}
	order := []string{}
	for _, fd := range sorted {
		qualifier := string(fd.Package())
		pkg, exists := pkgs[qualifier]
		if !exists {
			pkg = newProtoPackage(qualifier, fd.Path())
			pkgs[qualifier] = pkg
			order = append(order, qualifier)
		}
		appendFile(pkg, fd)
		stampFileOptions(ps, pkg, fd)
		// File-level enums precede nested enums so the produced
		// Enums slice mirrors the proto source order: top-level
		// declarations first, then the enums each message
		// contributes from inside its braces.
		convertEnums(ctx, pkg, fd)
		convertMessages(ctx, pkg, fd)
		convertServices(ctx, pkg, fd)
	}
	out := make([]*node.Package, 0, len(order))
	for _, qualifier := range order {
		pkg := pkgs[qualifier]
		dedupeImports(pkg)
		// The cache-hit path rewires here too — JSON breaks the
		// cycle with `json:"-"` on every Owner — so wiring on the
		// conversion path as well is what makes the two routes
		// agree on what lands in the store. Idempotent: every
		// assignment is an unconditional overwrite of a pointer the
		// converter either left nil or already set correctly.
		node.RewireOwners(pkg)
		out = append(out, pkg)
	}
	return out
}

// addPackages registers each converted package with the store.
//
// Split from [convertFiles] so a cache hit, which produces packages
// by deserialisation rather than conversion, joins the same
// registration path. Both routes must agree on what lands in the
// store, or a cached run and a fresh one diverge.
func addPackages(ctx *plugin.FrontendContext, ps *diag.PluginSink, pkgs []*node.Package) {
	for _, pkg := range pkgs {
		if err := ctx.Store.Nodes().AddPackage(pkg); err != nil {
			ps.Errorf(
				position.Pos{File: firstFilePath(pkg)},
				"protobuf: add package %s: %v", pkg.Path, err,
			)
		}
	}
}

// sortDescriptors returns descriptors sorted by [protoreflect.FileDescriptor.Path]
// so the per-package merge order is deterministic across runs.
// protocompile resolves files in dependency order, which can vary
// between runs that touch transitively-shared imports — sorting
// here normalises that variation.
func sortDescriptors(descriptors []protoreflect.FileDescriptor) []protoreflect.FileDescriptor {
	out := append([]protoreflect.FileDescriptor(nil), descriptors...)
	sort.Slice(out, func(i, j int) bool { return out[i].Path() < out[j].Path() })
	return out
}

// newProtoPackage allocates a fresh [node.Package] for the
// supplied proto package qualifier and stamps the cross-frontend
// provenance marker on it. Name is the last dotted segment of
// qualifier (`simple` for `eidos.protobuf.testdata.simple`); Path
// is the full qualifier verbatim. A qualifier without a dot uses
// the whole string as Name; the empty qualifier produces a
// package with empty Name and Path (callers reject such inputs
// upstream).
//
// firstFile is the first contributing proto file's path; it
// anchors [node.BaseNode.SourcePos] so consumers walking the
// package tree (the backend's Source: header attribution, the
// manifest's provenance trail, `eidos explain`) resolve every
// package back to a declaring source without inspecting child
// declarations.
func newProtoPackage(qualifier, firstFile string) *node.Package {
	name := qualifier
	if dot := strings.LastIndex(qualifier, "."); dot >= 0 {
		name = qualifier[dot+1:]
	}
	pkg := &node.Package{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: firstFile}},
		Name:     name,
		Path:     qualifier,
	}
	stampFrontendMarker(pkg)
	return pkg
}

// appendFile records one [node.File] on pkg derived from fd. Name
// is the file's basename; Path is the protocompile-resolved path
// (relative to the configured import root, matching the source-form
// `import "..."` declarations downstream consumers cross-reference
// against). The file's import declarations land on
// [node.File.Imports] in source order; the package-level
// deduplicated union is computed post-pass by [dedupeImports].
func appendFile(pkg *node.Package, fd protoreflect.FileDescriptor) {
	path := fd.Path()
	pkg.Files = append(pkg.Files, &node.File{
		BaseNode: node.BaseNode{SourcePos: position.Pos{File: path}},
		Name:     filepath.Base(path),
		Path:     path,
		Imports:  collectFileImports(fd),
	})
}

// firstFilePath returns the path of the first file recorded on pkg,
// or the package's path qualifier when no files have landed yet.
// Used as a fallback diagnostic position when a store-side
// AddPackage call fails — the file path anchors the message even
// though the failure isn't tied to any single declaration.
func firstFilePath(pkg *node.Package) string {
	if len(pkg.Files) > 0 {
		return pkg.Files[0].Path
	}
	return pkg.Path
}
