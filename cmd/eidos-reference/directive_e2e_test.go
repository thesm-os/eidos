// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package main

import (
	"path/filepath"
	"runtime"
	"slices"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/opt"
	frontendgolang "go.thesmos.sh/eidos/frontend/golang"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins"
	"go.thesmos.sh/eidos/store"
)

// TestMixinDirective_EndToEnd joins the links the per-layer suites
// cover separately: a real `//+gen:mixin a b c` comment in Go
// source, through the Go frontend's parse and directive
// attachment, into the shape annotator's stamping pass.
//
// It lives here rather than beside the plugin because depguard
// forbids plugins/** from importing a specific frontend ("read
// facts via meta keys"), and eidostest cannot host it either —
// plugins already requires eidostest, so that edge would be an
// import cycle. This module is the one that wires every layer
// together, which is exactly what the test exercises.
func TestMixinDirective_EndToEnd(t *testing.T) {
	t.Parallel()

	methods := loadFixtureMethods(t)

	t.Run("a batched directive stamps every name in written order", func(t *testing.T) {
		t.Parallel()
		got := shape.Mixins(methodNamed(t, methods, "Put").Meta())
		want := []string{"idempotent", "concurrent", "atomic"}
		if !slices.Equal(got, want) {
			t.Fatalf("Put mixins = %v, want %v", got, want)
		}
	})

	t.Run("a single name still carries its parameter", func(t *testing.T) {
		t.Parallel()
		list := methodNamed(t, methods, "List")
		if got := shape.Mixins(list.Meta()); !slices.Equal(got, []string{"bounded"}) {
			t.Fatalf("List mixins = %v, want [bounded]", got)
		}
		if got, _ := shape.MixinParamKey("bounded", "limit").Get(list.Meta()); got != "100" {
			t.Errorf("bounded limit = %q, want 100", got)
		}
	})
}

// loadFixtureMethods parses testdata/mixinsrc through the Go
// frontend and runs the shape annotator over the result, returning
// the interface methods the fixture declares.
func loadFixtureMethods(t *testing.T) []*node.Method {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("runtime.Caller failed")
	}
	srcDir := filepath.Join(filepath.Dir(thisFile), "testdata", "mixinsrc")

	parser, err := directive.NewParser("gen")
	if err != nil {
		t.Fatalf("NewParser: %v", err)
	}
	front := frontendgolang.New()
	if err := front.SetOptions(opt.New(front.OptionsSchema(), map[string]string{
		"dir": srcDir,
		// The fixture is a self-contained module deliberately kept
		// out of go.work, so the loader must respect its own go.mod
		// boundary rather than the enclosing workspace.
		"ignore_workspace": "true",
	})); err != nil {
		t.Fatalf("SetOptions: %v", err)
	}

	annotator := shape.New().Mixins(mixins.All()...)

	// Register the plugin's real schemas so the frontend validates
	// each parsed directive against them. Without this the test
	// would prove parsing and stamping but skip the opt-in that
	// permits several positionals in the first place.
	reg := directive.NewRegistry()
	for _, sc := range annotator.Directives() {
		if err := reg.Register(sc); err != nil {
			t.Fatalf("Register %q: %v", sc.Name, err)
		}
	}

	s := store.New()
	d := diag.New()
	if err := front.Load(&plugin.FrontendContext{
		Store:    s,
		Diag:     d,
		Registry: reg,
		Parser:   parser,
		Pattern:  "./...",
	}); err != nil {
		t.Fatalf("frontend Load: %v", err)
	}
	if d.HasErrors() {
		t.Fatalf("frontend diagnostics: %+v", d.Diagnostics())
	}

	if err := annotator.Annotate(&plugin.AnnotatorContext{
		Store: s, Reader: store.NewReader(s), Diag: d,
	}); err != nil {
		t.Fatalf("shape Annotate: %v", err)
	}
	if d.HasErrors() {
		t.Fatalf("annotator diagnostics: %+v", d.Diagnostics())
	}

	return store.NewReader(s).Methods().Slice()
}

// methodNamed returns the fixture method called name.
func methodNamed(t *testing.T, methods []*node.Method, name string) *node.Method {
	t.Helper()
	for _, m := range methods {
		if m.Name == name {
			return m
		}
	}
	t.Fatalf("fixture has no method %q (got %d methods)", name, len(methods))
	return nil
}
