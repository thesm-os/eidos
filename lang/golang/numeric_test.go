// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"math"
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
)

func TestNumericBounds(t *testing.T) {
	t.Parallel()

	t.Run("reports each fixed width's range", func(t *testing.T) {
		t.Parallel()
		lo, hi, ok := golang.NumericBounds("int8")
		if !ok || lo != math.MinInt8 || hi != math.MaxInt8 {
			t.Fatalf("NumericBounds(int8) = %d, %d, %v", lo, hi, ok)
		}
	})

	t.Run("an unsigned type starts at zero", func(t *testing.T) {
		t.Parallel()
		lo, _, ok := golang.NumericBounds("uint16")
		if !ok || lo != 0 {
			t.Fatalf("NumericBounds(uint16) low = %d, want 0", lo)
		}
	})

	t.Run("platform-dependent widths take the widest range", func(t *testing.T) {
		t.Parallel()
		// Generated code is compiled somewhere this tool is not, so
		// the host's width is not the target's; the wider range does
		// not reject a value the target accepts.
		_, hi, ok := golang.NumericBounds("int")
		if !ok || hi != math.MaxInt64 {
			t.Fatalf("NumericBounds(int) high = %d, want MaxInt64", hi)
		}
	})

	t.Run("a non-integer type has no range", func(t *testing.T) {
		t.Parallel()
		// The third result distinguishes this from a range that
		// happens to include zero.
		if _, _, ok := golang.NumericBounds("string"); ok {
			t.Fatalf("NumericBounds(string) reported a range")
		}
		if _, _, ok := golang.NumericBounds("float64"); ok {
			t.Fatalf("NumericBounds(float64) reported an integer range")
		}
	})

	t.Run("byte and rune report their aliases' ranges", func(t *testing.T) {
		t.Parallel()
		if _, hi, _ := golang.NumericBounds("byte"); hi != math.MaxUint8 {
			t.Fatalf("NumericBounds(byte) high = %d", hi)
		}
		if _, hi, _ := golang.NumericBounds("rune"); hi != math.MaxInt32 {
			t.Fatalf("NumericBounds(rune) high = %d", hi)
		}
	})
}

func TestFitsIn(t *testing.T) {
	t.Parallel()

	t.Run("accepts a value inside the range", func(t *testing.T) {
		t.Parallel()
		if !golang.FitsIn(127, "int8") {
			t.Fatalf("127 must fit in int8")
		}
	})

	t.Run("rejects a value past the boundary", func(t *testing.T) {
		t.Parallel()
		if golang.FitsIn(128, "int8") {
			t.Fatalf("128 must not fit in int8")
		}
	})

	t.Run("rejects a negative for an unsigned type", func(t *testing.T) {
		t.Parallel()
		if golang.FitsIn(-1, "uint8") {
			t.Fatalf("-1 must not fit in uint8")
		}
	})

	t.Run("an unrecognised type accepts nothing", func(t *testing.T) {
		t.Parallel()
		// The conservative answer: a caller that cannot prove a value
		// fits should emit nothing rather than a constant the
		// consumer's compiler rejects.
		if golang.FitsIn(0, "Weekday") {
			t.Fatalf("an unrecognised type must accept nothing")
		}
	})
}

func TestIsUnsigned(t *testing.T) {
	t.Parallel()

	t.Run("distinguishes the signed families", func(t *testing.T) {
		t.Parallel()
		for name, want := range map[string]bool{
			"uint8": true, "uint": true, "uintptr": true, "byte": true,
			"int8": false, "int": false, "rune": false,
		} {
			if golang.IsUnsigned(name) != want {
				t.Errorf("IsUnsigned(%s) = %v, want %v", name, !want, want)
			}
		}
	})
}

func TestBitSize(t *testing.T) {
	t.Parallel()

	t.Run("reports fixed widths", func(t *testing.T) {
		t.Parallel()
		for name, want := range map[string]int{
			"int8": 8, "int32": 32, "rune": 32, "byte": 8,
			"float64": 64, "complex128": 128,
		} {
			if got, ok := golang.BitSize(name); !ok || got != want {
				t.Errorf("BitSize(%s) = %d, %v; want %d", name, got, ok, want)
			}
		}
	})

	t.Run("a platform-dependent width is not claimed", func(t *testing.T) {
		t.Parallel()
		// The width belongs to the target build, which is not this
		// process.
		for _, name := range []string{"int", "uint", "uintptr"} {
			if _, ok := golang.BitSize(name); ok {
				t.Errorf("BitSize(%s) claimed a width", name)
			}
		}
	})
}

func TestNextOutOfRange(t *testing.T) {
	t.Parallel()

	t.Run("takes one past the largest", func(t *testing.T) {
		t.Parallel()
		// The boundary a hand-written fallback is most likely to get
		// wrong, so it is tried first.
		got, ok := golang.NextOutOfRange("int", []int64{0, 1, 2})
		if !ok || got != 3 {
			t.Fatalf("NextOutOfRange = %d, %v; want 3", got, ok)
		}
	})

	t.Run("walks down when the largest saturates the type", func(t *testing.T) {
		t.Parallel()
		got, ok := golang.NextOutOfRange("int8", []int64{127})
		if !ok || got != 126 {
			t.Fatalf("NextOutOfRange = %d, %v; want 126", got, ok)
		}
	})

	t.Run("an empty set starts at the minimum", func(t *testing.T) {
		t.Parallel()
		got, ok := golang.NextOutOfRange("uint8", nil)
		if !ok || got != 0 {
			t.Fatalf("NextOutOfRange = %d, %v; want 0", got, ok)
		}
	})

	t.Run("values outside the type are ignored", func(t *testing.T) {
		t.Parallel()
		// A value that does not fit cannot collide with one that does.
		got, ok := golang.NextOutOfRange("int8", []int64{0, 1, 9999})
		if !ok || got != 2 {
			t.Fatalf("NextOutOfRange = %d, %v; want 2", got, ok)
		}
	})

	t.Run("an unrecognised type has no answer", func(t *testing.T) {
		t.Parallel()
		if _, ok := golang.NextOutOfRange("Weekday", nil); ok {
			t.Fatalf("NextOutOfRange must refuse an unrecognised type")
		}
	})
}

func TestFormatVerb(t *testing.T) {
	t.Parallel()

	t.Run("quotes a string so an empty value is visible", func(t *testing.T) {
		t.Parallel()
		// %v on a string loses the quoting that shows a trailing space
		// or an empty value — the difference a failing assertion is
		// trying to explain.
		if got := golang.FormatVerb(builtinRef("string")); got != "%q" {
			t.Fatalf("FormatVerb(string) = %q, want %%q", got)
		}
	})

	t.Run("picks the family verb for each numeric kind", func(t *testing.T) {
		t.Parallel()
		for name, want := range map[string]string{
			"int8": "%d", "uint": "%d", "float64": "%g", "bool": "%t",
		} {
			if got := golang.FormatVerb(builtinRef(name)); got != want {
				t.Errorf("FormatVerb(%s) = %q, want %q", name, got, want)
			}
		}
	})

	t.Run("an unrecognised type falls back", func(t *testing.T) {
		t.Parallel()
		if got := golang.FormatVerb(namedTypeRef("x", "User")); got != "%v" {
			t.Fatalf("FormatVerb(User) = %q, want %%v", got)
		}
	})
}

func TestParseValues(t *testing.T) {
	t.Parallel()

	t.Run("reads a bare integer", func(t *testing.T) {
		t.Parallel()
		if got, ok := golang.ParseIntValue("42"); !ok || got != 42 {
			t.Fatalf("ParseIntValue = %d, %v", got, ok)
		}
	})

	t.Run("refuses a non-integer rather than answering zero", func(t *testing.T) {
		t.Parallel()
		// Zero is a value a declaration may hold; the second result is
		// what tells a caller a numeric derivation is unavailable.
		for _, raw := range []string{`"us-east"`, "3.14", "iota + 1", ""} {
			if _, ok := golang.ParseIntValue(raw); ok {
				t.Errorf("ParseIntValue(%q) reported success", raw)
			}
		}
	})

	t.Run("unquotes a string constant", func(t *testing.T) {
		t.Parallel()
		// Rendered without unquoting, the value becomes
		// `return "\"us-east\""` — which compiles and is wrong.
		if got, ok := golang.ParseStringValue(`"us-east"`); !ok || got != "us-east" {
			t.Fatalf("ParseStringValue = %q, %v", got, ok)
		}
	})

	t.Run("refuses an unquoted value", func(t *testing.T) {
		t.Parallel()
		if _, ok := golang.ParseStringValue("42"); ok {
			t.Fatalf("ParseStringValue(42) reported success")
		}
	})
}

func TestNumericEdges(t *testing.T) {
	t.Parallel()

	t.Run("a saturated type has no value outside it", func(t *testing.T) {
		t.Parallel()
		// Only a set covering the whole range has no answer, and the
		// walk terminates rather than looping at the minimum.
		used := make([]int64, 0, 256)
		for v := int64(-128); v <= 127; v++ {
			used = append(used, v)
		}
		if got, ok := golang.NextOutOfRange("int8", used); ok {
			t.Fatalf("NextOutOfRange = %d, true; want no answer", got)
		}
	})

	t.Run("finds a gap below a saturated top", func(t *testing.T) {
		t.Parallel()
		if got, ok := golang.NextOutOfRange("int8", []int64{125, 126, 127}); !ok || got != 124 {
			t.Fatalf("NextOutOfRange = %d, %v; want 124", got, ok)
		}
	})
}
