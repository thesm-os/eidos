// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/emit"
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

// A float-backed enum used to lose its out-of-range probe silently:
// every value went through ParseIntValue, the first non-integer
// returned false for the whole set, and the generator dropped the
// subtest that catches a missing `default:`. The rest of the library
// was already float-aware — FormatVerb picks %g for these types — so
// one half of the numeric vocabulary answered and the other did not.
func TestFloatEnumValues(t *testing.T) {
	t.Parallel()

	ratio := func() *node.Enum {
		return &node.Enum{
			Name: "Ratio", Package: "x",
			Underlying: builtinRef("float64"),
			Variants: []*node.EnumVariant{
				{Name: "RatioQuarter", Value: "0.25"},
				{Name: "RatioHalf", Value: "0.5"},
				{Name: "RatioFull", Value: "1.0"},
			},
		}
	}

	t.Run("reads every declared value", func(t *testing.T) {
		t.Parallel()
		got, ok := golang.EnumFloatValues(ratio())
		if !ok || !slices.Equal(got, []float64{0.25, 0.5, 1.0}) {
			t.Fatalf("EnumFloatValues = %v, %v", got, ok)
		}
	})

	t.Run("declines the set when one value does not read", func(t *testing.T) {
		t.Parallel()
		// All-or-nothing: a bound over the variants that happened to
		// parse is a bound over a set the source does not declare. The
		// exact-rational spelling a type checker folds a division into
		// is the case that reaches this.
		e := ratio()
		e.Variants[1].Value = "1/2"
		if _, ok := golang.EnumFloatValues(e); ok {
			t.Fatal("EnumFloatValues read a set carrying an exact rational")
		}
	})

	t.Run("derives a value past the largest", func(t *testing.T) {
		t.Parallel()
		// No walk and no saturation case: a float set cannot exhaust
		// its type, so the largest plus one is always outside it.
		got, ok := golang.OutOfRangeFloat(ratio())
		if !ok || got != "2" {
			t.Fatalf("OutOfRangeFloat = %q, %v; want 2", got, ok)
		}
	})

	t.Run("spells it the way FormatVerb prints it", func(t *testing.T) {
		t.Parallel()
		// The probe and the failure message reporting it have to agree,
		// or a check reads as failing on a digit the generator chose.
		e := ratio()
		e.Variants = append(e.Variants, &node.EnumVariant{Name: "RatioOdd", Value: "2.5"})
		got, _ := golang.OutOfRangeFloat(e)
		if got != "3.5" {
			t.Fatalf("OutOfRangeFloat = %q, want 3.5", got)
		}
	})

	t.Run("declines an integer set", func(t *testing.T) {
		t.Parallel()
		// `1` and `2` parse as floats, so the guard is the declared
		// underlying type — otherwise an int enum gets a float spelling.
		e := &node.Enum{
			Name: "Status", Package: "x",
			Underlying: builtinRef("int"),
			Variants:   []*node.EnumVariant{{Name: "A", Value: "1"}},
		}
		if _, ok := golang.OutOfRangeFloat(e); ok {
			t.Fatal("OutOfRangeFloat answered for an integer set")
		}
	})

	t.Run("declines a string set", func(t *testing.T) {
		t.Parallel()
		e := &node.Enum{
			Name: "Region", Package: "x",
			Underlying: builtinRef("string"),
			Variants:   []*node.EnumVariant{{Name: "US", Value: `"us-east"`}},
		}
		if _, ok := golang.OutOfRangeFloat(e); ok {
			t.Fatal("OutOfRangeFloat answered for a string set")
		}
	})
}

// The form a generator wants: a probe is rendered into source, so
// asking which numeric kind the set is declared in before asking for
// a value outside it is a question about the library rather than
// about the enum.
func TestOutOfRangeLiteral(t *testing.T) {
	t.Parallel()

	t.Run("answers for an integer set", func(t *testing.T) {
		t.Parallel()
		e := &node.Enum{
			Name: "Status", Package: "x",
			Underlying: builtinRef("int"),
			Variants: []*node.EnumVariant{
				{Name: "Draft", Value: "1"}, {Name: "Published", Value: "2"},
			},
		}
		if got, ok := golang.OutOfRangeLiteral(e); !ok || got != "3" {
			t.Fatalf("OutOfRangeLiteral = %q, %v; want 3", got, ok)
		}
	})

	t.Run("answers for a float set", func(t *testing.T) {
		t.Parallel()
		e := &node.Enum{
			Name: "Ratio", Package: "x",
			Underlying: builtinRef("float64"),
			Variants: []*node.EnumVariant{
				{Name: "Half", Value: "0.5"}, {Name: "Full", Value: "1"},
			},
		}
		if got, ok := golang.OutOfRangeLiteral(e); !ok || got != "2" {
			t.Fatalf("OutOfRangeLiteral = %q, %v; want 2", got, ok)
		}
	})

	t.Run("takes the integer reading where both parse", func(t *testing.T) {
		t.Parallel()
		// `1` and `2` are legal floats. A set declared over int means
		// the narrower one, and answering `3` rather than `3` spelled
		// as a float is what keeps the probe's type right.
		e := &node.Enum{
			Name: "Status", Package: "x",
			Underlying: builtinRef("int8"),
			Variants:   []*node.EnumVariant{{Name: "A", Value: "1"}},
		}
		got, _ := golang.OutOfRangeLiteral(e)
		if strings.Contains(got, ".") {
			t.Fatalf("OutOfRangeLiteral = %q, want an integer spelling", got)
		}
	})

	t.Run("declines a set saturating its type", func(t *testing.T) {
		t.Parallel()
		// The integer path reports no value outside the set, and the
		// float path declines an integer set — so neither answers,
		// which is the honest result.
		e := &node.Enum{
			Name: "Full", Package: "x",
			Underlying: builtinRef("int8"),
			Variants:   make([]*node.EnumVariant, 0, 256),
		}
		for i := -128; i <= 127; i++ {
			e.Variants = append(e.Variants, &node.EnumVariant{
				Name: "V" + strconv.Itoa(i+128), Value: strconv.Itoa(i),
			})
		}
		if got, ok := golang.OutOfRangeLiteral(e); ok {
			t.Fatalf("OutOfRangeLiteral = %q for a saturated set", got)
		}
	})
}

// TestEnumFallback pins the pairing of the out-of-set conversion
// with the verb that prints it.
//
// The two have drifted apart independently in two generators in this
// workspace, and both times the output compiled: `%d` against a
// `float64` is a vet finding in the consuming repository, and an
// unqualified conversion of a cross-package underlying type is a
// build failure there. Neither is visible in the generator's own
// tests, which is why the pairing is asserted here rather than left
// to the caller.
func TestEnumFallback(t *testing.T) {
	t.Parallel()

	over := func(underlying *node.TypeRef) *node.Enum {
		return &node.Enum{Name: "Status", Package: "example.com/x", Underlying: underlying}
	}

	assertBuiltin := func(t *testing.T, ref emit.Ref, want string) {
		t.Helper()
		b, ok := ref.(*emit.BuiltinRef)
		if !ok {
			t.Fatalf("conversion is %T, want *emit.BuiltinRef", ref)
		}
		if b.Name != want {
			t.Fatalf("conversion = %q, want %q", b.Name, want)
		}
	}

	t.Run("a builtin underlying converts through itself", func(t *testing.T) {
		t.Parallel()
		conv, verb := golang.EnumFallback(over(builtinRef("int64")))
		assertBuiltin(t, conv, "int64")
		if verb != "%d" {
			t.Fatalf("verb = %q, want %%d", verb)
		}
	})

	t.Run("a float set takes the float verb", func(t *testing.T) {
		t.Parallel()
		// The regression: %d against a float64 renders
		// %!d(float64=0.5), which go vet reports in the consumer's
		// repository, where nobody wrote it.
		conv, verb := golang.EnumFallback(over(builtinRef("float64")))
		assertBuiltin(t, conv, "float64")
		if verb != "%g" {
			t.Fatalf("verb = %q, want %%g", verb)
		}
	})

	t.Run("a string set takes the quoting verb", func(t *testing.T) {
		t.Parallel()
		conv, verb := golang.EnumFallback(over(builtinRef("string")))
		assertBuiltin(t, conv, "string")
		if verb != "%q" {
			t.Fatalf("verb = %q, want %%q", verb)
		}
	})

	t.Run("a cross-package underlying converts through a qualified reference", func(t *testing.T) {
		t.Parallel()
		// The whole reason the conversion is a ref rather than a name:
		// composed as text from EnumUnderlying this renders `Status(v)`,
		// naming a type the generated file never imported.
		conv, verb := golang.EnumFallback(over(namedTypeRef("example.com/cfg", "Status")))
		ext, ok := conv.(*emit.ExternalRef)
		if !ok {
			t.Fatalf("conversion is %T, want *emit.ExternalRef", conv)
		}
		if ext.Package != "example.com/cfg" || ext.Name != "Status" {
			t.Fatalf("conversion = %s.%s, want example.com/cfg.Status", ext.Package, ext.Name)
		}
		if verb != "%v" {
			t.Fatalf("verb = %q, want %%v", verb)
		}
	})

	t.Run("a set recording no underlying type converts through int", func(t *testing.T) {
		t.Parallel()
		// The same assumption OutOfRangeValue bounds such a set with: a
		// Go const group with no explicit type is an untyped integer.
		conv, verb := golang.EnumFallback(over(nil))
		assertBuiltin(t, conv, "int")
		if verb != "%d" {
			t.Fatalf("verb = %q, want %%d", verb)
		}
	})

	t.Run("a nil enum answers rather than yielding a nil ref", func(t *testing.T) {
		t.Parallel()
		// The conversion is total, so there is no absent answer to
		// report — and a nil ref is the one thing a caller cannot
		// render.
		conv, verb := golang.EnumFallback(nil)
		assertBuiltin(t, conv, "int")
		if verb != "%d" {
			t.Fatalf("verb = %q, want %%d", verb)
		}
	})
}

// TestForeignVariants pins the frontend fact a consumer otherwise has
// to know: constants coalesce into an enum only within one package.
//
// A `const Extra cfg.Status = 3` in another package is legal Go and
// never reaches Variants, so every generated answer about the set is
// then confidently false — and nothing in the run says so.
func TestForeignVariants(t *testing.T) {
	t.Parallel()

	status := func() *node.Enum {
		return &node.Enum{Name: "Status", Package: "example.com/cfg", Underlying: builtinRef("int")}
	}
	constOf := func(pkg, name string, typ *node.TypeRef) *node.Constant {
		return &node.Constant{Name: name, Package: pkg, Type: typ}
	}
	cfgStatus := func() *node.TypeRef { return namedTypeRef("example.com/cfg", "Status") }

	t.Run("names the packages declaring constants of the type elsewhere", func(t *testing.T) {
		t.Parallel()
		got := golang.ForeignVariants(status(), []*node.Constant{
			constOf("example.com/other", "StatusExtra", cfgStatus()),
			constOf("example.com/third", "StatusMore", cfgStatus()),
		})
		if !slices.Equal(got, []string{"example.com/other", "example.com/third"}) {
			t.Fatalf("ForeignVariants = %v", got)
		}
	})

	t.Run("sorts, so the diagnostic is the same on every run", func(t *testing.T) {
		t.Parallel()
		// Map iteration order would make one source produce two
		// different messages, which reads as the source having changed.
		got := golang.ForeignVariants(status(), []*node.Constant{
			constOf("example.com/zulu", "A", cfgStatus()),
			constOf("example.com/alpha", "B", cfgStatus()),
		})
		if !slices.IsSorted(got) {
			t.Fatalf("ForeignVariants = %v, want sorted", got)
		}
	})

	t.Run("reports one package once however many constants it declares", func(t *testing.T) {
		t.Parallel()
		got := golang.ForeignVariants(status(), []*node.Constant{
			constOf("example.com/other", "A", cfgStatus()),
			constOf("example.com/other", "B", cfgStatus()),
		})
		if len(got) != 1 {
			t.Fatalf("ForeignVariants = %v, want one entry", got)
		}
	})

	t.Run("ignores the enum's own package", func(t *testing.T) {
		t.Parallel()
		// A constant beside the enum was coalesced into it, or was
		// deliberately left out; either way it is not the cross-package
		// blindness this reports.
		got := golang.ForeignVariants(status(), []*node.Constant{
			constOf("example.com/cfg", "StatusDraft", cfgStatus()),
		})
		if got != nil {
			t.Fatalf("ForeignVariants = %v, want nil", got)
		}
	})

	t.Run("ignores a constant of another type", func(t *testing.T) {
		t.Parallel()
		got := golang.ForeignVariants(status(), []*node.Constant{
			constOf("example.com/other", "Limit", builtinRef("int")),
			constOf("example.com/other", "Tier", namedTypeRef("example.com/cfg", "Level")),
		})
		if got != nil {
			t.Fatalf("ForeignVariants = %v, want nil — neither constant is of Status", got)
		}
	})

	t.Run("survives a nil enum and a nil entry", func(t *testing.T) {
		t.Parallel()
		if got := golang.ForeignVariants(nil, []*node.Constant{constOf("x", "A", cfgStatus())}); got != nil {
			t.Fatalf("ForeignVariants(nil) = %v", got)
		}
		if got := golang.ForeignVariants(status(), []*node.Constant{nil}); got != nil {
			t.Fatalf("ForeignVariants with a nil entry = %v", got)
		}
	})
}

// TestEnumFloatValues_Refusals pins the all-or-nothing contract. A
// bound derived from part of a set is a bound over the variants that
// happened to parse, which is worse than no bound at all.
func TestEnumFloatValues_Refusals(t *testing.T) {
	t.Parallel()

	t.Run("a nil enum yields nothing", func(t *testing.T) {
		t.Parallel()
		if _, ok := golang.EnumFloatValues(nil); ok {
			t.Error("a nil enum reported float values")
		}
	})

	t.Run("an enum with no variants yields nothing", func(t *testing.T) {
		t.Parallel()
		e := &node.Enum{Name: "Ratio", Package: "x", Underlying: builtinRef("float64")}
		if _, ok := golang.EnumFloatValues(e); ok {
			t.Error("an empty set reported float values")
		}
	})

	t.Run("a nil variant refuses the whole set", func(t *testing.T) {
		t.Parallel()
		e := &node.Enum{
			Name: "Ratio", Package: "x", Underlying: builtinRef("float64"),
			Variants: []*node.EnumVariant{{Name: "Half", Value: "0.5"}, nil},
		}
		if _, ok := golang.EnumFloatValues(e); ok {
			t.Error("a set carrying a nil variant reported float values")
		}
	})
}
