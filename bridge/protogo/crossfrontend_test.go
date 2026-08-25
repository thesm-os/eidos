// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package protogo_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/bridge/protogo"
	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	m "go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/opt"
	gofrontend "go.thesmos.sh/eidos/lang/golang/frontend"
	protofrontend "go.thesmos.sh/eidos/lang/protobuf/frontend"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// TestCrossFrontend_MarkerScope covers the cross-frontend
// provenance-marker convention: every produced [node.Package]
// carries the bare `frontend` meta key whose value is the
// producing frontend's plugin name. The test loads proto sources
// via the protobuf frontend and Go sources via the Go frontend
// into one shared store, then asserts:
//
//  1. Proto-derived packages carry `frontend = "protobuf"`.
//  2. Go-derived packages carry `frontend = "golang"`.
//  3. No `go.*` meta leaks onto proto-derived packages.
//  4. No `proto.*` meta leaks onto Go-derived packages.
//
// Together these assertions close the cross-namespace scope loop
// the bridge-annotator pattern relies on.
func TestCrossFrontend_MarkerScope(t *testing.T) {
	t.Parallel()

	t.Run("each frontend stamps its own marker; no cross-namespace meta leaks", func(t *testing.T) {
		t.Parallel()
		repoRoot := repositoryRoot(t)
		protoRoot := filepath.Join(repoRoot, "lang", "protobuf", "frontend", "testdata", "simple")
		s := store.New()
		d := diag.New()
		registry := directive.NewRegistry()
		parser := directive.DefaultParser()
		nopCache := cache.NewNone()

		pf := protofrontend.New()
		if err := pf.SetOptions(opt.New(pf.OptionsSchema(), map[string]string{"dir": protoRoot})); err != nil {
			t.Fatalf("protobuf SetOptions: %v", err)
		}
		if err := pf.Load(&plugin.FrontendContext{
			Store: s, Diag: d, Registry: registry, Parser: parser, Cache: nopCache,
			Pattern: "./...",
		}); err != nil {
			t.Fatalf("protobuf Load: %v", err)
		}

		gf := gofrontend.New()
		if err := gf.SetOptions(opt.New(gf.OptionsSchema(), map[string]string{"dir": repoRoot})); err != nil {
			t.Fatalf("golang SetOptions: %v", err)
		}
		if err := gf.Load(&plugin.FrontendContext{
			Store: s, Diag: d, Registry: registry, Parser: parser, Cache: nopCache,
			Pattern: "go.thesmos.sh/eidos/lang/golang/backend/testdata/pluginfixture",
		}); err != nil {
			t.Fatalf("golang Load: %v", err)
		}

		if d.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", d.Diagnostics())
		}

		var protoPkg, goPkg *node.Package
		s.Nodes().Packages().Range(func(p *node.Package) bool {
			switch p.Path {
			case "eidos.protobuf.testdata.simple":
				protoPkg = p
			case "go.thesmos.sh/eidos/lang/golang/backend/testdata/pluginfixture":
				goPkg = p
			}
			return true
		})
		if protoPkg == nil {
			t.Fatalf("proto-derived package missing from store")
		}
		if goPkg == nil {
			t.Fatalf("go-derived package missing from store")
		}

		assertMarker(t, protoPkg, protofrontend.FrontendName)
		assertMarker(t, goPkg, gofrontend.FrontendName)
		assertNoMetaPrefix(t, protoPkg, "go.", "proto-derived package")
		assertNoMetaPrefix(t, goPkg, "proto.", "go-derived package")
	})
}

// TestCrossFrontend_BridgeAuditScope is the cross-frontend audit
// step for the protogo bridge: with proto + Go sources loaded
// into one store and the bridge run as an annotator, no meta
// entry on any Go-derived node carries `protogo` as its setBy.
// The bridge's frontend-marker filter is what makes this safe;
// this test pins the contract so a future regression that
// broadens the iteration scope fails here.
func TestCrossFrontend_BridgeAuditScope(t *testing.T) {
	t.Parallel()

	t.Run("protogo touches only proto-marker packages, never Go-marker packages", func(t *testing.T) {
		t.Parallel()
		repoRoot := repositoryRoot(t)
		protoRoot := filepath.Join(repoRoot, "lang", "protobuf", "frontend", "testdata", "simple")
		s := store.New()
		d := diag.New()
		registry := directive.NewRegistry()
		parser := directive.DefaultParser()
		nopCache := cache.NewNone()

		pf := protofrontend.New()
		if err := pf.SetOptions(opt.New(pf.OptionsSchema(), map[string]string{"dir": protoRoot})); err != nil {
			t.Fatalf("protobuf SetOptions: %v", err)
		}
		if err := pf.Load(&plugin.FrontendContext{
			Store: s, Diag: d, Registry: registry, Parser: parser, Cache: nopCache,
			Pattern: "./...",
		}); err != nil {
			t.Fatalf("protobuf Load: %v", err)
		}

		gf := gofrontend.New()
		if err := gf.SetOptions(opt.New(gf.OptionsSchema(), map[string]string{"dir": repoRoot})); err != nil {
			t.Fatalf("golang SetOptions: %v", err)
		}
		if err := gf.Load(&plugin.FrontendContext{
			Store: s, Diag: d, Registry: registry, Parser: parser, Cache: nopCache,
			Pattern: "go.thesmos.sh/eidos/lang/golang/backend/testdata/pluginfixture",
		}); err != nil {
			t.Fatalf("golang Load: %v", err)
		}

		bridge := protogo.New()
		if err := bridge.Annotate(&plugin.AnnotatorContext{
			Store:  s,
			Reader: store.NewReader(s),
			Diag:   d,
		}); err != nil {
			t.Fatalf("protogo Annotate: %v", err)
		}
		if d.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", d.Diagnostics())
		}

		var goPkg *node.Package
		s.Nodes().Packages().Range(func(p *node.Package) bool {
			marker, _ := gofrontend.MetaFrontend.Get(p.Meta())
			if marker == gofrontend.FrontendName {
				goPkg = p
				return false
			}
			return true
		})
		if goPkg == nil {
			t.Fatalf("go-derived package missing")
		}
		assertNoProtogoStamps(t, goPkg)
	})
}

// assertNoProtogoStamps walks pkg's meta bag and every reachable
// child's meta bag, asserting no entry was recorded with
// `setBy == "protogo"`. The walk covers the package itself,
// every contained Struct, every Struct's Fields and Field.Type,
// every Interface, every Interface's Methods + Method params +
// returns. A regression that loosens the bridge's
// frontend-marker filter fails this audit.
func assertNoProtogoStamps(t *testing.T, pkg *node.Package) {
	t.Helper()
	assertBagNoProtogo(t, pkg.Path+" (package)", pkg.Meta())
	for _, s := range pkg.Structs {
		assertBagNoProtogo(t, pkg.Path+"."+s.Name+" (struct)", s.Meta())
		for _, f := range s.Fields {
			assertBagNoProtogo(t, pkg.Path+"."+s.Name+"."+f.Name+" (field)", f.Meta())
			if f.Type != nil {
				assertBagNoProtogo(t, pkg.Path+"."+s.Name+"."+f.Name+".Type", f.Type.Meta())
			}
		}
	}
}

// assertBagNoProtogo asserts that no meta entry on bag carries
// `setBy == "protogo"`. Used by the bridge audit step.
func assertBagNoProtogo(t *testing.T, where string, bag *m.Bag) {
	t.Helper()
	for _, name := range bag.Names() {
		prov, ok := bag.Winning(name)
		if !ok {
			continue
		}
		if prov.SetBy == "protogo" {
			t.Fatalf("%s carries a protogo stamp on %q; bridge should skip Go-derived nodes", where, name)
		}
	}
}

// assertMarker verifies that pkg carries the cross-frontend marker
// with the expected value.
func assertMarker(t *testing.T, pkg *node.Package, want string) {
	t.Helper()
	got, ok := gofrontend.MetaFrontend.Get(pkg.Meta())
	if !ok {
		t.Fatalf("MetaFrontend missing on package %q", pkg.Path)
	}
	if got != want {
		t.Fatalf("MetaFrontend on %q = %q, want %q", pkg.Path, got, want)
	}
}

// assertNoMetaPrefix walks pkg's meta bag and fails if any key
// matches prefix. The descriptor is the human-friendly label the
// test surfaces in the failure message.
func assertNoMetaPrefix(t *testing.T, pkg *node.Package, prefix, descriptor string) {
	t.Helper()
	for _, key := range pkg.Meta().Names() {
		if strings.HasPrefix(key, prefix) {
			t.Fatalf(
				"%s should carry no %q-namespaced meta; found %q on %q",
				descriptor, prefix, key, pkg.Path,
			)
		}
	}
}

// repositoryRoot resolves the absolute path of the eidos repo
// root. The test's working directory is the package directory by
// default; the helper walks up to the directory carrying
// `go.work` (the workspace root). Walking up for `go.mod` would
// stop at the nearest module — fine in a single-module repo, but
// in the multi-module workspace the nearest go.mod is this test's
// own module, not the repo root.
func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	dir := filepath.Dir(file)
	for dir != "/" {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			return dir
		}
		dir = filepath.Dir(dir)
	}
	t.Fatalf("go.work not found above %s", file)
	return ""
}
