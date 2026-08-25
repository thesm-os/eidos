// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package main

import (
	"testing"

	"go.thesmos.sh/eidos/eidostest/golangtest"
	"go.thesmos.sh/eidos/eidostest/storefixture"
	backendgolang "go.thesmos.sh/eidos/lang/golang/backend"
	"go.thesmos.sh/eidos/plugins/annotator/sample"
	"go.thesmos.sh/eidos/plugins/generator/builder"
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

// TestPluginsRender_NonIntegerEnumUnderlying pins the two shapes the
// numeric fixture's `int` underlying type hides.
//
// The `String` fallback converts the value and prints it, and both
// halves used to be written into the template as `int(v)` and `%d`.
// That pair is right for exactly the enum demoproject carries — a
// typed-iota `int` — and wrong in two different ways elsewhere, neither
// of which a structural assertion about the emit graph can see.
func TestPluginsRender_NonIntegerEnumUnderlying(t *testing.T) {
	t.Parallel()

	t.Run("a float set converts and prints without truncating", func(t *testing.T) {
		t.Parallel()
		// The quiet one: `int(v)` on a float64 compiles and vets, and
		// prints Ratio(0.5) as "Ratio(0)". The output is wrong and
		// nothing in the toolchain says so, which is why this is
		// asserted on the method body rather than left to a build.
		fixture := storefixture.New().
			Package("shop", "example.com/shop").
			Enum("Ratio", func(e *storefixture.EnumBuilder) {
				e.Directive(storefixture.Directive("enum"))
				e.Underlying(storefixture.Named("float64"))
				e.Variant("RatioHalf", "0.5")
				e.Variant("RatioFull", "1")
			})
		gen := golangtest.Render(t, backendgolang.New(), fixture.PackageNode(), enum.New()).
			WithSource(golangtest.GoFile(fixture.GoSource()))

		body := gen.Primary(t).InMethod(t, "Ratio", "String")
		body.AssertNotContains(t, "int(v)")
		body.AssertContains(t, "float64(v)")
		body.AssertContains(t, "%g")
		gen.AssertVets(t)
	})

	t.Run("a set over another package's type converts through a qualified reference", func(t *testing.T) {
		t.Parallel()
		// The loud one, once the underlying type is not numeric:
		// `int(v)` applied to a string-defined type does not compile,
		// and the string branch does not catch it because the enum's
		// underlying type is spelled `Name`, not `string`.
		fixture := storefixture.New().
			Package("shop", "example.com/shop").
			Import("example.com/cfg").
			Enum("Tier", func(e *storefixture.EnumBuilder) {
				e.Directive(storefixture.Directive("enum"))
				e.Underlying(storefixture.PkgNamed("example.com/cfg", "Name"))
				e.Variant("TierFree", `"free"`)
				e.Variant("TierPaid", `"paid"`)
			})
		gen := golangtest.Render(t, backendgolang.New(), fixture.PackageNode(), enum.New()).
			WithSource(
				golangtest.GoFile("cfg/cfg.go", cfgSource),
				golangtest.GoFile("shop/tier.go", tierSource),
			)

		// The conversion has to name cfg and the file has to import it.
		// Composed from the underlying type's name alone it renders
		// `Name(v)`, which names nothing in scope.
		gen.AssertCompiles(t)
		gen.AssertVets(t)
	})
}

// cfgSource declares the package a cross-package enum's underlying type
// lives in. A string type, because that is what makes the old numeric
// conversion a compile error rather than a silent narrowing.
const cfgSource = `package cfg

// Name is the underlying type example.com/shop's Tier is defined over,
// declared here so the generated conversion has to qualify and import.
type Name string
`

// tierSource is the hand-written shop package. Written out rather than
// projected because pinning what the *generator* does with a
// cross-package underlying type should not also depend on what the
// fixture's projection does with one.
const tierSource = `package shop

import "example.com/cfg"

// Tier is defined over another package's type on purpose.
type Tier cfg.Name

const (
	TierFree Tier = "free"
	TierPaid Tier = "paid"
)
`

// TestPluginsRender_EnumSurface pins the members this generator grew
// and the two branches that decide whether one is written at all.
//
// The set was `String` plus a JSON codec pair; it is now a text codec
// pair, a validity test and a set accessor, each withheld where the
// type already declares it. Withholding is the half no emit-graph
// assertion can check: a member emitted beside a hand-written one of
// the same name is a duplicate declaration, which only a compiler
// reports.
func TestPluginsRender_EnumSurface(t *testing.T) {
	t.Parallel()

	t.Run("a set counting from zero gets the whole surface", func(t *testing.T) {
		t.Parallel()

		fixture := storefixture.New().
			Package("shop", "example.com/shop").
			Enum("Status", func(e *storefixture.EnumBuilder) {
				e.Directive(storefixture.Directive(enum.DirectiveName))
				e.Underlying(storefixture.Named("int"))
				e.Variant("StatusDraft", "0")
				e.Variant("StatusActive", "1")
				e.Variant("StatusArchived", "2")
			})
		gen := golangtest.Render(t, backendgolang.New(), fixture.PackageNode(), enum.New()).
			WithSource(golangtest.GoFile(fixture.GoSource()))

		gen.AssertVets(t)
		gen.AssertTestsPass(t)

		primary := gen.Primary(t)
		primary.AssertContains(t, "func StatusValues() []Status")
		primary.AssertContains(t, "func (v Status) IsValid() bool")
		primary.AssertContains(t, "func (v Status) MarshalText() ([]byte, error)")
		primary.AssertContains(t, "func (v *Status) UnmarshalText(text []byte) error")
		// Text rather than JSON, which is the port's headline change:
		// encoding/json reaches for TextMarshaler on its own and so does
		// YAML, and it is what makes the type legal as a map key.
		primary.AssertNotContains(t, "MarshalJSON")
	})

	t.Run("a set starting past zero asserts the opposite", func(t *testing.T) {
		t.Parallel()

		// The branch the fixture above cannot reach. An enumeration whose
		// zero is a declared variant and one whose zero is not need
		// opposite assertions, and a template writing one of them for
		// both passes against exactly half its inputs.
		fixture := storefixture.New().
			Package("shop", "example.com/shop").
			Enum("Level", func(e *storefixture.EnumBuilder) {
				e.Directive(storefixture.Directive(enum.DirectiveName))
				e.Underlying(storefixture.Named("int"))
				e.Variant("LevelLow", "1")
				e.Variant("LevelHigh", "2")
			})
		gen := golangtest.Render(t, backendgolang.New(), fixture.PackageNode(), enum.New()).
			WithSource(golangtest.GoFile(fixture.GoSource()))

		gen.AssertVets(t)
		gen.AssertTestsPass(t)
		gen.Suffixed(t, enum.GoTestSuffix).
			AssertContains(t, "the zero value is not a declared variant")
	})

	t.Run("a hand-written member is not shadowed", func(t *testing.T) {
		t.Parallel()

		// The loud failure this branch prevents: a second `String` on one
		// type does not compile, and the emit graph looks identical
		// whether the branch fired or not.
		fixture := storefixture.New().
			Package("shop", "example.com/shop").
			Enum("Grade", func(e *storefixture.EnumBuilder) {
				e.Directive(storefixture.Directive(enum.DirectiveName))
				e.Underlying(storefixture.Named("int"))
				e.Variant("GradeLow", "0")
				e.Variant("GradeHigh", "1")
				e.Method("String", func(m *storefixture.MethodBuilder) {
					m.Return(storefixture.Named("string"))
				})
			})
		// Hand-written rather than projected: a projection emits shape
		// with a stub body, and the emitted checks assert that two
		// variants render differently — which two stubs do not.
		gen := golangtest.Render(t, backendgolang.New(), fixture.PackageNode(), enum.New()).
			WithSource(golangtest.GoFile("shop/grade.go", gradeSource))

		gen.AssertCompiles(t)
		gen.AssertVets(t)
		gen.AssertTestsPass(t)

		primary := gen.Primary(t)
		primary.AssertNotContains(t, "func (v Grade) String() string")
		// The parser rides with the renderer, and the encoder rides with
		// the parser: an encoder emitted here would name a decoder
		// written against a function nothing declares.
		primary.AssertNotContains(t, "func ParseGrade(")
		primary.AssertNotContains(t, "MarshalText")
		primary.AssertContains(t, "func (v Grade) IsValid() bool")
	})

	t.Run("methods=off leaves the checks alone", func(t *testing.T) {
		t.Parallel()

		fixture := storefixture.New().
			Package("shop", "example.com/shop").
			Enum("Colour", func(e *storefixture.EnumBuilder) {
				e.Directive(storefixture.Directive(
					enum.DirectiveName,
					storefixture.KV(enum.MethodsKey, enum.MethodsOff),
				))
				e.Underlying(storefixture.Named("int"))
				e.Variant("ColourRed", "0")
				e.Variant("ColourBlue", "1")
			})
		gen := golangtest.Render(t, backendgolang.New(), fixture.PackageNode(), enum.New()).
			WithSource(golangtest.GoFile(fixture.GoSource()))

		// Every check that needs a generated member is withheld, which
		// leaves the two that need none. A template that rendered the
		// rest anyway would emit calls to methods nothing declares.
		gen.AssertVets(t)
		gen.AssertTestsPass(t)
	})
}

// gradeSource is the hand-written package whose renderer the generator
// must leave alone. Behaviour rather than shape, because the emitted
// checks assert that two variants render distinctly.
const gradeSource = `package shop

// Grade carries its own renderer, which is what the generator has to
// notice before it writes a second one.
type Grade int

const (
	GradeLow Grade = iota
	GradeHigh
)

// String is the author's, and the more specific statement.
func (g Grade) String() string {
	if g == GradeHigh {
		return "high"
	}
	return "low"
}
`

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
	pkg := fixture.Directive(storefixture.Directive(sentinel.DirectiveName)).PackageNode()

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
// reader nothing the wrapped value would not. It opens with the
// package prefix for the same reason a sentinel does: a custom error
// reaches the same logs and is read the same way.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("auth: validation field=%s code=%d width=%d ratio=%v",
		e.Field, e.Code, e.Width, e.Ratio)
}
`

// TestPluginsRender_AuthoredSample pins the one member shape that has
// no derivable value at all.
//
// A language works out what a value looks like from what the type is,
// and an interface has no literal form — so a member of one is refused
// a value and every check that needed one is dropped. The check is not
// wrong, it is absent, which is the quietest way for coverage to go
// missing.
//
// Naming a function restores it, and the restoration has to survive the
// whole path: the annotator resolves the name, the language prefers the
// stamp over what it would derive, and the template renders a call
// nobody wrote by hand. Only compiling and running the result proves
// all three.
func TestPluginsRender_AuthoredSample(t *testing.T) {
	t.Parallel()

	fixture := storefixture.New().
		Package("shop", "example.com/shop").
		Interface("Notifier", func(i *storefixture.InterfaceBuilder) {
			i.Directive(storefixture.Directive(
				sample.DirectiveName,
				storefixture.Arg("NewFakeNotifier"),
				storefixture.KV(sample.AlternateKey, "NewOtherNotifier"),
			))
			i.Method("Notify", func(m *storefixture.MethodBuilder) {
				m.Return(storefixture.Named("error"))
			})
		}).
		Struct("Order", func(s *storefixture.StructBuilder) {
			s.Directive(storefixture.Directive(builder.DirectiveName))
			s.Field("ID", storefixture.Named("string"), nil)
			s.Field("Notify", storefixture.PkgNamed("example.com/shop", "Notifier"), nil)
		})

	gen := golangtest.Driver(t, backendgolang.New(), fixture.PackageNode(), builder.New()).
		WithAnnotator(sample.New()).
		Build().
		Run("./...")

	out := golangtest.Rendered(t, gen).
		WithSource(golangtest.GoFile("shop/shop.go", notifierSource))

	t.Run("the generated checks compile and pass", func(t *testing.T) {
		t.Parallel()
		out.AssertVets(t)
		out.AssertTestsPass(t)
	})

	t.Run("the named function is what the check writes", func(t *testing.T) {
		t.Parallel()
		checks := out.Suffixed(t, builder.GoTestSuffix)
		// Derived, the member has no value and its setter is never
		// exercised — so this call appearing at all is the assertion.
		checks.AssertContains(t, "NewFakeNotifier()")
		// The second value keeps the replacing check from passing
		// vacuously, and it is authored too.
		checks.AssertContains(t, "NewOtherNotifier()")
	})
}

// notifierSource is the hand-written package the checks drive. The two
// constructors are what the directive names; without them the
// generated file would reference functions nothing declares.
const notifierSource = `package shop

// Notifier has no literal form, which is the whole reason its values
// have to be named rather than derived.
type Notifier interface {
	Notify() error
}

// Order carries one of them, so its builder has a setter whose check
// needs a value.
type Order struct {
	ID     string
	Notify Notifier
}

type fakeNotifier struct{ name string }

func (fakeNotifier) Notify() error { return nil }

// NewFakeNotifier is the value the directive names.
func NewFakeNotifier() Notifier { return fakeNotifier{name: "fake"} }

// NewOtherNotifier is the second, which must differ from the first or
// the check asserting the setter replaced something would pass against
// a setter that did nothing.
func NewOtherNotifier() Notifier { return fakeNotifier{name: "other"} }
`

// TestPluginsRender_InheritedErrorContract pins the idiom the declared
// method list misses entirely.
//
// A family of custom errors sharing one embedded base is the dominant
// Go shape, and reading only what each member declares finds no
// message method on any of them: the package's directive says its
// errors are a contract and the generated file covered half of them,
// silently. Nothing in the emit graph distinguishes that from a
// package whose members genuinely declare nothing.
func TestPluginsRender_InheritedErrorContract(t *testing.T) {
	t.Parallel()

	fixture := storefixture.New().
		Package("auth", "example.com/auth").
		Struct("BaseError", func(s *storefixture.StructBuilder) {
			s.Field("Op", storefixture.Named("string"), nil)
			s.Field("Cause", storefixture.Named("error"), nil)
			s.Method("Error", func(m *storefixture.MethodBuilder) {
				m.Return(storefixture.Named("string"))
			})
			s.Method("Unwrap", func(m *storefixture.MethodBuilder) {
				m.Return(storefixture.Named("error"))
			})
		}).
		Struct("NotFoundError", func(s *storefixture.StructBuilder) {
			s.Embed(storefixture.Named("BaseError"))
		})
	pkg := fixture.Directive(storefixture.Directive(sentinel.DirectiveName)).PackageNode()

	gen := golangtest.Render(t, backendgolang.New(), pkg, sentinel.New()).
		WithSource(golangtest.GoFile("auth/errors.go", inheritedSource))

	t.Run("the emitted suite type-checks and passes", func(t *testing.T) {
		t.Parallel()
		gen.AssertVets(t)
		gen.AssertTestsPass(t)
	})

	t.Run("the inheriting type earns its own checks", func(t *testing.T) {
		t.Parallel()
		// The whole point: NotFoundError declares nothing, and its
		// contract is real.
		gen.Suffixed(t, sentinel.GoSuffix).
			AssertContains(t, "func TestNotFoundErrorContract(")
	})

	t.Run("a cause reached through the embed is still assigned", func(t *testing.T) {
		t.Parallel()
		// Promotion makes the selector legal however deep the member
		// sits, which is what lets the check be written at all — a
		// composite-literal key could not name it.
		gen.Suffixed(t, sentinel.GoSuffix).AssertContains(t, "got.Cause = cause")
	})
}

// inheritedSource is the hand-written package the checks drive. The
// base carries the whole contract and the family member declares
// nothing, which is the shape the declared-method reading missed.
const inheritedSource = `package auth

// BaseError carries what every error in this package shares. Its
// methods are on the pointer receiver, which is the ordinary spelling
// and the one that decides how a check builds its subject.
type BaseError struct {
	Op    string
	Cause error
}

// Error opens with the package prefix, like a sentinel's message does.
func (e *BaseError) Error() string {
	if e.Cause == nil {
		return "auth: " + e.Op
	}
	return "auth: " + e.Op + ": " + e.Cause.Error()
}

// Unwrap exposes the cause it was given.
func (e *BaseError) Unwrap() error { return e.Cause }

// NotFoundError declares nothing and is an error all the same.
type NotFoundError struct {
	BaseError
}
`

// TestPluginsRender_BuilderShapes pins the setters every member shape
// owes, and that the checks emitted beside them pass.
//
// The builder had no render coverage at all: a deliberately broken
// template left every test in this repository green, because the emit
// graph is identical whatever the template does with it. Each shape
// below reaches a different arm of the setter dispatch, and the arms
// differ in ways only a compiler notices — a variadic against a
// slice parameter, an entry setter taking one argument or two, a
// pointer setter that addresses its argument.
func TestPluginsRender_BuilderShapes(t *testing.T) {
	t.Parallel()

	fixture := storefixture.New().
		Package("shop", "example.com/shop").
		Struct("Order", func(s *storefixture.StructBuilder) {
			s.Directive(storefixture.Directive("builder"))
			s.Field("ID", storefixture.Named("string"), nil)
			s.Field("Lines", storefixture.Slice(storefixture.Named("string")), nil)
			s.Field("Payload", storefixture.Slice(storefixture.Named("byte")), nil)
			s.Field("Totals", storefixture.Map(
				storefixture.Named("string"), storefixture.Named("int"),
			), nil)
			s.Field("Seen", storefixture.Map(
				storefixture.Named("string"), storefixture.AnonStruct(nil, nil),
			), nil)
			s.Field("Note", storefixture.Pointer(storefixture.Named("string")), nil)
		})

	gen := golangtest.Render(t, backendgolang.New(), fixture.PackageNode(), builder.New()).
		WithSource(golangtest.GoFile(fixture.GoSource()))

	t.Run("emits Go the consumer can build", func(t *testing.T) {
		t.Parallel()
		gen.AssertCompiles(t)
	})

	t.Run("emits checks that pass", func(t *testing.T) {
		t.Parallel()
		// The second output asserts the first one's contract. A check
		// that compiles proves nothing until it runs — a setter writing
		// the wrong member compiles perfectly.
		gen.AssertVets(t)
		gen.AssertTestsPass(t)
	})

	t.Run("a byte sequence gets its text-accepting setter", func(t *testing.T) {
		t.Parallel()
		// The arm a plain slice cannot reach: `[]byte` owes a second
		// setter taking a string, and a template treating it as an
		// ordinary sequence would emit a variadic `...byte` instead.
		gen.Primary(t).AssertContains(t, "WithPayloadString(s string)")
	})

	t.Run("a set entry setter asks for no value", func(t *testing.T) {
		t.Parallel()
		// Classified as a mapping, this would take a `struct{}` second
		// argument — asking the caller for the one thing they cannot
		// vary.
		gen.Primary(t).AssertContains(t, "WithSeenEntry(k string)")
	})

	t.Run("a scalar setter is checked against a second value", func(t *testing.T) {
		t.Parallel()
		// The check that cannot pass vacuously: setting a member to the
		// value it already holds proves nothing, so the pair is written
		// in sequence and the second is asserted to have won.
		gen.Suffixed(t, builder.GoTestSuffix).AssertContains(t, "replaces what was already there")
	})

	t.Run("an optional setter addresses its argument", func(t *testing.T) {
		t.Parallel()
		gen.Primary(t).AssertContains(t, "b.v.Note = &v")
	})
}
