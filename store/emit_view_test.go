// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package store_test

import (
	"errors"
	"sync"
	"testing"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/store"
)

// keyEmitViewMeta is the meta.Key used by the EmitView meta-key
// index test, declared once at package scope.
var keyEmitViewMeta = meta.NewKey("store.emit_view.meta", meta.BoolParser)

// makeUserEmitPackage builds a representative [emit.Package] with
// one file, two structs, an interface, function, variable, constant,
// enum, and alias — mirrored from the node-side fixture so emit
// tests exercise the same shape.
func makeUserEmitPackage() *emit.Package {
	target := emit.Target{Dir: "internal/users", Filename: "user_gen.go", Package: "users"}
	dir := directiveAt("repo", position.At("user.go", 1, 1))
	user := &emit.Struct{
		BaseEmit: emit.BaseEmit{DirectiveList: []*directive.Directive{dir}},
		Name:     "User",
		Package:  "users",
		Fields: []*emit.Field{
			{Name: "ID", Type: emit.Builtin("string")},
			{Name: "Email", Type: emit.Builtin("string")},
		},
		Methods: []*emit.Method{{Name: "Validate"}},
		Target:  target,
	}
	addr := &emit.Struct{
		Name: "Address", Package: "users",
		Fields: []*emit.Field{{Name: "City", Type: emit.Builtin("string")}},
		Target: target,
	}
	repo := &emit.Interface{
		Name: "Repo", Package: "users",
		Methods: []*emit.Method{{Name: "Get"}, {Name: "Save"}},
		Target:  target,
	}
	open := &emit.Function{Name: "Open", Package: "users", Target: target}
	def := &emit.Variable{Name: "Default", Package: "users", Target: target}
	pi := &emit.Constant{Name: "Pi", Package: "users", Target: target}
	status := &emit.Enum{
		Name: "Status", Package: "users",
		Variants: []*emit.EnumVariant{{Name: "Active"}, {Name: "Inactive"}},
		Target:   target,
	}
	id := &emit.Alias{Name: "ID", Package: "users", File: target}
	file := &emit.File{Name: "user_gen.go", Package: "users", Dir: "internal/users"}
	return &emit.Package{
		Name: "users", Path: "github.com/example/users", Dir: "internal/users",
		Files:      []*emit.File{file},
		Imports:    []*emit.Import{{Path: "context"}},
		Structs:    []*emit.Struct{user, addr},
		Interfaces: []*emit.Interface{repo},
		Functions:  []*emit.Function{open},
		Variables:  []*emit.Variable{def},
		Constants:  []*emit.Constant{pi},
		Enums:      []*emit.Enum{status},
		Aliases:    []*emit.Alias{id},
	}
}

func TestEmitView_AddPackage(t *testing.T) {
	t.Parallel()

	t.Run("indexes every emit kind from the supplied package", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Emit().AddPackage(makeUserEmitPackage()))

		v := s.Emit()
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
			t.Fatalf("Methods = %d, want 3", v.Methods().Len())
		}
		if v.Fields().Len() != 3 {
			t.Fatalf("Fields = %d, want 3", v.Fields().Len())
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
		assertNoError(t, s.Emit().AddPackage(makeUserEmitPackage()))

		if _, ok := s.Emit().Structs().ByQName("users.User"); !ok {
			t.Fatalf("Struct lookup by qname failed")
		}
		if _, ok := s.Emit().Methods().ByQName("users.User.Validate"); !ok {
			t.Fatalf("Method lookup by qname failed")
		}
		if _, ok := s.Emit().Files().ByQName("internal/users/user_gen.go"); !ok {
			t.Fatalf("File lookup by qname failed")
		}
	})

	t.Run("returns ErrNilEntry for a nil package", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		if err := s.Emit().AddPackage(nil); !errors.Is(err, store.ErrNilEntry) {
			t.Fatalf("AddPackage(nil) = %v, want ErrNilEntry", err)
		}
	})
}

func TestEmitView_AddPackage_DuplicateDetection(t *testing.T) {
	t.Parallel()

	t.Run("merges duplicate package paths into the existing emit.Package", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		// First contribution: one struct.
		first := &emit.Package{
			Name: "users", Path: "example.com/users",
			Structs: []*emit.Struct{{Name: "User", Package: "example.com/users"}},
		}
		assertNoError(t, s.Emit().AddPackage(first))
		// Second contribution targeting the same Path adds a sibling
		// struct rather than colliding on path.
		second := &emit.Package{
			Name: "users", Path: "example.com/users",
			Structs: []*emit.Struct{{Name: "Repo", Package: "example.com/users"}},
		}
		assertNoError(t, s.Emit().AddPackage(second))
		if pkgs := s.Emit().Packages(); pkgs.Len() != 1 {
			t.Fatalf("merged contribution should leave one emit.Package; got %d", pkgs.Len())
		}
		host, _ := s.Emit().Packages().ByQName("example.com/users")
		if host == nil || host != first {
			t.Fatalf("merge should preserve the first-AddPackage identity; got %p, want %p", host, first)
		}
		if len(host.Structs) != 2 {
			t.Fatalf("merged Structs slice should hold both contributions; got %d", len(host.Structs))
		}
	})

	t.Run("concurrent AddPackage with identical path resolves via the merge fast-path", func(t *testing.T) {
		t.Parallel()
		// EmitView.AddPackage serialises the check-then-create of the
		// packages bucket entry; concurrent callers targeting the same
		// Path either create the host (first arrival) or merge into
		// the existing host (every subsequent arrival). Race detector
		// flags any drift from this contract.
		s := store.New()
		const concurrency = 32
		errs := make(chan error, concurrency)
		var wg sync.WaitGroup
		for range concurrency {
			wg.Go(func() {
				errs <- s.Emit().AddPackage(&emit.Package{
					Name: "x", Path: "example.com/race",
				})
			})
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Errorf("concurrent AddPackage error: %v", err)
			}
		}
		if got := s.Emit().Packages().Len(); got != 1 {
			t.Fatalf("Packages.Len = %d, want 1 (merged via concurrent AddPackage)", got)
		}
	})

	t.Run("merge appends every per-kind slice into the existing emit.Package", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		// First contribution: a representative entity of every routable
		// kind plus a File and an Import. Second contribution: a
		// sibling of every kind targeting the same Package path so the
		// merge arm fires for each per-kind slice.
		first := &emit.Package{
			Name: "x", Path: "example.com/x",
			Files:      []*emit.File{{Name: "a.go", Dir: "x", Package: "x"}},
			Imports:    []*emit.Import{{Path: "io"}},
			Structs:    []*emit.Struct{{Name: "A", Package: "example.com/x"}},
			Interfaces: []*emit.Interface{{Name: "IA", Package: "example.com/x"}},
			Functions:  []*emit.Function{{Name: "FA", Package: "example.com/x"}},
			Variables:  []*emit.Variable{{Name: "VA", Package: "example.com/x"}},
			Constants:  []*emit.Constant{{Name: "CA", Package: "example.com/x"}},
			Enums:      []*emit.Enum{{Name: "EA", Package: "example.com/x"}},
			Aliases:    []*emit.Alias{{Name: "AA", Package: "example.com/x"}},
			Methods:    []*emit.Method{{Name: "MA"}},
		}
		assertNoError(t, s.Emit().AddPackage(first))
		second := &emit.Package{
			Name: "x", Path: "example.com/x",
			Files:      []*emit.File{{Name: "b.go", Dir: "x", Package: "x"}},
			Imports:    []*emit.Import{{Path: "fmt"}},
			Structs:    []*emit.Struct{{Name: "B", Package: "example.com/x"}},
			Interfaces: []*emit.Interface{{Name: "IB", Package: "example.com/x"}},
			Functions:  []*emit.Function{{Name: "FB", Package: "example.com/x"}},
			Variables:  []*emit.Variable{{Name: "VB", Package: "example.com/x"}},
			Constants:  []*emit.Constant{{Name: "CB", Package: "example.com/x"}},
			Enums:      []*emit.Enum{{Name: "EB", Package: "example.com/x"}},
			Aliases:    []*emit.Alias{{Name: "AB", Package: "example.com/x"}},
			Methods:    []*emit.Method{{Name: "MB"}},
		}
		assertNoError(t, s.Emit().AddPackage(second))
		host, _ := s.Emit().Packages().ByQName("example.com/x")
		cases := []struct {
			name string
			got  int
		}{
			{"Files", len(host.Files)},
			{"Imports", len(host.Imports)},
			{"Structs", len(host.Structs)},
			{"Interfaces", len(host.Interfaces)},
			{"Functions", len(host.Functions)},
			{"Variables", len(host.Variables)},
			{"Constants", len(host.Constants)},
			{"Enums", len(host.Enums)},
			{"Aliases", len(host.Aliases)},
			{"Methods", len(host.Methods)},
		}
		for _, tc := range cases {
			if tc.got != 2 {
				t.Errorf("%s after merge = %d, want 2 (both contributions)", tc.name, tc.got)
			}
		}
	})

	t.Run("merge preserves per-kind QName uniqueness", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		first := &emit.Package{
			Name: "x", Path: "x",
			Structs: []*emit.Struct{{Name: "A", Package: "x"}},
		}
		assertNoError(t, s.Emit().AddPackage(first))
		// Same Name + Package across two AddPackage calls — should
		// still surface ErrDuplicateQName from the per-kind bucket.
		dup := &emit.Package{
			Name: "x", Path: "x",
			Structs: []*emit.Struct{{Name: "A", Package: "x"}},
		}
		if err := s.Emit().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("merged AddPackage should still enforce per-kind QName uniqueness; got %v", err)
		}
	})

	t.Run("rejects duplicate file paths", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &emit.Package{
			Name: "x", Path: "x", Dir: "x",
			Files: []*emit.File{
				{Name: "a.go", Dir: "x", Package: "x"},
				{Name: "a.go", Dir: "x", Package: "x"},
			},
		}
		if err := s.Emit().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate struct qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &emit.Package{
			Name: "x", Path: "x",
			Structs: []*emit.Struct{
				{Name: "A", Package: "x"},
				{Name: "A", Package: "x"},
			},
		}
		if err := s.Emit().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate field qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &emit.Package{
			Name: "x", Path: "x",
			Structs: []*emit.Struct{{
				Name: "A", Package: "x",
				Fields: []*emit.Field{{Name: "F"}, {Name: "F"}},
			}},
		}
		if err := s.Emit().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate struct method qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &emit.Package{
			Name: "x", Path: "x",
			Structs: []*emit.Struct{{
				Name: "A", Package: "x",
				Methods: []*emit.Method{{Name: "M"}, {Name: "M"}},
			}},
		}
		if err := s.Emit().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate top-level method qnames", func(t *testing.T) {
		t.Parallel()
		// A method with no Owner composes its bucket qname from the
		// package path, so two same-named ownerless methods collide
		// on `<path>.<Name>` exactly as two methods on one struct do.
		s := store.New()
		dup := &emit.Package{
			Name: "x", Path: "x",
			Methods: []*emit.Method{{Name: "Ping"}, {Name: "Ping"}},
		}
		if err := s.Emit().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate interface qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &emit.Package{
			Name: "x", Path: "x",
			Interfaces: []*emit.Interface{
				{Name: "I", Package: "x"},
				{Name: "I", Package: "x"},
			},
		}
		if err := s.Emit().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate interface method qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &emit.Package{
			Name: "x", Path: "x",
			Interfaces: []*emit.Interface{{
				Name: "I", Package: "x",
				Methods: []*emit.Method{{Name: "M"}, {Name: "M"}},
			}},
		}
		if err := s.Emit().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate function qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &emit.Package{
			Name: "x", Path: "x",
			Functions: []*emit.Function{
				{Name: "F", Package: "x"},
				{Name: "F", Package: "x"},
			},
		}
		if err := s.Emit().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate variable qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &emit.Package{
			Name: "x", Path: "x",
			Variables: []*emit.Variable{
				{Name: "V", Package: "x"},
				{Name: "V", Package: "x"},
			},
		}
		if err := s.Emit().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate constant qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &emit.Package{
			Name: "x", Path: "x",
			Constants: []*emit.Constant{
				{Name: "C", Package: "x"},
				{Name: "C", Package: "x"},
			},
		}
		if err := s.Emit().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate enum qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &emit.Package{
			Name: "x", Path: "x",
			Enums: []*emit.Enum{
				{Name: "E", Package: "x"},
				{Name: "E", Package: "x"},
			},
		}
		if err := s.Emit().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate enum variant qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &emit.Package{
			Name: "x", Path: "x",
			Enums: []*emit.Enum{{
				Name: "E", Package: "x",
				Variants: []*emit.EnumVariant{{Name: "V"}, {Name: "V"}},
			}},
		}
		if err := s.Emit().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})

	t.Run("rejects duplicate alias qnames", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		dup := &emit.Package{
			Name: "x", Path: "x",
			Aliases: []*emit.Alias{
				{Name: "A", Package: "x"},
				{Name: "A", Package: "x"},
			},
		}
		if err := s.Emit().AddPackage(dup); !errors.Is(err, store.ErrDuplicateQName) {
			t.Fatalf("got %v, want ErrDuplicateQName", err)
		}
	})
}

func TestEmitView_AddPackage_FileImports(t *testing.T) {
	t.Parallel()

	t.Run("file-level imports register through the imports bucket", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		pkg := &emit.Package{
			Name: "x", Path: "x", Dir: "x",
			Files: []*emit.File{{
				Name: "a.go", Dir: "x", Package: "x",
				Imports: []*emit.Import{{Path: "fmt"}},
			}},
			Imports: []*emit.Import{{Path: "context"}},
		}
		assertNoError(t, s.Emit().AddPackage(pkg))
		if s.Emit().Imports().Len() != 2 {
			t.Fatalf("Imports = %d, want 2", s.Emit().Imports().Len())
		}
	})

	t.Run("repeated file-level imports dedup silently", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		pkg := &emit.Package{
			Name: "x", Path: "x", Dir: "x",
			Files: []*emit.File{
				{Name: "a.go", Dir: "x", Package: "x", Imports: []*emit.Import{{Path: "fmt"}}},
				{Name: "b.go", Dir: "x", Package: "x", Imports: []*emit.Import{{Path: "fmt"}}},
			},
		}
		assertNoError(t, s.Emit().AddPackage(pkg))
		if s.Emit().Imports().Len() != 1 {
			t.Fatalf("Imports = %d, want 1 (deduped)", s.Emit().Imports().Len())
		}
	})
}

func TestEmitView_ByPackage(t *testing.T) {
	t.Parallel()

	t.Run("collects every recorded entity under the package path", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Emit().AddPackage(makeUserEmitPackage()))
		got := s.Emit().ByPackage().Get("github.com/example/users")
		if len(got) == 0 {
			t.Fatalf("ByPackage should return non-empty for the recorded package")
		}
	})
}

func TestEmitView_ByDirective(t *testing.T) {
	t.Parallel()

	t.Run("collects entities carrying the named directive", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Emit().AddPackage(makeUserEmitPackage()))
		got := s.Emit().ByDirective().Get("repo")
		if len(got) != 1 {
			t.Fatalf("ByDirective(repo) = %d, want 1", len(got))
		}
	})
}

func TestEmitView_ByMetaKey(t *testing.T) {
	t.Parallel()

	t.Run("captures keys set after AddPackage via the observer hook", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		// See the node-side twin: the observer is opt-in and must be
		// enabled before ingest.
		s.Emit().EnableMetaKeyIndex()
		assertNoError(t, s.Emit().AddPackage(makeUserEmitPackage()))
		user, _ := s.Emit().Structs().ByQName("users.User")
		keyEmitViewMeta.Set(user.EnsureMeta(), true, "test")
		got := s.Emit().ByMetaKey().Get(keyEmitViewMeta.Name())
		if len(got) != 1 || got[0] != emit.Node(user) {
			t.Fatalf("ByMetaKey should record the post-add Set; got %+v", got)
		}
	})

	t.Run("captures keys already present when AddPackage runs", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		pre := makeUserEmitPackage()
		keyEmitViewMeta.Set(pre.Structs[0].EnsureMeta(), true, "pre-add")
		assertNoError(t, s.Emit().AddPackage(pre))
		got := s.Emit().ByMetaKey().Get(keyEmitViewMeta.Name())
		if len(got) != 1 {
			t.Fatalf("ByMetaKey should seed pre-existing keys; got %+v", got)
		}
	})
}

func TestEmitView_ByTarget(t *testing.T) {
	t.Parallel()

	t.Run("groups routable emit entities under their Target", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Emit().AddPackage(makeUserEmitPackage()))
		target := emit.Target{Dir: "internal/users", Filename: "user_gen.go", Package: "users"}
		got := s.Emit().ByTarget().Get(target)
		if len(got) == 0 {
			t.Fatalf("ByTarget should return entries for the routed Target")
		}
	})

	t.Run("returns nil for an unknown Target", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Emit().AddPackage(makeUserEmitPackage()))
		if s.Emit().ByTarget().Get(emit.Target{Dir: "missing"}) != nil {
			t.Fatalf("ByTarget for unknown Target should be nil")
		}
	})
}

func TestEmitView_RebuildByTarget(t *testing.T) {
	t.Parallel()

	t.Run("post-Router mutations to entity Targets surface through ByTarget", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Emit().AddPackage(makeUserEmitPackage()))

		original := emit.Target{Dir: "internal/users", Filename: "user_gen.go", Package: "users"}
		// Sanity check: ByTarget initially indexes under the
		// AddPackage-time target.
		if len(s.Emit().ByTarget().Get(original)) == 0 {
			t.Fatalf("expected entries for the original target before mutation")
		}

		// Simulate the pipeline router rewriting an entity's Target
		// — the alongside-source flow where the router fills Dir
		// after generation.
		rerouted := emit.Target{Dir: "alongside", Filename: "user_gen.go", Package: "users"}
		s.Emit().Structs().Range(func(st *emit.Struct) bool {
			st.Target = rerouted
			return true
		})
		s.Emit().Interfaces().Range(func(i *emit.Interface) bool {
			i.Target = rerouted
			return true
		})
		s.Emit().Functions().Range(func(f *emit.Function) bool {
			f.Target = rerouted
			return true
		})
		s.Emit().Variables().Range(func(v *emit.Variable) bool {
			v.Target = rerouted
			return true
		})
		s.Emit().Constants().Range(func(c *emit.Constant) bool {
			c.Target = rerouted
			return true
		})
		s.Emit().Enums().Range(func(e *emit.Enum) bool {
			e.Target = rerouted
			return true
		})
		s.Emit().Aliases().Range(func(a *emit.Alias) bool {
			a.File = rerouted
			return true
		})

		// Before RebuildByTarget: the index still reflects the
		// original targets because the index was populated at
		// AddPackage time.
		if len(s.Emit().ByTarget().Get(rerouted)) != 0 {
			t.Fatalf("ByTarget should not yet see the rerouted target before rebuild")
		}

		s.Emit().RebuildByTarget()

		// After rebuild: decls land under their current Target. The
		// original target retains only the [emit.File] entry —
		// Files keep their AddPackage-time Target unless an
		// explicit caller rewrites their Dir / Name / Package
		// fields too.
		if len(s.Emit().ByTarget().Get(rerouted)) == 0 {
			t.Fatalf("ByTarget should surface decls under the rerouted target after rebuild")
		}
		residual := s.Emit().ByTarget().Get(original)
		for _, n := range residual {
			if _, ok := n.(*emit.File); !ok {
				t.Fatalf("only File entries should remain at the original target; got %T", n)
			}
		}
	})

	t.Run("Files re-index against their own Target", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		// Create a File via FileFor so the file lives in the
		// internal files bucket; RebuildByTarget should re-add it
		// to byTarget under its current Target.
		target := emit.Target{Dir: "internal/users", Filename: "init.go", Package: "users"}
		_, err := s.Emit().FileFor(target)
		assertNoError(t, err)
		s.Emit().RebuildByTarget()
		if len(s.Emit().ByTarget().Get(target)) == 0 {
			t.Fatalf("ByTarget should include the File entry under its Target after rebuild")
		}
	})
}

func TestEmitView_Freeze(t *testing.T) {
	t.Parallel()

	t.Run("Freeze marks the view immutable", func(t *testing.T) {
		t.Parallel()
		v := store.New().Emit()
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
		v := store.New().Emit()
		v.Freeze()
		err := v.AddPackage(&emit.Package{Name: "x", Path: "x", Dir: "x"})
		if !errors.Is(err, store.ErrFrozen) {
			t.Fatalf("AddPackage on frozen view should return ErrFrozen; got %v", err)
		}
	})
}

func TestEmitView_FileFor(t *testing.T) {
	t.Parallel()

	t.Run("creates a fresh emit.File for an unknown Target", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		target := emit.Target{Dir: "out", Filename: "x.go", Package: "x"}
		f, err := s.Emit().FileFor(target)
		assertNoError(t, err)
		if f == nil || f.Name != "x.go" || f.Dir != "out" || f.Package != "x" {
			t.Fatalf("FileFor mismatch: %+v", f)
		}
	})

	t.Run("returns the same emit.File on repeat lookups (single per Target)", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		target := emit.Target{Dir: "out", Filename: "x.go", Package: "x"}
		first, err := s.Emit().FileFor(target)
		assertNoError(t, err)
		second, err := s.Emit().FileFor(target)
		assertNoError(t, err)
		if first != second {
			t.Fatalf("FileFor should return the same instance on repeat lookups")
		}
	})

	t.Run("returns ErrFrozen when creating after Freeze with no prior File", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		s.Emit().Freeze()
		target := emit.Target{Dir: "out", Filename: "x.go", Package: "x"}
		_, err := s.Emit().FileFor(target)
		if !errors.Is(err, store.ErrFrozen) {
			t.Fatalf("FileFor on frozen view should return ErrFrozen; got %v", err)
		}
	})

	t.Run("lookups for already-present Files succeed after Freeze", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		target := emit.Target{Dir: "out", Filename: "x.go", Package: "x"}
		first, err := s.Emit().FileFor(target)
		assertNoError(t, err)
		s.Emit().Freeze()
		second, err := s.Emit().FileFor(target)
		assertNoError(t, err)
		if first != second {
			t.Fatalf("post-freeze lookup should return the existing File")
		}
	})
}

// TestEmitView_AppendOriginSlot covers the origin-anchored slot
// primitive: synchronous validation rejects nil / empty inputs,
// valid contributions queue in registration order, snapshots are
// detached from the live list, and post-freeze appends are
// rejected.
func TestEmitView_AppendOriginSlot(t *testing.T) {
	t.Parallel()

	t.Run("nil origin returns ErrNilEntry without queueing", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		err := s.Emit().AppendOriginSlot(nil, "init", &emit.Stmt{}, emit.Provenance{})
		if !errors.Is(err, store.ErrNilEntry) {
			t.Fatalf("got %v, want ErrNilEntry", err)
		}
		if got := s.Emit().PendingOriginSlots(); len(got) != 0 {
			t.Fatalf("validation failure must not queue; got %d entries", len(got))
		}
	})

	t.Run("nil item returns ErrNilEntry without queueing", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		err := s.Emit().AppendOriginSlot(&node.Struct{Name: "Probe"}, "init", nil, emit.Provenance{})
		if !errors.Is(err, store.ErrNilEntry) {
			t.Fatalf("got %v, want ErrNilEntry", err)
		}
		if got := s.Emit().PendingOriginSlots(); len(got) != 0 {
			t.Fatalf("validation failure must not queue; got %d entries", len(got))
		}
	})

	t.Run("empty slot name returns ErrUnknownSlotName without queueing", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		err := s.Emit().AppendOriginSlot(
			&node.Struct{Name: "Probe"}, "", &emit.Stmt{}, emit.Provenance{},
		)
		if !errors.Is(err, store.ErrUnknownSlotName) {
			t.Fatalf("got %v, want ErrUnknownSlotName", err)
		}
		if got := s.Emit().PendingOriginSlots(); len(got) != 0 {
			t.Fatalf("validation failure must not queue; got %d entries", len(got))
		}
	})

	t.Run("documented File slots and plugin-defined names are both accepted", func(t *testing.T) {
		t.Parallel()
		// The framework's File.Slot(name) accepts any name; the
		// origin-anchored primitive matches that contract.
		for _, name := range []string{"top", "bottom", "init", "imports", "registry"} {
			s := store.New()
			err := s.Emit().AppendOriginSlot(
				&node.Struct{Name: "Probe"}, name, &emit.Stmt{}, emit.Provenance{SetBy: "stub"},
			)
			if err != nil {
				t.Fatalf("slot %q should be accepted; got %v", name, err)
			}
		}
	})

	t.Run("registrations queue in FIFO order", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		origin := &node.Struct{Name: "Probe"}
		assertNoError(t, s.Emit().AppendOriginSlot(origin, "init", &emit.Stmt{}, emit.Provenance{SetBy: "first"}))
		assertNoError(t, s.Emit().AppendOriginSlot(origin, "init", &emit.Stmt{}, emit.Provenance{SetBy: "second"}))
		entries := s.Emit().PendingOriginSlots()
		if len(entries) != 2 {
			t.Fatalf("expected 2 queued contributions; got %d", len(entries))
		}
		if entries[0].Prov.SetBy != "first" || entries[1].Prov.SetBy != "second" {
			t.Fatalf("registration order not preserved: %+v", entries)
		}
	})

	t.Run("PendingOriginSlots returns a snapshot detached from later appends", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		assertNoError(t, s.Emit().AppendOriginSlot(
			&node.Struct{Name: "Probe"}, "init", &emit.Stmt{}, emit.Provenance{SetBy: "first"},
		))
		snapshot := s.Emit().PendingOriginSlots()
		assertNoError(t, s.Emit().AppendOriginSlot(
			&node.Struct{Name: "Probe2"}, "init", &emit.Stmt{}, emit.Provenance{SetBy: "second"},
		))
		if len(snapshot) != 1 {
			t.Fatalf("snapshot should be unaffected by later appends; got %d entries", len(snapshot))
		}
	})

	t.Run("frozen view rejects further contributions with ErrFrozen", func(t *testing.T) {
		t.Parallel()
		s := store.New()
		s.Emit().Freeze()
		err := s.Emit().AppendOriginSlot(
			&node.Struct{Name: "Probe"}, "init", &emit.Stmt{}, emit.Provenance{},
		)
		if !errors.Is(err, store.ErrFrozen) {
			t.Fatalf("got %v, want ErrFrozen", err)
		}
	})
}
