// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"text/template"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

func TestSigFuncMap(t *testing.T) {
	t.Parallel()

	t.Run("every entry carries the prefix", func(t *testing.T) {
		t.Parallel()
		// The backend rejects two plugins registering the same
		// extension name outright, so an unprefixed bundle would fail
		// every run in which two plugins wanted it.
		for name := range golang.SigFuncMap("tk") {
			if !strings.HasPrefix(name, "tk") {
				t.Errorf("%q carries no prefix", name)
			}
		}
	})

	t.Run("two prefixes coexist", func(t *testing.T) {
		t.Parallel()
		a, b := golang.SigFuncMap("a"), golang.SigFuncMap("b")
		for name := range a {
			if _, clash := b[name]; clash {
				t.Fatalf("%q appears under both prefixes", name)
			}
		}
	})

	t.Run("renders a signature through a template", func(t *testing.T) {
		t.Parallel()
		tmpl := template.Must(template.New("t").
			Funcs(golang.SigFuncMap("go")).
			Parse(`{{ goargs . }}|{{ gocallFields . }}|{{ gofails . }}`))
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, golang.SigOf(getMethod())); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		want := "ctx, id|Ctx: ctx, ID: id|_, err"
		if buf.String() != want {
			t.Fatalf("rendered %q, want %q", buf.String(), want)
		}
	})

	t.Run("an empty prefix is accepted", func(t *testing.T) {
		t.Parallel()
		// For a consumer that has confirmed it is the only claimant.
		if _, ok := golang.SigFuncMap("")["args"]; !ok {
			t.Fatalf("an empty prefix must yield the bare names")
		}
	})
}

func TestQueryFuncMap(t *testing.T) {
	t.Parallel()

	t.Run("branches on a type through a template", func(t *testing.T) {
		t.Parallel()
		tmpl := template.Must(template.New("t").
			Funcs(golang.QueryFuncMap("go")).
			Parse(`{{ if goisError . }}err{{ else }}{{ gozeroLiteral . }}{{ end }}`))
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, builtinRef("string")); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if buf.String() != `""` {
			t.Fatalf("rendered %q", buf.String())
		}
	})
}

func TestConventionFuncMap(t *testing.T) {
	t.Parallel()

	t.Run("names a test function through a template", func(t *testing.T) {
		t.Parallel()
		tmpl := template.Must(template.New("t").
			Funcs(golang.ConventionFuncMap("go")).
			Parse(`{{ gotestFuncName "store" "records a call" }}`))
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, nil); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		if !strings.HasPrefix(buf.String(), "TestStore") {
			t.Fatalf("rendered %q", buf.String())
		}
	})
}

func TestFuncMapsAreInstallable(t *testing.T) {
	t.Parallel()

	t.Run("every bundle installs without panicking", func(t *testing.T) {
		t.Parallel()
		// text/template accepts a function returning one value, or two
		// where the second is an error; anything else panics at
		// registration rather than failing to compile. A helper whose
		// second result is a bool — the shape half this package uses —
		// therefore takes every consumer's pipeline down on the first
		// Build, and only a registration check catches it here.
		for name, bundle := range map[string]template.FuncMap{
			"canonical":  golang.FuncMap(),
			"sig":        golang.SigFuncMap("go"),
			"query":      golang.QueryFuncMap("go"),
			"convention": golang.ConventionFuncMap("go"),
			"all":        golang.AllFuncMap("go"),
		} {
			if _, err := template.New(name).Funcs(bundle).Parse(""); err != nil {
				t.Errorf("%s bundle: %v", name, err)
			}
		}
	})
}

func TestAllFuncMap(t *testing.T) {
	t.Parallel()

	t.Run("unions every bundle", func(t *testing.T) {
		t.Parallel()
		all := golang.AllFuncMap("go")
		want := 0
		for _, bundle := range bundles("go") {
			want += len(bundle)
		}
		if len(all) != want {
			t.Fatalf("AllFuncMap = %d entries, want %d", len(all), want)
		}
	})

	t.Run("the bundles declare no overlapping names", func(t *testing.T) {
		t.Parallel()
		// An overlap would silently drop one binding in the union, so
		// a template would call a helper answering a different
		// question than its name suggests.
		seen := map[string]struct{}{}
		for _, bundle := range bundles("go") {
			for name := range bundle {
				if _, clash := seen[name]; clash {
					t.Errorf("%q appears in two bundles", name)
				}
				seen[name] = struct{}{}
			}
		}
	})

	t.Run("does not collide with the canonical bundle", func(t *testing.T) {
		t.Parallel()
		// The canonical map is merged once by the backend; a name that
		// appeared in both would be one no plugin could contribute.
		canonical := golang.FuncMap()
		for name := range golang.AllFuncMap("") {
			if _, clash := canonical[name]; clash {
				t.Errorf("%q collides with the canonical funcmap", name)
			}
		}
	})
}

func TestTemplateZeroLiteral(t *testing.T) {
	t.Parallel()

	t.Run("renders a derivable zero", func(t *testing.T) {
		t.Parallel()
		if got, err := golang.TemplateZeroLiteral(builtinRef("int")); err != nil || got != "0" {
			t.Fatalf("TemplateZeroLiteral = %q, %v", got, err)
		}
	})

	t.Run("an underivable zero aborts the render", func(t *testing.T) {
		t.Parallel()
		// A template receiving the empty string instead would emit a
		// composite literal with a missing value, and the consumer's
		// compiler would report it against generated code.
		_, err := golang.TemplateZeroLiteral(namedTypeRef("time", "Duration"))
		if !errors.Is(err, golang.ErrNoZeroValue) {
			t.Fatalf("err = %v, want ErrNoZeroValue", err)
		}
	})
}

// bundles lists every per-area funcmap under one prefix.
//
// Named once so the union and the overlap check cannot drift out of
// step with [golang.AllFuncMap] — a bundle added to the union and not
// here would make both tests pass while covering nothing of it.
func bundles(prefix string) []template.FuncMap {
	return []template.FuncMap{
		golang.SigFuncMap(prefix),
		golang.QueryFuncMap(prefix),
		golang.ConventionFuncMap(prefix),
		golang.EnumFuncMap(prefix),
		golang.ShapeFuncMap(prefix),
		golang.EmbedFuncMap(prefix),
		golang.GenericsFuncMap(prefix),
	}
}

// The four bundles the library grew last. Before them AllFuncMap
// reached 37 of 163 exported functions, so a generator needing the
// enum vocabulary, the shape matchers, the embed walks or the witness
// helpers wrote a Go adapter per call — which is where a plugin
// re-decides something this package already decided.
func TestAreaBundles(t *testing.T) {
	t.Parallel()

	t.Run("every area contributes", func(t *testing.T) {
		t.Parallel()
		for name, bundle := range map[string]template.FuncMap{
			"enum":     golang.EnumFuncMap("go"),
			"shape":    golang.ShapeFuncMap("go"),
			"embed":    golang.EmbedFuncMap("go"),
			"generics": golang.GenericsFuncMap("go"),
		} {
			if len(bundle) == 0 {
				t.Fatalf("%s bundle is empty", name)
			}
		}
	})

	t.Run("an incomplete walk aborts the render", func(t *testing.T) {
		t.Parallel()
		// The loud failure, and the right one: the problem slice says
		// the answer is smaller than the truth, and a template
		// rendering it emits a builder short a setter.
		s := &node.Struct{
			Name: "User", Package: "x",
			Embeds: []*node.Embed{{Type: namedTypeRef("io", "Reader")}},
		}
		_, err := golang.TemplateFieldSet(s, mapResolver{})
		if !errors.Is(err, golang.ErrIncompleteWalk) {
			t.Fatalf("TemplateFieldSet err = %v, want ErrIncompleteWalk", err)
		}
		if !strings.Contains(err.Error(), "io.Reader") {
			t.Fatalf("err = %q, want it to name the embed", err)
		}
	})

	t.Run("a complete walk returns no error", func(t *testing.T) {
		t.Parallel()
		s := &node.Struct{
			Name: "User", Package: "x",
			Fields: []*node.Field{{Name: "ID", Type: builtinRef("string")}},
		}
		got, err := golang.TemplateFieldSet(s, mapResolver{})
		if err != nil || len(got) != 1 {
			t.Fatalf("TemplateFieldSet = %v, %v", got, err)
		}
	})

	t.Run("an ordinary absence is not an error", func(t *testing.T) {
		t.Parallel()
		// A typed-iota set starting at one has no zero variant, and a
		// set saturating its type has no value past it. Both are the
		// common case, so both travel as an empty value the template
		// tests with `{{ if }}` rather than as a failed render.
		e := &node.Enum{
			Name: "Status", Package: "x",
			Underlying: builtinRef("int"),
			Variants:   []*node.EnumVariant{{Name: "Draft", Value: "1"}},
		}
		if got := golang.TemplateZeroVariant(e); got != nil {
			t.Fatalf("TemplateZeroVariant = %+v, want nil", got)
		}
		if got := golang.TemplateOutOfRange(e); got != "2" {
			t.Fatalf("TemplateOutOfRange = %q, want 2", got)
		}
	})

	t.Run("satisfaction is a branch rather than a failure", func(t *testing.T) {
		t.Parallel()
		// False is an ordinary answer for a template branching on it,
		// so the missing-method detail is dropped rather than raised.
		want := []*node.Method{{Name: "Read"}}
		if golang.TemplateSatisfies(nil, want) {
			t.Fatal("TemplateSatisfies accepted an empty method set")
		}
		if !golang.TemplateSatisfies(want, want) {
			t.Fatal("TemplateSatisfies rejected an identical set")
		}
	})
}
