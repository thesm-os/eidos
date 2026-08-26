// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/store"
)

func TestRenderBodySeparation(t *testing.T) {
	t.Parallel()

	t.Run("declarations are separated by one blank line", func(t *testing.T) {
		t.Parallel()
		s, err := newRenderState(New().tmpl, emit.Target{ImportPath: "./out"}, nil)
		if err != nil {
			t.Fatalf("newRenderState: %v", err)
		}

		body, err := s.renderBody([]emit.Node{
			&emit.Interface{Name: "A", Fields: []*emit.Field{{Name: "x", Type: emit.Builtin("string")}}},
			&emit.Interface{Name: "B", Fields: []*emit.Field{{Name: "y", Type: emit.Builtin("string")}}},
		})
		if err != nil {
			t.Fatalf("renderBody: %v", err)
		}
		if !strings.Contains(body, "}\n\nexport interface B") {
			t.Fatalf("declarations not separated:\n%s", body)
		}
	})

	t.Run("an unrenderable entity is skipped rather than failing the file", func(t *testing.T) {
		t.Parallel()
		// A sub-element reaching the loop is not an error — it simply
		// has no top-level rendering.
		s, err := newRenderState(New().tmpl, emit.Target{}, nil)
		if err != nil {
			t.Fatalf("newRenderState: %v", err)
		}
		body, err := s.renderBody([]emit.Node{&emit.Method{Name: "m"}})
		if err != nil {
			t.Fatalf("renderBody: %v", err)
		}
		if body != "" {
			t.Fatalf("renderBody = %q, want nothing", body)
		}
	})

	t.Run("the import block precedes the body", func(t *testing.T) {
		t.Parallel()
		// Which modules a file imports is not known until every type
		// in it has been spelled, so the block is assembled last and
		// prepended.
		s, err := newRenderState(New().tmpl, emit.Target{ImportPath: "./out"}, nil)
		if err != nil {
			t.Fatalf("newRenderState: %v", err)
		}
		body, err := s.renderBody([]emit.Node{&emit.Interface{
			Name:   "A",
			Fields: []*emit.Field{{Name: "p", Type: emit.External("./person", "Person")}},
		}})
		if err != nil {
			t.Fatalf("renderBody: %v", err)
		}
		if !strings.HasPrefix(body, "import type { Person } from './person';") {
			t.Fatalf("imports not first:\n%s", body)
		}
	})
}

func TestRenderFileHelpers(t *testing.T) {
	t.Parallel()

	t.Run("every declaration kind reaches the render set", func(t *testing.T) {
		t.Parallel()
		// A kind the collector misses renders to nothing without a
		// diagnostic, because there is no failure — the entity simply
		// never arrives.
		tgt := emit.Target{Dir: "out", Filename: "a.ts", ImportPath: "./out/a"}
		st := store.New()
		err := st.Emit().AddPackage(&emit.Package{
			Name: "out", Path: "./out",
			Structs:    []*emit.Struct{{Name: "S", Target: tgt}},
			Interfaces: []*emit.Interface{{Name: "I", Target: tgt}},
			Enums:      []*emit.Enum{{Name: "E", Target: tgt}},
			Aliases:    []*emit.Alias{{Name: "A", File: tgt, Target: emit.Builtin("string")}},
			Functions:  []*emit.Function{{Name: "F", Target: tgt}},
			Variables:  []*emit.Variable{{Name: "V", Target: tgt, Type: emit.Builtin("string")}},
			Constants:  []*emit.Constant{{Name: "K", Target: tgt, Type: emit.Builtin("string")}},
		})
		if err != nil {
			t.Fatalf("seed: %v", err)
		}

		if got := len(renderableEntities(st)); got != 7 {
			t.Fatalf("collected %d entities, want one of each kind", got)
		}
	})

	t.Run("every declaration kind reports its own output file", func(t *testing.T) {
		t.Parallel()
		// An alias names its file in File rather than Target — its
		// Target field is the type it aliases — so the lookup is
		// per-kind rather than through one accessor.
		tgt := emit.Target{ImportPath: "./m"}
		for name, n := range map[string]emit.Node{
			"struct":    &emit.Struct{Name: "S", Target: tgt},
			"interface": &emit.Interface{Name: "I", Target: tgt},
			"enum":      &emit.Enum{Name: "E", Target: tgt},
			"alias":     &emit.Alias{Name: "A", File: tgt},
			"function":  &emit.Function{Name: "F", Target: tgt},
			"variable":  &emit.Variable{Name: "V", Target: tgt},
			"constant":  &emit.Constant{Name: "K", Target: tgt},
		} {
			if got := targetOf(n); got.ImportPath != "./m" {
				t.Errorf("%s: targetOf = %+v", name, got)
			}
		}
	})

	t.Run("a kind that belongs to no file reports the zero target", func(t *testing.T) {
		t.Parallel()
		// A sub-element never reaches the grouping in practice, so it
		// buckets under the zero target rather than being an error.
		if got := targetOf(&emit.Method{Name: "m"}); got != (emit.Target{}) {
			t.Fatalf("targetOf = %+v, want the zero target", got)
		}
	})

	t.Run("targets sort so the sink sees one order", func(t *testing.T) {
		t.Parallel()
		st := store.New()
		err := st.Emit().AddPackage(&emit.Package{
			Name: "out", Path: "./out",
			Interfaces: []*emit.Interface{
				{Name: "Z", Target: emit.Target{Dir: "out", Filename: "z.ts"}},
				{Name: "A", Target: emit.Target{Dir: "out", Filename: "a.ts"}},
			},
		})
		if err != nil {
			t.Fatalf("seed: %v", err)
		}

		groups := groupByTarget(st)
		if len(groups) != 2 {
			t.Fatalf("groups = %d, want 2", len(groups))
		}
		if groups[0].target.Filename != "a.ts" {
			t.Fatalf("targets not sorted: %s before %s",
				groups[0].target.Filename, groups[1].target.Filename)
		}
	})

	t.Run("the sort key is the qualified name where a kind has one", func(t *testing.T) {
		t.Parallel()
		if got := qualifiedName(&emit.Interface{Name: "I", Package: "p"}); got != "p.I" {
			t.Fatalf("qualifiedName = %q, want p.I", got)
		}
		if got := qualifiedName(&emit.Interface{Name: "I"}); got != "I" {
			t.Fatalf("qualifiedName = %q, want I", got)
		}
	})

	t.Run("a kind carrying no qualified name sorts under the empty key", func(t *testing.T) {
		t.Parallel()
		// Not a crash: the entity sorts first and renders whatever its
		// template makes of it. An embed is a sub-element and never
		// reaches the sort in practice, which is why it has no name to
		// sort by.
		if got := qualifiedName(&emit.Embed{}); got != "" {
			t.Fatalf("qualifiedName = %q, want empty", got)
		}
	})
}
