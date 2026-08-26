// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescript_test

import (
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// enumOf builds an enum from name/value pairs, an empty value meaning
// the member took the implicit counter.
func enumOf(name string, pairs ...[2]string) *node.Enum {
	e := &node.Enum{Name: name}
	for _, p := range pairs {
		e.Variants = append(e.Variants, &node.EnumVariant{
			Name: p[0], Value: p[1], Owner: e,
		})
	}
	return e
}

// texts reduces a projection's variants to their rendered literals.
func texts(info emit.EnumInfo) []string {
	out := make([]string, 0, len(info.Variants))
	for _, v := range info.Variants {
		out = append(out, v.Text)
	}
	return out
}

func TestEnumOf(t *testing.T) {
	t.Parallel()

	t.Run("a string enum takes its text from the declared value", func(t *testing.T) {
		t.Parallel()
		// `Role.Admin = 'admin'` round-trips against text arriving from
		// outside the program; deriving the identifier instead only
		// round-trips against the generated parser.
		e := enumOf("Role", [2]string{"Admin", `'admin'`}, [2]string{"Guest", `"guest"`})
		got := typescript.EnumOf(e, nil)

		if got.Form != emit.EnumFormValue {
			t.Errorf("Form = %q, want value", got.Form)
		}
		want := []string{"'admin'", "'guest'"}
		for i, text := range texts(got) {
			if text != want[i] {
				t.Errorf("variant %d text = %q, want %q", i, text, want[i])
			}
		}
	})

	t.Run("a numeric enum takes its text from the identifier", func(t *testing.T) {
		t.Parallel()
		// Rendering a numeric variant as `1` says less than its name
		// does.
		e := enumOf("Level", [2]string{"Low", ""}, [2]string{"High", ""})
		got := typescript.EnumOf(e, nil)

		if got.Form != emit.EnumFormIdentifier {
			t.Errorf("Form = %q, want the identifier form", got.Form)
		}
		if texts(got)[0] != "'Low'" {
			t.Errorf("variant text = %q, want 'Low'", texts(got)[0])
		}
	})

	t.Run("a mixed enum is numeric", func(t *testing.T) {
		t.Parallel()
		// A member with no value took the counter, so the enum is
		// numeric whatever its other members were assigned.
		e := enumOf("Mixed", [2]string{"A", `'a'`}, [2]string{"B", ""})
		if got := typescript.EnumOf(e, nil); got.Form != emit.EnumFormIdentifier {
			t.Fatalf("Form = %q, want the identifier form", got.Form)
		}
	})

	t.Run("the fallback is the underlying scalar", func(t *testing.T) {
		t.Parallel()
		// A TypeScript enum member is already a string or a number, so
		// a value outside the set needs no conversion to be printed —
		// and there is no format token to pair with it.
		str := typescript.EnumOf(enumOf("S", [2]string{"A", `'a'`}), nil)
		if b, ok := str.Fallback.(*emit.BuiltinRef); !ok || b.Name != typescript.ScalarString {
			t.Errorf("string enum fallback = %+v, want string", str.Fallback)
		}
		if str.FallbackFormat != "" {
			t.Errorf("FallbackFormat = %q, want empty", str.FallbackFormat)
		}

		num := typescript.EnumOf(enumOf("N", [2]string{"A", ""}), nil)
		if b, ok := num.Fallback.(*emit.BuiltinRef); !ok || b.Name != typescript.ScalarNumber {
			t.Errorf("numeric enum fallback = %+v, want number", num.Fallback)
		}
	})

	t.Run("the zero is the member the counter starts on", func(t *testing.T) {
		t.Parallel()
		// The frontend records what the source wrote and leaves the
		// implicit values empty, so the counter is applied here.
		e := enumOf("L", [2]string{"Low", ""}, [2]string{"High", ""})
		if got := typescript.EnumOf(e, nil); got.Zero != "Low" {
			t.Errorf("Zero = %q, want Low", got.Zero)
		}

		// A declared value moves it: the counter resumes from there.
		shifted := enumOf("L", [2]string{"Low", "1"}, [2]string{"High", ""})
		if got := typescript.EnumOf(shifted, nil); got.Zero != "" {
			t.Errorf("Zero = %q, want none", got.Zero)
		}

		explicit := enumOf("L", [2]string{"Low", "3"}, [2]string{"Off", "0"})
		if got := typescript.EnumOf(explicit, nil); got.Zero != "Off" {
			t.Errorf("Zero = %q, want Off", got.Zero)
		}
	})

	t.Run("a string enum has no zero", func(t *testing.T) {
		t.Parallel()
		e := enumOf("Role", [2]string{"Admin", `'admin'`})
		if got := typescript.EnumOf(e, nil); got.Zero != "" {
			t.Fatalf("Zero = %q, want none", got.Zero)
		}
	})

	t.Run("a computed member stops the counter rather than guessing", func(t *testing.T) {
		t.Parallel()
		// `A = 1 << 2` makes every value after it unknowable, and a
		// zero derived past one would name the wrong variant.
		e := enumOf("Flags", [2]string{"A", "1 << 2"}, [2]string{"B", ""})
		got := typescript.EnumOf(e, nil)
		if got.Zero != "" {
			t.Errorf("Zero = %q, want none", got.Zero)
		}
		if got.OutOfRange != "" {
			t.Errorf("OutOfRange = %q, want none", got.OutOfRange)
		}
	})

	t.Run("a shared textual form is reported", func(t *testing.T) {
		t.Parallel()
		// A parser maps text to one variant, so a collision makes the
		// other unreachable — and the generated round trip then fails
		// without naming the cause.
		e := enumOf("Dup", [2]string{"A", `'x'`}, [2]string{"B", `"x"`})
		if got := typescript.EnumOf(e, nil); got.Duplicate != "'x'" {
			t.Fatalf("Duplicate = %q, want 'x'", got.Duplicate)
		}
	})

	t.Run("the unknown probe stays outside the declared set", func(t *testing.T) {
		t.Parallel()
		e := enumOf("Role", [2]string{"Admin", `'admin'`})
		got := typescript.EnumOf(e, nil)
		if got.UnknownText == "" {
			t.Fatal("no unknown probe was derived")
		}
		for _, v := range got.Variants {
			if v.Text == got.UnknownText {
				t.Fatalf("the probe %q is a declared variant", got.UnknownText)
			}
		}
	})

	t.Run("a set containing the probe gets none", func(t *testing.T) {
		t.Parallel()
		// The one case where the probe would assert the opposite of
		// what it means.
		e := enumOf("Odd", [2]string{"A", `'__eidos_unknown__'`})
		if got := typescript.EnumOf(e, nil); got.UnknownText != "" {
			t.Fatalf("UnknownText = %q, want none", got.UnknownText)
		}
	})

	t.Run("out of range is one past the highest member", func(t *testing.T) {
		t.Parallel()
		e := enumOf("L", [2]string{"A", ""}, [2]string{"B", "5"}, [2]string{"C", ""})
		if got := typescript.EnumOf(e, nil); got.OutOfRange != "7" {
			t.Fatalf("OutOfRange = %q, want 7", got.OutOfRange)
		}
	})

	t.Run("a string enum has no boundary", func(t *testing.T) {
		t.Parallel()
		// Its values have no ordering to be past, so the checks that
		// need one are dropped rather than written against a guess.
		e := enumOf("Role", [2]string{"Admin", `'admin'`})
		if got := typescript.EnumOf(e, nil); got.OutOfRange != "" {
			t.Fatalf("OutOfRange = %q, want none", got.OutOfRange)
		}
	})

	t.Run("no variant of this type is declared elsewhere", func(t *testing.T) {
		t.Parallel()
		// Declaration merging can add members to an enum, but only
		// within one module — so this is empty rather than unknown, and
		// the loose constants a caller holds contribute nothing.
		e := enumOf("Role", [2]string{"Admin", `'admin'`})
		got := typescript.EnumOf(e, []*node.Constant{{Name: "Elsewhere"}})
		if len(got.Foreign) != 0 {
			t.Fatalf("Foreign = %v, want empty", got.Foreign)
		}
	})

	t.Run("nil and an empty enum project to nothing", func(t *testing.T) {
		t.Parallel()
		if got := typescript.EnumOf(nil, nil); got.Form != emit.EnumFormIdentifier || got.Variants != nil {
			t.Errorf("EnumOf(nil) = %+v", got)
		}
		if got := typescript.EnumOf(&node.Enum{Name: "E"}, nil); len(got.Variants) != 0 {
			t.Errorf("an empty enum projected %d variants", len(got.Variants))
		}
	})

	t.Run("a nameless variant is skipped", func(t *testing.T) {
		t.Parallel()
		e := enumOf("E", [2]string{"", ""}, [2]string{"A", ""})
		if got := typescript.EnumOf(e, nil); len(got.Variants) != 1 {
			t.Fatalf("projected %d variants, want 1", len(got.Variants))
		}
	})
}
