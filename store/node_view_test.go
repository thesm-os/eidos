// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store_test

import (
	"errors"
	"testing"

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
