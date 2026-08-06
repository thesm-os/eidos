// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store_test

import (
	"errors"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/store"
)

// keyNodeViewMeta is the meta.Key used by the NodeView meta-key
// index test, declared once at package scope so the global key
// registry never sees re-registration on -count > 1.
var keyNodeViewMeta = meta.NewKey("store.node_view.meta", meta.BoolParser)

func TestNodeView_AddPackage(t *testing.T) {
	t.Parallel()

	t.Run("indexes every declaration kind from the supplied package", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))

		v := s.Nodes()
		if v.Packages().Len() != 1 {
			t.Fatalf("Packages = %d, want 1", v.Packages().Len())
		}
		if v.Files().Len() != 1 {
			t.Fatalf("Files = %d, want 1", v.Files().Len())
		}
		if v.Imports().Len() != 1 {
			t.Fatalf("Imports = %d, want 1", v.Imports().Len())
		}
		if v.Structs().Len() != 2 {
			t.Fatalf("Structs = %d, want 2", v.Structs().Len())
		}
		if v.Interfaces().Len() != 1 {
			t.Fatalf("Interfaces = %d, want 1", v.Interfaces().Len())
		}
		if v.Methods().Len() != 3 {
			t.Fatalf("Methods = %d, want 3 (User.Validate + Repo.Get + Repo.Save)", v.Methods().Len())
		}
		if v.Fields().Len() != 3 {
			t.Fatalf("Fields = %d, want 3 (User.ID + User.Email + Address.City)", v.Fields().Len())
		}
		if v.Functions().Len() != 1 {
			t.Fatalf("Functions = %d, want 1", v.Functions().Len())
		}
		if v.Variables().Len() != 1 {
			t.Fatalf("Variables = %d, want 1", v.Variables().Len())
		}
		if v.Constants().Len() != 1 {
			t.Fatalf("Constants = %d, want 1", v.Constants().Len())
		}
		if v.Enums().Len() != 1 {
			t.Fatalf("Enums = %d, want 1", v.Enums().Len())
		}
		if v.EnumVariants().Len() != 2 {
			t.Fatalf("EnumVariants = %d, want 2", v.EnumVariants().Len())
		}
		if v.Aliases().Len() != 1 {
			t.Fatalf("Aliases = %d, want 1", v.Aliases().Len())
		}
	})

	t.Run("looks up entries by qualified name", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))

		if _, ok := s.Nodes().Structs().ByQName("github.com/example/users.User"); !ok {
			t.Fatalf("Struct lookup by qname failed")
		}
		if _, ok := s.Nodes().Methods().ByQName("github.com/example/users.User.Validate"); !ok {
			t.Fatalf("Method lookup by qname failed")
		}
		if _, ok := s.Nodes().Fields().ByQName("github.com/example/users.User.Email"); !ok {
			t.Fatalf("Field lookup by qname failed")
		}
		if _, ok := s.Nodes().EnumVariants().ByQName("github.com/example/users.Status.Active"); !ok {
			t.Fatalf("EnumVariant lookup by qname failed")
		}
	})

	t.Run("returns ErrNilEntry for a nil package", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		if err := s.Nodes().AddPackage(nil); !errors.Is(err, store.ErrNilEntry) {
			t.Fatalf("AddPackage(nil) = %v, want ErrNilEntry", err)
		}
	})
}

func TestNodeView_AddPackage_DuplicateDetection(t *testing.T) {
	t.Parallel()

	t.Run("rejects two packages sharing the same path", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		err := s.Nodes().AddPackage(makeUserPackage())
		if !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("re-adding the same package should fail with ErrDuplicateQName; got %v", err)
		}
	})

	t.Run("rejects duplicate struct qnames within one package", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &node.Package{
			Name: "x",
			Path: "x",
			Structs: []*node.Struct{
				{Name: "A", Package: "x"},
				{Name: "A", Package: "x"},
			},
		}
		err := s.Nodes().AddPackage(dup)
		if !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("expected ErrDuplicateQName for duplicate struct; got %v", err)
		}
	})

	t.Run("rejects duplicate field qnames within one struct", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &node.Package{
			Name: "x",
			Path: "x",
			Structs: []*node.Struct{{
				Name:    "A",
				Package: "x",
				Fields: []*node.Field{
					{Name: "F"},
					{Name: "F"},
				},
			}},
		}
		err := s.Nodes().AddPackage(dup)
		if !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("expected ErrDuplicateQName for duplicate field; got %v", err)
		}
	})

	t.Run("rejects duplicate method qnames within one struct", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &node.Package{
			Name: "x",
			Path: "x",
			Structs: []*node.Struct{{
				Name:    "A",
				Package: "x",
				Methods: []*node.Method{
					{Name: "M"},
					{Name: "M"},
				},
			}},
		}
		err := s.Nodes().AddPackage(dup)
		if !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("expected ErrDuplicateQName for duplicate method; got %v", err)
		}
	})

	t.Run("rejects duplicate interface qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &node.Package{
			Name: "x", Path: "x",
			Interfaces: []*node.Interface{
				{Name: "I", Package: "x"},
				{Name: "I", Package: "x"},
			},
		}
		if err := s.Nodes().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate interface method qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &node.Package{
			Name: "x", Path: "x",
			Interfaces: []*node.Interface{{
				Name: "I", Package: "x",
				Methods: []*node.Method{{Name: "M"}, {Name: "M"}},
			}},
		}
		if err := s.Nodes().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate function qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &node.Package{
			Name: "x", Path: "x",
			Functions: []*node.Function{
				{Name: "F", Package: "x"},
				{Name: "F", Package: "x"},
			},
		}
		if err := s.Nodes().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate variable qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &node.Package{
			Name: "x", Path: "x",
			Variables: []*node.Variable{
				{Name: "V", Package: "x"},
				{Name: "V", Package: "x"},
			},
		}
		if err := s.Nodes().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate constant qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &node.Package{
			Name: "x", Path: "x",
			Constants: []*node.Constant{
				{Name: "C", Package: "x"},
				{Name: "C", Package: "x"},
			},
		}
		if err := s.Nodes().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate enum qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &node.Package{
			Name: "x", Path: "x",
			Enums: []*node.Enum{
				{Name: "E", Package: "x"},
				{Name: "E", Package: "x"},
			},
		}
		if err := s.Nodes().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate enum variant qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &node.Package{
			Name: "x", Path: "x",
			Enums: []*node.Enum{{
				Name: "E", Package: "x",
				Variants: []*node.EnumVariant{{Name: "V"}, {Name: "V"}},
			}},
		}
		if err := s.Nodes().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate alias qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &node.Package{
			Name: "x", Path: "x",
			Aliases: []*node.Alias{
				{Name: "A", Package: "x"},
				{Name: "A", Package: "x"},
			},
		}
		if err := s.Nodes().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate file paths", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &node.Package{
			Name: "x", Path: "x",
			Files: []*node.File{
				{Name: "a.go", Path: "x/a.go"},
				{Name: "a.go", Path: "x/a.go"},
			},
		}
		if err := s.Nodes().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})
}

func TestNodeView_AddPackage_FileImports(t *testing.T) {
	t.Parallel()

	t.Run("file-level imports dedup against the package import bucket", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		// File declares an import that is not in the Package.Imports
		// list — the file's import should still register through the
		// imports bucket.
		pkg := &node.Package{
			Name: "x", Path: "x",
			Files: []*node.File{{
				Name: "a.go", Path: "x/a.go",
				Imports: []*node.Import{{Path: "fmt"}},
			}},
			Imports: []*node.Import{{Path: "context"}},
		}
		assertNoError(t, s.Nodes().AddPackage(pkg))
		if s.Nodes().Imports().Len() != 2 {
			t.Fatalf("Imports = %d, want 2 (file fmt + package context)", s.Nodes().Imports().Len())
		}
	})

	t.Run("repeated file-level imports dedup silently", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		pkg := &node.Package{
			Name: "x", Path: "x",
			Files: []*node.File{
				{Name: "a.go", Path: "x/a.go", Imports: []*node.Import{{Path: "fmt"}}},
				{Name: "b.go", Path: "x/b.go", Imports: []*node.Import{{Path: "fmt"}}},
			},
		}
		assertNoError(t, s.Nodes().AddPackage(pkg))
		if s.Nodes().Imports().Len() != 1 {
			t.Fatalf("Imports = %d, want 1 (deduped)", s.Nodes().Imports().Len())
		}
	})
}

func TestNodeView_ByPackage(t *testing.T) {
	t.Parallel()

	t.Run("collects every recorded node under the package path", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		got := s.Nodes().ByPackage().Get("github.com/example/users")
		// Package + File + Import + 2 Structs + 3 Fields + 3 Methods (User.Validate,
		// Repo.Get, Repo.Save) + Interface + Function + Variable + Constant + Enum +
		// 2 EnumVariants + Alias = 19
		const want = 19
		if len(got) != want {
			t.Fatalf("ByPackage count = %d, want %d", len(got), want)
		}
	})

	t.Run("returns nil for unknown packages", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		if s.Nodes().ByPackage().Get("missing") != nil {
			t.Fatalf("ByPackage(unknown) should be nil")
		}
	})
}

func TestNodeView_ByMetaKey(t *testing.T) {
	t.Parallel()

	t.Run("captures keys set after AddPackage via the observer hook", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		// Post-ingest capture is what the observer provides, and the
		// observer is opt-in. Enabling has to precede AddPackage:
		// registration happens per node as it is ingested.
		s.Nodes().EnableMetaKeyIndex()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		user, _ := s.Nodes().Structs().ByQName("github.com/example/users.User")
		keyNodeViewMeta.Set(user.EnsureMeta(), true, "test")
		got := s.Nodes().ByMetaKey().Get(keyNodeViewMeta.Name())
		if len(got) != 1 || got[0] != node.Node(user) {
			t.Fatalf("ByMetaKey should record the post-add Set; got %+v", got)
		}
	})

	t.Run("captures keys already present when AddPackage runs", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		pre := makeUserPackage()
		// Pre-stamp metadata before adding the package.
		keyNodeViewMeta.Set(pre.Structs[0].EnsureMeta(), true, "pre-add")
		assertNoError(t, s.Nodes().AddPackage(pre))
		got := s.Nodes().ByMetaKey().Get(keyNodeViewMeta.Name())
		if len(got) != 1 {
			t.Fatalf("ByMetaKey should seed pre-existing keys; got %+v", got)
		}
	})

	t.Run("returns nil for unset keys", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		if s.Nodes().ByMetaKey().Get("never.set") != nil {
			t.Fatalf("ByMetaKey on unset key should be nil")
		}
	})
}

func TestNodeView_ByDirective(t *testing.T) {
	t.Parallel()

	t.Run("collects nodes carrying the named directive", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		got := s.Nodes().ByDirective().Get("repo")
		if len(got) != 1 {
			t.Fatalf("ByDirective(repo) = %d, want 1 (User struct)", len(got))
		}
	})

	t.Run("returns nil for directives no node carries", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		if s.Nodes().ByDirective().Get("missing") != nil {
			t.Fatalf("ByDirective(unknown) should be nil")
		}
	})
}

func TestNodeView_Freeze(t *testing.T) {
	t.Parallel()

	t.Run("Freeze marks the view immutable", func(t *testing.T) {
		t.Parallel()
		v := store.New().Nodes()
		if v.IsFrozen() {
			t.Fatalf("fresh view should not be frozen")
		}
		v.Freeze()
		if !v.IsFrozen() {
			t.Fatalf("Freeze should flip IsFrozen to true")
		}
	})

	t.Run("AddPackage after Freeze returns ErrFrozen", func(t *testing.T) {
		t.Parallel()
		v := store.New().Nodes()
		v.Freeze()
		err := v.AddPackage(&node.Package{Name: "x", Path: "x"})
		if !errors.Is(err, store.ErrFrozen) {
			t.Fatalf("AddPackage on frozen view should return ErrFrozen; got %v", err)
		}
	})

	t.Run("Freeze is idempotent", func(t *testing.T) {
		t.Parallel()
		v := store.New().Nodes()
		v.Freeze()
		v.Freeze()
		if !v.IsFrozen() {
			t.Fatalf("Freeze should remain set after repeat calls")
		}
	})
}

// TestNodeView_MetaKeyIndexIsOptIn covers the gate on the
// by-metadata-key observer.
//
// Registration is what costs: a closure and an observer slice per
// ingested node, a metadata bag for nodes that would otherwise never
// need one, and — since observers fire synchronously while the bag
// holds its write lock — an index-wide mutex acquisition on every
// later Set, during a phase the pipeline dispatches concurrently.
// Nothing in this repository reads the index.
func TestNodeView_MetaKeyIndexIsOptIn(t *testing.T) {
	t.Parallel()

	t.Run("keys present at ingest are recorded either way", func(t *testing.T) {
		t.Parallel()
		// The seed loop is unconditional. Only the observer is gated,
		// so a disabled index is incomplete rather than empty.
		s := store.New()
		pre := makeUserPackage()
		keyNodeViewMeta.Set(pre.Structs[0].EnsureMeta(), true, "pre-add")
		assertNoError(t, s.Nodes().AddPackage(pre))
		if got := s.Nodes().ByMetaKey().Get(keyNodeViewMeta.Name()); len(got) != 1 {
			t.Fatalf("ingest-time keys should be seeded regardless of the flag; got %+v", got)
		}
	})

	t.Run("keys set after ingest are dropped when disabled", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		user, _ := s.Nodes().Structs().ByQName("github.com/example/users.User")
		keyNodeViewMeta.Set(user.EnsureMeta(), true, "post-add")
		if got := s.Nodes().ByMetaKey().Get(keyNodeViewMeta.Name()); len(got) != 0 {
			t.Fatalf("disabled index recorded a post-ingest Set; got %+v", got)
		}
	})

	t.Run("ingest leaves no metadata bag on an unstamped node", func(t *testing.T) {
		t.Parallel()
		// The seed reads through the nil-tolerant accessor, so a node
		// carrying no metadata never acquires a bag — which is what
		// keeps an empty "meta" block out of its serialised form.
		s := store.New()
		assertNoError(t, s.Nodes().AddPackage(makeUserPackage()))
		user, _ := s.Nodes().Structs().ByQName("github.com/example/users.User")
		if user.Meta() != nil {
			t.Fatalf("ingest created a metadata bag on an unstamped node")
		}
	})
}

// TestNodeView_ByMetaKeyReportsWhenDisabled covers the signal that
// keeps the opt-in from being a silent contract change.
//
// ByMetaKey is exported and documented; an out-of-tree annotator that
// reads it gets an incomplete index rather than an error. Without a
// notice the only way to discover that is by diffing generated
// output, which is the worst way to learn it.
func TestNodeView_ByMetaKeyReportsWhenDisabled(t *testing.T) {
	t.Parallel()

	t.Run("a disabled read emits one Info", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		d := diag.New()
		s.SetDiag(d)
		_ = s.Nodes().ByMetaKey()
		if got := infoCount(d, "metadata-key index is disabled"); got != 1 {
			t.Fatalf("expected exactly one notice; got %d in %+v", got, d.Diagnostics())
		}
	})

	t.Run("repeated reads do not repeat the notice", func(t *testing.T) {
		t.Parallel()
		// An annotator consulting the index per node would otherwise
		// bury the run in identical lines.
		s := store.New()
		d := diag.New()
		s.SetDiag(d)
		for range 5 {
			_ = s.Nodes().ByMetaKey()
		}
		if got := infoCount(d, "metadata-key index is disabled"); got != 1 {
			t.Fatalf("expected the notice once; got %d", got)
		}
	})

	t.Run("an enabled index is silent", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		d := diag.New()
		s.SetDiag(d)
		s.Nodes().EnableMetaKeyIndex()
		_ = s.Nodes().ByMetaKey()
		if got := infoCount(d, "metadata-key index is disabled"); got != 0 {
			t.Fatalf("enabled index emitted %d notices", got)
		}
	})

	t.Run("no sink is not a panic", func(t *testing.T) {
		t.Parallel()
		// Library and test callers construct stores without one, and
		// they behaved fine before the sink existed.
		_ = store.New().Nodes().ByMetaKey()
	})
}

// infoCount returns how many Info diagnostics in d mention substr.
func infoCount(d *diag.Sink, substr string) int {
	n := 0
	for _, dg := range d.Diagnostics() {
		if dg.Severity == diag.Info && strings.Contains(dg.Message, substr) {
			n++
		}
	}
	return n
}

// TestNodeView_MetaKeyIndexRegistrationCost measures what the opt-in
// buys: the observer registration, which is the part that was
// unconditional and that nothing in this repository reads.
//
// Asserted as a comparison rather than an absolute, so it stays true
// as the rest of ingest changes — and as an allocation count rather
// than a duration, so it is host-independent.
//
//nolint:paralleltest // testing.AllocsPerRun panics in a parallel test.
func TestNodeView_MetaKeyIndexRegistrationCost(t *testing.T) {
	ingest := func(enable bool) float64 {
		return testing.AllocsPerRun(20, func() {
			s := store.New()
			if enable {
				s.Nodes().EnableMetaKeyIndex()
			}
			if err := s.Nodes().AddPackage(makeUserPackage()); err != nil {
				t.Fatalf("AddPackage: %v", err)
			}
		})
	}

	off, on := ingest(false), ingest(true)
	if off >= on {
		t.Fatalf("ingest allocated %v with the index off and %v with it on; "+
			"the observer registration should be the difference", off, on)
	}
	t.Logf("ingest allocations: index off %v, index on %v (delta %v)", off, on, on-off)
}
