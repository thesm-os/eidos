// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package gofixture_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
	"go.thesmos.sh/eidos/node"
)

// TestGoSource_TypeExpressions pins the spelling of every type
// reference variant the projection renders.
//
// Asserted through a projected field rather than against a renderer
// directly, because the field is where a wrong spelling does its
// damage: a support package whose field type reads `struct{…}`
// compiles nowhere, and the same text in a diagnostic is correct —
// which is exactly why [golang.TypeString] cannot be reused here.
func TestGoSource_TypeExpressions(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		ref  *node.TypeRef
		want string
	}{
		{
			name: "a builtin",
			ref:  gofixture.Named("string"),
			want: "X string",
		},
		{
			name: "a pointer",
			ref:  gofixture.Pointer(gofixture.Named("User")),
			want: "X *User",
		},
		{
			name: "a slice",
			ref:  gofixture.Slice(gofixture.Named("byte")),
			want: "X []byte",
		},
		{
			name: "a fixed-length array",
			ref:  gofixture.Array(gofixture.Named("byte"), 16),
			want: "X [16]byte",
		},
		{
			name: "a map",
			ref: gofixture.Map(gofixture.Named("string"),
				gofixture.Slice(gofixture.Pointer(gofixture.Named("User")))),
			want: "X map[string][]*User",
		},
		{
			name: "a cross-package reference",
			ref:  gofixture.PkgNamed("context", "Context"),
			want: "X context.Context",
		},
		{
			name: "a generic instantiation",
			ref: gofixture.WithArgs(gofixture.Named("List"),
				gofixture.Named("string")),
			want: "X List[string]",
		},
		{
			name: "a function type with no return",
			ref:  gofixture.Func([]*node.TypeRef{gofixture.Named("int")}, nil),
			want: "X func(int)",
		},
		{
			name: "a function type with one return",
			ref: gofixture.Func(nil,
				[]*node.TypeRef{gofixture.Named("error")}),
			want: "X func() error",
		},
		{
			name: "a function type with several returns",
			ref: gofixture.Func(
				[]*node.TypeRef{gofixture.Named("string")},
				[]*node.TypeRef{gofixture.Named("int"), gofixture.Named("error")},
			),
			want: "X func(string) (int, error)",
		},
		{
			name: "an empty anonymous struct",
			ref:  gofixture.AnonStruct(nil, nil),
			want: "X struct{}",
		},
		{
			name: "an anonymous struct with a tagged field",
			ref: gofixture.AnonStruct([]*node.Field{
				{Name: "Name", Type: gofixture.Named("string"), Tag: `json:"name"`},
			}, nil),
			want: "X struct { Name string `json:\"name\"` }",
		},
		{
			name: "an anonymous struct with an embed",
			ref: gofixture.AnonStruct(nil, []*node.Embed{
				{Type: gofixture.PkgNamed("sync", "Mutex")},
			}),
			want: "X struct{ sync.Mutex }",
		},
		{
			name: "an empty anonymous interface",
			ref:  gofixture.AnonInterface(nil, nil),
			want: "X any",
		},
		{
			name: "an anonymous interface with a method",
			ref: gofixture.AnonInterface([]*node.Method{{
				Name:    "Read",
				Params:  []*node.Param{{Name: "p", Type: gofixture.Slice(gofixture.Named("byte"))}},
				Returns: []*node.Return{{Type: gofixture.Named("int")}, {Type: gofixture.Named("error")}},
			}}, nil),
			want: "X interface{ Read(p []byte) (int, error) }",
		},
		{
			name: "a bidirectional channel",
			ref:  gofixture.Chan(gofixture.Named("int")),
			want: "X chan int",
		},
		{
			name: "a send-only channel",
			ref:  gofixture.SendChan(gofixture.Named("int")),
			want: "X chan<- int",
		},
		{
			name: "a receive-only channel",
			ref:  gofixture.RecvChan(gofixture.Named("int")),
			want: "X <-chan int",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Whitespace-collapsed: gofmt lays an anonymous struct out
			// across lines and aligns a field list's columns, neither of
			// which is what these cases are about.
			_, src := gofixture.New().
				Struct("S", func(s *gofixture.StructBuilder) {
					s.Field("X", tc.ref, nil)
				}).
				GoSource()
			if flat := strings.Join(strings.Fields(src), " "); !strings.Contains(flat, tc.want) {
				t.Errorf("projected field is missing %q:\n%s", tc.want, src)
			}
		})
	}
}

// TestGoSource_TypeParameters covers the generic-declaration halves
// no field type reaches: the bound on a declared parameter, and the
// argument list a generic type's own members refer to it by.
func TestGoSource_TypeParameters(t *testing.T) {
	t.Parallel()

	t.Run("an unconstrained parameter is bounded by any", func(t *testing.T) {
		t.Parallel()
		requireProjects(t, gofixture.New().
			Struct("Box", func(s *gofixture.StructBuilder) {
				s.TypeParam("T", nil)
			}), "type Box[T any] struct")
	})

	t.Run("a single bound renders as the bound itself", func(t *testing.T) {
		t.Parallel()
		requireProjects(t, gofixture.New().
			Struct("Box", func(s *gofixture.StructBuilder) {
				s.TypeParam("T", gofixture.Constraint(gofixture.Named("comparable")))
			}), "type Box[T comparable] struct")
	})

	t.Run("several bounds compose into an embedding interface", func(t *testing.T) {
		t.Parallel()
		// Embedding, not union: `A | B` admits either type set, while
		// `interface{ A; B }` admits only what satisfies both. A
		// constraint that compiles and admits the wrong types is the
		// worst of the two failures available here.
		requireProjects(t, gofixture.New().
			Struct("Box", func(s *gofixture.StructBuilder) {
				s.TypeParam("T", gofixture.Constraint(
					gofixture.Named("comparable"),
					gofixture.PkgNamed("fmt", "Stringer"),
				))
			}), "type Box[T interface { comparable fmt.Stringer }] struct")
	})

	t.Run("several parameters keep their declared order", func(t *testing.T) {
		t.Parallel()
		requireProjects(t, gofixture.New().
			Struct("Pair", func(s *gofixture.StructBuilder) {
				s.TypeParam("K", gofixture.Constraint(gofixture.Named("comparable")))
				s.TypeParam("V", nil)
			}), "type Pair[K comparable, V any] struct")
	})

	t.Run("a type-set bound renders the source form it was written in", func(t *testing.T) {
		t.Parallel()
		// Go's `~int | ~string` has no [node.Constraint.Embedded]
		// representation, so the structured field is empty and
		// IsAny reads the constraint as unbounded. Printing `any`
		// for it compiles and admits every type the author excluded
		// — the same failure mode as getting union and embedding
		// backwards, one level down.
		requireProjects(t, gofixture.New().
			Struct("Num", func(s *gofixture.StructBuilder) {
				s.TypeParam("T", gofixture.Bound("~int | ~string"))
			}), "type Num[T ~int | ~string] struct")
	})

	t.Run("a bound carrying both prefers the structured bounds", func(t *testing.T) {
		t.Parallel()
		// Embedded refs register their own imports; raw text cannot.
		// Where both are present the projection stays correct by
		// construction, which is the property it exists for.
		requireProjects(t, gofixture.New().
			Struct("Box", func(s *gofixture.StructBuilder) {
				s.TypeParam("T", gofixture.Bound("fmt.Stringer",
					gofixture.PkgNamed("fmt", "Stringer")))
			}), "type Box[T fmt.Stringer] struct")
	})
}

// TestGoSource_TypeRefusals covers the type-side stops. Each is a
// shape a hand-wired fixture can build and no Go expression spells.
func TestGoSource_TypeRefusals(t *testing.T) {
	t.Parallel()

	t.Run("a named reference with no name", func(t *testing.T) {
		t.Parallel()
		requireUnspellable(t, "named type reference with no name", "field X", func() {
			gofixture.New().Struct("S", func(s *gofixture.StructBuilder) {
				s.Field("X", gofixture.Named(""), nil)
			}).GoSource()
		})
	})

	t.Run("a type-parameter reference with no name", func(t *testing.T) {
		t.Parallel()
		requireUnspellable(t, "type-parameter reference with no name", "field X", func() {
			gofixture.New().Struct("S", func(s *gofixture.StructBuilder) {
				s.Field("X", gofixture.TypeParamRef(""), nil)
			}).GoSource()
		})
	})

	t.Run("a declared type parameter with no name", func(t *testing.T) {
		t.Parallel()
		requireUnspellable(t, "type parameter with no name", "struct S", func() {
			gofixture.New().Struct("S", func(s *gofixture.StructBuilder) {
				s.TypeParam("", nil)
			}).GoSource()
		})
	})

	t.Run("a channel with no element type", func(t *testing.T) {
		t.Parallel()
		// The model has no channel variant — the element rides on the
		// ref's first type argument — so a stamped ref carrying none
		// would otherwise render as a bare `chan`.
		ref := gofixture.Named("chan")
		golang.MetaIsChannel.Set(ref.EnsureMeta(), true, "test")
		requireUnspellable(t, "channel type reference with no element", "field X", func() {
			gofixture.New().Struct("S", func(s *gofixture.StructBuilder) {
				s.Field("X", ref, nil)
			}).GoSource()
		})
	})

	t.Run("an anonymous-struct field with no name", func(t *testing.T) {
		t.Parallel()
		requireUnspellable(t, "anonymous-struct field with no name", "field X", func() {
			gofixture.New().Struct("S", func(s *gofixture.StructBuilder) {
				s.Field("X", gofixture.AnonStruct([]*node.Field{
					{Type: gofixture.Named("string")},
				}, nil), nil)
			}).GoSource()
		})
	})

	t.Run("an import path with no package name", func(t *testing.T) {
		t.Parallel()
		requireUnspellable(t, "import path with no package name", "field X", func() {
			gofixture.New().Struct("S", func(s *gofixture.StructBuilder) {
				s.Field("X", gofixture.PkgNamed("/", "T"), nil)
			}).GoSource()
		})
	})
}

// requireProjects asserts that b's projection contains want, matched
// against the whitespace-collapsed source.
func requireProjects(t *testing.T, b *gofixture.Builder, want string) {
	t.Helper()
	_, src := b.GoSource()
	if flat := strings.Join(strings.Fields(src), " "); !strings.Contains(flat, want) {
		t.Errorf("projection is missing %q:\n%s", want, src)
	}
}
