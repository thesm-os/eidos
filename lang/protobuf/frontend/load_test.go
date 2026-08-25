// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/opt"
	"go.thesmos.sh/eidos/lang/protobuf/frontend"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// TestLoad_ParseableProto3 covers the happy path: a one-file
// proto3 fixture compiles cleanly under protocompile, produces no
// error diagnostics, and the frontend records no warnings.
func TestLoad_ParseableProto3(t *testing.T) {
	t.Parallel()

	t.Run("simple proto3 fixture parses without diagnostics", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "simple", "./...")
		if env.diag.HasErrors() {
			t.Fatalf("expected no error diagnostics; got %+v", env.diag.Diagnostics())
		}
		warnsFromProtobuf := 0
		for _, d := range env.diag.Diagnostics() {
			if d.Plugin == frontend.FrontendName && d.Severity == diag.Warn {
				warnsFromProtobuf++
			}
		}
		if warnsFromProtobuf != 0 {
			t.Fatalf(
				"expected zero protobuf-frontend warnings; got %d (%+v)",
				warnsFromProtobuf,
				env.diag.Diagnostics(),
			)
		}
	})
}

// TestLoad_RejectsEditions covers the proto-editions guard: a
// fixture file whose first declaration is `edition = "..."` must
// surface a positioned error diagnostic attributed to the frontend's
// plugin name; remaining files in the same load keep parsing.
func TestLoad_RejectsEditions(t *testing.T) {
	t.Parallel()

	t.Run("edition source surfaces a positioned diag.Error", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "editions", "./...")
		var matched *diag.Diag
		diagnostics := env.diag.Diagnostics()
		for i := range diagnostics {
			d := &diagnostics[i]
			if d.Plugin != frontend.FrontendName {
				continue
			}
			if d.Severity != diag.Error {
				continue
			}
			if !strings.Contains(strings.ToLower(d.Message), "edition") {
				continue
			}
			matched = d
			break
		}
		if matched == nil {
			t.Fatalf(
				"expected a protobuf-frontend Error diagnostic mentioning 'edition'; got %+v",
				diagnostics,
			)
		}
		if matched.Pos.File == "" {
			t.Fatalf(
				"diagnostic position should carry the offending file path; got %+v",
				matched.Pos,
			)
		}
	})
}

// TestLoad_SurfacesParseErrors covers the parse-error path: a
// malformed proto3 source must surface a positioned `diag.Error`
// attributed to the frontend, with the file path on the diagnostic
// matching the offending source. protocompile's reporter callbacks
// drive the funnel; the test pins the contract end-to-end.
func TestLoad_SurfacesParseErrors(t *testing.T) {
	t.Parallel()

	t.Run("malformed message body surfaces a positioned diag.Error", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "parseerror", "./...")
		var matched *diag.Diag
		diagnostics := env.diag.Diagnostics()
		for i := range diagnostics {
			d := &diagnostics[i]
			if d.Plugin != frontend.FrontendName {
				continue
			}
			if d.Severity != diag.Error {
				continue
			}
			matched = d
			break
		}
		if matched == nil {
			t.Fatalf(
				"expected a protobuf-frontend Error diagnostic; got %+v",
				diagnostics,
			)
		}
		if matched.Pos.File == "" {
			t.Fatalf(
				"diagnostic position should carry the offending file path; got %+v",
				matched.Pos,
			)
		}
		if !strings.HasSuffix(filepath.ToSlash(matched.Pos.File), "broken.proto") {
			t.Fatalf(
				"diagnostic should anchor on broken.proto; got %q",
				matched.Pos.File,
			)
		}
		if matched.Pos.Line == 0 {
			t.Fatalf(
				"diagnostic should carry a non-zero line; got %+v",
				matched.Pos,
			)
		}
	})
}

// TestLoad_GoStyleGlobOutsideProtoTree pins the resolver's
// silent-skip behaviour for Go-style recursive patterns that
// resolve to directories holding no `.proto` files. The protobuf
// frontend shares a pattern slice with sibling frontends in a
// multi-frontend pipeline; a Go-flavoured `./blog/...` pattern
// (intended for the Go frontend) used to trip the proto compiler
// with a misleading "open ...: no such file" error. Now those
// patterns produce zero entries and no diagnostics.
func TestLoad_GoStyleGlobOutsideProtoTree(t *testing.T) {
	t.Parallel()

	t.Run("Go-style /... pattern naming a directory with no proto files is a no-op", func(t *testing.T) {
		t.Parallel()
		// "simple" fixture root holds proto files at the top
		// level only — the messages.proto et al. Driving Load
		// with "./nonexistent_subdir/..." resolves a directory
		// inside the fixture root that doesn't exist; the
		// frontend treats that as "no proto input for this
		// pattern" rather than an error.
		env := loadFixture(t, "simple", "./nonexistent_subdir/...")
		for _, d := range env.diag.Diagnostics() {
			if d.Plugin == frontend.FrontendName && d.Severity == diag.Error {
				t.Errorf(
					"Go-style pattern naming a non-proto subtree must not error; "+
						"got: %s",
					d.Message,
				)
			}
		}
	})

	t.Run("Go-style /... pattern naming a non-existent path is a no-op", func(t *testing.T) {
		t.Parallel()
		env := loadFixture(t, "simple", "./doesnotexist/...")
		for _, d := range env.diag.Diagnostics() {
			if d.Plugin == frontend.FrontendName && d.Severity == diag.Error {
				t.Errorf(
					"Go-style pattern resolving to a non-existent path "+
						"must not error; got: %s",
					d.Message,
				)
			}
		}
	})
}

// TestLoad_EmptyPatternIsNoop pins the documented empty-Pattern
// guard: callers that drive the frontend without a Pattern get a
// no-op rather than a panic, which keeps the plugin-shape tests
// independent of any actual proto source root.
func TestLoad_EmptyPatternIsNoop(t *testing.T) {
	t.Parallel()

	t.Run("empty Pattern returns nil and emits no diagnostics", func(t *testing.T) {
		t.Parallel()
		d := diag.New()
		f := frontend.New()
		ctx := &plugin.FrontendContext{
			Store:    store.New(),
			Diag:     d,
			Registry: directive.NewRegistry(),
			Parser:   directive.DefaultParser(),
			Cache:    cache.NewNone(),
			Pattern:  "",
		}
		if err := f.Load(ctx); err != nil {
			t.Fatalf("Load with empty Pattern returned error: %v", err)
		}
		if got := len(d.Diagnostics()); got != 0 {
			t.Fatalf("empty-Pattern Load should emit no diagnostics; got %d", got)
		}
	})
}

// fixtureEnv bundles the per-Load context the load-side tests
// need to assert against. Fields are unexported because the
// struct is consumed entirely within the package's test surface.
type fixtureEnv struct {
	frontend *frontend.Frontend
	diag     *diag.Sink
	store    *store.Store
}

// loadFixture configures a frontend.Frontend pointing at the
// named testdata directory and drives Load against pattern. The
// returned fixtureEnv exposes the diagnostic sink + store so
// callers assert on the captured state.
//
// The fixture-root resolution uses runtime.Caller so the test
// works regardless of the cwd at test time — the same pattern the
// Go frontend's test fixtures use.
func loadFixture(t *testing.T, name, pattern string) fixtureEnv {
	t.Helper()
	root := fixtureRoot(t, name)
	d := diag.New()
	f := frontend.New()
	if err := f.SetOptions(opt.New(f.OptionsSchema(), map[string]string{"dir": root})); err != nil {
		t.Fatalf("SetOptions: %v", err)
	}
	ctx := &plugin.FrontendContext{
		Store:    store.New(),
		Diag:     d,
		Registry: directive.NewRegistry(),
		Parser:   directive.DefaultParser(),
		Cache:    cache.NewNone(),
		Pattern:  pattern,
	}
	if err := f.Load(ctx); err != nil {
		t.Fatalf("Load: %v", err)
	}
	return fixtureEnv{frontend: f, diag: d, store: ctx.Store}
}

// fixtureRoot returns the absolute path of frontend/protobuf/testdata/<name>.
// Resolves through runtime.Caller so the path is independent of
// the test's working directory.
func fixtureRoot(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "testdata", name)
}
