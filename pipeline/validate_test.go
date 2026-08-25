// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/kind"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/sink"
	"go.thesmos.sh/eidos/store"
)

// What these pin used to be pinned in the Go frontend, which is where
// the walk used to live.
//
// Moving them here is the point of the change rather than a
// consequence of it: the frontend validated during conversion, so a
// warm cache skipped it; it spelled the call once per node kind, so a
// kind it did not name went unchecked; and the other two frontends in
// this repository never validated at all. A pipeline walk cannot do
// any of those, and these are the assertions that say so.

// at is a position, so a diagnostic points at something.
func at(line int) position.Pos { return position.At("a.go", line, 1) }

// dir builds a directive carrying name.
func dir(name directive.Name, line int) *directive.Directive {
	return &directive.Directive{Name: name, Pos: at(line)}
}

// runWith drives a pipeline over one synthetic package, returning the
// diagnostics the run recorded.
//
// A synthetic frontend rather than a real one: what is under test is
// the pipeline's walk, and driving Go source through a converter would
// make every assertion here depend on the converter agreeing about
// which node carries which directive.
func runWith(t *testing.T, pkg *node.Package, schemas ...directive.Schema) *diag.Sink {
	t.Helper()
	d := diag.New()
	b := pipeline.New().
		WithFrontend(&recFE{name: "fe", loadFn: func(s *store.Store) {
			if err := s.Nodes().AddPackage(pkg); err != nil {
				t.Fatalf("AddPackage: %v", err)
			}
		}}).
		WithBackend(&recBE{name: "be", lang: "stub"}).
		WithSink(sink.NewMemory()).
		WithDiag(d)
	for _, s := range schemas {
		b.WithDirective(s)
	}
	p, err := b.Build()
	assertNoError(t, err)
	_ = p.Run(t.Context(), "x")
	return d
}

// errorsFrom returns the run's error messages.
func errorsFrom(d *diag.Sink) []string {
	var out []string
	for _, dg := range d.Diagnostics() {
		if dg.Severity == diag.Error {
			out = append(out, dg.Message)
		}
	}
	return out
}

// A directive no schema claims produces exactly what a declaration
// with no directive produces, so nothing downstream can notice it.
func TestValidate_ReportsAnUnclaimedName(t *testing.T) {
	t.Parallel()

	t.Run("names a plausible near-miss", func(t *testing.T) {
		t.Parallel()
		d := runWith(t, &node.Package{
			Name: "x", Path: "x",
			BaseNode: node.BaseNode{DirectiveList: []*directive.Directive{dir("buildr", 1)}},
		}, directive.NewSchema("builder").Build())

		got := errorsFrom(d)
		if len(got) != 1 || !strings.Contains(got[0], `did you mean "builder"`) {
			t.Errorf("got %v, want one error naming the registered neighbour", got)
		}
	})

	t.Run("says so plainly when nothing is close", func(t *testing.T) {
		t.Parallel()
		d := runWith(t, &node.Package{
			Name: "x", Path: "x",
			BaseNode: node.BaseNode{DirectiveList: []*directive.Directive{dir("xyzzy", 1)}},
		}, directive.NewSchema("builder").Build())

		if got := errorsFrom(d); len(got) != 1 || strings.Contains(got[0], "did you mean") {
			t.Errorf("got %v; suggesting an unrelated name sends the author to "+
				"rename something they never wrote", got)
		}
	})

	t.Run("is silenced by the narrow-run option", func(t *testing.T) {
		t.Parallel()
		d := diag.New()
		p, err := pipeline.New().
			WithFrontend(&recFE{name: "fe", loadFn: func(s *store.Store) {
				_ = s.Nodes().AddPackage(&node.Package{
					Name: "x", Path: "x",
					BaseNode: node.BaseNode{
						DirectiveList: []*directive.Directive{dir("anything", 1)},
					},
				})
			}}).
			WithBackend(&recBE{name: "be", lang: "stub"}).
			WithSink(sink.NewMemory()).
			WithDiag(d).
			WithUnclaimedDirectives().
			Build()
		assertNoError(t, err)
		assertNoError(t, p.Run(t.Context(), "x"))
	})
}

// Every directive-bearing node is reached, whatever nests it.
//
// The frontend walk this replaced spelled one call per kind and named
// a struct's fields, embeds and methods, an interface's methods and
// embeds, and a function's params — but not a method's params, which
// went unvalidated with nothing saying so. The walk derives what to
// descend into from the node, so the omission is not expressible.
func TestValidate_ReachesEveryNestedNode(t *testing.T) {
	t.Parallel()

	// Scoped to packages alone, so any node the walk reaches carrying it
	// reports an applies-to violation — which is how each case below
	// proves it was reached at all.
	schema := directive.NewSchema("scoped").On(kind.Kind(node.KindPackage)).Build()

	cases := []struct {
		name string
		pkg  *node.Package
	}{
		{
			name: "a struct field",
			pkg: &node.Package{Name: "x", Path: "x", Structs: []*node.Struct{{
				Name: "S", Package: "x",
				Fields: []*node.Field{{
					Name:     "F",
					BaseNode: node.BaseNode{DirectiveList: []*directive.Directive{dir("scoped", 1)}},
				}},
			}}},
		},
		{
			name: "a struct embed",
			pkg: &node.Package{Name: "x", Path: "x", Structs: []*node.Struct{{
				Name: "S", Package: "x",
				Embeds: []*node.Embed{{
					BaseNode: node.BaseNode{DirectiveList: []*directive.Directive{dir("scoped", 1)}},
				}},
			}}},
		},
		{
			name: "an interface embed",
			pkg: &node.Package{Name: "x", Path: "x", Interfaces: []*node.Interface{{
				Name: "I", Package: "x",
				Embeds: []*node.Embed{{
					BaseNode: node.BaseNode{DirectiveList: []*directive.Directive{dir("scoped", 1)}},
				}},
			}}},
		},
		{
			name: "a function parameter",
			pkg: &node.Package{Name: "x", Path: "x", Functions: []*node.Function{{
				Name: "F", Package: "x",
				Params: []*node.Param{{
					Name:     "p",
					BaseNode: node.BaseNode{DirectiveList: []*directive.Directive{dir("scoped", 1)}},
				}},
			}}},
		},
		{
			// The one the per-kind walk missed. A method's params were
			// validated when the method hung off a function and not when
			// it hung off an interface, which is not a distinction any
			// author would predict.
			name: "a method parameter",
			pkg: &node.Package{Name: "x", Path: "x", Interfaces: []*node.Interface{{
				Name: "I", Package: "x",
				Methods: []*node.Method{{
					Name: "M",
					Params: []*node.Param{{
						Name:     "p",
						BaseNode: node.BaseNode{DirectiveList: []*directive.Directive{dir("scoped", 1)}},
					}},
				}},
			}}},
		},
		{
			name: "an enum variant",
			pkg: &node.Package{Name: "x", Path: "x", Enums: []*node.Enum{{
				Name: "E", Package: "x",
				Variants: []*node.EnumVariant{{
					Name:     "V",
					BaseNode: node.BaseNode{DirectiveList: []*directive.Directive{dir("scoped", 1)}},
				}},
			}}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name+" is validated", func(t *testing.T) {
			t.Parallel()
			got := errorsFrom(runWith(t, tc.pkg, schema))
			if len(got) == 0 {
				t.Errorf("nothing reported; the node was never handed to the validator")
			}
		})
	}
}

// A schema's positional default is folded in before anything reads the
// arguments.
//
// Validation runs inside the frontend phase for this reason. Folding is
// a side effect of validating, and the annotators are the first readers
// of the folded list — one phase later and the first of them sees the
// list the author wrote rather than the one the schema completes.
func TestValidate_FoldsDefaultsBeforeAnnotators(t *testing.T) {
	t.Parallel()

	d := dir("mode", 1)
	pkg := &node.Package{
		Name: "x", Path: "x",
		BaseNode: node.BaseNode{DirectiveList: []*directive.Directive{d}},
	}
	runWith(t, pkg, directive.NewSchema("mode").
		Positional("how", directive.Default("strict")).
		Build())

	if len(d.Args) != 1 || d.Args[0] != "strict" {
		t.Errorf("Args = %v, want the schema's default folded in", d.Args)
	}
}

// A self-referential type terminates.
//
// The walk descends into type references, and `type Node struct { Next
// *Node }` is ordinary source — so an unguarded traversal would not
// return on input nobody would call malformed.
func TestValidate_TerminatesOnASelfReferentialType(t *testing.T) {
	t.Parallel()

	self := &node.Struct{Name: "Node", Package: "x"}
	self.Fields = []*node.Field{{
		Name: "Next",
		Type: &node.TypeRef{TypeKind: node.TypeRefPointer, Elem: &node.TypeRef{
			TypeKind: node.TypeRefNamed, Name: "Node", Package: "x",
		}},
	}}
	runWith(t, &node.Package{Name: "x", Path: "x", Structs: []*node.Struct{self}})
}
