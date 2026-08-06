// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
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

// TestPruneImports_MatchesResolverDeletion is the differential: the
// same body finalised with the prune applied and with the full
// import set left intact must produce identical bytes, because
// goimports deletes exactly what the prune dropped.
//
// This is what proves the replacement reproduces the resolver
// rather than merely resembling it. It is inherently temporary —
// when the goimports pass is removed the two chains stop being two
// chains, and this test goes with it. The durable statement of the
// same property is the "declaring an unused import changes no byte"
// subtest in render_file_test.go.
func TestPruneImports_MatchesResolverDeletion(t *testing.T) {
	t.Parallel()

	const body = "func f() { _ = kept.Symbol }\n"
	full := []writer.Import{
		{Path: "example.com/kept", Alias: "kept"},
		{Path: "strings", Alias: "strings"},
	}
	refs := refsOf(t, "package p\n\n"+body)
	pruned := pruneImports(full, refs)

	compose := func(imports []writer.Import) []byte {
		var buf bytes.Buffer
		buf.WriteString("package p\n")
		writeImportBlock(&buf, imports)
		buf.WriteString("\n" + body)
		return buf.Bytes()
	}

	target := emit.Target{Dir: "p", Filename: "x.go", Package: "p"}
	finalise := func(imports []writer.Import) []byte {
		d := diag.New()
		return finaliseBody(compose(imports), target, d.For(Name), imports)
	}

	t.Run("the prune drops exactly the unreferenced entry", func(t *testing.T) {
		t.Parallel()
		if len(pruned) != 1 || pruned[0].Path != "example.com/kept" {
			t.Fatalf("expected only the referenced import to survive; got %+v", pruned)
		}
	})

	t.Run("both chains finalise to the same bytes", func(t *testing.T) {
		t.Parallel()
		withPrune, withResolver := finalise(pruned), finalise(full)
		if !bytes.Equal(withPrune, withResolver) {
			t.Fatalf("prune and resolver disagree:\npruned:\n%s\nresolver:\n%s", withPrune, withResolver)
		}
	})
}
