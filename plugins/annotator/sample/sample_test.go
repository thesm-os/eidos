// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sample_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
	sampleplugin "go.thesmos.sh/eidos/plugins/annotator/sample"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// TestConformance runs the framework's annotator suites.
func TestConformance(t *testing.T) {
	t.Parallel()

	t.Run("framework contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunSuite(t, sampleplugin.New())
	})

	t.Run("annotator contracts", func(t *testing.T) {
		t.Parallel()
		plugintest.RunAnnotatorSuite(t, sampleplugin.New(), []plugintest.AnnotatorFixture{
			{
				Name: "empty store",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					return gofixture.New().Build()
				},
			},
			{
				Name: "package with no annotated types",
				BuildStore: func(t *testing.T) *sdk.Store {
					t.Helper()
					return gofixture.New().
						Package("blog", "example.com/blog").
						Build()
				},
			},
		})
	})
}

// annotated runs the plugin over a package configured by fn.
func annotated(t *testing.T, fn func(*gofixture.Builder)) (*sdk.Store, *diag.Sink) {
	t.Helper()
	b := gofixture.New().Package("blog", "example.com/blog")
	fn(b)
	s := b.Build()
	d := diag.Capture()
	if err := sampleplugin.New().Annotate(&sdk.AnnotatorContext{
		Store: s, Reader: store.NewReader(s), Diag: d,
	}); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	return s, d
}

// stampedOn returns the two values stamped on the named struct.
func stampedOn(t *testing.T, s *sdk.Store, name string) (sdk.Sample, sdk.Sample) {
	t.Helper()
	decl, found := store.NewReader(s).Structs().
		Where(func(x *sdk.Struct) bool { return x.Name == name }).First()
	if !found {
		t.Fatalf("fixture lost its %q", name)
	}
	value, _ := sdk.AuthoredSampleOf(decl.Meta())
	alternate, _ := sdk.AuthoredAlternateOf(decl.Meta())
	return value, alternate
}

// Either half stands alone, because a type may need one and accept the
// derived other.
func TestEitherHalfStandsAlone(t *testing.T) {
	t.Parallel()

	t.Run("the positional names the first value", func(t *testing.T) {
		t.Parallel()
		s, d := annotated(t, func(b *gofixture.Builder) {
			b.Struct("Account", func(sb *gofixture.StructBuilder) {
				sb.Directive(gofixture.Directive(
					sampleplugin.DirectiveName, gofixture.Arg("NewTestAccount"),
				))
			})
		})
		value, alternate := stampedOn(t, s, "Account")
		if !value.OK() {
			t.Errorf("nothing stamped; diagnostics: %+v", d.Diagnostics())
		}
		if alternate.OK() {
			t.Error("naming one value should leave the other derived")
		}
	})

	t.Run("the key names the second", func(t *testing.T) {
		t.Parallel()
		s, _ := annotated(t, func(b *gofixture.Builder) {
			b.Struct("Account", func(sb *gofixture.StructBuilder) {
				sb.Directive(gofixture.Directive(
					sampleplugin.DirectiveName,
					gofixture.KV(sampleplugin.AlternateKey, "OtherAccount"),
				))
			})
		})
		value, alternate := stampedOn(t, s, "Account")
		if value.OK() {
			t.Error("naming the second value should leave the first derived")
		}
		if !alternate.OK() {
			t.Error("the alternate key should stamp the second value")
		}
	})
}

// A directive naming nothing is a line the author did not finish.
func TestEmptyDirectiveIsReported(t *testing.T) {
	t.Parallel()

	s, d := annotated(t, func(b *gofixture.Builder) {
		b.Struct("Account", func(sb *gofixture.StructBuilder) {
			sb.Directive(gofixture.Directive(sampleplugin.DirectiveName))
		})
	})
	if value, alternate := stampedOn(t, s, "Account"); value.OK() || alternate.OK() {
		t.Error("nothing should be stamped for a directive that named nothing")
	}
	if !d.HasErrors() {
		t.Error("a directive that stamped nothing should say why")
	}
}

// The declaring package is recorded even for a bare name.
//
// A consumer renders the call from wherever its output was routed, and
// a bare name there binds to whatever else is in scope rather than
// failing — so the qualifier is what makes a wrong answer a compile
// error instead of a silent one.
func TestBareNameResolvesAgainstItsOwnPackage(t *testing.T) {
	t.Parallel()

	s, _ := annotated(t, func(b *gofixture.Builder) {
		b.Struct("Account", func(sb *gofixture.StructBuilder) {
			sb.Directive(gofixture.Directive(
				sampleplugin.DirectiveName, gofixture.Arg("NewTestAccount"),
			))
		})
	})
	decl, _ := store.NewReader(s).Structs().
		Where(func(x *sdk.Struct) bool { return x.Name == "Account" }).First()
	pkg, symbol, ok := sdk.AuthoredSample(decl.Meta())
	if !ok || symbol != "NewTestAccount" {
		t.Fatalf("stamped %q, want the named function", symbol)
	}
	if pkg != "example.com/blog" {
		t.Errorf("package = %q, want the declaring package", pkg)
	}
}

// An interface is the case with the least choice in it: it has no
// literal form, so a member of one is refused a value and every check
// that needed one is dropped.
func TestInterfacesAreAnnotated(t *testing.T) {
	t.Parallel()

	s, _ := annotated(t, func(b *gofixture.Builder) {
		b.Interface("Writer", func(ib *gofixture.InterfaceBuilder) {
			ib.Directive(gofixture.Directive(
				sampleplugin.DirectiveName, gofixture.Arg("NewFakeWriter"),
			))
		})
	})
	decl, found := store.NewReader(s).Interfaces().
		Where(func(x *sdk.Interface) bool { return x.Name == "Writer" }).First()
	if !found {
		t.Fatal("fixture lost its interface")
	}
	if _, ok := sdk.AuthoredSampleOf(decl.Meta()); !ok {
		t.Error("an interface member has no derivable value, so naming one is " +
			"the only way its checks are written at all")
	}
}
