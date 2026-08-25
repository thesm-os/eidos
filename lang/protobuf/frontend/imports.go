// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// collectFileImports returns one [node.Import] per declaration in
// fd's import list, in source order. Each import carries the
// imported proto file's path verbatim on [node.Import.Path];
// [node.Import.Alias] stays empty because proto has no
// import-alias syntax. Public and weak modifiers are not surfaced
// on the import node — consumers needing them read the underlying
// descriptor through the cache layer.
//
// Per-import [node.BaseNode.SourcePos] anchors to the declaring
// file's path. protoreflect does not surface a per-import line
// number, so the position is file-only — sufficient for
// provenance attribution without requiring source-line precision.
func collectFileImports(fd protoreflect.FileDescriptor) []*node.Import {
	imps := fd.Imports()
	count := imps.Len()
	if count == 0 {
		return nil
	}
	pos := position.Pos{File: fd.Path()}
	out := make([]*node.Import, 0, count)
	for i := range count {
		imp := imps.Get(i)
		out = append(out, &node.Import{
			BaseNode: node.BaseNode{SourcePos: pos},
			Path:     imp.Path(),
		})
	}
	return out
}

// dedupeImports populates pkg.Imports with the deduplicated union of
// every contributing file's imports. The first File declaring a
// given path wins the package-level entry; later duplicates are
// dropped. Iteration is per-file in pkg.Files order (which the
// converter already sorts alphabetically), so the resulting union
// is byte-stable across runs.
//
// Each package-level [node.Import] is a fresh allocation rather
// than a back-reference to the per-file instance — value-sharing
// the BaseNode would alias the lazily-allocated meta bag and let
// per-file meta mutations leak into the package-level view. The
// fresh allocation carries the winning file's path on its own
// SourcePos so the package-level view still attributes each
// import to a declaring file.
func dedupeImports(pkg *node.Package) {
	seen := map[string]struct{}{}
	for _, f := range pkg.Files {
		for _, imp := range f.Imports {
			if _, dup := seen[imp.Path]; dup {
				continue
			}
			seen[imp.Path] = struct{}{}
			pkg.Imports = append(pkg.Imports, &node.Import{
				BaseNode: node.BaseNode{SourcePos: position.Pos{File: f.Path}},
				Path:     imp.Path,
			})
		}
	}
}
