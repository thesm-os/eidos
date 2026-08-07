// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugin_test

import (
	"io/fs"
	"slices"
	"testing"
	"text/template"

	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/priority"
)

// halfTemplateProvider declares Templates and TemplateFuncs but not
// TemplateOverrides — the shape a plugin author reaches most often,
// because a plugin overriding nothing has no override to write.
type halfTemplateProvider struct{ name string }

func (p *halfTemplateProvider) Name() string                            { return p.name }
func (*halfTemplateProvider) Generate(_ *plugin.GeneratorContext) error { return nil }
func (*halfTemplateProvider) Templates(string) (fs.FS, bool)            { return nil, false }
func (*halfTemplateProvider) TemplateFuncs(string) template.FuncMap     { return nil }

// halfCapabilityProvider declares Priority alone.
type halfCapabilityProvider struct{ name string }

func (p *halfCapabilityProvider) Name() string                            { return p.name }
func (*halfCapabilityProvider) Generate(_ *plugin.GeneratorContext) error { return nil }
func (*halfCapabilityProvider) Priority() priority.Priority               { return priority.Default }

// doublyPartial is partial in both capabilities at once.
type doublyPartial struct{ name string }

func (p *doublyPartial) Name() string                            { return p.name }
func (*doublyPartial) Generate(_ *plugin.GeneratorContext) error { return nil }
func (*doublyPartial) Templates(string) (fs.FS, bool)            { return nil, false }
func (*doublyPartial) Priority() priority.Priority               { return priority.Default }

// wholeProvider declares both capabilities in full.
type wholeProvider struct{ name string }

func (p *wholeProvider) Name() string                            { return p.name }
func (*wholeProvider) Generate(_ *plugin.GeneratorContext) error { return nil }
func (*wholeProvider) Templates(string) (fs.FS, bool)            { return nil, false }
func (*wholeProvider) TemplateFuncs(string) template.FuncMap     { return nil }
func (*wholeProvider) TemplateOverrides(string) template.FuncMap { return nil }
func (*wholeProvider) Priority() priority.Priority               { return priority.Default }
func (*wholeProvider) Provides() []string                        { return nil }
func (*wholeProvider) Requires() []string                        { return nil }

// bareGenerator opts out of every optional capability.
type bareGenerator struct{ name string }

func (p *bareGenerator) Name() string                            { return p.name }
func (*bareGenerator) Generate(_ *plugin.GeneratorContext) error { return nil }

// TestCapabilities pins the description the whole partial-detection
// rests on.
//
// The list lives beside the interfaces it describes so a method
// added to one of them cannot leave the probe behind. These checks
// are what make that placement load-bearing rather than decorative:
// a capability whose method list drifts from its interface stops
// being detectable, and the failure is silent — the probe reports
// "complete" for a plugin missing the new method.
func TestCapabilities(t *testing.T) {
	t.Parallel()

	t.Run("describes every multi-method optional capability", func(t *testing.T) {
		t.Parallel()
		caps := plugin.Capabilities()
		names := make([]string, 0, len(caps))
		for _, c := range caps {
			names = append(names, c.Name)
		}
		for _, want := range []string{"plugin.TemplateProvider", "plugin.CapabilityProvider"} {
			if !slices.Contains(names, want) {
				t.Errorf("Capabilities() omits %q; got %v", want, names)
			}
		}
	})

	t.Run("every described capability declares at least two methods", func(t *testing.T) {
		t.Parallel()
		// A single-method capability cannot be partially implemented,
		// so one listed here would be dead weight the detection walks
		// on every plugin forever.
		for _, c := range plugin.Capabilities() {
			if len(c.Methods) < 2 {
				t.Errorf("capability %q declares %d method(s); "+
					"a single-method capability cannot be partial", c.Name, len(c.Methods))
			}
		}
	})

	t.Run("every method carries a name and a probe", func(t *testing.T) {
		t.Parallel()
		for _, c := range plugin.Capabilities() {
			for i, m := range c.Methods {
				if m.Name == "" {
					t.Errorf("capability %q method %d has no name", c.Name, i)
				}
				if m.Declared == nil {
					t.Errorf("capability %q method %q has no probe", c.Name, m.Name)
				}
			}
		}
	})

	t.Run("a whole implementer satisfies every probe", func(t *testing.T) {
		t.Parallel()
		// Ties each probe to the interface it claims to detect: a
		// probe asserting the wrong single-method interface would
		// report a complete plugin as missing that method.
		p := &wholeProvider{name: "whole"}
		for _, c := range plugin.Capabilities() {
			for _, m := range c.Methods {
				if !m.Declared(p) {
					t.Errorf("capability %q probe for %q rejects a whole implementer", c.Name, m.Name)
				}
			}
		}
	})
}

// TestGaps pins the detection both the pipeline's Build rejection
// and the conformance suite's check consume.
//
// Holding it once is the point: two copies would eventually
// disagree about what "partial" means, and the disagreement would
// surface as a plugin the suite passes and Build rejects.
func TestGaps(t *testing.T) {
	t.Parallel()

	t.Run("a plugin implementing nothing optional has no gaps", func(t *testing.T) {
		t.Parallel()
		// Opting out is the common case and must stay free.
		if got := plugin.Gaps(&bareGenerator{name: "bare"}); got != nil {
			t.Fatalf("Gaps = %+v, want nil", got)
		}
	})

	t.Run("a plugin implementing both capabilities in full has no gaps", func(t *testing.T) {
		t.Parallel()
		if got := plugin.Gaps(&wholeProvider{name: "whole"}); got != nil {
			t.Fatalf("Gaps = %+v, want nil", got)
		}
	})

	t.Run("reports the missing method of a partial TemplateProvider", func(t *testing.T) {
		t.Parallel()
		got := plugin.Gaps(&halfTemplateProvider{name: "half"})
		if len(got) != 1 {
			t.Fatalf("Gaps = %+v, want one gap", got)
		}
		if got[0].Capability != "plugin.TemplateProvider" {
			t.Fatalf("Capability = %q, want plugin.TemplateProvider", got[0].Capability)
		}
		if !slices.Equal(got[0].Missing, []string{"TemplateOverrides"}) {
			t.Fatalf("Missing = %v, want [TemplateOverrides]", got[0].Missing)
		}
	})

	t.Run("reports the declared half alongside the missing one", func(t *testing.T) {
		t.Parallel()
		// The declared half is what tells an author which capability
		// they were reaching for; the missing half alone reads as an
		// arbitrary demand.
		got := plugin.Gaps(&halfTemplateProvider{name: "half"})
		if !slices.Equal(got[0].Declared, []string{"Templates", "TemplateFuncs"}) {
			t.Fatalf("Declared = %v, want [Templates TemplateFuncs]", got[0].Declared)
		}
	})

	t.Run("reports both missing methods of a partial CapabilityProvider", func(t *testing.T) {
		t.Parallel()
		got := plugin.Gaps(&halfCapabilityProvider{name: "ordered"})
		if len(got) != 1 {
			t.Fatalf("Gaps = %+v, want one gap", got)
		}
		if !slices.Equal(got[0].Missing, []string{"Provides", "Requires"}) {
			t.Fatalf("Missing = %v, want [Provides Requires]", got[0].Missing)
		}
	})

	t.Run("reports one gap per partially-implemented capability", func(t *testing.T) {
		t.Parallel()
		got := plugin.Gaps(&doublyPartial{name: "both"})
		if len(got) != 2 {
			t.Fatalf("Gaps = %+v, want two gaps", got)
		}
	})

	t.Run("every gap carries a non-empty declared and missing half", func(t *testing.T) {
		t.Parallel()
		// The documented invariant: a plugin declaring all or none
		// produces no Gap at all, so a Gap with an empty half would
		// mean the partition disagrees with the emptiness guard.
		for _, p := range []plugin.Plugin{
			&halfTemplateProvider{name: "a"},
			&halfCapabilityProvider{name: "b"},
			&doublyPartial{name: "c"},
		} {
			for _, g := range plugin.Gaps(p) {
				if len(g.Declared) == 0 || len(g.Missing) == 0 {
					t.Errorf("%s: gap %+v has an empty half", p.Name(), g)
				}
			}
		}
	})

	t.Run("method order follows the capability's declaration order", func(t *testing.T) {
		t.Parallel()
		// Diagnostics list methods in the order the godoc does, so a
		// reader can find them.
		got := plugin.Gaps(&doublyPartial{name: "both"})
		for _, g := range got {
			if g.Capability != "plugin.TemplateProvider" {
				continue
			}
			if !slices.Equal(g.Missing, []string{"TemplateFuncs", "TemplateOverrides"}) {
				t.Fatalf("Missing = %v, want declaration order [TemplateFuncs TemplateOverrides]", g.Missing)
			}
		}
	})
}
