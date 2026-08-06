// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package writer_test

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"go.thesmos.sh/eidos/writer"
)

// fuzzMaxImportPaths caps how many paths a single fuzz input
// registers. The suffix-escalation loop in [writer.ImportSet.Imp] is
// quadratic in the number of colliding paths, so an unbounded blob of
// newlines would present to the fuzzer as a hang — a timeout report
// that buries whatever property violation the same corpus entry might
// actually have found. Sixty-four is well past the largest import
// block the backend emits in practice and still finishes instantly.
const fuzzMaxImportPaths = 64

// impSink defends the benchmark loop bodies from dead-code
// elimination. Without a store to a package-level variable the
// compiler is free to drop a call whose result is discarded, and the
// benchmark would then report the cost of an empty loop as if it were
// the cost of registering an import.
var impSink string

func TestDefaultAlias(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		path string
		want string
	}{
		{"single-segment path", "context", "context"},
		{"multi-segment path", "github.com/foo/bar", "bar"},
		{"trailing separator does not swallow the last segment", "github.com/foo/", "foo"},
		{"repeated trailing separators still yield the last segment", "github.com/foo///", "foo"},
		{"a path of only separators has no alias", "///", ""},
		{"empty path returns empty", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := writer.DefaultAlias(tc.path); got != tc.want {
				t.Fatalf("DefaultAlias(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

func TestNewImportSet(t *testing.T) {
	t.Parallel()

	t.Run("returns an empty ImportSet ready for use", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(nil)
		if is.Len() != 0 {
			t.Fatalf("new ImportSet should be empty")
		}
	})

	t.Run("nil derive function defaults to DefaultAlias", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(nil)
		alias, err := is.Imp("github.com/foo/bar")
		assertNoError(t, err)
		if alias != "bar" {
			t.Fatalf("default derivation should pick last segment; got %q", alias)
		}
	})
}

func TestImportSet_Imp(t *testing.T) {
	t.Parallel()

	t.Run("first call returns the derived alias", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(nil)
		alias, err := is.Imp("context")
		assertNoError(t, err)
		if alias != "context" {
			t.Fatalf("alias = %q, want context", alias)
		}
	})

	t.Run("repeat calls for the same path return the same alias", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(nil)
		first, err := is.Imp("github.com/foo/bar")
		assertNoError(t, err)
		second, err := is.Imp("github.com/foo/bar")
		assertNoError(t, err)
		if first != second {
			t.Fatalf("repeat Imp returned different aliases: %q vs %q", first, second)
		}
	})

	t.Run("colliding aliases get a numeric suffix deterministically", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(nil)
		a1, err := is.Imp("context")
		assertNoError(t, err)
		a2, err := is.Imp("github.com/foo/context")
		assertNoError(t, err)
		a3, err := is.Imp("github.com/bar/context")
		assertNoError(t, err)
		if a1 != "context" || a2 != "context2" || a3 != "context3" {
			t.Fatalf("collision aliases mismatch: %q %q %q", a1, a2, a3)
		}
	})

	t.Run("empty path returns ErrEmptyPath", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(nil)
		if _, err := is.Imp(""); !errors.Is(err, writer.ErrEmptyPath) {
			t.Fatalf("Imp(\"\") should return ErrEmptyPath; got %v", err)
		}
	})

	t.Run("custom derive function controls aliasing", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(func(p string) string { return "x_" + writer.DefaultAlias(p) })
		alias, err := is.Imp("context")
		assertNoError(t, err)
		if alias != "x_context" {
			t.Fatalf("custom derive should drive alias; got %q", alias)
		}
	})
}

func TestImportSet_Alias(t *testing.T) {
	t.Parallel()

	t.Run("explicit alias overrides the derived default", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(nil)
		assertNoError(t, is.Alias("context", "ctx"))
		alias, err := is.Imp("context")
		assertNoError(t, err)
		if alias != "ctx" {
			t.Fatalf("explicit alias should win; got %q", alias)
		}
	})

	t.Run("Alias after Imp returns ErrAliasAfterImp", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(nil)
		_, err := is.Imp("context")
		assertNoError(t, err)
		err = is.Alias("context", "ctx")
		if !errors.Is(err, writer.ErrAliasAfterImp) {
			t.Fatalf("Alias after Imp should return ErrAliasAfterImp; got %v", err)
		}
	})

	t.Run("empty path returns ErrEmptyPath", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(nil)
		if err := is.Alias("", "ctx"); !errors.Is(err, writer.ErrEmptyPath) {
			t.Fatalf("Alias(\"\") should return ErrEmptyPath; got %v", err)
		}
	})
}

func TestImportSet_AliasOf(t *testing.T) {
	t.Parallel()

	t.Run("returns the assigned alias and true for a known path", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(nil)
		_, err := is.Imp("context")
		assertNoError(t, err)
		alias, ok := is.AliasOf("context")
		if !ok || alias != "context" {
			t.Fatalf("AliasOf mismatch: %q ok=%v", alias, ok)
		}
	})

	t.Run("returns \"\" and false for an unknown path", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(nil)
		if alias, ok := is.AliasOf("missing"); ok || alias != "" {
			t.Fatalf("AliasOf(unknown) should be (\"\", false); got %q ok=%v", alias, ok)
		}
	})
}

func TestImportSet_Imports(t *testing.T) {
	t.Parallel()

	t.Run("returns imports in insertion order", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(nil)
		_, _ = is.Imp("c")
		_, _ = is.Imp("a")
		_, _ = is.Imp("b")
		got := is.Imports()
		names := []string{got[0].Path, got[1].Path, got[2].Path}
		if !slices.Equal(names, []string{"c", "a", "b"}) {
			t.Fatalf("insertion order mismatch: %v", names)
		}
	})

	t.Run("returns a defensive copy", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(nil)
		_, _ = is.Imp("a")
		snap := is.Imports()
		snap[0].Path = "MUTATED"
		fresh := is.Imports()
		if fresh[0].Path != "a" {
			t.Fatalf("Imports should return a defensive copy")
		}
	})
}

func TestImportSet_SetSelf(t *testing.T) {
	t.Parallel()

	t.Run("no-op when both arguments are empty", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(nil)
		is.SetSelf("", "")
		// Self-elision should not have activated: a regular import
		// still gets its derived alias.
		alias, err := is.Imp("context")
		assertNoError(t, err)
		if alias != "context" {
			t.Fatalf("Imp(\"context\") = %q, want context", alias)
		}
	})

	t.Run("path-only SetSelf enables same-package elision", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(nil)
		is.SetSelf("example.com/foo", "")
		// Imp for the self path returns the empty alias and does
		// not register an import.
		alias, err := is.Imp("example.com/foo")
		assertNoError(t, err)
		if alias != "" {
			t.Fatalf("Imp(self) = %q, want empty alias for elision", alias)
		}
		if is.Len() != 0 {
			t.Fatalf("self-path Imp should not register an import; Len=%d", is.Len())
		}
	})

	t.Run("name-only SetSelf reserves the short name from collisions", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(nil)
		is.SetSelf("", "foo")
		// A cross-package import whose derived alias would collide
		// with the reserved short name falls back to a suffixed alias.
		alias, err := is.Imp("example.com/bar/foo")
		assertNoError(t, err)
		if alias == "foo" {
			t.Fatalf("Imp should not steal the reserved short name; got %q", alias)
		}
		if alias != "foo2" {
			t.Fatalf("Imp = %q, want foo2 (first collision suffix)", alias)
		}
	})

	t.Run("both arguments wire elision + reservation in one call", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(nil)
		is.SetSelf("example.com/foo", "foo")
		// Self-path elides.
		alias, err := is.Imp("example.com/foo")
		assertNoError(t, err)
		if alias != "" {
			t.Fatalf("self elision failed; got %q", alias)
		}
		// Cross-package collision avoids the reserved name.
		alias, err = is.Imp("example.com/other/foo")
		assertNoError(t, err)
		if alias == "foo" {
			t.Fatalf("collision should not produce reserved %q", alias)
		}
	})
}

func TestImportSet_ConcurrentImp(t *testing.T) {
	t.Parallel()

	t.Run("concurrent Imp calls are safe under -race and produce stable aliases", func(t *testing.T) {
		t.Parallel()
		is := writer.NewImportSet(nil)
		var wg sync.WaitGroup
		for range 16 {
			wg.Go(func() {
				_, _ = is.Imp("context")
			})
		}
		wg.Wait()
		if is.Len() != 1 {
			t.Fatalf("16 concurrent Imp(\"context\") should record one path; got Len=%d", is.Len())
		}
	})
}

// FuzzImportSet_Imp drives alias derivation and the collision-suffix
// loop over a *sequence* of import paths.
//
// Imp is the one place in the writer where a plugin-supplied string
// becomes an identifier in generated source, and its dangerous
// failure is silent: a wrong alias still renders, still formats, and
// only surfaces at the compiler in the consumer's repository. So the
// properties asserted here are the four that make an import block a
// legal declaration set, not the absence of a panic.
//
//   - Distinct paths never share an alias. Two imports under one
//     identifier is a redeclaration error in the generated file.
//   - One path resolves to one alias for the life of the set. An
//     alias that drifted mid-file would qualify half the references
//     with a name the import block never declares.
//   - Only the self path resolves to the empty alias. The Go backend
//     reads "" as same-package elision and drops the qualifier
//     (backend/golang/render_type.go), so an empty alias handed back
//     for a foreign path emits a bare, undefined symbol.
//   - The self path always elides and is never recorded as an import.
//
// Plus determinism, which is a repo-wide contract rather than a
// property of this type alone: replaying the same sequence against a
// fresh set must reproduce the same import block entry for entry, or
// regenerating an untouched project churns its output.
//
// Paths arrive newline-separated in one argument because collisions
// exist only *between* paths — a single-path target could never reach
// the suffix loop at all. Self path and self short name are separate
// arguments so the fuzzer can aim them at a path in the sequence
// (elision) or at a derived alias (reservation) independently.
func FuzzImportSet_Imp(f *testing.F) {
	seeds := [][3]string{
		// The empty path, which Imp must reject rather than alias.
		{"", "", ""},
		// One stdlib-shaped path taking the plain derived branch.
		{"context", "", ""},
		// A repeat, so the second call takes the cached branch.
		{strings.Repeat("context\n", 2), "", ""},
		// Three paths sharing a last segment: suffix escalation.
		{"context\nfoo.com/x/context\nbar.com/y/context", "", ""},
		// A derived name that collides with an already-issued suffix.
		{"a.com/ctx\nb.com/ctx\nc.com/ctx2", "", ""},
		// Self elision together with short-name reservation.
		{"example.com/foo", "example.com/foo", "foo"},
		// Reservation with no self path: nothing elides, "foo" is taken.
		{"example.com/bar/foo", "", "foo"},
		// Elision beside a genuine import from the same prefix.
		{"example.com/foo\nexample.com/foo/bar", "example.com/foo", ""},
		// Every element empty: the whole sequence must be rejected.
		{"\n\n", "", ""},
		// A trailing separator, which yields an empty tail element.
		{"context\n", "", ""},
		// Self appearing last rather than first in the sequence.
		{"a\nb\nc\nd", "d", ""},
		// A deeply nested path, exercising the last-segment scan.
		{"a/b/c/d/e/f/g/h", "", ""},
		// Invalid UTF-8 in a path: aliases are byte strings, not text.
		{"\xff\xfe/pkg", "", ""},
		// Duplicates interleaved with collisions.
		{strings.Repeat("x/ctx\n", 2) + strings.Repeat("y/ctx\n", 2) + "z/ctx", "", ""},
		// Regression for a defect this target found: a path whose
		// last segment was empty derived to the empty alias, which
		// Imp returned as though the path were the file's own
		// package — so a foreign package rendered unqualified. Two
		// hand-written unit tests asserted that behaviour as
		// intended before the fuzzer contradicted them.
		{"example.com/foo/", "", ""},
		// The minimised form of the same defect, where trimming
		// leaves nothing at all and Imp must reject rather than
		// alias.
		{"/", "", ""},
	}
	for _, seed := range seeds {
		f.Add(seed[0], seed[1], seed[2])
	}

	f.Fuzz(func(t *testing.T, blob, selfPath, selfName string) {
		paths := strings.Split(blob, "\n")
		if len(paths) > fuzzMaxImportPaths {
			paths = paths[:fuzzMaxImportPaths]
		}

		first := registerSequence(t, paths, selfPath, selfName)
		second := registerSequence(t, paths, selfPath, selfName)
		if !slices.Equal(first, second) {
			t.Fatalf("replaying the same sequence produced a different import block:\n first: %+v\nsecond: %+v",
				first, second)
		}
	})
}

// registerSequence registers paths against a fresh [writer.ImportSet]
// and asserts the invariants that make the resulting import block a
// legal declaration set. It returns the recorded imports so the caller
// can compare two independent replays and catch any dependence on map
// iteration order.
//
// Every failure message names the input that produced it: a fuzz
// corpus entry is only useful if the report identifies which path in
// the sequence broke the invariant.
func registerSequence(t *testing.T, paths []string, selfPath, selfName string) []writer.Import {
	t.Helper()

	is := writer.NewImportSet(nil)
	is.SetSelf(selfPath, selfName)

	assigned := make(map[string]string, len(paths)) // path -> alias
	owner := make(map[string]string, len(paths))    // alias -> path
	order := make([]string, 0, len(paths))          // first-registration order

	for _, p := range paths {
		alias, err := is.Imp(p)

		// Self-path elision is decided before aliasability, because
		// the two rules overlap and this one wins. SetSelf declares p
		// to be the rendered file's own package, so Imp returns the
		// empty alias by contract whether or not p would otherwise
		// carry a derivable identifier — SetSelf("/") being the case
		// the fuzzer produced. Testing aliasability first reported
		// that legitimate elision as a missing rejection.
		if selfPath != "" && p == selfPath {
			if err != nil {
				t.Fatalf("Imp(%q) rejected the self path: %v", p, err)
			}
			if alias != "" {
				t.Fatalf("Imp(%q) = %q for the self path; want the empty alias (same-package elision)", p, alias)
			}
			continue
		}

		// A path that is nothing but separators carries no derivable
		// identifier, and Imp rejects it rather than returning the
		// empty alias — which the caller cannot distinguish from
		// same-package elision. "" and "///" are one case; only the
		// spelling differs. Keying the arm on that property rather
		// than on p == "" is what lets the corpus entry for "/"
		// assert the rejection instead of reporting it as a refusal
		// of a legitimate path.
		// Widened from "trims to nothing" to "yields no identifier
		// rune". A segment may be non-empty and still unaliasable —
		// " " and "---" carry nothing an alias can be built from —
		// and since the derived alias is emitted explicitly, an
		// alias that is not a valid Go identifier would produce
		// source the parser rejects rather than a usable import.
		unaliasable := writer.DefaultAlias(p) == ""
		switch {
		case unaliasable && !errors.Is(err, writer.ErrEmptyPath):
			t.Fatalf("Imp(%q) returned alias %q, err %v; want ErrEmptyPath", p, alias, err)
		case unaliasable:
			continue
		case err != nil:
			t.Fatalf("Imp(%q) rejected a path with a derivable alias: %v", p, err)
		}

		// An empty alias for anything other than the self path is a
		// miscompile: renderType treats "" as same-package elision and
		// emits the bare symbol name for a foreign package.
		if alias == "" {
			t.Fatalf("Imp(%q) returned the empty alias for a path that is not self (self=%q); "+
				"the Go backend reads \"\" as same-package elision and drops the qualifier, "+
				"so the rendered file references an undefined bare symbol", p, selfPath)
		}

		// SetSelf reserves the rendered file's own package identifier.
		// An import that took it would shadow the name the file
		// declares, and every unqualified reference in the file would
		// silently resolve to the imported package instead.
		if selfName != "" && alias == selfName {
			t.Fatalf("Imp(%q) = %q, the rendered file's own package name; the import shadows it", p, alias)
		}

		if prev, seen := assigned[p]; seen {
			if prev != alias {
				t.Fatalf("Imp(%q) drifted: first returned %q, later %q", p, prev, alias)
			}
			continue
		}

		if other, taken := owner[alias]; taken {
			t.Fatalf("alias %q assigned to two distinct paths: %q and %q", alias, other, p)
		}
		assigned[p] = alias
		owner[alias] = p
		order = append(order, p)
	}

	// The recorded set must agree with what Imp handed back. A
	// disagreement means the rendered import block declares different
	// names than the body of the file uses.
	imports := is.Imports()
	if len(imports) != len(order) {
		t.Fatalf("Imports() has %d entries, want %d (one per distinct non-self path)", len(imports), len(order))
	}
	if is.Len() != len(order) {
		t.Fatalf("Len() = %d, want %d", is.Len(), len(order))
	}
	for k, imp := range imports {
		if imp.Path != order[k] {
			t.Fatalf("Imports()[%d].Path = %q, want %q (insertion order)", k, imp.Path, order[k])
		}
		if imp.Alias != assigned[imp.Path] {
			t.Fatalf("Imports()[%d].Alias = %q, but Imp returned %q", k, imp.Alias, assigned[imp.Path])
		}
		if a, ok := is.AliasOf(imp.Path); !ok || a != imp.Alias {
			t.Fatalf("AliasOf(%q) = %q, %v; want %q, true", imp.Path, a, ok, imp.Alias)
		}
	}
	return imports
}

// BenchmarkImportSet_Imp measures import registration, the operation
// the Go backend performs once per type reference in every rendered
// file.
//
// Three shapes, because Imp has three cost regimes and only the last
// can degrade:
//
//   - repeat lookups of an already-registered path: the branch a
//     template hits for the second and every later reference to the
//     same package. Allocation-free by construction, so the reported
//     0 B/op is the assertion rather than an artifact of an
//     eliminated loop body — the stored alias keeps the call live.
//   - registering distinct paths: linear, one map insert each.
//   - registering colliding last segments: every path derives to the
//     same alias, so the suffix loop rescans the used table for each
//     new registration and formats a fresh candidate name per probe.
//     This is the only superlinear path in the type, which is why the
//     scaling sizes run out to 1000.
//
// One op is one whole import block. The [writer.ImportSet] allocation
// sits inside the timed region for the two scaling shapes because a
// fresh set per iteration is what keeps the measurement on the
// registration path instead of the cached path; the path slices
// themselves are built once, above the loop.
func BenchmarkImportSet_Imp(b *testing.B) {
	b.ReportAllocs()

	sizes := []int{1, 10, 100, 1000}

	b.Run("repeat lookups of a registered path", func(b *testing.B) {
		b.ReportAllocs()
		is := writer.NewImportSet(nil)
		if _, err := is.Imp("context"); err != nil {
			b.Fatalf("Imp: %v", err)
		}
		for b.Loop() {
			alias, err := is.Imp("context")
			if err != nil {
				b.Fatalf("Imp: %v", err)
			}
			impSink = alias
		}
	})

	b.Run("registering distinct paths", func(b *testing.B) {
		for _, n := range sizes {
			paths := benchDistinctPaths(n)
			b.Run(strconv.Itoa(n), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					impSink = registerAll(paths)
				}
				if impSink == "" {
					b.Fatal("registerAll reported a rejected path; the benchmark measured an error path")
				}
			})
		}
	})

	b.Run("registering colliding last segments", func(b *testing.B) {
		for _, n := range sizes {
			paths := benchCollidingPaths(n)
			b.Run(strconv.Itoa(n), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					impSink = registerAll(paths)
				}
				if impSink == "" {
					b.Fatal("registerAll reported a rejected path; the benchmark measured an error path")
				}
			})
		}
	})
}

// registerAll builds one complete import block from paths and returns
// the last alias issued.
//
// It deliberately takes no *testing.B: a b.Helper() call inside the
// timed region costs a locked map insert per iteration and would
// swamp the very cost being measured. A rejected path collapses the
// return to the empty string instead, which the caller checks once
// after the loop.
func registerAll(paths []string) string {
	is := writer.NewImportSet(nil)
	last := ""
	for _, p := range paths {
		alias, err := is.Imp(p)
		if err != nil {
			return ""
		}
		last = alias
	}
	return last
}

// benchDistinctPaths returns n import paths whose derived aliases are
// all distinct, so registration cost is the map insert alone and the
// suffix loop never runs.
func benchDistinctPaths(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "example.com/bench/pkg" + strconv.Itoa(i)
	}
	return out
}

// benchCollidingPaths returns n distinct import paths that all derive
// to the same alias, forcing Imp's suffix loop to escalate through
// every alias already issued. This is the shape that turns
// registration quadratic, and the reason the scaling case exists.
func benchCollidingPaths(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "example.com/bench/p" + strconv.Itoa(i) + "/ctx"
	}
	return out
}

// TestDefaultAlias_Sanitisation pins that a derived alias is always
// a usable Go identifier.
//
// A path's last segment is not required to be one, and hyphenated
// segments are ordinary in Go — go-redis, go-cmp. Emitting such a
// segment as a qualifier produces source the parser rejects, and the
// backend then writes the file anyway.
func TestDefaultAlias_Sanitisation(t *testing.T) {
	t.Parallel()

	for name, tc := range map[string]struct{ path, want string }{
		"ordinary segment is unchanged":  {"go.thesmos.sh/eidos/emit", "emit"},
		"hyphens are dropped":            {"example.com/x/if-absent", "ifabsent"},
		"dots are dropped":               {"gopkg.in/yaml.v3", "yamlv3"},
		"a keyword segment is suffixed":  {"example.com/x/go", "go_"},
		"a leading digit gains a prefix": {"example.com/x/2fa", "pkg2fa"},
		// Nothing survives sanitisation, so no alias is derivable
		// and Imp must reject the path rather than bind it under a
		// name nothing declares.
		"an all-punctuation segment yields no alias": {"example.com/x/---", ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := writer.DefaultAlias(tc.path); got != tc.want {
				t.Fatalf("DefaultAlias(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// TestNeedsExplicitAlias covers the decision that makes a sanitised
// alias correct rather than a guess.
//
// An unaliased import binds to the package's declared name, which the
// writer never sees. Stating the alias makes the qualifier true
// regardless — `import yamlv3 "gopkg.in/yaml.v3"` binds even though
// the package declares `package yaml`.
func TestNeedsExplicitAlias(t *testing.T) {
	t.Parallel()

	t.Run("a verbatim segment stays implicit", func(t *testing.T) {
		t.Parallel()
		// The common case, and the one where convention makes the
		// implicit binding right. Emitting it would add noise to
		// every generated import block.
		if writer.NeedsExplicitAlias("go.thesmos.sh/eidos/emit", "emit") {
			t.Errorf("an unmodified segment should not need an explicit alias")
		}
	})

	t.Run("a sanitised segment must be stated", func(t *testing.T) {
		t.Parallel()
		if !writer.NeedsExplicitAlias("example.com/x/if-absent", "ifabsent") {
			t.Errorf("a sanitised alias must be written into the import")
		}
	})

	t.Run("a collision suffix must be stated", func(t *testing.T) {
		t.Parallel()
		// Imp's suffix loop produces these; left implicit, the
		// second import would bind the same name as the first.
		if !writer.NeedsExplicitAlias("example.com/y/context", "context2") {
			t.Errorf("a suffixed alias must be written into the import")
		}
	})
}

// TestImportSet_Reset covers the clear-in-place path the backend uses
// between files.
//
// Each field gets its own subtest because forgetting any one of them
// produces a different, silent defect on the next file, and a single
// combined assertion would not say which.
func TestImportSet_Reset(t *testing.T) {
	t.Parallel()

	seeded := func(t *testing.T) *writer.ImportSet {
		t.Helper()
		i := writer.NewImportSet(nil)
		i.SetSelf("example.com/self", "self")
		if err := i.Alias("example.com/x/users", "renamed"); err != nil {
			t.Fatalf("Alias: %v", err)
		}
		// z collides with y on the derived base, so the suffix
		// bookkeeping is actually populated — without a real
		// collision here, lastSfx stays zero and a Reset that
		// forgot it would look correct.
		for _, p := range []string{
			"context", "example.com/x/users",
			"example.com/y/users", "example.com/z/users",
		} {
			if _, err := i.Imp(p); err != nil {
				t.Fatalf("Imp %q: %v", p, err)
			}
		}
		i.Reset()
		return i
	}

	t.Run("the recorded imports are gone", func(t *testing.T) {
		t.Parallel()
		if got := seeded(t).Imports(); len(got) != 0 {
			t.Fatalf("Reset left %d imports: %+v", len(got), got)
		}
	})

	t.Run("same-package elision no longer fires", func(t *testing.T) {
		t.Parallel()
		// self is the field a careless Reset forgets. It makes a
		// path equal to it render bare — no qualifier, no import —
		// so leaving it set makes the next file emit unqualified
		// names for a package it does not live in. That compiles
		// against the wrong symbol or not at all, with no
		// diagnostic.
		i := seeded(t)
		alias, err := i.Imp("example.com/self")
		if err != nil {
			t.Fatalf("Imp: %v", err)
		}
		if alias == "" {
			t.Fatalf("Reset left self set; the path still elides")
		}
	})

	t.Run("an explicit alias override is gone", func(t *testing.T) {
		t.Parallel()
		i := seeded(t)
		alias, err := i.Imp("example.com/x/users")
		if err != nil {
			t.Fatalf("Imp: %v", err)
		}
		if alias != "users" {
			t.Fatalf("alias = %q, want the derived %q — the override survived Reset", alias, "users")
		}
	})

	t.Run("collision suffixes restart", func(t *testing.T) {
		t.Parallel()
		// used and lastSfx both feed suffix assignment. A Reset that
		// cleared one but not the other would hand the first import
		// of the next file a suffix it did not earn.
		i := seeded(t)
		first, err := i.Imp("example.com/a/users")
		if err != nil {
			t.Fatalf("Imp: %v", err)
		}
		if first != "users" {
			t.Fatalf("first alias after Reset = %q, want %q", first, "users")
		}
		second, err := i.Imp("example.com/b/users")
		if err != nil {
			t.Fatalf("Imp: %v", err)
		}
		if second != "users2" {
			t.Fatalf("second alias after Reset = %q, want %q", second, "users2")
		}
	})

	t.Run("the derive function survives", func(t *testing.T) {
		t.Parallel()
		// derive is the one field that must not be cleared: it is
		// construction-time configuration, not per-file state.
		i := writer.NewImportSet(func(string) string { return "fixed" })
		if _, err := i.Imp("example.com/anything"); err != nil {
			t.Fatalf("Imp: %v", err)
		}
		i.Reset()
		alias, err := i.Imp("example.com/other")
		if err != nil {
			t.Fatalf("Imp: %v", err)
		}
		if alias != "fixed" {
			t.Fatalf("alias = %q, want %q — Reset cleared the derive func", alias, "fixed")
		}
	})
}
