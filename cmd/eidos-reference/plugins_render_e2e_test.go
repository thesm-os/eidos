// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	backendgolang "go.thesmos.sh/eidos/backend/golang"
	"go.thesmos.sh/eidos/eidostest/golangtest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	"go.thesmos.sh/eidos/plugins/generator/enum"
	"go.thesmos.sh/eidos/plugins/generator/sentinel"
)

// The plugins module may not import a backend — `.golangci.yml` denies
// it under `**/plugins/**`, on the grounds that a plugin ships
// templates rather than depending on one renderer. That rule is right,
// and it means the only assertion that matters for a code generator —
// does the Go it emits compile — cannot live beside the generator.
//
// It lives here. This binary already registers every plugin and every
// backend, nothing depends on it, and its path is outside the denied
// tree, so the import is legal and adds no module edge that release
// layering has to order.
//
// Scope is deliberately narrow. `eidostest/acceptancetest` already
// drives the whole ensemble over the demoproject fixture and covers
// the happy shapes. What follows is only the shapes that fixture does
// not carry and that broke consumers' builds when they regressed.

// TestPluginsRender_StringValuedEnum pins the branch a numeric fixture
// cannot reach.
//
// The generated String method converts through the enum's underlying
// type. For a numeric enum that is `fmt.Sprintf("Status(%d)", int(v))`;
// for a string-valued one `int(v)` does not compile, so a template that
// takes the numeric arm emits a file that fails the consumer's build
// and no emit-graph assertion notices. The demoproject fixture carries
// only `blog.Status`, a typed-iota numeric enum.
func TestPluginsRender_StringValuedEnum(t *testing.T) {
	t.Parallel()

	fixture := storefixture.New().
		Package("shop", "example.com/shop").
		Enum("Region", func(e *storefixture.EnumBuilder) {
			e.Directive(storefixture.Directive("enum"))
			e.Underlying(storefixture.Named("string"))
			e.Variant("RegionUS", `"us-east"`)
			e.Variant("RegionEU", `"eu-west"`)
		})

	// The hand-written package the output references, projected from
	// the same fixture that drove the run — so the declarations and the
	// node graph cannot drift into disagreement.
	gen := golangtest.Render(t, backendgolang.New(), fixture.PackageNode(), enum.New()).
		WithSource(golangtest.GoFile(fixture.GoSource()))

	t.Run("emits Go the consumer can build", func(t *testing.T) {
		t.Parallel()
		gen.AssertCompiles(t)
	})

	t.Run("emits a companion suite that passes", func(t *testing.T) {
		t.Parallel()
		// The plugin's second output is a test suite asserting the
		// first output's contract. A suite that exists and compiles
		// proves nothing until it runs.
		gen.AssertVets(t)
		gen.AssertTestsPass(t)
	})

	t.Run("converts through the string underlying type", func(t *testing.T) {
		t.Parallel()
		// The defect this test exists for: `int(v)` applied to a
		// string-valued type. Asserted on the body rather than the
		// file, so an unrelated numeric conversion elsewhere cannot
		// satisfy it.
		gen.Primary(t).InMethod(t, "Region", "String").
			AssertNotContains(t, "int(v)")
	})
}

// TestPluginsRender_NarrowWidthSentinelFields pins the guard the
// demoproject fixture cannot reach.
//
// The generated suite binds a sample per field with `:=`, which gives
// an untyped constant its default type `int`. An `int32` field rejects
// that, so the emitted suite does not compile — and every structural
// assertion about it passes, because the text is exactly what was
// intended. demoproject's ValidationError carries two string fields.
func TestPluginsRender_NarrowWidthSentinelFields(t *testing.T) {
	t.Parallel()

	fixture := storefixture.New().
		Package("auth", "example.com/auth").
		Struct("ValidationError", func(s *storefixture.StructBuilder) {
			s.Field("Code", storefixture.Named("int32"), nil)
			s.Field("Width", storefixture.Named("int8"), nil)
			s.Field("Ratio", storefixture.Named("float64"), nil)
			s.Field("Field", storefixture.Named("string"), nil)
			// Error() is what marks the struct a custom error type;
			// without it the plugin declines it and emits nothing.
			s.Method("Error", func(m *storefixture.MethodBuilder) {
				m.Return(storefixture.Named("string"))
			})
		})
	// The directive is package-scoped, not per-type.
	pkg := fixture.PackageNode()
	pkg.DirectiveList = append(pkg.DirectiveList, storefixture.Directive(sentinel.DirectiveName))

	// Hand-written rather than projected: the emitted suite exercises
	// behaviour, and a projection emits shape with panicking bodies.
	gen := golangtest.Render(t, backendgolang.New(), pkg, sentinel.New()).
		WithSource(golangtest.GoFile("auth/errors.go", authSource))

	t.Run("the emitted suite type-checks against narrow fields", func(t *testing.T) {
		t.Parallel()
		// go build skips _test.go entirely; vet is what compiles it.
		gen.AssertVets(t)
	})

	t.Run("the emitted suite passes", func(t *testing.T) {
		t.Parallel()
		gen.AssertTestsPass(t)
	})
}

// authSource is the hand-written package the sentinel output checks.
// Behaviour rather than shape: the emitted suite calls Error() and
// compares its result, so a panicking stub would fail at run time for
// a reason unrelated to what the test pins.
const authSource = `package auth

import "fmt"

// ValidationError carries narrow-width fields on purpose: the emitted
// suite binds a sample per field, and ` + "`:=`" + ` gives an untyped constant its
// default type int, which an int32 field rejects.
type ValidationError struct {
	Code   int32
	Width  int8
	Ratio  float64
	Field  string
}

// Error names every field, because the emitted suite asserts that it
// does — an error whose message drops a field it carries tells the
// reader nothing the wrapped value would not.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("validation: field=%s code=%d width=%d ratio=%v",
		e.Field, e.Code, e.Width, e.Ratio)
}
`
