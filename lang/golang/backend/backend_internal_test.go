// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"bytes"
	"go/build"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"golang.org/x/tools/imports"

	"go.thesmos.sh/eidos/writer"
)

// importCorpus spans every branch of the grouping rule plus the
// shapes where eidos and gofmt could plausibly disagree: a
// version-suffixed path, a path whose last segment is not its
// package name, a single-component non-stdlib path, and the
// appengine prefix that is neither stdlib nor dotted.
var importCorpus = []writer.Import{
	{Path: "context", Alias: "context"},
	{Path: "fmt", Alias: "fmt"},
	{Path: "net/http", Alias: "http"},
	{Path: "appengine", Alias: "appengine"},
	{Path: "appengine/datastore", Alias: "datastore"},
	{Path: "example.com/a", Alias: "a"},
	{Path: "github.com/x/y", Alias: "y"},
	{Path: "gopkg.in/yaml.v3", Alias: "yamlv3"},
	// These two exist to separate a path sort from an alias sort:
	// their aliases order the opposite way round from their paths.
	// Without them the corpus passes under either rule, because
	// every other entry's alias happens to track its path.
	{Path: "zzz.example.com/early", Alias: "aaa"},
	{Path: "aaa.example.com/late", Alias: "zzz"},
}

// composeCorpusFile writes a file whose only content is the import
// block, so any difference between the two arrangements is the
// arrangement itself.
func composeCorpusFile(t *testing.T, imports []writer.Import) []byte {
	t.Helper()
	var buf bytes.Buffer
	buf.WriteString("package p\n")
	writeImportBlock(&buf, imports)
	return buf.Bytes()
}

// TestImportGroup_MatchesResolver is the guard on the duplicated
// grouping rule.
//
// [importGroup] re-implements a rule that lives in x/tools'
// internal/imports, which is unimportable, so the copy can drift
// from a future release of the module. A golden file cannot see
// that: it pins what eidos emits and keeps passing while the two
// diverge. Comparing against the resolver's own arrangement of the
// same imports can, and does so at the version go.mod pins — so the
// day the rule changes upstream, this fails on the dependency bump
// rather than in someone's generated output.
//
// x/tools is retained as a test-only dependency for exactly this.
// The cost the series removed was the per-file subprocess fork in
// the render path, and nothing here runs during a render.
// FormatOnly keeps even this call away from the resolver: it
// formats and regroups without reaching ProcessEnv, so no `go env`
// is invoked.
func TestImportGroup_MatchesResolver(t *testing.T) {
	t.Parallel()

	resolverFormat := func(t *testing.T, src []byte) string {
		t.Helper()
		out, err := imports.Process("p/x.go", src, &imports.Options{
			Comments:   true,
			TabIndent:  true,
			TabWidth:   8,
			FormatOnly: true,
		})
		if err != nil {
			t.Fatalf("imports.Process: %v", err)
		}
		return string(out)
	}

	t.Run("the emitted block matches the resolver's arrangement", func(t *testing.T) {
		t.Parallel()
		ours := composeCorpusFile(t, importCorpus)
		if got, want := string(ours), resolverFormat(t, ours); got != want {
			t.Fatalf("grouping diverged from x/tools:\nours:\n%s\nresolver:\n%s", got, want)
		}
	})

	t.Run("the arrangement survives a shuffled input order", func(t *testing.T) {
		t.Parallel()
		// The ImportSet hands entries over in Imp-call order, which
		// is template order and arbitrary with respect to path. If
		// the sort were order-sensitive, the corpus above would pass
		// only because it happens to be written sorted.
		shuffled := make([]writer.Import, len(importCorpus))
		for i, imp := range importCorpus {
			shuffled[len(importCorpus)-1-i] = imp
		}
		ours := composeCorpusFile(t, shuffled)
		if got, want := string(ours), resolverFormat(t, ours); got != want {
			t.Fatalf("grouping diverged on reversed input:\nours:\n%s\nresolver:\n%s", got, want)
		}
	})

	t.Run("appengine is its own group, after the external one", func(t *testing.T) {
		t.Parallel()
		// Stated separately because it is the rule a reader will be
		// tempted to delete as a relic. It is not gated on
		// LocalPrefix, so it is reachable, and dropping it would send
		// `appengine` into the standard-library block.
		if got := importGroup("appengine/datastore"); got != 2 {
			t.Fatalf("importGroup(appengine/datastore) = %d, want 2", got)
		}
		if got := importGroup("example.com/a"); got != 1 {
			t.Fatalf("importGroup(example.com/a) = %d, want 1", got)
		}
		if got := importGroup("net/http"); got != 0 {
			t.Fatalf("importGroup(net/http) = %d, want 0", got)
		}
	})
}

// TestWriteImportBlock covers the emitted shape directly, at the
// level where the blank-line placement is decided.
//
// [go/format.Source] re-sorts each blank-line-delimited run, so a
// within-group ordering mistake is corrected downstream and a
// blank-line mistake is not. These assertions run against the raw
// block, before any formatter has had a chance to hide one.
func TestWriteImportBlock(t *testing.T) {
	t.Parallel()

	t.Run("an empty set emits nothing", func(t *testing.T) {
		t.Parallel()
		var buf bytes.Buffer
		writeImportBlock(&buf, nil)
		if buf.Len() != 0 {
			t.Fatalf("expected no output for an empty set; got %q", buf.String())
		}
	})

	t.Run("stdlib and external are separated by a blank line", func(t *testing.T) {
		t.Parallel()
		got := string(composeCorpusFile(t, importCorpus))
		if !strings.Contains(got, "\t\"net/http\"\n\n\tzzz \"aaa.example.com/late\"") {
			t.Fatalf("expected a blank line between groups; got:\n%s", got)
		}
	})

	t.Run("a single-group set gets no blank line", func(t *testing.T) {
		t.Parallel()
		// The separator is emitted on group change, so a set that
		// never changes group must not acquire a leading or trailing
		// blank inside the parentheses.
		got := string(composeCorpusFile(t, []writer.Import{
			{Path: "fmt", Alias: "fmt"},
			{Path: "context", Alias: "context"},
		}))
		if strings.Contains(got, "\n\n\t") {
			t.Fatalf("unexpected blank line within one group:\n%s", got)
		}
	})

	t.Run("entries are sorted by path within a group", func(t *testing.T) {
		t.Parallel()
		got := string(composeCorpusFile(t, []writer.Import{
			{Path: "fmt", Alias: "fmt"},
			{Path: "context", Alias: "context"},
		}))
		if strings.Index(got, `"context"`) > strings.Index(got, `"fmt"`) {
			t.Fatalf("expected path order within the group; got:\n%s", got)
		}
	})

	t.Run("the input slice is not reordered in place", func(t *testing.T) {
		t.Parallel()
		// The caller holds this slice for the shadowed-import and
		// unresolved-qualifier checks; sorting through it would
		// reorder their reports.
		in := []writer.Import{
			{Path: "fmt", Alias: "fmt"},
			{Path: "context", Alias: "context"},
		}
		var buf bytes.Buffer
		writeImportBlock(&buf, in)
		if in[0].Path != "fmt" {
			t.Fatalf("writeImportBlock reordered its input: %+v", in)
		}
	})
}

// TestRenderPathForksNoSubprocess is the standing guard on the win
// this series bought.
//
// The goimports resolve pass forked `go env -json` once per
// generated file — proven with a PATH shim logging argv, which
// recorded exactly one line per target. A shim cannot serve as a
// regression test: cmd/go rewrites PATH for the test process, so
// the shim is invisible from inside `go test`.
//
// The dependency is the next best thing, and is arguably the better
// statement. Nothing in this package's production build can fork
// unless it reaches a package that can, so asserting the production
// imports carry neither the resolver nor an exec facility says "no
// subprocess" without depending on how one would be spawned.
// x/tools stays available to the tests above, which is why the
// check distinguishes the two import sets rather than scanning the
// module.
func TestRenderPathForksNoSubprocess(t *testing.T) {
	t.Parallel()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller(0) failed; cannot locate the package directory")
	}
	pkg, err := build.ImportDir(filepath.Dir(thisFile), 0)
	if err != nil {
		t.Fatalf("build.ImportDir: %v", err)
	}

	forbidden := []string{"golang.org/x/tools/imports", "os/exec"}

	t.Run("no production file imports a forking dependency", func(t *testing.T) {
		t.Parallel()
		for _, imp := range pkg.Imports {
			if slices.Contains(forbidden, imp) {
				t.Fatalf("%q is back in the render path; the per-file subprocess fork returns with it", imp)
			}
		}
	})

	t.Run("the guard is looking at a populated import set", func(t *testing.T) {
		t.Parallel()
		// A typo in the directory, or an ImportDir that silently
		// returned nothing, would make the assertion above vacuous.
		if !slices.Contains(pkg.Imports, "go/format") {
			t.Fatalf("expected go/format among the production imports; got %v", pkg.Imports)
		}
	})

	t.Run("the resolver is still reachable from tests", func(t *testing.T) {
		t.Parallel()
		// TestImportGroup_MatchesResolver depends on it. If it ever
		// leaves the test imports too, that differential has silently
		// stopped comparing against anything.
		if !slices.Contains(pkg.TestImports, "golang.org/x/tools/imports") {
			t.Fatalf("expected x/tools among the test imports; got %v", pkg.TestImports)
		}
	})
}
