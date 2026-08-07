// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// builtinRef returns a builtin reference — a named ref with no
// package — carrying a fresh meta bag.
func builtinRef(name string) *node.TypeRef {
	return namedTypeRef("", name)
}

// TestZeroLiteral_Numerics pins that every numeric builtin is
// derivable, and derives to zero rather than to nil.
//
// This is the regression the helper exists for. A private table
// covering only the common widths returned nil for the rest, and a
// generator writing `Code: nil` into a composite literal for an
// int8 field produces a file that does not compile — a failure that
// surfaces in the consumer's build, not in the generator's tests.
func TestZeroLiteral_Numerics(t *testing.T) {
	t.Parallel()

	for _, name := range []string{
		"int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"uintptr", "byte", "rune",
		"float32", "float64", "complex64", "complex128",
	} {
		t.Run(name+" derives to zero", func(t *testing.T) {
			t.Parallel()
			got, ok := golang.ZeroLiteral(builtinRef(name))
			if !ok {
				t.Fatalf("ZeroLiteral(%s) reported not derivable", name)
			}
			if got != "0" {
				t.Fatalf("ZeroLiteral(%s) = %q, want \"0\"", name, got)
			}
		})
	}
}

func TestZeroLiteral_Builtins(t *testing.T) {
	t.Parallel()

	for name, want := range map[string]string{
		"bool":   "false",
		"string": `""`,
		"error":  "nil",
		"any":    "nil",
	} {
		t.Run(name+" derives to its zero", func(t *testing.T) {
			t.Parallel()
			got, ok := golang.ZeroLiteral(builtinRef(name))
			if !ok || got != want {
				t.Fatalf("ZeroLiteral(%s) = %q, %v; want %q, true", name, got, ok, want)
			}
		})
	}
}

// TestZeroLiteral_ReferenceShapes pins the shapes whose zero is the
// nil keyword.
func TestZeroLiteral_ReferenceShapes(t *testing.T) {
	t.Parallel()

	elem := builtinRef("int")
	for name, ref := range map[string]*node.TypeRef{
		"pointer":   {TypeKind: node.TypeRefPointer, Elem: elem},
		"slice":     {TypeKind: node.TypeRefSlice, Elem: elem},
		"map":       {TypeKind: node.TypeRefMap, MapKey: builtinRef("string"), MapValue: elem},
		"func":      {TypeKind: node.TypeRefFunc},
		"interface": {TypeKind: node.TypeRefAnonInterface},
	} {
		t.Run("a "+name+" zeroes to nil", func(t *testing.T) {
			t.Parallel()
			got, ok := golang.ZeroLiteral(ref)
			if !ok || got != "nil" {
				t.Fatalf("ZeroLiteral(%s) = %q, %v; want \"nil\", true", name, got, ok)
			}
		})
	}

	t.Run("a channel zeroes to nil", func(t *testing.T) {
		t.Parallel()
		// A channel has no variant of its own — it arrives as a named
		// ref under a synthetic package, so the stamp is the only way
		// to tell it from a user type of that name.
		ch := namedTypeRef("go.chan", "chan")
		golang.MetaIsChannel.Set(ch.EnsureMeta(), true, "test")
		got, ok := golang.ZeroLiteral(ch)
		if !ok || got != "nil" {
			t.Fatalf("ZeroLiteral(chan) = %q, %v; want \"nil\", true", got, ok)
		}
	})

	t.Run("a stamped interface zeroes to nil whatever it is named", func(t *testing.T) {
		t.Parallel()
		r := namedTypeRef("io", "Reader")
		golang.MetaIsInterface.Set(r.EnsureMeta(), true, "test")
		got, ok := golang.ZeroLiteral(r)
		if !ok || got != "nil" {
			t.Fatalf("ZeroLiteral(io.Reader) = %q, %v; want \"nil\", true", got, ok)
		}
	})
}

// TestZeroLiteral_NotDerivable pins the shapes the helper refuses.
//
// Refusing is the contract's whole value. A caller that gets an
// answer for every input has no way to tell a real zero from a
// guess, and the guess is what compiles nowhere.
func TestZeroLiteral_NotDerivable(t *testing.T) {
	t.Parallel()

	t.Run("a named non-interface type is left to the caller", func(t *testing.T) {
		t.Parallel()
		// The zero of a defined numeric type is 0 and of a struct is
		// T{}; the model records a package and a name and nothing that
		// tells them apart.
		if got, ok := golang.ZeroLiteral(namedTypeRef("time", "Duration")); ok {
			t.Fatalf("ZeroLiteral(time.Duration) = %q, true; want not derivable", got)
		}
	})

	t.Run("an array needs the element spelling the render site owns", func(t *testing.T) {
		t.Parallel()
		arr := &node.TypeRef{TypeKind: node.TypeRefArray, ArrayLen: 4, Elem: builtinRef("byte")}
		if got, ok := golang.ZeroLiteral(arr); ok {
			t.Fatalf("ZeroLiteral([4]byte) = %q, true; want not derivable", got)
		}
	})

	t.Run("an anonymous struct is not derivable", func(t *testing.T) {
		t.Parallel()
		if got, ok := golang.ZeroLiteral(&node.TypeRef{TypeKind: node.TypeRefAnonStruct}); ok {
			t.Fatalf("ZeroLiteral(anon struct) = %q, true; want not derivable", got)
		}
	})

	t.Run("a type parameter is not derivable", func(t *testing.T) {
		t.Parallel()
		// `var zero T` is the only spelling, and it is a statement.
		p := &node.TypeRef{TypeKind: node.TypeRefTypeParam, Name: "T"}
		if got, ok := golang.ZeroLiteral(p); ok {
			t.Fatalf("ZeroLiteral(T) = %q, true; want not derivable", got)
		}
	})

	t.Run("an unrecognised builtin is not guessed at", func(t *testing.T) {
		t.Parallel()
		if got, ok := golang.ZeroLiteral(builtinRef("Widget")); ok {
			t.Fatalf("ZeroLiteral(Widget) = %q, true; want not derivable", got)
		}
	})

	t.Run("a nil ref is not derivable", func(t *testing.T) {
		t.Parallel()
		if got, ok := golang.ZeroLiteral(nil); ok {
			t.Fatalf("ZeroLiteral(nil) = %q, true; want not derivable", got)
		}
	})
}

func TestStructTag(t *testing.T) {
	t.Parallel()

	t.Run("renders one entry", func(t *testing.T) {
		t.Parallel()
		if got := golang.StructTag(golang.TagEntry{Key: "json", Value: "id"}); got != `json:"id"` {
			t.Fatalf("StructTag = %q, want %s", got, `json:"id"`)
		}
	})

	t.Run("preserves the supplied order", func(t *testing.T) {
		t.Parallel()
		// A struct tag is read by humans as often as by reflection,
		// and the conventional order carries meaning no sort keeps.
		got := golang.StructTag(
			golang.TagEntry{Key: "json", Value: "id"},
			golang.TagEntry{Key: "db", Value: "id_col"},
		)
		if got != `json:"id" db:"id_col"` {
			t.Fatalf("StructTag = %q, want json then db", got)
		}
	})

	t.Run("escapes a value containing a quote", func(t *testing.T) {
		t.Parallel()
		// Concatenated rather than quoted, the tag truncates at the
		// first quote and the field silently loses its remaining
		// options.
		got := golang.StructTag(golang.TagEntry{Key: "json", Value: `a"b`})
		if got != `json:"a\"b"` {
			t.Fatalf("StructTag = %q, want the quote escaped", got)
		}
	})

	t.Run("excludes the surrounding backticks", func(t *testing.T) {
		t.Parallel()
		// The backend owns the quoting of the tag as a whole; a
		// backtick here would produce a literal it cannot nest.
		if got := golang.StructTag(golang.TagEntry{Key: "json", Value: "id"}); strings.Contains(got, "`") {
			t.Fatalf("StructTag = %q, want no backticks", got)
		}
	})

	t.Run("skips an entry with no key without leaving a gap", func(t *testing.T) {
		t.Parallel()
		got := golang.StructTag(
			golang.TagEntry{Key: "json", Value: "id"},
			golang.TagEntry{},
			golang.TagEntry{Key: "db", Value: "id_col"},
		)
		if got != `json:"id" db:"id_col"` {
			t.Fatalf("StructTag = %q, want no doubled separator", got)
		}
	})

	t.Run("a leading keyless entry does not indent the first pair", func(t *testing.T) {
		t.Parallel()
		got := golang.StructTag(golang.TagEntry{}, golang.TagEntry{Key: "json", Value: "id"})
		if got != `json:"id"` {
			t.Fatalf("StructTag = %q, want no leading space", got)
		}
	})

	t.Run("no entries render nothing", func(t *testing.T) {
		t.Parallel()
		if got := golang.StructTag(); got != "" {
			t.Fatalf("StructTag() = %q, want empty", got)
		}
	})
}
