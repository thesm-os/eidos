// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline

import (
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/store"
)

// naiveSplitOutDirectivePath is a deliberately independent
// reimplementation of [splitOutDirectivePath]'s split step, used as
// the differential oracle in [FuzzSplitOutDirectivePath].
//
// It splits by hand on the last separator instead of delegating to
// [filepath.Split], and trims by explicit index arithmetic instead of
// [strings.TrimLeft] / [strings.TrimRight]. The two implementations
// therefore share only the [filepath.ToSlash] normalisation step, so
// a rewrite of the production splitter — swapping in [path.Split],
// hoisting the trim, reordering the trim against the split — has to
// keep agreeing with a splitter that made none of those choices.
//
// The oracle models the split alone and encodes no policy on `.` or
// `..` segments, so the fuzz target only consults it on values that
// carry neither (see [hasDotSegment]), and compares the directory
// modulo [path.Clean] so that collapsing interior duplicate
// separators — another normalisation the contract does not speak to
// — is not read as disagreement. An oracle that took a position on
// either would bake today's behaviour in as expected, and a target
// that pins an implementation cannot challenge it.
func naiveSplitOutDirectivePath(value string) (dir, filename string) {
	v := filepath.ToSlash(value)
	for len(v) > 0 && v[0] == '/' {
		v = v[1:]
	}
	i := strings.LastIndexByte(v, '/')
	if i < 0 {
		return "", v
	}
	d := v[:i]
	for len(d) > 0 && d[len(d)-1] == '/' {
		d = d[:len(d)-1]
	}
	return d, v[i+1:]
}

// hasDotSegment reports whether the slash-normalised value contains a
// "." or ".." path segment — the segments whose handling is exactly
// what the containment property is arguing about, and therefore the
// domain on which [naiveSplitOutDirectivePath] must not be consulted.
func hasDotSegment(value string) bool {
	for seg := range strings.SplitSeq(filepath.ToSlash(value), "/") {
		if seg == "." || seg == ".." {
			return true
		}
	}
	return false
}

// escapesOriginDir reports whether stacking dir onto an origin's
// source directory walks above that directory.
//
// This is the shape the Layout phase actually applies:
// composeTarget computes `filepath.Join(t.Dir, dir)`, and Join cleans
// its result, so a leading `..` segment in dir cancels a component of
// the origin's directory instead of being rejected. Cleaning dir on
// its own reproduces that cancellation exactly — a dir that cleans to
// ".." or to something under "../" is a dir that can leave the source
// tree.
func escapesOriginDir(dir string) bool {
	cleaned := path.Clean("./" + dir)
	return cleaned == ".." || strings.HasPrefix(cleaned, "../")
}

// FuzzSplitOutDirectivePath drives the `+gen:out` path splitter over
// arbitrary directive values.
//
// The splitter is the only sanitiser between a string written in a
// user's source comment (or on the `eidos run -o` command line) and
// [emit.Target.Dir], which the disk sink joins under its output root
// without any containment check of its own. Its documented contract
// is that "the directive cannot escape the source tree", so the
// properties asserted here are about where the result can land, not
// merely that nothing panicked — a splitter that quietly hands back
// a traversing directory returns cleanly and is exactly the failure
// worth catching.
//
// Four properties, strongest first:
//
//  1. Containment: the returned dir must not walk above the origin's
//     directory. This is the security-shaped invariant.
//  2. Differential: on values carrying no `.` / `..` segment, the
//     split must agree with an independently written splitter
//     ([naiveSplitOutDirectivePath]).
//  3. Losslessness: dir and filename must rejoin to the input, so no
//     segment is silently dropped.
//  4. Shape: dir is never absolute and never leads with a separator;
//     filename never carries one.
//
// Every property is stated so that it holds for any splitter obeying
// the documented contract, not just the current one. That distinction
// is load-bearing here: an earlier splitter stripped only a leading
// separator and let `..` through, and a target written against its
// observed behaviour would have certified the traversal as correct
// instead of reporting it. So losslessness compares root-anchored
// cleaned paths — `..` then cancels symmetrically on both sides,
// leaving the check blind to clamping policy but not to a dropped
// segment — and the differential oracle, which necessarily encodes
// some policy, is consulted only where no policy applies.
//
// The seeds cover every branch the splitter takes — bare filename,
// one directory segment, deep nesting, absolute, separator-only,
// empty — plus the boundaries around each: duplicate separators, a
// trailing separator with no filename, `.` and `..` as whole values,
// interior traversal, Windows separators, and invalid UTF-8.
func FuzzSplitOutDirectivePath(f *testing.F) {
	for _, seed := range []string{
		"",
		"handler.go",
		"test/handler.go",
		"a/b/c/d/handler.go",
		"/etc/passwd",
		"/",
		"//",
		".",
		"..",
		"../",
		"../../x",
		"../../x/y.go",
		"a/../../b.go",
		`..\..\x.go`,
		"a//b.go",
		"dir/",
		"./x.go",
		" ",
		"\xff\xfe.go",
		strings.Repeat("a/", 64) + "x.go",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, value string) {
		dir, filename := splitOutDirectivePath(value)

		// Property 4 — shape. An absolute dir bypasses the sink's
		// root entirely (sink.Disk.Write branches on
		// filepath.IsAbs(target.Dir)), so it is the loudest possible
		// containment failure.
		if filepath.IsAbs(dir) {
			t.Fatalf("splitOutDirectivePath(%q) returned an absolute dir %q", value, dir)
		}
		if strings.HasPrefix(dir, "/") || strings.HasPrefix(dir, string(filepath.Separator)) {
			t.Fatalf("splitOutDirectivePath(%q) returned dir %q leading with a separator", value, dir)
		}
		if strings.Contains(filename, "/") {
			t.Fatalf("splitOutDirectivePath(%q) returned filename %q carrying a separator", value, filename)
		}
		// A trailing separator would make "internal/users" and
		// "internal/users/" two distinct emit.Target values naming one
		// file — the manifest stores Dir verbatim and keys prune and
		// drift detection off it, so the duplicate is not cosmetic.
		if strings.HasSuffix(dir, "/") {
			t.Fatalf("splitOutDirectivePath(%q) returned dir %q with a trailing separator", value, dir)
		}

		// Property 3 — losslessness. Both sides are cleaned against a
		// synthetic root: that absorbs the duplicate-separator and
		// trailing-separator normalisation the splitter is entitled to
		// perform, and cancels `..` identically on each side, so the
		// check stays neutral on whether the splitter clamps. What it
		// still catches is a vanished segment.
		stripped := strings.TrimLeft(filepath.ToSlash(value), "/")
		if got, want := path.Clean("/"+dir+"/"+filename), path.Clean("/"+stripped); got != want {
			t.Fatalf("splitOutDirectivePath(%q) split to (%q, %q), which rejoins to %q, want %q",
				value, dir, filename, got, want)
		}

		// Property 2 — differential, restricted to the domain where
		// traversal policy cannot differ.
		if !hasDotSegment(value) {
			wantDir, wantFile := naiveSplitOutDirectivePath(value)
			if path.Clean("/"+dir) != path.Clean("/"+wantDir) || filename != wantFile {
				t.Fatalf("splitOutDirectivePath(%q) = (%q, %q), reference splitter = (%q, %q)",
					value, dir, filename, wantDir, wantFile)
			}
		}

		// Property 1 — containment. Asserted last so the cheaper
		// shape and agreement failures surface first when several
		// break at once.
		if escapesOriginDir(dir) {
			t.Fatalf("splitOutDirectivePath(%q) returned dir %q, which escapes the origin's "+
				"source directory: composeTarget applies filepath.Join(originDir, %q) and Join "+
				"cleans, so each leading `..` cancels a component of the origin path. Nothing "+
				"downstream re-checks it — sink.Disk joins Target.Dir under its root without a "+
				"containment test — so a source comment reaching this branch writes a generated "+
				"file outside the source tree. The splitter is the only place the contract "+
				"\"the directive cannot escape the source tree\" is enforced",
				value, dir, dir)
		}
	})
}

// benchLayoutSizes are the routable-decl populations the Layout
// benchmarks sweep. 1 exposes the phase's fixed overhead (the
// per-run plugin-Outputs map, the locations map, the byTarget
// rebuild); 1000 is above the size at which a hidden per-decl scan
// stops hiding — a real run over a mid-size module routes low
// thousands of declarations.
var benchLayoutSizes = []int{1, 10, 100, 1000}

// benchLayoutConflictSizes mirrors [benchLayoutSizes] for the
// conflicting sweep, which arranges decls in pairs and so has no
// meaningful n=1 case: a one-file-one-package violation needs two
// decls disagreeing about the same file.
var benchLayoutConflictSizes = []int{2, 10, 100, 1000}

// BenchmarkRunLayout measures one full Layout phase over n routable
// decls — the per-decl precedence pipeline (policy lookup, source
// package resolution, suffix composition, directive selection, CLI
// override, the `_test.go` package shift), the resolved-layout
// recording, the output-path dispatch, the one-file-one-package
// conflict pass, and the byTarget rebuild.
//
// Layout is the most complex phase and the only one whose cost is
// not obviously linear: enforceOneFileOnePackage groups every decl
// by (Dir, Filename), and each group it finds in violation triggers
// a clearConflictedTargets call that rescans every routable bucket
// in the store. With k violations over n decls that pass is O(k·n),
// so the two sweeps below answer different questions and both are
// needed:
//
//   - "distinct" routes every decl to its own file (k = 0). Per-op
//     time must grow by roughly the same factor as n.
//   - "conflicting" routes every pair of decls to one file under two
//     different package names (k = n/2). If per-op time grows faster
//     than n here while staying linear above, the conflict pass is
//     the quadratic term.
//
// Store and pipeline construction are hoisted above the timed
// region: the measurement is the phase's own cost, not the fixture's.
// The phase is re-entrant over one store — routeDecls recomputes
// each Target from its origin rather than reading the previous
// value, and the resolved-layout map is keyed by Target so it
// reaches a fixed size on the first iteration — so the loop measures
// steady-state work rather than a first-pass fill.
//
// The conflicting sweep runs against a discarding diagnostic sink.
// Every iteration reports k violations, and retaining them would
// grow the sink without bound and measure that growth instead of
// the phase. The formatting cost of each diagnostic is still paid
// inside the timed region, which is correct — it is work the phase
// really does. A pre-check outside the loop confirms against a
// recording sink that the fixture produces exactly the expected
// violation count, so the discard cannot hide a fixture that
// silently stopped conflicting.
func BenchmarkRunLayout(b *testing.B) {
	b.ReportAllocs()

	b.Run("distinct", func(b *testing.B) {
		b.ReportAllocs()
		for _, n := range benchLayoutSizes {
			b.Run(strconv.Itoa(n), func(b *testing.B) {
				b.ReportAllocs()
				d := diag.Capture()
				p := newLayoutBenchPipeline(b, d)
				s, first := newDistinctLayoutStore(b, n)

				for b.Loop() {
					p.runLayout(s)
				}

				// The loop is load-bearing only if it routed: an
				// empty Filename here means every decl fell out of
				// the precedence pipeline and the benchmark timed
				// the error path.
				if first.Target.Filename == "" {
					b.Fatalf("first decl left unrouted; the phase measured nothing")
				}
				if d.HasErrors() {
					b.Fatalf("distinct fixture produced routing errors: %v", d.Diagnostics())
				}
			})
		}
	})

	b.Run("shared", func(b *testing.B) {
		b.ReportAllocs()
		for _, n := range benchLayoutSizes {
			b.Run(strconv.Itoa(n), func(b *testing.B) {
				b.ReportAllocs()
				d := diag.Capture()
				p := newLayoutBenchPipeline(b, d)
				s, first := newSharedLayoutStore(b, n)

				for b.Loop() {
					p.runLayout(s)
				}

				if first.Target.Filename == "" {
					b.Fatalf("first decl left unrouted; the phase measured nothing")
				}
				if d.HasErrors() {
					b.Fatalf("shared fixture produced routing errors: %v", d.Diagnostics())
				}
			})
		}
	})

	b.Run("conflicting", func(b *testing.B) {
		b.ReportAllocs()
		for _, n := range benchLayoutConflictSizes {
			b.Run(strconv.Itoa(n), func(b *testing.B) {
				b.ReportAllocs()
				wantConflicts := n / 2

				// Pre-check: prove on a throwaway store that the
				// fixture really trips the conflict pass, before the
				// timed region swaps in a sink that cannot say so.
				check := diag.Capture()
				checkPipe := newLayoutBenchPipeline(b, check)
				checkStore, _ := newConflictingLayoutStore(b, n)
				checkPipe.runLayout(checkStore)
				if got := check.Count(diag.Error); got != wantConflicts {
					b.Fatalf("fixture reported %d one-file-one-package violations, want %d",
						got, wantConflicts)
				}

				p := newLayoutBenchPipeline(b, diag.Discard())
				s, _ := newConflictingLayoutStore(b, n)

				for b.Loop() {
					p.runLayout(s)
				}
			})
		}
	})
}

// layoutBenchGenName is the plugin attribution stamped on every
// benchmark decl. It has to match the registered generator's Name so
// the Layout phase's SetBy → declared-Outputs lookup resolves a
// filename suffix; an unattributed decl would take the
// ErrMissingFilenameProvider path and the benchmark would measure
// error reporting.
const layoutBenchGenName = "bench-generator"

// newLayoutBenchPipeline builds the minimum pipeline the Layout
// phase needs: one frontend and one backend to satisfy Build's role
// cardinality checks, and one generator declaring a filename suffix
// so decls attributed to it are routable. No sink is configured
// because the phase never writes.
func newLayoutBenchPipeline(tb testing.TB, d *diag.Sink) *Pipeline {
	tb.Helper()
	p, err := New().
		WithFrontend(&layoutBenchFE{}).
		WithGenerator(&layoutBenchGen{}).
		WithBackend(&layoutBenchBE{}).
		WithDiag(d).
		Build()
	if err != nil {
		tb.Fatalf("Build: %v", err)
	}
	return p
}

// newDistinctLayoutStore builds a store holding n emit structs, each
// anchored to a source file in its own directory so every decl
// resolves to a unique (Dir, Filename) and the conflict pass finds
// nothing to clear. The returned struct is the first decl, kept so
// the caller can prove routing actually happened.
//
// The source package is registered on the node side because
// composeTarget resolves it per decl through a ByQName lookup;
// omitting it would exercise the nil-package branch instead of the
// path a real run takes.
func newDistinctLayoutStore(tb testing.TB, n int) (*store.Store, *emit.Struct) {
	tb.Helper()
	s := store.New()
	srcPkg := &node.Package{Name: "bench", Path: "example.com/bench"}
	emitPkg := &emit.Package{Name: "bench", Path: "example.com/bench"}
	for i := range n {
		id := strconv.Itoa(i)
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/bench/pkg" + id + "/entity" + id + ".go"},
			},
			Name:    "Entity" + id,
			Package: "example.com/bench",
		}
		srcPkg.Structs = append(srcPkg.Structs, origin)
		emitPkg.Structs = append(emitPkg.Structs, &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, SetByName: layoutBenchGenName},
			Name:     "Entity" + id + "Gen",
			Package:  "example.com/bench",
		})
	}
	if err := s.Nodes().AddPackage(srcPkg); err != nil {
		tb.Fatalf("Nodes().AddPackage: %v", err)
	}
	if err := s.Emit().AddPackage(emitPkg); err != nil {
		tb.Fatalf("Emit().AddPackage: %v", err)
	}
	return s, emitPkg.Structs[0]
}

// newConflictingLayoutStore builds a store holding n emit structs
// arranged as n/2 one-file-one-package violations: each source file
// is claimed by two decls that agree on Dir and Filename (same
// origin, same plugin suffix) but disagree on Package, because they
// were emitted into two differently-named emit packages.
//
// That is the realistic shape of the violation — two generators
// landing output for one type in one file under different package
// clauses — and it is the only shape that drives
// clearConflictedTargets, whose per-violation full-store rescan is
// the phase's candidate quadratic term.
// sharedLayoutFanout is how many decls newSharedLayoutStore routes
// into each Target. Eight is arbitrary but representative: a
// generator emitting a type plus a handful of methods into one file
// is the shape that makes d > 1, and the exact number matters less
// than that it is not one.
const sharedLayoutFanout = 8

// newSharedLayoutStore builds n routable decls sharing n/8 Targets:
// one origin per group, eight emit structs per origin, so every decl
// in a group composes the same (Dir, Filename, Package).
//
// This is the population neither existing fixture produces. Both
// newDistinctLayoutStore and newConflictingLayoutStore route exactly
// one decl per Target, so the branch recordResolvedLayout exists for
// — d decls composing into one file, where d-1 calls hash, compare
// and discard — has never been measured. A remedy aimed at that
// branch scored against those fixtures is scored against nothing.
func newSharedLayoutStore(tb testing.TB, n int) (*store.Store, *emit.Struct) {
	tb.Helper()
	s := store.New()
	srcPkg := &node.Package{Name: "bench", Path: "example.com/bench"}
	emitPkg := &emit.Package{Name: "bench", Path: "example.com/bench"}
	groups := max(n/sharedLayoutFanout, 1)
	for g := range groups {
		gid := strconv.Itoa(g)
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/bench/pkg" + gid + "/entity" + gid + ".go"},
			},
			Name:    "Entity" + gid,
			Package: "example.com/bench",
		}
		srcPkg.Structs = append(srcPkg.Structs, origin)
		for d := range sharedLayoutFanout {
			emitPkg.Structs = append(emitPkg.Structs, &emit.Struct{
				BaseEmit: emit.BaseEmit{OriginNode: origin, SetByName: layoutBenchGenName},
				Name:     "Entity" + gid + "Gen" + strconv.Itoa(d),
				Package:  "example.com/bench",
			})
		}
	}
	if err := s.Nodes().AddPackage(srcPkg); err != nil {
		tb.Fatalf("Nodes().AddPackage: %v", err)
	}
	if err := s.Emit().AddPackage(emitPkg); err != nil {
		tb.Fatalf("Emit().AddPackage: %v", err)
	}
	return s, emitPkg.Structs[0]
}

func newConflictingLayoutStore(tb testing.TB, n int) (*store.Store, *emit.Struct) {
	tb.Helper()
	s := store.New()
	srcPkg := &node.Package{Name: "bench", Path: "example.com/bench"}
	alpha := &emit.Package{Name: "alpha", Path: "example.com/bench/alpha"}
	beta := &emit.Package{Name: "beta", Path: "example.com/bench/beta"}
	for i := range n / 2 {
		id := strconv.Itoa(i)
		origin := &node.Struct{
			BaseNode: node.BaseNode{
				SourcePos: position.Pos{File: "internal/bench/entity" + id + ".go"},
			},
			Name:    "Entity" + id,
			Package: "example.com/bench",
		}
		srcPkg.Structs = append(srcPkg.Structs, origin)
		alpha.Structs = append(alpha.Structs, &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, SetByName: layoutBenchGenName},
			Name:     "Alpha" + id,
			Package:  alpha.Path,
		})
		beta.Structs = append(beta.Structs, &emit.Struct{
			BaseEmit: emit.BaseEmit{OriginNode: origin, SetByName: layoutBenchGenName},
			Name:     "Beta" + id,
			Package:  beta.Path,
		})
	}
	if err := s.Nodes().AddPackage(srcPkg); err != nil {
		tb.Fatalf("Nodes().AddPackage: %v", err)
	}
	if err := s.Emit().AddPackage(alpha); err != nil {
		tb.Fatalf("Emit().AddPackage(alpha): %v", err)
	}
	if err := s.Emit().AddPackage(beta); err != nil {
		tb.Fatalf("Emit().AddPackage(beta): %v", err)
	}
	return s, alpha.Structs[0]
}

// layoutBenchFE satisfies Build's "at least one frontend"
// requirement. The Layout benchmarks drive runLayout directly
// against a pre-populated store, so Load is never called.
type layoutBenchFE struct{}

func (*layoutBenchFE) Name() string                         { return "bench-frontend" }
func (*layoutBenchFE) Load(_ *plugin.FrontendContext) error { return nil }

// layoutBenchGen is the attribution target for every benchmark decl.
// It declares a single default output so the Layout phase resolves a
// suffix; Generate is never called because the benchmarks populate
// the store themselves.
type layoutBenchGen struct{}

func (*layoutBenchGen) Name() string { return layoutBenchGenName }
func (*layoutBenchGen) Outputs(_ string) []plugin.Output {
	return []plugin.Output{{Suffix: "_gen.go"}}
}
func (*layoutBenchGen) Generate(_ *plugin.GeneratorContext) error { return nil }

// layoutBenchBE satisfies Build's "exactly one backend" requirement
// and supplies the language key the Layout phase passes to
// Outputs. Render is never called.
type layoutBenchBE struct{}

func (*layoutBenchBE) Name() string                          { return "bench-backend" }
func (*layoutBenchBE) Language() string                      { return "bench" }
func (*layoutBenchBE) Render(_ *plugin.BackendContext) error { return nil }

// TestRunLayout_AllocationBudget is the enforced half of the
// per-declaration allocation work.
//
// BenchmarkRunLayout records the figure but nothing fails a build on
// a benchmark, and bench/baseline.txt is per-developer and
// gitignored. A ceiling asserted here is what makes the
// reintroduction of any one construct fail rather than be absorbed by
// the others.
//
// The budget is per decl, so it holds as n changes, and it is an
// allocation count rather than a duration — deterministic for a given
// input, where the wall-clock on this fixture swings by double digits
// between runs.
//
// Not parallel: testing.AllocsPerRun panics in a parallel test.
//
//nolint:paralleltest // testing.AllocsPerRun panics in a parallel test.
func TestRunLayout_AllocationBudget(t *testing.T) {
	const (
		decls = 100
		// 11.08 per decl before this work, of which 1.03 is the
		// store's byTarget rebuild and out of scope here. The
		// ceiling sits just above what the phase measures so a
		// single construct coming back is visible.
		perDecl = 4.5
	)

	for _, tc := range []struct {
		name  string
		build func(tb testing.TB, n int) *store.Store
	}{
		{"distinct", func(tb testing.TB, n int) *store.Store {
			tb.Helper()
			s, _ := newDistinctLayoutStore(tb, n)
			return s
		}},
		{"shared", func(tb testing.TB, n int) *store.Store {
			tb.Helper()
			s, _ := newSharedLayoutStore(tb, n)
			return s
		}},
	} {
		s := tc.build(t, decls)
		p := newLayoutBenchPipeline(t, diag.Discard())
		got := testing.AllocsPerRun(5, func() { p.runLayout(s) })
		if budget := perDecl * decls; got > budget {
			t.Fatalf("%s: runLayout allocated %v for %d decls, budget %v",
				tc.name, got, decls, budget)
		}
		t.Logf("%s: %v allocations for %d decls (%.2f/decl)", tc.name, got, decls, got/decls)
	}
}
