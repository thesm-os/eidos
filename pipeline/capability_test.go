// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline_test

import (
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"text/template"

	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/priority"
)

// partialTemplateGen ships templates and funcmap entries but omits
// TemplateOverrides — the shape a plugin author reaches most often,
// because a plugin overriding nothing has no override to declare
// and the nil-returning stub reads as pointless.
type partialTemplateGen struct{ name string }

func (g *partialTemplateGen) Name() string                            { return g.name }
func (*partialTemplateGen) Generate(_ *plugin.GeneratorContext) error { return nil }
func (*partialTemplateGen) Templates(string) (fs.FS, bool) {
	return fstest.MapFS{"x.tmpl": &fstest.MapFile{Data: []byte(`{{define "x"}}{{end}}`)}}, true
}
func (*partialTemplateGen) TemplateFuncs(string) template.FuncMap { return nil }

// wholeTemplateGen declares all three and must build cleanly.
type wholeTemplateGen struct{ name string }

func (g *wholeTemplateGen) Name() string                            { return g.name }
func (*wholeTemplateGen) Generate(_ *plugin.GeneratorContext) error { return nil }
func (*wholeTemplateGen) Templates(string) (fs.FS, bool)            { return nil, false }
func (*wholeTemplateGen) TemplateFuncs(string) template.FuncMap     { return nil }
func (*wholeTemplateGen) TemplateOverrides(string) template.FuncMap { return nil }

// partialCapabilityGen declares Priority but neither Provides nor
// Requires, so the pipeline reads no ordering intent from it and
// runs it in the default bucket.
type partialCapabilityGen struct{ name string }

func (g *partialCapabilityGen) Name() string                            { return g.name }
func (*partialCapabilityGen) Generate(_ *plugin.GeneratorContext) error { return nil }
func (*partialCapabilityGen) Priority() priority.Priority               { return priority.Default }

// TestBuilder_Build_PartialCapability pins Build's rejection of a
// plugin that implements part of a multi-method optional
// capability.
//
// What "partial" means is [plugin.Gaps]'s to decide and is pinned
// in that package; what this covers is the pipeline's own half —
// that Build consults it at all, surfaces every gap it reports
// rather than the first, wraps them in a sentinel callers can match
// with errors.Is, and still admits the two shapes that are not
// faults.
//
// Before the check the whole contribution vanished silently: the
// assertion failed, the plugin was skipped without a diagnostic,
// and the rendered output simply came out short.
func TestBuilder_Build_PartialCapability(t *testing.T) {
	t.Parallel()

	t.Run("a plugin missing TemplateOverrides is rejected", func(t *testing.T) {
		t.Parallel()
		_, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&partialTemplateGen{name: "half"}).
			WithBackend(&stubBE{name: "be"}).
			Build()
		if !errors.Is(err, pipeline.ErrPartialCapability) {
			t.Fatalf("Build error = %v, want ErrPartialCapability", err)
		}
	})

	t.Run("the error names the plugin and the missing method", func(t *testing.T) {
		t.Parallel()
		// A plugin author reading "partial capability" cannot act on
		// it; the method name is the entire actionable content.
		_, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&partialTemplateGen{name: "half"}).
			WithBackend(&stubBE{name: "be"}).
			Build()
		msg := err.Error()
		for _, want := range []string{"half", "TemplateOverrides"} {
			if !strings.Contains(msg, want) {
				t.Errorf("error %q omits %q", msg, want)
			}
		}
	})

	t.Run("a plugin declaring all three template methods builds", func(t *testing.T) {
		t.Parallel()
		_, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&wholeTemplateGen{name: "whole"}).
			WithBackend(&stubBE{name: "be"}).
			Build()
		if err != nil {
			t.Fatalf("Build = %v, want nil", err)
		}
	})

	t.Run("a plugin declaring no template method builds", func(t *testing.T) {
		t.Parallel()
		// Opting out entirely is the common case and must stay free.
		_, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&stubGen{name: "plain"}).
			WithBackend(&stubBE{name: "be"}).
			Build()
		if err != nil {
			t.Fatalf("Build = %v, want nil", err)
		}
	})

	t.Run("a plugin declaring Priority alone is rejected", func(t *testing.T) {
		t.Parallel()
		// The same mechanism covers CapabilityProvider: a declared
		// priority the pipeline ignores is an author's stated
		// ordering intent going nowhere.
		_, err := pipeline.New().
			WithFrontend(&stubFE{name: "fe"}).
			WithGenerator(&partialCapabilityGen{name: "ordered"}).
			WithBackend(&stubBE{name: "be"}).
			Build()
		if !errors.Is(err, pipeline.ErrPartialCapability) {
			t.Fatalf("Build error = %v, want ErrPartialCapability", err)
		}
	})
}
