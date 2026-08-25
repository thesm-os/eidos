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

	t.Run("entries register under the names templates call", func(t *testing.T) {
		t.Parallel()
		// A helper is addressed by the name declared here. An earlier
		// form took a prefix and mangled every entry with it, so a
		// template spelled a name that appeared in no declaration —
		// and it existed only because each plugin was handed its own
		// copy of the bundle. The backend registers one copy now.
		for _, name := range []string{"args", "callFields", "fails"} {
			if _, ok := golang.SigFuncMap()[name]; !ok {
				t.Errorf("SigFuncMap has no %q entry", name)
			}
		}
	})

	t.Run("renders a signature through a template", func(t *testing.T) {
		t.Parallel()
		tmpl := template.Must(template.New("t").
			Funcs(golang.SigFuncMap()).
			Parse(`{{ args . }}|{{ callFields . }}|{{ fails . }}`))
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, golang.SigOf(getMethod())); err != nil {
			t.Fatalf("Execute: %v", err)
		}
		want := "ctx, id|Ctx: ctx, ID: id|_, err"
		if buf.String() != want {
			t.Fatalf("rendered %q, want %q", buf.String(), want)
		}
	})
}

func TestQueryFuncMap(t *testing.T) {
	t.Parallel()

	t.Run("branches on a type through a template", func(t *testing.T) {
		t.Parallel()
		tmpl := template.Must(template.New("t").
			Funcs(golang.QueryFuncMap()).
			Parse(`{{ if isError . }}err{{ else }}{{ zeroLiteral . }}{{ end }}`))
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
			Funcs(golang.ConventionFuncMap()).
			Parse(`{{ testFuncName "store" "records a call" }}`))
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
			"sig":        golang.SigFuncMap(),
			"query":      golang.QueryFuncMap(),
			"convention": golang.ConventionFuncMap(),
			"all":        golang.AllFuncMap(),
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
		all := golang.AllFuncMap()
		want := 0
		for _, bundle := range bundles() {
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
		for _, bundle := range bundles() {
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
		// Both are merged into the backend's overrideable bucket, so a
		// name in both would leave whichever merged second silently
		// answering for the other.
		canonical := golang.FuncMap()
		for name := range golang.AllFuncMap() {
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

// bundles lists every per-area funcmap.
//
// Named once so the union and the overlap check cannot drift out of
// step with [golang.AllFuncMap] — a bundle added to the union and not
// here would make both tests pass while covering nothing of it.
func bundles() []template.FuncMap {
	return []template.FuncMap{
		golang.SigFuncMap(),
		golang.QueryFuncMap(),
		golang.ConventionFuncMap(),
		golang.EnumFuncMap(),
		golang.ShapeFuncMap(),
		golang.EmbedFuncMap(),
		golang.GenericsFuncMap(),
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
			"enum":     golang.EnumFuncMap(),
			"shape":    golang.ShapeFuncMap(),
			"embed":    golang.EmbedFuncMap(),
			"generics": golang.GenericsFuncMap(),
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

// TestTemplateWalkAdapters covers the three siblings of
// [golang.TemplateFieldSet]. Each drops the problem slice for an
// error, which is what makes an incomplete walk abort the render
// instead of emitting an answer smaller than the truth.
func TestTemplateWalkAdapters(t *testing.T) {
	t.Parallel()

	base := &node.Struct{
		Name: "Base", Package: "x",
		Fields:  []*node.Field{field("ID", builtinRef("string")), field("created", builtinRef("int"))},
		Methods: []*node.Method{{Name: "Read"}},
	}
	resolved := mapResolver{"x.Base": base}

	reachable := func() *node.Struct {
		return &node.Struct{
			Name: "User", Package: "x",
			Fields: []*node.Field{field("Name", builtinRef("string"))},
			Embeds: []*node.Embed{embed("x", "Base", false)},
		}
	}
	unreachable := func() *node.Struct {
		return &node.Struct{
			Name: "User", Package: "x",
			Embeds: []*node.Embed{{Type: namedTypeRef("io", "Reader")}},
		}
	}

	t.Run("PromotedFields reports an unreachable embed", func(t *testing.T) {
		t.Parallel()
		_, err := golang.TemplatePromotedFields(unreachable(), mapResolver{})
		if !errors.Is(err, golang.ErrIncompleteWalk) {
			t.Fatalf("err = %v, want ErrIncompleteWalk", err)
		}
	})

	t.Run("PromotedFields returns the promoted set when the walk completes", func(t *testing.T) {
		t.Parallel()
		got, err := golang.TemplatePromotedFields(reachable(), resolved)
		if err != nil {
			t.Fatalf("TemplatePromotedFields: %v", err)
		}
		if len(got) == 0 {
			t.Error("a reachable embed promoted nothing")
		}
	})

	t.Run("ExportedFieldSet reports an unreachable embed", func(t *testing.T) {
		t.Parallel()
		_, err := golang.TemplateExportedFieldSet(unreachable(), mapResolver{})
		if !errors.Is(err, golang.ErrIncompleteWalk) {
			t.Fatalf("err = %v, want ErrIncompleteWalk", err)
		}
	})

	t.Run("ExportedFieldSet drops the unexported field", func(t *testing.T) {
		t.Parallel()
		// The distinction from FieldSet, and the reason a template has
		// both: an unexported promoted field is not addressable from
		// the generated package.
		got, err := golang.TemplateExportedFieldSet(reachable(), resolved)
		if err != nil {
			t.Fatalf("TemplateExportedFieldSet: %v", err)
		}
		for _, name := range names(got) {
			if name == "created" {
				t.Error("the unexported field survived the exported set")
			}
		}
	})

	t.Run("PromotedMethods reports an unreachable embed", func(t *testing.T) {
		t.Parallel()
		_, err := golang.TemplatePromotedMethods(unreachable(), mapResolver{})
		if !errors.Is(err, golang.ErrIncompleteWalk) {
			t.Fatalf("err = %v, want ErrIncompleteWalk", err)
		}
	})

	t.Run("PromotedMethods returns the embedded method when the walk completes", func(t *testing.T) {
		t.Parallel()
		got, err := golang.TemplatePromotedMethods(reachable(), resolved)
		if err != nil {
			t.Fatalf("TemplatePromotedMethods: %v", err)
		}
		if len(got) == 0 {
			t.Error("a reachable embed promoted no method")
		}
	})
}

// TestTemplateComparableDeep pins the one walk whose refusal is not
// about embeds. An unreachable type is not evidence of comparability,
// so the adapter reports false with an error rather than a verdict.
func TestTemplateComparableDeep(t *testing.T) {
	t.Parallel()

	t.Run("a builtin answers without an error", func(t *testing.T) {
		t.Parallel()
		got, err := golang.TemplateComparableDeep(builtinRef("string"), mapResolver{})
		if err != nil || !got {
			t.Fatalf("TemplateComparableDeep(string) = %v, %v, want true, nil", got, err)
		}
	})

	t.Run("a slice is not comparable and that is not an error", func(t *testing.T) {
		t.Parallel()
		got, err := golang.TemplateComparableDeep(sliceRef(builtinRef("string")), mapResolver{})
		if err != nil || got {
			t.Fatalf("TemplateComparableDeep([]string) = %v, %v, want false, nil", got, err)
		}
	})

	t.Run("an unreachable type reports rather than answering", func(t *testing.T) {
		t.Parallel()
		// False-with-an-error, not false: a template keying a map on a
		// bare false would emit one the consumer cannot compile.
		got, err := golang.TemplateComparableDeep(namedTypeRef("io", "Reader"), mapResolver{})
		if !errors.Is(err, golang.ErrIncompleteWalk) {
			t.Fatalf("err = %v, want ErrIncompleteWalk", err)
		}
		if got {
			t.Error("an unreachable type was reported comparable")
		}
	})
}

// TestTemplateValueAdapters covers the adapters that drop a bool for
// an empty string. Each absence is an ordinary answer a template
// branches on with `{{ if }}`, not a failure.
func TestTemplateValueAdapters(t *testing.T) {
	t.Parallel()

	t.Run("SentinelSubject strips the prefix a sentinel carries", func(t *testing.T) {
		t.Parallel()
		if got := golang.TemplateSentinelSubject("ErrNotFound"); got != "NotFound" {
			t.Errorf("TemplateSentinelSubject = %q, want NotFound", got)
		}
	})

	t.Run("SentinelSubject returns an unprefixed name whole", func(t *testing.T) {
		t.Parallel()
		// The dropped flag carries no failure: a name with no prefix is
		// already the subject a template wants to interpolate.
		if got := golang.TemplateSentinelSubject("NotFound"); got != "NotFound" {
			t.Errorf("TemplateSentinelSubject = %q, want NotFound", got)
		}
	})

	t.Run("OutOfRangeText yields a marker outside the declared set", func(t *testing.T) {
		t.Parallel()
		e := stringEnum([2]string{"EU", "eu"}, [2]string{"US", "us"})
		got := golang.TemplateOutOfRangeText(e)
		if got == "" {
			t.Fatal("a set without the marker derived no out-of-range text")
		}
		for _, text := range golang.EnumTexts(e) {
			if text == got {
				t.Errorf("out-of-range text %q is in the declared set", got)
			}
		}
	})

	t.Run("OutOfRangeText withholds when the set carries the marker", func(t *testing.T) {
		t.Parallel()
		// The one case a probe would assert the opposite of what it
		// means, so the adapter yields the empty string a template skips.
		marker := golang.TemplateOutOfRangeText(stringEnum([2]string{"EU", "eu"}))
		e := stringEnum([2]string{"EU", "eu"}, [2]string{"Unknown", marker})
		if got := golang.TemplateOutOfRangeText(e); got != "" {
			t.Errorf("TemplateOutOfRangeText = %q, want empty", got)
		}
	})

	t.Run("EmbedIdent names the field an embed contributes", func(t *testing.T) {
		t.Parallel()
		if got := golang.TemplateEmbedIdent(embed("x", "Base", false)); got != "Base" {
			t.Errorf("TemplateEmbedIdent = %q, want Base", got)
		}
	})

	t.Run("EmbedIdent names it identically through a pointer", func(t *testing.T) {
		t.Parallel()
		// The dropped half: a template asking for the name is spelling a
		// selector, and a pointer embed spells the same one.
		if got := golang.TemplateEmbedIdent(embed("x", "Base", true)); got != "Base" {
			t.Errorf("TemplateEmbedIdent = %q, want Base", got)
		}
	})

	t.Run("DuplicateText names a text two variants share", func(t *testing.T) {
		t.Parallel()
		e := stringEnum([2]string{"EU", "eu"}, [2]string{"Europe", "eu"})
		if got := golang.TemplateDuplicateText(e); got != "eu" {
			t.Errorf("TemplateDuplicateText = %q, want eu", got)
		}
	})

	t.Run("DuplicateText is empty for a set with no collision", func(t *testing.T) {
		t.Parallel()
		e := stringEnum([2]string{"EU", "eu"}, [2]string{"US", "us"})
		if got := golang.TemplateDuplicateText(e); got != "" {
			t.Errorf("TemplateDuplicateText = %q, want empty", got)
		}
	})
}

// TestTemplateEnumAdapters covers the halves of the enum adapters the
// existing absence test does not reach: the variant that exists, and
// the set with no value past it.
func TestTemplateEnumAdapters(t *testing.T) {
	t.Parallel()

	t.Run("ZeroVariant returns the variant whose value is the zero", func(t *testing.T) {
		t.Parallel()
		e := numericEnum("0", "1")
		got := golang.TemplateZeroVariant(e)
		if got == nil {
			t.Fatal("a set declaring a zero variant returned nil")
		}
		if got.Value != "0" {
			t.Errorf("TemplateZeroVariant = %q, want the variant valued 0", got.Value)
		}
	})

	t.Run("OutOfRange is empty for a set with no numeric bound", func(t *testing.T) {
		t.Parallel()
		// A string-valued set has no value "past" it, and a generator's
		// correct response is to omit the probe rather than abort.
		e := stringEnum([2]string{"EU", "eu"}, [2]string{"US", "us"})
		if got := golang.TemplateOutOfRange(e); got != "" {
			t.Errorf("TemplateOutOfRange = %q, want empty", got)
		}
	})
}
