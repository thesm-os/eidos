// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sentinel_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/lang/golang/golangtest/gofixture"
	sentinelplugin "go.thesmos.sh/eidos/plugins/generator/sentinel"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// The projection these tests assert on is what the seam bought.
//
// Whether a declaration takes part in the error protocol, whether its
// contract is inherited rather than declared, and whether a value has
// to be addressed to carry it were each answered privately here, from
// the declared method list alone — which finds nothing at all on the
// dominant Go idiom for a family of errors. Asking the language
// through [sdk.ErrorRules] moved the answers into one place, where a
// declaration against an expected contract is an ordinary test.

// annotated builds a package carrying the directive, configured by fn.
func annotated(t *testing.T, fn func(*gofixture.Builder)) *sdk.Store {
	t.Helper()
	b := gofixture.New().Package("blog", "example.com/blog")
	fn(b)
	b.Directive(gofixture.Directive(sentinelplugin.DirectiveName))
	s := b.Build()
	sdk.MetaFrontend.Set(b.PackageNode().EnsureMeta(), golang.Language, "test")
	return s
}

// run drives Generate over s and returns what it queued.
func run(t *testing.T, s *sdk.Store) (*sentinelplugin.Tests, *diag.Sink) {
	t.Helper()
	d := diag.Capture()
	if err := sentinelplugin.New().Generate(&sdk.GeneratorContext{
		Store: s, Reader: store.NewReader(s), Diag: d,
	}); err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, slot := range s.Emit().PendingOriginSlots() {
		if item, ok := slot.Item.(*sentinelplugin.Tests); ok {
			return item, d
		}
	}
	return nil, d
}

// typed returns the projected error type by name.
func typed(t *testing.T, tests *sentinelplugin.Tests, name string) sentinelplugin.ErrType {
	t.Helper()
	for _, e := range tests.ErrTypes {
		if e.Name == name {
			return e
		}
	}
	t.Fatalf("no error type %q in the projection", name)
	return sentinelplugin.ErrType{}
}

// Declared error values are found by the language's naming
// convention.
func TestFindsDeclaredErrorValues(t *testing.T) {
	t.Parallel()

	tests, _ := run(t, annotated(t, func(b *gofixture.Builder) {
		b.Variable("ErrNotFound", nil)
		b.Variable("ErrConflict", nil)
		b.Variable("DefaultTimeout", nil)
	}))
	if tests == nil {
		t.Fatal("plugin queued nothing for a package declaring errors")
	}

	t.Run("the conventionally named ones are taken", func(t *testing.T) {
		t.Parallel()
		if len(tests.Sentinels) != 2 {
			t.Errorf("found %d, want the two named by the convention", len(tests.Sentinels))
		}
	})

	t.Run("a variable named otherwise is not", func(t *testing.T) {
		t.Parallel()
		for _, s := range tests.Sentinels {
			if s.Name == "DefaultTimeout" {
				t.Error("a value's declared type says nothing here — every error is " +
					"the same interface, so the name is what marks one")
			}
		}
	})
}

// A contract inherited through embedding is found.
//
// The idiom the declared method list misses entirely: reading only
// what NotFoundError writes finds no message method at all, and the
// package's directive says its errors are a contract while the
// generated file covers half of them.
func TestInheritedContractIsFound(t *testing.T) {
	t.Parallel()

	tests, _ := run(t, annotated(t, func(b *gofixture.Builder) {
		b.Struct("BaseError", func(sb *gofixture.StructBuilder) {
			sb.Field("Cause", gofixture.Named("error"), nil)
			sb.Method("Error", func(mb *gofixture.MethodBuilder) {
				mb.Return(gofixture.Named("string"))
			})
			sb.Method("Unwrap", func(mb *gofixture.MethodBuilder) {
				mb.Return(gofixture.Named("error"))
			})
		})
		b.Struct("NotFoundError", func(sb *gofixture.StructBuilder) {
			sb.Embed(gofixture.Pointer(gofixture.PkgNamed(
				"example.com/blog", "BaseError",
			)))
			sb.Field("Key", gofixture.Named("string"), nil)
		})
	}))
	if tests == nil {
		t.Fatal("plugin queued nothing")
	}

	t.Run("the embedding type is recognised", func(t *testing.T) {
		t.Parallel()
		names := make([]string, 0, len(tests.ErrTypes))
		for _, e := range tests.ErrTypes {
			names = append(names, e.Name)
		}
		found := false
		for _, n := range names {
			if n == "NotFoundError" {
				found = true
			}
		}
		if !found {
			t.Errorf("projected %v; the inherited contract is the whole point of "+
				"walking what a declaration folds in", names)
		}
	})

	t.Run("a cause reached through a pointer embed is not written to", func(t *testing.T) {
		t.Parallel()
		// The zero of an embedded pointer is nil, so assigning through
		// it panics on a type whose contract is perfectly fine.
		if typed(t, tests, "NotFoundError").Cause != "" {
			t.Error("a cause behind a nil embed cannot be assigned by a check")
		}
	})

	t.Run("only members a literal can set are carried", func(t *testing.T) {
		t.Parallel()
		// A promoted member is named by a selector, and a selector is
		// not a composite-literal key.
		for _, f := range typed(t, tests, "NotFoundError").Fields {
			if f.Name == "Cause" {
				t.Error("a promoted member emitted as a literal key produces " +
					"`invalid field name` in the consumer's build")
			}
		}
	})
}

// A type declaring no protocol member is not an error type.
func TestNonErrorTypesAreDeclined(t *testing.T) {
	t.Parallel()

	tests, d := run(t, annotated(t, func(b *gofixture.Builder) {
		b.Struct("Article", func(sb *gofixture.StructBuilder) {
			sb.Field("Title", gofixture.Named("string"), nil)
		})
	}))

	t.Run("nothing is queued", func(t *testing.T) {
		t.Parallel()
		if tests != nil {
			t.Error("a file asserting nothing about an empty set reads as though " +
				"the errors had been checked")
		}
	})

	t.Run("and the directive is reported", func(t *testing.T) {
		t.Parallel()
		if !d.HasErrors() {
			t.Error("the directive says these errors are a contract; a package " +
				"with none should say so")
		}
	})
}

// The optional halves earn a check each, and a type declaring neither
// gets neither.
func TestOptionalHalvesAreDetected(t *testing.T) {
	t.Parallel()

	tests, _ := run(t, annotated(t, func(b *gofixture.Builder) {
		b.Struct("WrapError", func(sb *gofixture.StructBuilder) {
			sb.Field("Cause", gofixture.Named("error"), nil)
			sb.Method("Error", func(mb *gofixture.MethodBuilder) {
				mb.Return(gofixture.Named("string"))
			})
			sb.Method("Unwrap", func(mb *gofixture.MethodBuilder) {
				mb.Return(gofixture.Named("error"))
			})
		})
		b.Struct("PlainError", func(sb *gofixture.StructBuilder) {
			sb.Method("Error", func(mb *gofixture.MethodBuilder) {
				mb.Return(gofixture.Named("string"))
			})
		})
	}))
	if tests == nil {
		t.Fatal("plugin queued nothing")
	}

	t.Run("a declared cause accessor is seen", func(t *testing.T) {
		t.Parallel()
		if !typed(t, tests, "WrapError").Unwraps {
			t.Error("a wrapper that does not return its cause truncates every " +
				"chain it takes part in, which is what the check exists to notice")
		}
	})

	t.Run("its member is named", func(t *testing.T) {
		t.Parallel()
		if got := typed(t, tests, "WrapError").Cause; got != "Cause" {
			t.Errorf("Cause = %q, want the member holding the wrapped error", got)
		}
	})

	t.Run("a type with neither gets neither", func(t *testing.T) {
		t.Parallel()
		e := typed(t, tests, "PlainError")
		if e.Unwraps || e.Compares {
			t.Error("a vacuous check is worse than no check: it reads as though " +
				"something had been asserted")
		}
	})

	t.Run("each names the other as a peer", func(t *testing.T) {
		t.Parallel()
		if len(typed(t, tests, "PlainError").Peers) != 1 {
			t.Error("two error types that match each other collapse a caller's " +
				"branches, and the caller cannot see it from the declarations")
		}
	})
}

// The prefix resolves through the directive, the option and the
// package name, in that order.
func TestPrefixResolution(t *testing.T) {
	t.Parallel()

	withDirective := func(dir *directive.Directive) *sdk.Store {
		b := gofixture.New().Package("blog", "example.com/blog")
		b.Variable("ErrNotFound", nil)
		b.Directive(dir)
		s := b.Build()
		sdk.MetaFrontend.Set(b.PackageNode().EnsureMeta(), golang.Language, "test")
		return s
	}

	t.Run("the package name by default", func(t *testing.T) {
		t.Parallel()
		tests, _ := run(t, withDirective(
			gofixture.Directive(sentinelplugin.DirectiveName),
		))
		if tests.Prefix != "blog: " {
			t.Errorf("Prefix = %q, want the package's own name", tests.Prefix)
		}
	})

	t.Run("the directive's value where one is given", func(t *testing.T) {
		t.Parallel()
		tests, _ := run(t, withDirective(gofixture.Directive(
			sentinelplugin.DirectiveName,
			gofixture.KV(sentinelplugin.PrefixKey, "app"),
		)))
		if tests.Prefix != "app: " {
			t.Errorf("Prefix = %q, want the declared override", tests.Prefix)
		}
	})

	t.Run("suppressed rather than empty", func(t *testing.T) {
		t.Parallel()
		tests, _ := run(t, withDirective(gofixture.Directive(
			sentinelplugin.DirectiveName,
			gofixture.KV(sentinelplugin.PrefixKey, sentinelplugin.PrefixOff),
		)))
		// Every string begins with the empty string, so a check written
		// against one passes for any input and reads as though the
		// contract had been examined.
		if tests.Prefix != "" {
			t.Errorf("Prefix = %q, want the check withheld entirely", tests.Prefix)
		}
	})
}

// A no-overlap directive naming this package is refused.
func TestSelfOverlapIsRefused(t *testing.T) {
	t.Parallel()

	b := gofixture.New().Package("blog", "example.com/blog")
	b.Variable("ErrNotFound", nil)
	b.Directive(gofixture.Directive(sentinelplugin.DirectiveName))
	b.Directive(gofixture.Directive(
		sentinelplugin.NoOverlapName, gofixture.Arg("example.com/blog"),
	))
	s := b.Build()
	sdk.MetaFrontend.Set(b.PackageNode().EnsureMeta(), golang.Language, "test")

	tests, d := run(t, s)
	if len(tests.Neighbours) != 0 {
		t.Error("every value matches itself, so the check could only ever fail")
	}
	if !d.HasErrors() {
		t.Error("a directive that generates no check should say why")
	}
}
