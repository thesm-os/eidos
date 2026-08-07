// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// numericEnum builds `type Status int` with iota-derived variants.
func numericEnum(values ...string) *node.Enum {
	e := &node.Enum{
		Name:       "Status",
		Package:    "example.com/x",
		Underlying: builtinRef("int"),
	}
	names := []string{"StatusActive", "StatusInactive", "StatusBanned", "StatusPending"}
	for i, v := range values {
		e.Variants = append(e.Variants, &node.EnumVariant{Name: names[i], Value: v, Owner: e})
	}
	return e
}

// stringEnum builds `type Region string` with declared values.
func stringEnum(pairs ...[2]string) *node.Enum {
	e := &node.Enum{
		Name:       "Region",
		Package:    "example.com/x",
		Underlying: builtinRef("string"),
	}
	for _, p := range pairs {
		e.Variants = append(e.Variants, &node.EnumVariant{Name: p[0], Value: p[1], Owner: e})
	}
	return e
}

func TestEnumForm(t *testing.T) {
	t.Parallel()

	t.Run("a string-valued enum takes its text from the value", func(t *testing.T) {
		t.Parallel()
		if got := golang.EnumFormOf(stringEnum()); got != golang.FormValue {
			t.Fatalf("EnumFormOf = %q, want value", got)
		}
	})

	t.Run("a numeric enum takes its text from the identifier", func(t *testing.T) {
		t.Parallel()
		// Its declared value is `1`, and rendering String() as "1" says
		// less than the identifier does.
		if got := golang.EnumFormOf(numericEnum("0")); got != golang.FormIdentifier {
			t.Fatalf("EnumFormOf = %q, want identifier", got)
		}
	})

	t.Run("a typeless enum is numeric", func(t *testing.T) {
		t.Parallel()
		// The only thing a Go const group without an explicit type can
		// be.
		e := &node.Enum{Name: "Status"}
		if got := golang.EnumFormOf(e); got != golang.FormIdentifier {
			t.Fatalf("EnumFormOf = %q, want identifier", got)
		}
		if got := golang.EnumUnderlying(e); got != "" {
			t.Fatalf("EnumUnderlying = %q, want empty", got)
		}
	})
}

func TestVariantText(t *testing.T) {
	t.Parallel()

	t.Run("a string enum renders its declared value", func(t *testing.T) {
		t.Parallel()
		// Deriving `US` instead discards the only thing the declaration
		// said: a value arriving from JSON no longer parses, while
		// still round-tripping against itself.
		e := stringEnum([2]string{"US", `"us-east"`})
		if got := golang.VariantText(e, e.Variants[0]); got != "us-east" {
			t.Fatalf("VariantText = %q, want us-east", got)
		}
	})

	t.Run("a numeric enum strips the type prefix", func(t *testing.T) {
		t.Parallel()
		// The type name is already context wherever the value appears,
		// and repeating it is noise in every log line.
		e := numericEnum("0")
		if got := golang.VariantText(e, e.Variants[0]); got != "Active" {
			t.Fatalf("VariantText = %q, want Active", got)
		}
	})

	t.Run("a directive override wins over both", func(t *testing.T) {
		t.Parallel()
		e := stringEnum([2]string{"US", `"us-east"`})
		e.Variants[0].DirectiveList = append(e.Variants[0].DirectiveList,
			&directive.Directive{Name: "value", Args: []string{"US-EAST-1"}})
		if got := golang.VariantText(e, e.Variants[0]); got != "US-EAST-1" {
			t.Fatalf("VariantText = %q, want the override", got)
		}
		if reason, ok := golang.VariantOverride(e.Variants[0]); !ok || reason != "US-EAST-1" {
			t.Fatalf("VariantOverride = %q, %v", reason, ok)
		}
	})

	t.Run("an unquotable string value falls back to the raw form", func(t *testing.T) {
		t.Parallel()
		// The raw form is what the declaration says; the identifier is
		// not.
		e := stringEnum([2]string{"US", "prefix + suffix"})
		if got := golang.VariantText(e, e.Variants[0]); got != "prefix + suffix" {
			t.Fatalf("VariantText = %q", got)
		}
	})

	t.Run("the literal form is quoted with escaping", func(t *testing.T) {
		t.Parallel()
		// An authored override is arbitrary text, and a template
		// concatenating quotes produces a literal that truncates.
		e := stringEnum([2]string{"US", `"a\"b"`})
		if got := golang.EnumTextLiteral(e, e.Variants[0]); got != `"a\"b"` {
			t.Fatalf("EnumTextLiteral = %q", got)
		}
	})

	t.Run("nil yields nothing", func(t *testing.T) {
		t.Parallel()
		if got := golang.VariantText(nil, nil); got != "" {
			t.Fatalf("VariantText(nil) = %q", got)
		}
	})
}

func TestDuplicateText(t *testing.T) {
	t.Parallel()

	t.Run("finds a collision", func(t *testing.T) {
		t.Parallel()
		// Parse maps text to exactly one variant, so a collision makes
		// one unreachable and the generated round-trip fails with no
		// indication of the cause.
		e := stringEnum([2]string{"A", `"x"`}, [2]string{"B", `"x"`})
		got, dup := golang.DuplicateText(e)
		if !dup || got != "x" {
			t.Fatalf("DuplicateText = %q, %v", got, dup)
		}
	})

	t.Run("a distinct set has none", func(t *testing.T) {
		t.Parallel()
		e := numericEnum("0", "1")
		if _, dup := golang.DuplicateText(e); dup {
			t.Fatalf("DuplicateText reported a collision")
		}
	})
}

func TestZeroVariant(t *testing.T) {
	t.Parallel()

	t.Run("finds the numeric zero", func(t *testing.T) {
		t.Parallel()
		e := numericEnum("0", "1")
		got, ok := golang.ZeroVariant(e)
		if !ok || got.Name != "StatusActive" {
			t.Fatalf("ZeroVariant = %v, %v", got, ok)
		}
	})

	t.Run("finds the empty string", func(t *testing.T) {
		t.Parallel()
		e := stringEnum([2]string{"Unset", `""`}, [2]string{"US", `"us"`})
		got, ok := golang.ZeroVariant(e)
		if !ok || got.Name != "Unset" {
			t.Fatalf("ZeroVariant = %v, %v", got, ok)
		}
	})

	t.Run("an enum with no zero has an unnameable default", func(t *testing.T) {
		t.Parallel()
		// A fact a generated validity check has to state rather than
		// assume.
		if _, ok := golang.ZeroVariant(numericEnum("1", "2")); ok {
			t.Fatalf("ZeroVariant found one where none is declared")
		}
	})
}

func TestEnumValues(t *testing.T) {
	t.Parallel()

	t.Run("reads a numeric set", func(t *testing.T) {
		t.Parallel()
		got, ok := golang.EnumValues(numericEnum("0", "1", "2"))
		if !ok || !slices.Equal(got, []int64{0, 1, 2}) {
			t.Fatalf("EnumValues = %v, %v", got, ok)
		}
	})

	t.Run("one unreadable value kills the whole set", func(t *testing.T) {
		t.Parallel()
		// A partial read would compute a maximum over the variants that
		// happened to parse.
		if _, ok := golang.EnumValues(numericEnum("0", "iota + 1")); ok {
			t.Fatalf("EnumValues succeeded on a partial set")
		}
	})

	t.Run("a string enum reads as none", func(t *testing.T) {
		t.Parallel()
		if _, ok := golang.EnumValues(stringEnum([2]string{"US", `"us"`})); ok {
			t.Fatalf("EnumValues succeeded on a string enum")
		}
	})
}

func TestOutOfRange(t *testing.T) {
	t.Parallel()

	t.Run("a numeric enum takes one past the largest", func(t *testing.T) {
		t.Parallel()
		got, ok := golang.OutOfRangeValue(numericEnum("0", "1", "2"))
		if !ok || got != 3 {
			t.Fatalf("OutOfRangeValue = %d, %v; want 3", got, ok)
		}
	})

	t.Run("a string enum has no numeric answer", func(t *testing.T) {
		t.Parallel()
		if _, ok := golang.OutOfRangeValue(stringEnum([2]string{"US", `"us"`})); ok {
			t.Fatalf("OutOfRangeValue answered for a string enum")
		}
	})

	t.Run("the text marker is checked against the declared set", func(t *testing.T) {
		t.Parallel()
		// A corpus containing the marker would otherwise produce a
		// check that passes for the wrong reason.
		got, ok := golang.OutOfRangeText(stringEnum([2]string{"US", `"us"`}))
		if !ok || got == "us" {
			t.Fatalf("OutOfRangeText = %q, %v", got, ok)
		}
		colliding := stringEnum([2]string{"Marker", `"` + got + `"`})
		if _, ok := golang.OutOfRangeText(colliding); ok {
			t.Fatalf("OutOfRangeText returned a value the set declares")
		}
	})

	t.Run("an empty enum has no answer", func(t *testing.T) {
		t.Parallel()
		if _, ok := golang.OutOfRangeText(numericEnum()); ok {
			t.Fatalf("OutOfRangeText answered for an empty enum")
		}
	})
}

func TestEnumMethods(t *testing.T) {
	t.Parallel()

	t.Run("offers every method the type does not declare", func(t *testing.T) {
		t.Parallel()
		got := golang.EnumMethods(numericEnum("0"))
		if !slices.Contains(got, golang.MethodString) {
			t.Fatalf("EnumMethods = %v, want String among them", got)
		}
	})

	t.Run("an author's own method is skipped silently", func(t *testing.T) {
		t.Parallel()
		// An author who wrote their own String meant to keep it, and a
		// generator that refused to run until they deleted it would be
		// demanding they give up the more specific statement.
		e := numericEnum("0")
		e.Methods = append(e.Methods, &node.Method{
			Name:    golang.MethodString,
			Returns: []*node.Return{{Type: builtinRef("string")}},
		})
		if slices.Contains(golang.EnumMethods(e), golang.MethodString) {
			t.Fatalf("EnumMethods offered a method the type declares")
		}
		if !golang.EnumDeclares(e, golang.MethodString) {
			t.Fatalf("EnumDeclares = false")
		}
	})
}

func TestIsIotaDerived(t *testing.T) {
	t.Parallel()

	t.Run("a consecutive set from zero is iota-derived", func(t *testing.T) {
		t.Parallel()
		// Which licenses a generated IsValid to be a range check.
		if !golang.IsIotaDerived(numericEnum("0", "1", "2")) {
			t.Fatalf("IsIotaDerived = false")
		}
	})

	t.Run("a gap is not", func(t *testing.T) {
		t.Parallel()
		// A gap is usually a deleted variant whose value a wire format
		// still carries, and a range check would admit it.
		if golang.IsIotaDerived(numericEnum("0", "2")) {
			t.Fatalf("IsIotaDerived = true for a gapped set")
		}
	})

	t.Run("a set not starting at zero is not", func(t *testing.T) {
		t.Parallel()
		if golang.IsIotaDerived(numericEnum("1", "2")) {
			t.Fatalf("IsIotaDerived = true for a set starting at one")
		}
	})

	t.Run("a string enum is not", func(t *testing.T) {
		t.Parallel()
		if golang.IsIotaDerived(stringEnum([2]string{"US", `"us"`})) {
			t.Fatalf("IsIotaDerived = true for a string enum")
		}
	})
}

func TestEnumNilAndEdges(t *testing.T) {
	t.Parallel()

	t.Run("every accessor tolerates nil", func(t *testing.T) {
		t.Parallel()
		if golang.EnumTexts(nil) != nil {
			t.Errorf("EnumTexts(nil) yielded entries")
		}
		if _, ok := golang.ZeroVariant(nil); ok {
			t.Errorf("ZeroVariant(nil) found one")
		}
		if _, ok := golang.EnumValues(nil); ok {
			t.Errorf("EnumValues(nil) succeeded")
		}
		if golang.EnumMethods(nil) != nil {
			t.Errorf("EnumMethods(nil) offered entries")
		}
		if golang.EnumDeclares(nil, golang.MethodString) {
			t.Errorf("EnumDeclares(nil) = true")
		}
		if _, ok := golang.VariantOverride(nil); ok {
			t.Errorf("VariantOverride(nil) found one")
		}
	})

	t.Run("a nil variant in the list is not the zero", func(t *testing.T) {
		t.Parallel()
		e := numericEnum("1")
		e.Variants = append([]*node.EnumVariant{nil}, e.Variants...)
		if _, ok := golang.ZeroVariant(e); ok {
			t.Fatalf("ZeroVariant matched a nil variant")
		}
		if _, ok := golang.EnumValues(e); ok {
			t.Fatalf("EnumValues succeeded with a nil variant")
		}
	})

	t.Run("a directive of another name is not the override", func(t *testing.T) {
		t.Parallel()
		e := numericEnum("0")
		e.Variants[0].DirectiveList = append(e.Variants[0].DirectiveList,
			&directive.Directive{Name: "other", Args: []string{"X"}},
			nil,
		)
		if _, ok := golang.VariantOverride(e.Variants[0]); ok {
			t.Fatalf("VariantOverride matched a different directive")
		}
	})

	t.Run("an override with no argument is not one", func(t *testing.T) {
		t.Parallel()
		e := numericEnum("0")
		e.Variants[0].DirectiveList = append(e.Variants[0].DirectiveList,
			&directive.Directive{Name: "value"})
		if _, ok := golang.VariantOverride(e.Variants[0]); ok {
			t.Fatalf("VariantOverride matched a directive with no value")
		}
	})

	t.Run("a typeless enum bounds its values as int", func(t *testing.T) {
		t.Parallel()
		// A const group with no explicit type is an untyped integer.
		e := &node.Enum{Name: "Status", Variants: []*node.EnumVariant{
			{Name: "StatusA", Value: "0"}, {Name: "StatusB", Value: "1"},
		}}
		got, ok := golang.OutOfRangeValue(e)
		if !ok || got != 2 {
			t.Fatalf("OutOfRangeValue = %d, %v; want 2", got, ok)
		}
	})

	t.Run("an unreadable value set has no out-of-range answer", func(t *testing.T) {
		t.Parallel()
		if _, ok := golang.OutOfRangeValue(numericEnum("iota")); ok {
			t.Fatalf("OutOfRangeValue answered for an unreadable set")
		}
	})

	t.Run("an empty enum has no iota shape", func(t *testing.T) {
		t.Parallel()
		if golang.IsIotaDerived(numericEnum()) {
			t.Fatalf("IsIotaDerived = true for an empty enum")
		}
	})
}
