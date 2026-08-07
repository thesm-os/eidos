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
		want := len(golang.SigFuncMap("go")) +
			len(golang.QueryFuncMap("go")) +
			len(golang.ConventionFuncMap("go"))
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
		for _, bundle := range []template.FuncMap{
			golang.SigFuncMap("go"), golang.QueryFuncMap("go"), golang.ConventionFuncMap("go"),
		} {
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
