// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"errors"
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

// TestIsWellFormedLiteral pins the line between what this refuses and
// what it hands to the consumer's compiler.
//
// The failure it prevents is the one with no attribution: an
// unbalanced quote stamped into generated source does not fail at the
// directive that carried it, it fails as a syntax error somewhere else
// in a file the author never wrote.
func TestIsWellFormedLiteral(t *testing.T) {
	t.Parallel()

	accepted := []struct{ name, src string }{
		{"an integer", "42"},
		{"a negative float", "-1.5"},
		{"a quoted string", `"hello"`},
		{"a string carrying an escaped quote", `"say \"hi\""`},
		{"a raw string", "`raw`"},
		{"a raw string carrying a quote", "`he said \"hi\"`"},
		{"a rune", `'a'`},
		{"an escaped rune", `'\n'`},
		{"a quote rune", `'\''`},
		{"a named constant", "MaxRetries"},
		{"a qualified constant", "time.Second"},
		{"a conversion", "time.Duration(5)"},
		{"a composite literal", `map[string]int{"a": 1}`},
		{"a concatenation", `"a" + "b"`},
		{"a boolean", "true"},
		{"nil", "nil"},
	}
	for _, tc := range accepted {
		t.Run("accepts "+tc.name, func(t *testing.T) {
			t.Parallel()
			// Everything the check cannot resolve goes to the compiler,
			// which can. Refusing these would reject values authors
			// legitimately write.
			if err := golang.IsWellFormedLiteral(tc.src); err != nil {
				t.Fatalf("IsWellFormedLiteral(%q) = %v, want accepted", tc.src, err)
			}
		})
	}

	refused := []struct{ name, src string }{
		{"an empty value", ""},
		{"whitespace only", "   "},
		{"an unterminated string", `"hello`},
		{"an unterminated raw string", "`raw"},
		{"an unterminated rune", `'a`},
		{"a string closed only by an escaped quote", `"hi\"`},
	}
	for _, tc := range refused {
		t.Run("refuses "+tc.name, func(t *testing.T) {
			t.Parallel()
			err := golang.IsWellFormedLiteral(tc.src)
			if !errors.Is(err, golang.ErrMalformedLiteral) {
				t.Fatalf("IsWellFormedLiteral(%q) = %v, want ErrMalformedLiteral", tc.src, err)
			}
		})
	}

	t.Run("a rune's escaped quote does not close it early", func(t *testing.T) {
		t.Parallel()
		// The character the private copies forgot. Read as closing, the
		// remainder of the value is scanned as if unquoted and a later
		// quote pairs with nothing.
		if err := golang.IsWellFormedLiteral(`'\''`); err != nil {
			t.Fatalf("IsWellFormedLiteral: %v", err)
		}
	})
}

// TestLiteralFor covers the step a tag value needs and a directive
// value does not.
//
// Go's tag grammar consumes one layer of quoting to delimit its own
// entry, so the text that reaches a consumer is the bare form an
// author would have written unquoted. That is the right literal for a
// number and the wrong one for a string, and the member's type is the
// only thing that says which.
func TestLiteralFor(t *testing.T) {
	t.Parallel()

	named := func(n string) *node.TypeRef {
		return &node.TypeRef{TypeKind: node.TypeRefNamed, Name: n}
	}

	t.Run("a textual member quotes bare text", func(t *testing.T) {
		t.Parallel()
		// The case that shipped broken: stamped verbatim this names an
		// identifier, and the consumer's build fails on a symbol
		// nobody declared.
		got, ok := golang.LiteralFor(named("string"), "localhost", nil)
		if !ok || got != `"localhost"` {
			t.Errorf("got (%q, %v), want a quoted literal", got, ok)
		}
	})

	t.Run("a textual member passes an already-quoted value through", func(t *testing.T) {
		t.Parallel()
		// An author who wrote the escaped form gets what they wrote,
		// rather than a second layer of quoting around it.
		got, ok := golang.LiteralFor(named("string"), `"localhost"`, nil)
		if !ok || got != `"localhost"` {
			t.Errorf("got (%q, %v), want the value unchanged", got, ok)
		}
	})

	t.Run("a numeric member passes a number through", func(t *testing.T) {
		t.Parallel()
		got, ok := golang.LiteralFor(named("int"), "8080", nil)
		if !ok || got != "8080" {
			t.Errorf("got (%q, %v), want the number unchanged", got, ok)
		}
	})

	t.Run("a numeric member refuses a word", func(t *testing.T) {
		t.Parallel()
		// Refused rather than quoted: a string is not a value an int
		// admits, and the caller reports it at the declaration.
		if _, ok := golang.LiteralFor(named("int"), "localhost!", nil); ok {
			t.Error("an int member accepted text that is not a number")
		}
	})

	t.Run("a numeric member keeps a named constant", func(t *testing.T) {
		t.Parallel()
		// A name is a reference, and the scope it resolves in is not
		// visible here — so refusing it would reject what an author
		// legitimately writes.
		got, ok := golang.LiteralFor(named("int"), "MaxRetries", nil)
		if !ok || got != "MaxRetries" {
			t.Errorf("got (%q, %v), want the identifier unchanged", got, ok)
		}
	})

	t.Run("a bool member takes only true or false", func(t *testing.T) {
		t.Parallel()
		if _, ok := golang.LiteralFor(named("bool"), "true", nil); !ok {
			t.Error("a bool member refused true")
		}
		if _, ok := golang.LiteralFor(named("bool"), "yes", nil); ok {
			t.Error("a bool member accepted a word that is not a bool")
		}
	})

	t.Run("an empty value is refused whatever the type", func(t *testing.T) {
		t.Parallel()
		// An empty tag is not a declared default, and stamping one
		// would be indistinguishable from having declared nothing.
		if _, ok := golang.LiteralFor(named("string"), "", nil); ok {
			t.Error("an empty value was accepted")
		}
	})
}
