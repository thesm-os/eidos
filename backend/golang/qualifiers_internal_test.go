// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/writer"
)

// refsOf parses src as a file body and returns the collected sets,
// failing the test when the source does not parse. Tests that want
// the failure path call [collectRefs] directly.
func refsOf(t *testing.T, src string) fileRefs {
	t.Helper()
	refs := collectRefs([]byte(src), "x.go")
	if !refs.parsed {
		t.Fatalf("source did not parse:\n%s", src)
	}
	return refs
}

// keysOf returns the sorted keys of a set, for readable diffs.
func keysOf(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// assertHas fails unless every want is present in set.
func assertHas(t *testing.T, set map[string]struct{}, want ...string) {
	t.Helper()
	for _, w := range want {
		if _, ok := set[w]; !ok {
			t.Fatalf("expected %q in set; got %v", w, keysOf(set))
		}
	}
}

// assertLacks fails when any unwanted name is present in set.
func assertLacks(t *testing.T, set map[string]struct{}, unwanted ...string) {
	t.Helper()
	for _, u := range unwanted {
		if _, ok := set[u]; ok {
			t.Fatalf("did not expect %q in set; got %v", u, keysOf(set))
		}
	}
}

// TestCollectRefs_Qualifiers covers every syntactic position a
// package qualifier can occupy in generated output. A position the
// walk misses is an import silently pruned out of a file that
// needed it, so each shape gets its own subtest rather than one
// combined fixture — a combined one passes as long as any single
// position is found.
func TestCollectRefs_Qualifiers(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
		want string
	}{
		{"embedded struct field", "type S struct{ pkg.T }", "pkg"},
		{"embedded interface method set", "type I interface{ pkg.J }", "pkg"},
		{"generic instantiation", "var x pkg.T[int]", "pkg"},
		{"map key", "var m map[pkg.K]string", "pkg"},
		{"map value", "var m map[string]pkg.V", "pkg"},
		{"channel element", "var c chan pkg.E", "pkg"},
		{"composite literal", "var v = pkg.T{}", "pkg"},
		{"pointer field type", "type S struct{ F *pkg.T }", "pkg"},
		{"slice element", "var s []pkg.T", "pkg"},
		{"variadic parameter", "func f(v ...pkg.T) {}", "pkg"},
		{"named result", "func f() (out pkg.T) { return }", "pkg"},
		{"type constraint", "type C interface{ pkg.Num }", "pkg"},
		{"method value in init", "func init() { f := pkg.Fn; _ = f }", "pkg"},
		{"call in init", "func init() { pkg.Register() }", "pkg"},
		{"type assertion", "func f(a any) { _ = a.(pkg.T) }", "pkg"},
		{"function type parameter", "type F func(pkg.T) error", "pkg"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			refs := refsOf(t, "package p\n\n"+tc.body+"\n")
			assertHas(t, refs.qualifiers, tc.want)
		})
	}
}

// TestCollectRefs_QualifierBoundaries pins what must NOT count as a
// package qualifier. Over-counting keeps a dead import; it also
// masks the unresolved-qualifier report that consumes the same set.
func TestCollectRefs_QualifierBoundaries(t *testing.T) {
	t.Parallel()

	t.Run("a chained selector records only its root", func(t *testing.T) {
		t.Parallel()
		// In a.b.c only `a` can be an import: `b` is a field of
		// whatever `a` resolves to.
		refs := refsOf(t, "package p\n\nfunc f() { _ = a.b.c }\n")
		assertHas(t, refs.qualifiers, "a")
		assertLacks(t, refs.qualifiers, "b", "c")
	})

	t.Run("a selector on a call result records no qualifier", func(t *testing.T) {
		t.Parallel()
		// The X of the outer selector is a CallExpr, not an Ident,
		// so nothing here could name an import.
		refs := refsOf(t, "package p\n\nfunc f() { _ = g().Field }\n")
		assertLacks(t, refs.qualifiers, "g", "Field")
	})

	t.Run("a struct field name is not a qualifier", func(t *testing.T) {
		t.Parallel()
		refs := refsOf(t, "package p\n\ntype S struct{ time int }\n")
		assertLacks(t, refs.qualifiers, "time")
	})
}

// TestCollectRefs_Declared covers the binding positions the
// unresolved-qualifier subtraction rests on. A missed binding turns
// into an invented report about a name the file declares itself.
func TestCollectRefs_Declared(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
		want string
	}{
		{"function name", "func alpha() {}", "alpha"},
		{"receiver name", "func (rcv S) M() {}", "rcv"},
		{"parameter", "func f(param int) {}", "param"},
		{"named result", "func f() (res int) { return }", "res"},
		{"type parameter", "func f[T any](v T) {}", "T"},
		{"package-level var", "var pkgvar int", "pkgvar"},
		{"package-level const", "const pkgconst = 1", "pkgconst"},
		{"type name", "type Named struct{}", "Named"},
		{"local var", "func f() { var local int; _ = local }", "local"},
		{"short declaration", "func f() { short := 1; _ = short }", "short"},
		{"range key", "func f(s []int) { for key := range s { _ = key } }", "key"},
		{"range value", "func f(s []int) { for _, val := range s { _ = val } }", "val"},
		{"type switch binding", "func f(a any) { switch bound := a.(type) { default: _ = bound } }", "bound"},
		{"generic type parameter on a type", "type G[E any] struct{ v E }", "E"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			refs := refsOf(t, "package p\n\n"+tc.body+"\n")
			assertHas(t, refs.declared, tc.want)
		})
	}

	t.Run("the blank identifier is never a declared name", func(t *testing.T) {
		t.Parallel()
		// `_` binds nothing, and recording it would make every file
		// containing `_ = x` look like it shadows a blank import.
		refs := refsOf(t, "package p\n\nfunc f() { _, v := g(); _ = v }\n")
		assertLacks(t, refs.declared, writer.BlankAlias)
	})
}

// TestCollectRefs_TopLevel pins the package-scope set. It is what a
// sibling target in the same package may select on without importing
// anything, so a name wrongly included here suppresses a real
// report and a name wrongly excluded invents one.
func TestCollectRefs_TopLevel(t *testing.T) {
	t.Parallel()

	const src = `package p

type Exported struct{}

func (Exported) Method() {}

func Free() {}

var V = 1

const C = 2

func withLocal() { inner := 3; _ = inner }
`

	t.Run("package-scope declarations are recorded", func(t *testing.T) {
		t.Parallel()
		refs := refsOf(t, src)
		assertHas(t, refs.topLevel, "Exported", "Free", "V", "C", "withLocal")
	})

	t.Run("a method is not package scope", func(t *testing.T) {
		t.Parallel()
		// A method lives in its receiver's namespace; a sibling file
		// cannot write `Method()` unqualified.
		refs := refsOf(t, src)
		assertLacks(t, refs.topLevel, "Method")
	})

	t.Run("a local is not package scope", func(t *testing.T) {
		t.Parallel()
		refs := refsOf(t, src)
		assertLacks(t, refs.topLevel, "inner")
	})
}

// TestCollectRefs_ParseFailure pins the degrade path. An empty
// qualifier set is indistinguishable from "this file references
// nothing", which would prune every import out of a file that only
// failed to parse.
func TestCollectRefs_ParseFailure(t *testing.T) {
	t.Parallel()

	t.Run("unparseable source reports parsed false", func(t *testing.T) {
		t.Parallel()
		refs := collectRefs([]byte("package p\n\nfunc ( {"), "x.go")
		if refs.parsed {
			t.Fatalf("expected parsed=false for unparseable source")
		}
	})

	t.Run("an unparsed walk prunes nothing", func(t *testing.T) {
		t.Parallel()
		tracked := []writer.Import{{Path: "context", Alias: "context"}}
		got := pruneImports(tracked, fileRefs{})
		if len(got) != 1 {
			t.Fatalf("expected every import kept when the walk did not run; got %+v", got)
		}
	})
}

// TestPruneImports covers the keep rules directly, at the level
// where they are decided. The rendered-output consequences are
// covered in render_file_test.go; these pin the rule itself.
func TestPruneImports(t *testing.T) {
	t.Parallel()

	refs := refsOf(t, "package p\n\nfunc f() { _ = used.Symbol }\n")

	t.Run("a referenced import is kept", func(t *testing.T) {
		t.Parallel()
		got := pruneImports([]writer.Import{{Path: "example.com/used", Alias: "used"}}, refs)
		if len(got) != 1 {
			t.Fatalf("expected the referenced import kept; got %+v", got)
		}
	})

	t.Run("an unreferenced import is dropped", func(t *testing.T) {
		t.Parallel()
		got := pruneImports([]writer.Import{{Path: "strings", Alias: "strings"}}, refs)
		if len(got) != 0 {
			t.Fatalf("expected the unreferenced import dropped; got %+v", got)
		}
	})

	t.Run("blank and dot imports survive unreferenced", func(t *testing.T) {
		t.Parallel()
		// No body text can name either, so their absence from the
		// qualifier set carries no information at all.
		in := []writer.Import{
			{Path: "embed", Alias: writer.BlankAlias},
			{Path: "example.com/dot", Alias: writer.DotAlias},
		}
		got := pruneImports(in, refs)
		if len(got) != 2 {
			t.Fatalf("expected both side-effect imports kept; got %+v", got)
		}
	})

	t.Run("the kept order is the order supplied", func(t *testing.T) {
		t.Parallel()
		// Sorting and grouping belong to writeImportBlock; a prune
		// that reordered would make that ownership ambiguous.
		in := []writer.Import{
			{Path: "example.com/used", Alias: "used"},
			{Path: "strings", Alias: "strings"},
			{Path: "embed", Alias: writer.BlankAlias},
		}
		got := pruneImports(in, refs)
		paths := make([]string, 0, len(got))
		for _, imp := range got {
			paths = append(paths, imp.Path)
		}
		if want := []string{"example.com/used", "embed"}; !slices.Equal(paths, want) {
			t.Fatalf("expected %v; got %v", want, paths)
		}
	})

	t.Run("the input slice is not modified in place", func(t *testing.T) {
		t.Parallel()
		// The caller still holds the full ImportSet view; a filter
		// that wrote through the same backing array would corrupt it.
		in := []writer.Import{
			{Path: "strings", Alias: "strings"},
			{Path: "example.com/used", Alias: "used"},
		}
		_ = pruneImports(in, refs)
		if in[0].Path != "strings" || in[1].Path != "example.com/used" {
			t.Fatalf("prune modified its input: %+v", in)
		}
	})
}

// TestShadowedImports pins the reported-not-acted-on case: an
// import kept only because a local shares its alias. Dropping it is
// the under-counting direction, which changes what compiling code
// does, so the rule names it instead.
func TestShadowedImports(t *testing.T) {
	t.Parallel()

	t.Run("an import aliased to a declared local is reported", func(t *testing.T) {
		t.Parallel()
		refs := refsOf(t, "package p\n\nfunc f() { strings := newThing(); _ = strings.Field }\n")
		kept := pruneImports([]writer.Import{{Path: "strings", Alias: "strings"}}, refs)
		got := shadowedImports(kept, refs)
		if len(got) != 1 || got[0].Path != "strings" {
			t.Fatalf("expected the shadowed import reported; got %+v", got)
		}
	})

	t.Run("a genuinely referenced import is not reported", func(t *testing.T) {
		t.Parallel()
		refs := refsOf(t, "package p\n\nfunc f() { _ = strings.Builder{} }\n")
		kept := pruneImports([]writer.Import{{Path: "strings", Alias: "strings"}}, refs)
		if got := shadowedImports(kept, refs); len(got) != 0 {
			t.Fatalf("expected no report for an unshadowed import; got %+v", got)
		}
	})

	t.Run("blank and dot imports are never reported", func(t *testing.T) {
		t.Parallel()
		// `_` is excluded from declared, and `.` binds no name, so
		// neither can be shadowed by construction.
		refs := refsOf(t, "package p\n\nfunc f() { _ = 1 }\n")
		in := []writer.Import{
			{Path: "embed", Alias: writer.BlankAlias},
			{Path: "example.com/dot", Alias: writer.DotAlias},
		}
		if got := shadowedImports(in, refs); len(got) != 0 {
			t.Fatalf("expected no report for side-effect imports; got %+v", got)
		}
	})
}

// BenchmarkCollectRefs records the cost of the walk the prune and
// the unresolved-qualifier diagnostic share. The parse dominates;
// the traversal is well under a fifth of it. Kept in the baseline so
// the fused parse-and-print follow-up has a number to beat.
func BenchmarkCollectRefs(b *testing.B) {
	for _, n := range []int{1, 16, 100, 1000} {
		src := []byte(benchRefsSource(n))
		b.Run(fmt.Sprintf("decls=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if refs := collectRefs(src, "bench.go"); !refs.parsed {
					b.Fatal("fixture must parse")
				}
			}
		})
	}
}

// benchRefsSource builds a file body of n declarations, each
// carrying a package qualifier, a local binding and a selector — the
// three shapes the walk actually pays for.
func benchRefsSource(n int) string {
	var b strings.Builder
	b.WriteString("package bench\n\n")
	for i := range n {
		fmt.Fprintf(&b, `type T%d struct {
	Ctx context.Context
	Buf bytes.Buffer
}

func (t *T%d) Do(in fmt.Stringer) error {
	s := in.String()
	_ = s
	return errors.New(s)
}

`, i, i)
	}
	return b.String()
}

// TestUnresolvedCandidates covers the subtraction directly.
// candidates = qualifiers − declared − bound.
func TestUnresolvedCandidates(t *testing.T) {
	t.Parallel()

	t.Run("a qualifier with no import and no binding is a candidate", func(t *testing.T) {
		t.Parallel()
		refs := refsOf(t, "package p\n\nfunc f() { _ = missing.Symbol }\n")
		got := unresolvedCandidates(refs, nil)
		if !slices.Equal(got, []string{"missing"}) {
			t.Fatalf("expected [missing]; got %v", got)
		}
	})

	t.Run("an imported qualifier is not a candidate", func(t *testing.T) {
		t.Parallel()
		refs := refsOf(t, "package p\n\nfunc f() { _ = ctx.Symbol }\n")
		got := unresolvedCandidates(refs, []writer.Import{{Path: "context", Alias: "ctx"}})
		if len(got) != 0 {
			t.Fatalf("expected no candidates; got %v", got)
		}
	})

	t.Run("a locally declared name is not a candidate", func(t *testing.T) {
		t.Parallel()
		refs := refsOf(t, "package p\n\nfunc f() { local := g(); _ = local.Field }\n")
		if got := unresolvedCandidates(refs, nil); len(got) != 0 {
			t.Fatalf("expected no candidates; got %v", got)
		}
	})

	t.Run("candidates come out sorted", func(t *testing.T) {
		t.Parallel()
		refs := refsOf(t, "package p\n\nfunc f() { _ = zulu.A; _ = alpha.B; _ = mike.C }\n")
		got := unresolvedCandidates(refs, nil)
		if !slices.Equal(got, []string{"alpha", "mike", "zulu"}) {
			t.Fatalf("expected sorted candidates; got %v", got)
		}
	})

	t.Run("a dot import suspends the check entirely", func(t *testing.T) {
		t.Parallel()
		// A dot import merges an unknown set of exported names into
		// file scope, so any qualifier could legitimately come from
		// it and every report would be a guess.
		refs := refsOf(t, "package p\n\nfunc f() { _ = anything.Symbol }\n")
		in := []writer.Import{{Path: "example.com/dot", Alias: writer.DotAlias}}
		if got := unresolvedCandidates(refs, in); got != nil {
			t.Fatalf("expected the check suspended under a dot import; got %v", got)
		}
	})

	t.Run("a blank import binds no name and shields nothing", func(t *testing.T) {
		t.Parallel()
		refs := refsOf(t, "package p\n\nfunc f() { _ = missing.Symbol }\n")
		in := []writer.Import{{Path: "embed", Alias: writer.BlankAlias}}
		if got := unresolvedCandidates(refs, in); !slices.Equal(got, []string{"missing"}) {
			t.Fatalf("expected [missing]; got %v", got)
		}
	})

	t.Run("an unparsed walk yields no candidates", func(t *testing.T) {
		t.Parallel()
		// Reporting off an empty qualifier set would be silent; the
		// risk is the reverse — inventing reports from a set that was
		// never populated.
		if got := unresolvedCandidates(fileRefs{}, nil); got != nil {
			t.Fatalf("expected no candidates from an unparsed walk; got %v", got)
		}
	})
}

// TestUnionTopLevel covers the grouping that makes the check
// package-scoped. Targets sharing a (Dir, Package) are one Go
// package; anything else is a different one.
func TestUnionTopLevel(t *testing.T) {
	t.Parallel()

	keys := []emit.Target{
		{Dir: "x", Filename: "a.go", Package: "x"},
		{Dir: "x", Filename: "b.go", Package: "x"},
		{Dir: "y", Filename: "c.go", Package: "y"},
	}
	results := []renderResult{
		{topLevel: map[string]struct{}{"Alpha": {}}},
		{topLevel: map[string]struct{}{"Beta": {}}},
		{topLevel: map[string]struct{}{"Gamma": {}}},
	}

	t.Run("names merge across targets in one package", func(t *testing.T) {
		t.Parallel()
		got := unionTopLevel(keys, results)
		assertHas(t, got[packageKey{dir: "x", pkg: "x"}], "Alpha", "Beta")
	})

	t.Run("a different package does not contribute", func(t *testing.T) {
		t.Parallel()
		got := unionTopLevel(keys, results)
		assertLacks(t, got[packageKey{dir: "x", pkg: "x"}], "Gamma")
	})

	t.Run("a skipped target contributes nothing", func(t *testing.T) {
		t.Parallel()
		// It produced no file, so its declarations do not exist.
		skipped := []renderResult{
			{topLevel: map[string]struct{}{"Alpha": {}}, skip: true},
			{topLevel: map[string]struct{}{"Beta": {}}},
			{},
		}
		got := unionTopLevel(keys, skipped)
		assertLacks(t, got[packageKey{dir: "x", pkg: "x"}], "Alpha")
		assertHas(t, got[packageKey{dir: "x", pkg: "x"}], "Beta")
	})
}

// TestUnresolvedAfterPackage covers the final subtraction.
func TestUnresolvedAfterPackage(t *testing.T) {
	t.Parallel()

	t.Run("a name the package declares is removed", func(t *testing.T) {
		t.Parallel()
		got := unresolvedAfterPackage([]string{"Sibling", "Missing"},
			map[string]struct{}{"Sibling": {}})
		if !slices.Equal(got, []string{"Missing"}) {
			t.Fatalf("expected [Missing]; got %v", got)
		}
	})

	t.Run("order survives the subtraction", func(t *testing.T) {
		t.Parallel()
		got := unresolvedAfterPackage([]string{"alpha", "mike", "zulu"}, nil)
		if !slices.Equal(got, []string{"alpha", "mike", "zulu"}) {
			t.Fatalf("expected the sorted order preserved; got %v", got)
		}
	})

	t.Run("the input slice is not modified in place", func(t *testing.T) {
		t.Parallel()
		in := []string{"Sibling", "Missing"}
		_ = unresolvedAfterPackage(in, map[string]struct{}{"Sibling": {}})
		if !slices.Equal(in, []string{"Sibling", "Missing"}) {
			t.Fatalf("subtraction modified its input: %v", in)
		}
	})
}
