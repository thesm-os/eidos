// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package enum_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
	enumplugin "go.thesmos.sh/eidos/plugins/generator/enum"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// The projection these tests assert on is what the seam bought.
//
// Which variant is the zero, what an undeclared value looks like, and
// whether a variant's text comes from its identifier or from its
// declared value were all decided at render time before, inside a
// template — so the only way to check any of them was to render Go and
// read it, which this module cannot do. Asking the language through
// [sdk.EnumRules] moved the answers into Go, where a declaration
// against an expected projection is an ordinary test.

// fixture is a store holding one annotated enum, configured by fn.
func fixture(t *testing.T, underlying string, fn func(*gofixture.EnumBuilder)) *sdk.Store {
	t.Helper()
	b := gofixture.New().
		Package("blog", "example.com/blog").
		Enum("Status", func(eb *gofixture.EnumBuilder) {
			eb.Directive(gofixture.Directive(enumplugin.DirectiveName))
			if underlying != "" {
				eb.Underlying(gofixture.Named(underlying))
			}
			fn(eb)
		})
	s := b.Build()
	return s
}

// run drives Generate over s and returns everything it queued.
func run(t *testing.T, s *sdk.Store) (*enumplugin.API, *enumplugin.Tests, *diag.Sink) {
	t.Helper()
	d := diag.Capture()
	if err := enumplugin.New().Generate(&sdk.GeneratorContext{
		Store: s, Reader: store.NewReader(s), Diag: d,
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	var api *enumplugin.API
	var checks *enumplugin.Tests
	for _, slot := range s.Emit().PendingOriginSlots() {
		switch item := slot.Item.(type) {
		case *enumplugin.API:
			api = item
		case *enumplugin.Tests:
			checks = item
		}
	}
	return api, checks, d
}

// numeric is the common fixture: three variants counting from zero.
func numeric(t *testing.T) (*enumplugin.API, *enumplugin.Tests) {
	t.Helper()
	api, checks, _ := run(t, fixture(t, "int", func(eb *gofixture.EnumBuilder) {
		eb.Variant("StatusDraft", "0")
		eb.Variant("StatusActive", "1")
		eb.Variant("StatusArchived", "2")
	}))
	if api == nil || checks == nil {
		t.Fatal("plugin queued nothing for a well-formed enum")
	}
	return api, checks
}

// A numeric enum takes its textual form from the identifier, with the
// type's own name stripped.
func TestNumericTextComesFromTheIdentifier(t *testing.T) {
	t.Parallel()

	api, _ := numeric(t)

	t.Run("the form is the identifier", func(t *testing.T) {
		t.Parallel()
		if api.Textual() {
			// Reading the declared value would render `StatusActive` as
			// "1", which says less than the identifier does.
			t.Error("a numeric enum has no textual form but its identifier")
		}
	})

	t.Run("the type's name is stripped", func(t *testing.T) {
		t.Parallel()
		want := map[string]string{
			"StatusDraft":    `"Draft"`,
			"StatusActive":   `"Active"`,
			"StatusArchived": `"Archived"`,
		}
		for _, v := range api.Variants {
			if v.Text != want[v.Name] {
				t.Errorf("%s renders as %s, want %s — the type name is context "+
					"wherever the value appears", v.Name, v.Text, want[v.Name])
			}
		}
	})
}

// A string enum's declared value IS its textual form.
//
// The one fact the two generators this plugin was merged from
// disagreed about. Deriving the identifier instead round-trips against
// its own parser and fails against every value arriving from outside
// the program, so only a comparison against the declared value tells
// the two apart.
func TestTextualEnumKeepsItsDeclaredValue(t *testing.T) {
	t.Parallel()

	api, _, _ := run(t, fixture(t, "string", func(eb *gofixture.EnumBuilder) {
		eb.Variant("RegionUS", `"us-east"`)
		eb.Variant("RegionEU", `"eu-west"`)
	}))
	if api == nil {
		t.Fatal("plugin queued nothing")
	}

	t.Run("the form is the value", func(t *testing.T) {
		t.Parallel()
		if !api.Textual() {
			t.Error("a string enum's value is already the textual form")
		}
	})

	t.Run("the declared value survives", func(t *testing.T) {
		t.Parallel()
		if api.Variants[0].Text != `"us-east"` {
			t.Errorf("RegionUS renders as %s, want the declared \"us-east\"",
				api.Variants[0].Text)
		}
	})
}

// The identifiers are composed from the language's words, not spelled
// in the core.
func TestNamesComeFromTheLanguage(t *testing.T) {
	t.Parallel()

	api, _ := numeric(t)

	for _, tc := range []struct{ got, want, why string }{
		{api.Render, "String", "fmt reaches for String by name"},
		{api.Parse, "ParseStatus", "the verb leads and the type follows"},
		{api.Values, "StatusValues", "the type leads and the noun follows"},
		{api.Valid, "IsValid", "a predicate reads as a question"},
		{api.Encode, "MarshalText", "encoding/json and YAML both reach for it"},
		{api.Decode, "UnmarshalText", "the encoding pair travels together"},
		{api.Sentinel, "ErrUnknownStatus", "Go's own refusal convention"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Errorf("got %q, want %q — %s", tc.got, tc.want, tc.why)
			}
		})
	}
}

// The zero variant is found by value rather than by position.
//
// Declaration order and zero-ness agree only for a set that declares
// its zero first, and a check built from position asserted that the
// zero equalled a variant the assertion did not mention.
func TestZeroVariantIsFoundByValue(t *testing.T) {
	t.Parallel()

	t.Run("a set declaring its zero last still finds it", func(t *testing.T) {
		t.Parallel()
		_, checks, _ := run(t, fixture(t, "string", func(eb *gofixture.EnumBuilder) {
			eb.Variant("RegionUS", `"us-east"`)
			eb.Variant("RegionUnset", `""`)
		}))
		if checks.ZeroName != "RegionUnset" {
			t.Errorf("ZeroName = %q, want RegionUnset — position is not zero-ness",
				checks.ZeroName)
		}
	})

	t.Run("a set with no zero says so", func(t *testing.T) {
		t.Parallel()
		_, checks, _ := run(t, fixture(t, "int", func(eb *gofixture.EnumBuilder) {
			eb.Variant("StatusActive", "1")
			eb.Variant("StatusArchived", "2")
		}))
		if checks.ZeroName != "" {
			t.Errorf("ZeroName = %q, want empty — an unset value is invalid here, "+
				"and the two cases read as opposite assertions", checks.ZeroName)
		}
	})
}

// The probes a check needs come from the declared set.
func TestProbesAreDerivedFromTheSet(t *testing.T) {
	t.Parallel()

	_, checks := numeric(t)

	t.Run("a numeric set names a value past its largest", func(t *testing.T) {
		t.Parallel()
		if checks.OutOfRange != "3" {
			t.Errorf("OutOfRange = %q, want 3 — one past the largest declared",
				checks.OutOfRange)
		}
	})

	t.Run("every set names text nothing declares", func(t *testing.T) {
		t.Parallel()
		if checks.UnknownText == "" {
			t.Error("without a probe the parse-refusal check cannot be written")
		}
	})
}

// A member the type already declares is skipped, and the package-level
// halves that ride with it go too.
func TestExistingMembersAreNotShadowed(t *testing.T) {
	t.Parallel()

	api, checks, _ := run(t, fixture(t, "int", func(eb *gofixture.EnumBuilder) {
		eb.Variant("StatusDraft", "0")
		eb.Variant("StatusActive", "1")
		eb.Method("String", func(mb *gofixture.MethodBuilder) {
			mb.Return(gofixture.Named("string"))
		})
	}))

	t.Run("the declared member is not written", func(t *testing.T) {
		t.Parallel()
		if api.Emits(enumplugin.SurfaceRender) {
			t.Error("an author who wrote their own renderer meant to keep it")
		}
	})

	t.Run("the parser rides with it", func(t *testing.T) {
		t.Parallel()
		// A type keeping its own renderer almost always keeps its own
		// parser, and generating one that shadows theirs is the worse
		// guess.
		if api.Emits(enumplugin.SurfaceParse) {
			t.Error("the parser must not be generated against a hand-written renderer")
		}
	})

	t.Run("the checks still exercise the declared one", func(t *testing.T) {
		t.Parallel()
		if !checks.Renders {
			t.Error("a hand-written renderer is still a renderer, and the checks " +
				"pinning it are the point of generating them at all")
		}
	})
}

// The encoding pair is never shipped half-finished.
//
// A type that encodes as text and decodes from something else is what
// no author asks for, so the encoder goes when no parser exists to
// write the decoder against.
func TestEncodingPairTravelsTogether(t *testing.T) {
	t.Parallel()

	api, _, _ := run(t, fixture(t, "int", func(eb *gofixture.EnumBuilder) {
		eb.Variant("StatusDraft", "0")
		eb.Method("String", func(mb *gofixture.MethodBuilder) {
			mb.Return(gofixture.Named("string"))
		})
	}))

	if api != nil && api.Emits(enumplugin.SurfaceEncode) {
		t.Error("the encoder was emitted with no decoder to match it")
	}
}

// A hand-written parser beside the type is enough to earn the decoder.
//
// Package-level, so the declaration cannot see it. The rule guessed
// from the renderer before, which gave a type keeping both its own
// renderer and its own parser the encoder alone.
func TestDeclaredParserEarnsTheDecoder(t *testing.T) {
	t.Parallel()

	b := gofixture.New().
		Package("blog", "example.com/blog").
		Enum("Status", func(eb *gofixture.EnumBuilder) {
			eb.Directive(gofixture.Directive(enumplugin.DirectiveName))
			eb.Underlying(gofixture.Named("int"))
			eb.Variant("StatusDraft", "0")
			eb.Method("String", func(mb *gofixture.MethodBuilder) {
				mb.Return(gofixture.Named("string"))
			})
		}).
		Function("ParseStatus", func(fb *gofixture.FunctionBuilder) {
			fb.Param("s", gofixture.Named("string"))
			fb.Return(gofixture.Named("Status"))
			fb.Return(gofixture.Named("error"))
		})
	s := b.Build()

	api, _, _ := run(t, s)
	if api == nil || !api.Emits(enumplugin.SurfaceDecode) {
		t.Error("a decoder written against an existing parser compiles, and is " +
			"the half a type carrying its own parser still owes")
	}
}

// `methods=off` leaves the checks and nothing else.
func TestMethodsOffKeepsOnlyTheChecks(t *testing.T) {
	t.Parallel()

	api, checks, _ := run(t, fixture(t, "int", func(eb *gofixture.EnumBuilder) {
		eb.Directive(gofixture.Directive(
			enumplugin.DirectiveName,
			gofixture.KV(enumplugin.MethodsKey, enumplugin.MethodsOff),
		))
		eb.Variant("StatusDraft", "0")
	}))

	t.Run("no surface is queued", func(t *testing.T) {
		t.Parallel()
		if api != nil {
			// A file carrying only a generated-by header reads as a
			// generator that failed.
			t.Error("a surface with nothing in it should not be emitted at all")
		}
	})

	t.Run("the checks are", func(t *testing.T) {
		t.Parallel()
		if checks == nil {
			t.Error("pinning a hand-written surface is the whole point of the key")
		}
	})
}

// Two variants rendering alike is refused rather than generated
// around.
func TestCollidingTextIsRefused(t *testing.T) {
	t.Parallel()

	api, _, d := run(t, fixture(t, "string", func(eb *gofixture.EnumBuilder) {
		eb.Variant("RegionUS", `"us-east"`)
		eb.Variant("RegionUSA", `"us-east"`)
	}))

	t.Run("nothing is queued", func(t *testing.T) {
		t.Parallel()
		if api != nil {
			t.Error("a parser maps text to exactly one variant, so one of these " +
				"is unreachable through it")
		}
	})

	t.Run("and it is reported", func(t *testing.T) {
		t.Parallel()
		if !d.HasErrors() {
			t.Error("the generated round trip would fail with no indication of why")
		}
	})
}

// An annotated enum with no variants has nothing to generate against.
func TestEmptySetIsReported(t *testing.T) {
	t.Parallel()

	api, _, d := run(t, fixture(t, "int", func(*gofixture.EnumBuilder) {}))
	if api != nil {
		t.Error("nothing should be queued for a declaration that was refused")
	}
	if !d.HasErrors() {
		t.Error("a directive that generated nothing should say why")
	}
}

// The generated identifiers are stamped where a second generator can
// read them.
func TestPublishesItsSurfaceNames(t *testing.T) {
	t.Parallel()

	s := fixture(t, "int", func(eb *gofixture.EnumBuilder) {
		eb.Variant("StatusDraft", "0")
	})
	if _, _, _ = run(t, s); true {
		decl, found := store.NewReader(s).Enums().
			Where(func(x *sdk.Enum) bool { return x.Name == "Status" }).First()
		if !found {
			t.Fatal("fixture lost its enum")
		}
		parse, _ := enumplugin.MetaParse.Get(decl.Meta())
		sentinel, _ := enumplugin.MetaSentinel.Get(decl.Meta())
		if parse != "ParseStatus" || sentinel != "ErrUnknownStatus" {
			t.Errorf("stamped %q and %q; a downstream generator reads these rather "+
				"than re-deriving a convention a configured word would break",
				parse, sentinel)
		}
	}
}
