// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest

import (
	"bytes"
	"fmt"
	"slices"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
	"go.thesmos.sh/eidos/store"
)

// BackendFixture describes a single emit-graph input the
// [RunBackendSuite] drives a [plugin.Backend] against. The
// fixture supplies pre-built [emit.Package] values whose decls
// already carry their [emit.Target] — the backend suite
// bypasses the routing phase the pipeline normally runs.
//
// BuildEmitPackages is invoked once per subtest. The function
// must return freshly-allocated packages each call so the
// determinism check can build two independent inputs without
// state-bleed between them.
type BackendFixture struct {
	// Name labels the fixture in subtest paths and failure
	// messages. Required and unique within a single
	// [RunBackendSuite] call.
	Name string

	// BuildEmitPackages returns the emit packages the backend
	// renders. The packages and every contained decl must be
	// fresh allocations on each call — the suite uses two
	// independent invocations for byte-stability checks.
	BuildEmitPackages func(t *testing.T) []*emit.Package

	// Command pins the literal text the backend stamps into the
	// "Command:" header line. Empty defers to whatever fallback
	// the backend ships; tests asserting against committed
	// fixtures set this to a stable string so the header is
	// reproducible across processes.
	Command string

	// SourcesOverride pins the literal entries the backend
	// stamps into rendered files' "Source:" header line. Nil
	// leaves the backend to derive sources from each entity's
	// [emit.Node.Origin] back-pointer.
	SourcesOverride []string
}

// RunBackendSuite runs the conformance checks every
// [plugin.Backend] must satisfy: Render must not panic on an
// empty emit graph; for each fixture, Render must not panic or
// surface Error-severity diagnostics on the pre-built input;
// and byte-stability holds — two independent runs with the
// same fixture (same Command, same SourcesOverride) write
// identical bytes to their sinks.
//
// The suite skips the pipeline's routing layer: BackendFixture
// supplies pre-built [emit.Target] values on every decl. Tests
// covering routing decisions belong in pipeline-level suites;
// the backend suite isolates backend-internal contracts
// (template selection, import resolution, formatting, slot
// composition, header determinism).
func RunBackendSuite(t *testing.T, b plugin.Backend, fixtures []BackendFixture) {
	t.Helper()
	t.Run("Render on empty emit graph does not panic", func(t *testing.T) {
		assertRenderEmptyEmitDoesNotPanic(t, b)
	})
	assertBackendFixtureNamesUnique(t, fixtures)
	for _, fx := range fixtures {
		t.Run("fixture="+fx.Name+"/Render does not panic", func(t *testing.T) {
			pkgs := buildEmitPackages(t, fx)
			assertRenderDoesNotPanic(t, b, fx, pkgs)
		})
		t.Run("fixture="+fx.Name+"/Render produces no Error-severity diagnostics", func(t *testing.T) {
			pkgs := buildEmitPackages(t, fx)
			assertRenderCarriesNoErrors(t, b, fx, pkgs)
		})
		t.Run("fixture="+fx.Name+"/Render output is byte-stable across two runs", func(t *testing.T) {
			a := buildEmitPackages(t, fx)
			b2 := buildEmitPackages(t, fx)
			assertRenderIsByteStable(t, b, fx, a, b2)
		})
	}
}

// assertBackendFixtureNamesUnique fails when two fixtures
// share a Name.
func assertBackendFixtureNamesUnique(tb testing.TB, fixtures []BackendFixture) {
	tb.Helper()
	seen := make(map[string]struct{}, len(fixtures))
	for _, fx := range fixtures {
		if fx.Name == "" {
			tb.Fatalf("RunBackendSuite: fixture has empty Name; every BackendFixture must declare one")
		}
		if _, dup := seen[fx.Name]; dup {
			tb.Fatalf("RunBackendSuite: duplicate fixture Name %q", fx.Name)
		}
		seen[fx.Name] = struct{}{}
	}
}

// buildEmitPackages invokes fx.BuildEmitPackages and surfaces
// nil / builder failures as test fatals.
func buildEmitPackages(t *testing.T, fx BackendFixture) []*emit.Package {
	t.Helper()
	if fx.BuildEmitPackages == nil {
		t.Fatalf("RunBackendSuite: fixture %q has nil BuildEmitPackages", fx.Name)
	}
	pkgs := fx.BuildEmitPackages(t)
	if len(pkgs) == 0 {
		t.Fatalf("RunBackendSuite: fixture %q BuildEmitPackages returned an empty slice", fx.Name)
	}
	return pkgs
}

// assertRenderEmptyEmitDoesNotPanic drives the backend against
// an empty store with no emit decls. The backend should write
// zero files and complete cleanly — the pipeline runs the
// backend unconditionally and an empty-emit panic crashes the
// process on projects whose generators produced no output.
func assertRenderEmptyEmitDoesNotPanic(tb testing.TB, b plugin.Backend) {
	tb.Helper()
	s := store.New()
	mem := sink.NewMemory()
	if err := renderRecovering(b, BackendFixture{}, s, mem); err != nil {
		tb.Errorf("Render %s on an empty emit graph: %v", probeVerb(err), err)
	}
}

// assertRenderDoesNotPanic drives the backend against the
// fixture's emit graph and fails if Render panics or returns
// an error.
func assertRenderDoesNotPanic(tb testing.TB, b plugin.Backend, fx BackendFixture, pkgs []*emit.Package) {
	tb.Helper()
	s, err := buildStoreFromEmit(pkgs)
	if err != nil {
		tb.Fatalf("seed store from fixture %q emit packages: %v", fx.Name, err)
	}
	if err := renderRecovering(b, fx, s, sink.NewMemory()); err != nil {
		tb.Errorf("Render %s on fixture %q: %v", probeVerb(err), fx.Name, err)
	}
}

// assertRenderCarriesNoErrors drives the backend and fails when
// the diagnostic sink records any [diag.SeverityError] entry.
// Fixtures the plugin author supplies represent inputs the
// backend should handle cleanly; an error diagnostic indicates
// a contract failure (missing template, unresolvable import,
// formatting failure) the author intends to surface.
func assertRenderCarriesNoErrors(tb testing.TB, b plugin.Backend, fx BackendFixture, pkgs []*emit.Package) {
	tb.Helper()
	s, err := buildStoreFromEmit(pkgs)
	if err != nil {
		tb.Fatalf("seed store from fixture %q emit packages: %v", fx.Name, err)
	}
	d := diag.Capture()
	mem := sink.NewMemory()
	if err := renderRecoveringWithDiag(b, fx, s, mem, d); err != nil {
		tb.Fatalf("Render %s on fixture %q: %v", probeVerb(err), fx.Name, err)
	}
	if d.HasErrors() {
		tb.Errorf(
			"backend produced Error-severity diagnostics on fixture %q; "+
				"fixtures should represent inputs the backend handles cleanly\n  diagnostics: %+v",
			fx.Name, d.Diagnostics(),
		)
	}
}

// assertRenderIsByteStable drives the backend twice with two
// independent emit-package allocations built from the same
// fixture and fails when the rendered sinks differ in any
// recorded file's bytes. Stability against equivalent inputs
// is the foundation of byte-identical CI rebuilds and the
// pipeline's manifest provenance hashing.
func assertRenderIsByteStable(
	tb testing.TB,
	b plugin.Backend,
	fx BackendFixture,
	pkgsA, pkgsB []*emit.Package,
) {
	tb.Helper()
	memA, err := renderToMemory(b, fx, pkgsA)
	if err != nil {
		tb.Fatalf("first render of fixture %q: %v", fx.Name, err)
	}
	memB, err := renderToMemory(b, fx, pkgsB)
	if err != nil {
		tb.Fatalf("second render of fixture %q: %v", fx.Name, err)
	}
	reportSinkDifference(tb, fx.Name, memA, memB)
}

// renderToMemory wires opts and drives Render against a fresh
// memory sink. Returns the populated sink for caller-side
// inspection or wraps any Render failure.
func renderToMemory(b plugin.Backend, fx BackendFixture, pkgs []*emit.Package) (*sink.Memory, error) {
	s, err := buildStoreFromEmit(pkgs)
	if err != nil {
		return nil, fmt.Errorf("seed store: %w", err)
	}
	mem := sink.NewMemory()
	if err := renderRecoveringWithDiag(b, fx, s, mem, diag.Discard()); err != nil {
		return nil, fmt.Errorf("render: %w", err)
	}
	return mem, nil
}

// renderRecovering invokes Render with the supplied store and
// sink, recovering any panic into a returned error. Used by
// the "does not panic" probes.
func renderRecovering(b plugin.Backend, fx BackendFixture, s *store.Store, mem sink.Sink) error {
	return renderRecoveringWithDiag(b, fx, s, mem, diag.Discard())
}

// renderRecoveringWithDiag is [renderRecovering] with a
// caller-supplied diagnostic sink, so a check that inspects what was
// emitted still gets the panic containment.
//
// Every backend probe goes through here. Two of them used to call
// [renderWithSinkAndDiag] directly, so a backend that panicked failed
// one subtest through the recovering probe and then took the whole
// test binary down on the next one — losing every sibling result the
// author needed to see.
func renderRecoveringWithDiag(
	b plugin.Backend,
	fx BackendFixture,
	s *store.Store,
	mem sink.Sink,
	d *diag.Sink,
) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%w: %v", ErrProbePanicked, r)
		}
	}()
	if rerr := renderWithSinkAndDiag(b, fx, s, mem, d); rerr != nil {
		return fmt.Errorf("%w: %w", ErrProbeReturnedError, rerr)
	}
	return nil
}

// renderWithSinkAndDiag drives Render with explicit sink and
// diagnostic sink wiring. Used by every backend-suite probe so
// the harness stays consistent across checks.
func renderWithSinkAndDiag(
	b plugin.Backend,
	fx BackendFixture,
	s *store.Store,
	mem sink.Sink,
	d *diag.Sink,
) error {
	ctx := &plugin.BackendContext{
		Store:           s,
		Reader:          store.NewReader(s),
		Diag:            d,
		Sink:            mem,
		Lang:            b.Language(),
		Plugins:         []plugin.Plugin{b},
		Ordered:         []plugin.Plugin{b},
		Command:         fx.Command,
		SourcesOverride: fx.SourcesOverride,
	}
	if err := b.Render(ctx); err != nil {
		return fmt.Errorf("backend render: %w", err)
	}
	return nil
}

// buildStoreFromEmit allocates a fresh store, adds every
// supplied emit package, and rebuilds the byTarget index so
// the backend's grouping pass sees the freshly-seeded decls.
// Mirrors the wiring [backendtest.Run] performs for the
// language-agnostic harness.
func buildStoreFromEmit(pkgs []*emit.Package) (*store.Store, error) {
	s := store.New()
	for _, pkg := range pkgs {
		if err := s.Emit().AddPackage(pkg); err != nil {
			return nil, fmt.Errorf("AddPackage %q: %w", pkg.Name, err)
		}
	}
	s.Emit().RebuildByTarget()
	return s, nil
}

// reportSinkDifference compares two memory sinks and fails t
// with the first diverging file's name and side-by-side bytes,
// or the differing target sets when the two sinks recorded
// different files entirely.
func reportSinkDifference(tb testing.TB, fixtureName string, lhs, rhs *sink.Memory) {
	tb.Helper()
	lhsFiles, rhsFiles := lhs.Files(), rhs.Files()
	if len(lhsFiles) != len(rhsFiles) {
		tb.Errorf(
			"backend wrote a different number of files on two equivalent runs of fixture %q: "+
				"first=%d second=%d\n  first targets:  %v\n  second targets: %v",
			fixtureName, len(lhsFiles), len(rhsFiles),
			sortedTargets(lhsFiles), sortedTargets(rhsFiles),
		)
		return
	}
	for tgt, lbytes := range lhsFiles {
		rbytes, ok := rhsFiles[tgt]
		if !ok {
			tb.Errorf(
				"target %s present in first run but missing in second run of fixture %q",
				formatTarget(tgt), fixtureName,
			)
			return
		}
		if !bytes.Equal(lbytes, rbytes) {
			tb.Errorf(
				"rendered bytes differ at target %s across two equivalent runs of fixture %q; "+
					"backend is not byte-stable\n  first run:  %s\n  second run: %s",
				formatTarget(tgt), fixtureName, lbytes, rbytes,
			)
			return
		}
	}
}

// sortedTargets returns the keys of m as a sorted slice of
// formatted target strings. Used in failure messages so a diff
// on the target set is greppable.
func sortedTargets(m map[emit.Target][]byte) []string {
	out := make([]string, 0, len(m))
	for tgt := range m {
		out = append(out, formatTarget(tgt))
	}
	slices.Sort(out)
	return out
}
