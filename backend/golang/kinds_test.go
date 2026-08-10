// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"strings"
	"testing"

	backendgolang "go.thesmos.sh/eidos/backend/golang"
	"go.thesmos.sh/eidos/core/kind"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/eidostest/golangtest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/emit"
	emitbuilder "go.thesmos.sh/eidos/emit/builder"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
)

// unrenderable is a plugin-defined emit value whose kind no template
// resolves — the shape a plugin produces when its `define` name is
// misspelled or its template tree is rooted one directory too high.
type unrenderable struct {
	emit.BaseEmit

	name string
}

func (u *unrenderable) Kind() kind.Kind { return kind.Kind("kindsprobe." + u.name) }

// kindsGenerator queues one unrenderable value per configured name
// against the fixture's only interface.
type kindsGenerator struct{ kinds []string }

func (*kindsGenerator) Name() string { return "kindsprobe" }

// Outputs declares the suffix Layout composes a filename from.
// Without one the run stops before the backend sees the graph at all.
func (*kindsGenerator) Outputs(lang string) []plugin.Output {
	if lang != backendgolang.Language {
		return nil
	}
	return []plugin.Output{{Suffix: "_probe.gen.go"}}
}

func (g *kindsGenerator) Generate(ctx *plugin.GeneratorContext) error {
	prov := emitbuilder.For(g.Name())
	for _, iface := range ctx.Reader.Interfaces().Slice() {
		for _, k := range g.kinds {
			value := &unrenderable{name: k}
			value.BaseEmit = emitbuilder.Base(prov, iface)
			if err := emitbuilder.Queue(
				ctx.Store.Emit(), prov, "top", iface, value,
			); err != nil {
				return err
			}
		}
	}
	return nil
}

// fixturePkg is one annotated interface carrying a position, so
// Layout composes a filename rather than a bare suffix.
func fixturePkg() *node.Package {
	return storefixture.New().
		Package("cfg", "example.com/cfg").
		Interface("Store", func(i *storefixture.InterfaceBuilder) {
			i.Pos(position.At("cfg/store.go", 1, 1))
			i.Method("Get", nil)
		}).
		PackageNode()
}

// renderKinds drives the Go backend over a generator queueing the
// named kinds and returns the run's diagnostics.
func renderKinds(t *testing.T, kinds ...string) []string {
	t.Helper()
	p := golangtest.Driver(t, backendgolang.New(), fixturePkg(),
		&kindsGenerator{kinds: kinds}).
		Build().
		Run()
	diags := p.Diagnostics().Diagnostics()
	out := make([]string, 0, len(diags))
	for _, d := range diags {
		out = append(out, d.Message)
	}
	return out
}

func TestUnrenderableKinds(t *testing.T) {
	t.Parallel()

	t.Run("reports a plugin-defined kind with no template", func(t *testing.T) {
		t.Parallel()
		// The render site catches this too, but only after the run is
		// midway through a target — one kind, one file, then it stops.
		got := renderKinds(t, "double")
		if len(got) != 1 || !strings.Contains(got[0], `kindsprobe.double`) {
			t.Fatalf("diagnostics = %v", got)
		}
	})

	t.Run("names the plugin that has to fix it", func(t *testing.T) {
		t.Parallel()
		got := renderKinds(t, "double")
		if len(got) != 1 || !strings.Contains(got[0], `plugin "kindsprobe"`) {
			t.Fatalf("diagnostics = %v", got)
		}
	})

	t.Run("reports every missing kind at once", func(t *testing.T) {
		t.Parallel()
		// The whole point of checking before rendering: a plugin that
		// shipped no templates at all would otherwise surface one kind
		// per run, each fix revealing the next.
		got := renderKinds(t, "double", "suite", "options")
		if len(got) != 3 {
			t.Fatalf("diagnostics = %v, want one per kind", got)
		}
	})

	t.Run("orders the diagnostics deterministically", func(t *testing.T) {
		t.Parallel()
		// Collected through a map, and a golden may pin them.
		first := renderKinds(t, "double", "suite", "options")
		second := renderKinds(t, "double", "suite", "options")
		for i := range first {
			if first[i] != second[i] {
				t.Fatalf("diagnostic order is unstable:\n%v\n%v", first, second)
			}
		}
	})

	t.Run("reports each kind once however many values carry it", func(t *testing.T) {
		t.Parallel()
		// A plugin that forgot a template did so for every value of that
		// kind, and a diagnostic per declaration buries the one fact.
		if got := renderKinds(t, "double", "double"); len(got) != 1 {
			t.Fatalf("diagnostics = %v, want one per kind", got)
		}
	})

	t.Run("says what to check", func(t *testing.T) {
		t.Parallel()
		// A missing template resolves by the kind's string value, so the
		// two things worth checking are the define name and the tree's
		// root — neither of which the bare failure names.
		got := renderKinds(t, "double")
		for _, want := range []string{"define", "rooted"} {
			if len(got) != 1 || !strings.Contains(got[0], want) {
				t.Fatalf("diagnostic %q does not mention %q", got, want)
			}
		}
	})

	t.Run("leaves a run whose kinds all render alone", func(t *testing.T) {
		t.Parallel()
		// The core `emit.` namespace is rendered two ways — declaration
		// kinds by their own templates, expressions and statements by
		// dedicated funcmap helpers that have no template. Demanding one
		// per kind would report every statement in every body.
		if got := renderKinds(t); len(got) != 0 {
			t.Fatalf("diagnostics = %v, want none", got)
		}
	})
}
