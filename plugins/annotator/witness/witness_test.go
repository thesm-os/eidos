// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package witness_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
	witnessplugin "go.thesmos.sh/eidos/plugins/annotator/witness"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// TestConformance runs the framework's annotator suites.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, witnessplugin.New())
	})

	t.Run("annotator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunAnnotatorSuite(t, witnessplugin.New(), []plugintest.AnnotatorFixture{
			{
				Name: "empty store",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					return gofixture.New().Build()
				},
			},
			{
				Name: "a parameterised declaration naming its witness",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					b := gofixture.New().Package("blog", "example.com/blog")
					b.Struct("Bag", func(s *gofixture.StructBuilder) {
						s.Directive(gofixture.Directive("witness", gofixture.KV("T", "int")))
						s.TypeParam("T", gofixture.Bound("constraints.Ordered"))
					})
					s := b.Build()
					return s
				},
			},
		})
	})
}

// annotated runs the plugin over one built package and returns the
// store it stamped and whatever it reported.
func annotated(t *testing.T, fn func(*gofixture.Builder)) (*sdk.Store, *diag.Sink) {
	t.Helper()
	b := gofixture.New().Package("blog", "example.com/blog")
	fn(b)
	s := b.Build()
	d := diag.Capture()
	if err := witnessplugin.New().Annotate(&sdk.AnnotatorContext{
		Store: s, Reader: store.NewReader(s), Diag: d,
	}); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	return s, d
}

// witnessOn returns the reference stamped on a named parameter.
func witnessOn(t *testing.T, s *sdk.Store, decl, param string) (sdk.Ref, bool) {
	t.Helper()
	found, ok := store.NewReader(s).Structs().
		Where(func(x *sdk.Struct) bool { return x.Name == decl }).First()
	if !ok {
		t.Fatalf("fixture lost its %q", decl)
	}
	for _, p := range found.TypeParams {
		if p.Name == param {
			return sdk.AuthoredWitnessRef(p.Meta())
		}
	}
	t.Fatalf("%s declares no %q", decl, param)
	return nil, false
}

// bounded builds a declaration whose constraint admits no derived
// witness, which is the only case this plugin exists for.
func bounded(b *gofixture.Builder, opts ...gofixture.DirectiveOption) {
	b.Struct("Bag", func(s *gofixture.StructBuilder) {
		if len(opts) > 0 {
			s.Directive(gofixture.Directive("witness", opts...))
		}
		s.TypeParam("T", gofixture.Bound("constraints.Ordered"))
		s.TypeParam("U", gofixture.Bound("constraints.Ordered"))
	})
}

// TestAnnotateReachesEveryDeclaredKind pins the walk against the
// schema.
//
// A kind the schema admits and the walk skips is the quietest failure
// this plugin has: the directive validates, nothing is stamped, no
// diagnostic is raised, and [sdk.SourceRules.Witnesses] falls back to
// derivation — which answers nil for exactly the constraint an
// authored witness exists to serve. The author's line is discarded and
// the run stays green.
//
// Interfaces shipped broken for that reason: every fixture exercised a
// struct, and a generator that doubles a contract sees interfaces
// exclusively.
func TestAnnotateReachesEveryDeclaredKind(t *testing.T) {
	t.Parallel()

	// The constraint matters: derivation answers for `any` and
	// `comparable`, so a fixture bounded by either passes whether or
	// not the stamp was ever written.
	named := func() *sdk.Constraint { return gofixture.Bound("constraints.Ordered") }
	witness := gofixture.Directive("witness", gofixture.KV("T", "int"))

	kinds := []struct {
		name   string
		build  func(*gofixture.Builder)
		params func(*sdk.Store) []*sdk.TypeParam
	}{
		{
			name: "a struct",
			build: func(b *gofixture.Builder) {
				b.Struct("Bag", func(x *gofixture.StructBuilder) {
					x.Directive(witness)
					x.TypeParam("T", named())
				})
			},
			params: func(s *sdk.Store) []*sdk.TypeParam {
				d, _ := store.NewReader(s).Structs().First()
				return d.TypeParams
			},
		},
		{
			name: "an interface",
			build: func(b *gofixture.Builder) {
				b.Interface("Store", func(x *gofixture.InterfaceBuilder) {
					x.Directive(witness)
					x.TypeParam("T", named())
				})
			},
			params: func(s *sdk.Store) []*sdk.TypeParam {
				d, _ := store.NewReader(s).Interfaces().First()
				return d.TypeParams
			},
		},
		{
			name: "an alias",
			build: func(b *gofixture.Builder) {
				b.Alias("Pair", func(x *gofixture.AliasBuilder) {
					x.Directive(witness)
					x.TypeParam("T", named())
				})
			},
			params: func(s *sdk.Store) []*sdk.TypeParam {
				d, _ := store.NewReader(s).Aliases().First()
				return d.TypeParams
			},
		},
	}

	for _, k := range kinds {
		t.Run("stamps "+k.name, func(t *testing.T) {
			t.Parallel()
			s, d := annotated(t, k.build)
			if len(d.Diagnostics()) != 0 {
				t.Fatalf("reported %v over a well-formed declaration", d.Diagnostics())
			}
			params := k.params(s)
			if len(params) != 1 {
				t.Fatalf("fixture lost its type parameter")
			}
			ref, ok := sdk.AuthoredWitnessRef(params[0].Meta())
			if !ok {
				t.Fatalf("%s naming its witness was not stamped", k.name)
			}
			b, builtin := ref.(*sdk.BuiltinRef)
			if !builtin || b.Name != "int" {
				t.Errorf("stamped %#v, want the int the directive named", ref)
			}
		})
	}
}

func TestAnnotate(t *testing.T) {
	t.Parallel()

	t.Run("stamps a builtin witness with no package", func(t *testing.T) {
		t.Parallel()
		// The distinction from an authored sample, which always names a
		// function and so always has a package: qualifying `int` would
		// render an import nothing can resolve.
		s, d := annotated(t, func(b *gofixture.Builder) {
			bounded(b, gofixture.KV("T", "int"))
		})
		requireNoErrors(t, d)
		ref, ok := witnessOn(t, s, "Bag", "T")
		if !ok {
			t.Fatal("no witness stamped for T")
		}
		if _, qualified := ref.(*sdk.ExternalRef); qualified {
			t.Errorf("a builtin witness was stamped with a package: %#v", ref)
		}
	})

	t.Run("stamps a qualified witness with its package", func(t *testing.T) {
		t.Parallel()
		// The half that decides whether the rendered file imports
		// anything: a witness is written into an instantiation, and one
		// spelled bare would name a package the file never imported.
		s, d := annotated(t, func(b *gofixture.Builder) {
			// The qualifier resolves against the imports of the file
			// that declared the parameter, which is the same rule an
			// authored sample's name follows.
			b.File("bag.go", func(f *gofixture.FileBuilder) { f.Import("time") })
			bounded(b, gofixture.KV("T", "time.Duration"))
		})
		requireNoErrors(t, d)
		ref, ok := witnessOn(t, s, "Bag", "T")
		if !ok {
			t.Fatal("no witness stamped for T")
		}
		ext, qualified := ref.(*sdk.ExternalRef)
		if !qualified || ext.Package == "" {
			t.Errorf("a qualified witness lost its package: %#v", ref)
		}
	})

	t.Run("leaves an unnamed parameter unstamped", func(t *testing.T) {
		t.Parallel()
		// Naming one of two is not an error: a declaration may mix a
		// parameter the language can derive with one it cannot. What it
		// costs is reported by whoever asks for the list.
		s, _ := annotated(t, func(b *gofixture.Builder) {
			bounded(b, gofixture.KV("T", "int"))
		})
		if _, ok := witnessOn(t, s, "Bag", "U"); ok {
			t.Error("U was stamped though nothing named it")
		}
	})

	t.Run("reports a key naming no parameter", func(t *testing.T) {
		t.Parallel()
		// The likeliest way to write this wrongly, and otherwise
		// silent: the parameter keeps whatever it had, which for this
		// constraint is nothing.
		_, d := annotated(t, func(b *gofixture.Builder) {
			bounded(b, gofixture.KV("K", "int"))
		})
		requireMentions(t, d, `names "K"`)
	})

	t.Run("reports a directive naming nothing", func(t *testing.T) {
		t.Parallel()
		_, d := annotated(t, func(b *gofixture.Builder) {
			b.Struct("Bag", func(s *gofixture.StructBuilder) {
				s.Directive(gofixture.Directive("witness"))
				s.TypeParam("T", gofixture.Bound("constraints.Ordered"))
			})
		})
		requireMentions(t, d, "names no witness")
	})

	t.Run("reports a directive on a declaration with no parameters", func(t *testing.T) {
		t.Parallel()
		_, d := annotated(t, func(b *gofixture.Builder) {
			b.Struct("Plain", func(s *gofixture.StructBuilder) {
				s.Directive(gofixture.Directive("witness", gofixture.KV("T", "int")))
			})
		})
		requireMentions(t, d, "declares no type parameter")
	})
}

// requireNoErrors fails when the run reported anything at error
// severity.
func requireNoErrors(t *testing.T, d *diag.Sink) {
	t.Helper()
	if d.HasErrors() {
		t.Fatalf("the run reported errors: %v", d.Diagnostics())
	}
}

// requireMentions fails unless some reported diagnostic names want.
func requireMentions(t *testing.T, d *diag.Sink, want string) {
	t.Helper()
	for _, got := range d.Diagnostics() {
		if strings.Contains(got.Message, want) {
			return
		}
	}
	t.Fatalf("no diagnostic mentions %q; got %v", want, d.Diagnostics())
}
