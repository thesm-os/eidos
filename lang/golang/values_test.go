// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
)

// mapResolver answers from a fixed table, standing in for the
// store-backed index a real generator supplies.
type mapResolver map[string]node.Node

func (r mapResolver) Resolve(t *node.TypeRef) (node.Node, bool) {
	if t == nil {
		return nil, false
	}
	n, ok := r[golang.QName(t)]
	return n, ok
}

func TestSampleValues(t *testing.T) {
	t.Parallel()

	t.Run("returns a distinguishable pair", func(t *testing.T) {
		t.Parallel()
		// A check comparing against a single value passes whenever the
		// subject already held it, and what it held is not always
		// knowable.
		s, a := golang.SampleValues("int", "Code")
		if s == "" || s == a {
			t.Fatalf("SampleValues = %q, %q; want a distinct pair", s, a)
		}
	})

	t.Run("a string sample carries the field's name", func(t *testing.T) {
		t.Parallel()
		// So a value appearing in a failure message says where it came
		// from.
		s, _ := golang.SampleValues("string", "Title")
		if s != `"test-title"` {
			t.Fatalf("SampleValues = %q", s)
		}
	})

	t.Run("bool exhausts its values", func(t *testing.T) {
		t.Parallel()
		// The strictest pair: code that assigned nothing fails against
		// one arm no matter what was there before.
		s, a := golang.SampleValues("bool", "Active")
		if s != "true" || a != "false" {
			t.Fatalf("SampleValues(bool) = %q, %q", s, a)
		}
	})

	t.Run("every numeric width answers", func(t *testing.T) {
		t.Parallel()
		// The narrow widths are what a partial private table omits.
		for _, name := range []string{
			"int8", "int16", "uint32", "uintptr", "byte", "rune",
			"float32", "complex128",
		} {
			if s, _ := golang.SampleValues(name, "F"); s == "" {
				t.Errorf("SampleValues(%s) yielded nothing", name)
			}
		}
	})

	t.Run("a type admitting no literal yields nothing", func(t *testing.T) {
		t.Parallel()
		// The caller's signal to omit the check rather than emit one
		// that cannot fail.
		if s, _ := golang.SampleValues("Weekday", "Day"); s != "" {
			t.Fatalf("SampleValues(Weekday) = %q, want empty", s)
		}
	})
}

func TestSampleFor(t *testing.T) {
	t.Parallel()

	t.Run("a builtin answers directly", func(t *testing.T) {
		t.Parallel()
		if s, _ := golang.SampleFor(builtinRef("int"), "Code", nil); s != "42" {
			t.Fatalf("SampleFor = %q, want 42", s)
		}
	})

	t.Run("any takes the string pair", func(t *testing.T) {
		t.Parallel()
		if s, _ := golang.SampleFor(builtinRef("any"), "V", nil); s == "" {
			t.Fatalf("SampleFor(any) yielded nothing")
		}
	})

	t.Run("a defined type keeps its own spelling", func(t *testing.T) {
		t.Parallel()
		// A bare 42 compiles today and stops compiling the moment the
		// field's type moves.
		r := mapResolver{"x.Weekday": &node.Alias{
			Name: "Weekday", Package: "x", Target: builtinRef("int"),
		}}
		s, a := golang.SampleFor(namedTypeRef("x", "Weekday"), "Day", r)
		if s != "x.Weekday(42)" || a != "x.Weekday(7)" {
			t.Fatalf("SampleFor = %q, %q", s, a)
		}
	})

	t.Run("a type the resolver cannot reach yields nothing", func(t *testing.T) {
		t.Parallel()
		if s, _ := golang.SampleFor(namedTypeRef("x", "Opaque"), "F", mapResolver{}); s != "" {
			t.Fatalf("SampleFor = %q, want empty", s)
		}
	})

	t.Run("no resolver answers for builtins only", func(t *testing.T) {
		t.Parallel()
		if s, _ := golang.SampleFor(namedTypeRef("x", "Weekday"), "Day", nil); s != "" {
			t.Fatalf("SampleFor with no resolver = %q, want empty", s)
		}
	})

	t.Run("a self-referential chain terminates", func(t *testing.T) {
		t.Parallel()
		// A defined type may name another, and the walk needs a stop.
		r := mapResolver{"x.A": &node.Alias{
			Name: "A", Package: "x", Target: namedTypeRef("x", "A"),
		}}
		if s, _ := golang.SampleFor(namedTypeRef("x", "A"), "F", r); s != "" {
			t.Fatalf("SampleFor = %q, want empty", s)
		}
	})
}

func TestZeroLiteralFor(t *testing.T) {
	t.Parallel()

	t.Run("keeps the narrow answer where one exists", func(t *testing.T) {
		t.Parallel()
		if got, ok := golang.ZeroLiteralFor(builtinRef("int"), nil); !ok || got != "0" {
			t.Fatalf("ZeroLiteralFor = %q, %v", got, ok)
		}
	})

	t.Run("resolves a defined numeric type", func(t *testing.T) {
		t.Parallel()
		// The answer ZeroLiteral refuses, available once the caller
		// supplies the graph.
		r := mapResolver{"x.Weekday": &node.Alias{
			Name: "Weekday", Package: "x", Target: builtinRef("int"),
		}}
		got, ok := golang.ZeroLiteralFor(namedTypeRef("x", "Weekday"), r)
		if !ok || got != "x.Weekday(0)" {
			t.Fatalf("ZeroLiteralFor = %q, %v", got, ok)
		}
	})

	t.Run("a struct zeroes to a composite literal", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"x.User": &node.Struct{Name: "User", Package: "x"}}
		got, ok := golang.ZeroLiteralFor(namedTypeRef("x", "User"), r)
		if !ok || got != "x.User{}" {
			t.Fatalf("ZeroLiteralFor = %q, %v", got, ok)
		}
	})

	t.Run("a named interface zeroes to nil", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"io.Reader": &node.Interface{Name: "Reader", Package: "io"}}
		got, ok := golang.ZeroLiteralFor(namedTypeRef("io", "Reader"), r)
		if !ok || got != "nil" {
			t.Fatalf("ZeroLiteralFor = %q, %v", got, ok)
		}
	})

	t.Run("an alias to a reference type needs no conversion", func(t *testing.T) {
		t.Parallel()
		// nil is already assignable to it.
		r := mapResolver{"x.Handler": &node.Alias{
			Name: "Handler", Package: "x", Target: &node.TypeRef{TypeKind: node.TypeRefFunc},
		}}
		got, ok := golang.ZeroLiteralFor(namedTypeRef("x", "Handler"), r)
		if !ok || got != "nil" {
			t.Fatalf("ZeroLiteralFor = %q, %v", got, ok)
		}
	})

	t.Run("an array zeroes once the element is spellable", func(t *testing.T) {
		t.Parallel()
		arr := &node.TypeRef{TypeKind: node.TypeRefArray, ArrayLen: 4, Elem: builtinRef("byte")}
		got, ok := golang.ZeroLiteralFor(arr, mapResolver{})
		if !ok || got != "[4]byte{}" {
			t.Fatalf("ZeroLiteralFor = %q, %v", got, ok)
		}
	})

	t.Run("an unresolvable type stays underivable", func(t *testing.T) {
		t.Parallel()
		if _, ok := golang.ZeroLiteralFor(namedTypeRef("x", "Opaque"), mapResolver{}); ok {
			t.Fatalf("ZeroLiteralFor must refuse a type the resolver cannot reach")
		}
	})
}

func TestParseTag(t *testing.T) {
	t.Parallel()

	t.Run("reads every entry", func(t *testing.T) {
		t.Parallel()
		got := golang.ParseTag("`json:\"id\" db:\"id_col\"`")
		if got["json"] != "id" || got["db"] != "id_col" {
			t.Fatalf("ParseTag = %v", got)
		}
	})

	t.Run("trims the backticks the source carries", func(t *testing.T) {
		t.Parallel()
		// A caller that forgets sees the first key start with a
		// backtick and every lookup miss.
		withTicks := golang.ParseTag("`json:\"id\"`")
		without := golang.ParseTag(`json:"id"`)
		if withTicks["json"] != without["json"] {
			t.Fatalf("backtick handling differs: %v vs %v", withTicks, without)
		}
	})

	t.Run("an unparseable tag yields an empty map, not nil", func(t *testing.T) {
		t.Parallel()
		// So a caller ranges over the result without a guard.
		got := golang.ParseTag("not a tag")
		if got == nil {
			t.Fatalf("ParseTag returned nil")
		}
	})

	t.Run("an absent key is told apart from an empty one", func(t *testing.T) {
		t.Parallel()
		// `json:""` is a field deliberately left unnamed, which is not
		// the same as a field carrying no json tag.
		if v, ok := golang.TagValue("`json:\"\"`", "json"); !ok || v != "" {
			t.Fatalf("TagValue = %q, %v; want empty and present", v, ok)
		}
		if _, ok := golang.TagValue("`db:\"x\"`", "json"); ok {
			t.Fatalf("TagValue reported an absent key as present")
		}
	})

	t.Run("a value holding an escaped quote survives", func(t *testing.T) {
		t.Parallel()
		got := golang.ParseTag(`json:"a\"b"`)
		if got["json"] != `a"b` {
			t.Fatalf("ParseTag = %v", got)
		}
	})
}

func TestQuoting(t *testing.T) {
	t.Parallel()

	t.Run("interpreted quoting escapes", func(t *testing.T) {
		t.Parallel()
		if got := golang.Quote(`a"b`); got != `"a\"b"` {
			t.Fatalf("Quote = %q", got)
		}
	})

	t.Run("raw quoting avoids doubling backslashes", func(t *testing.T) {
		t.Parallel()
		// Worth having for generated regular expressions, where
		// interpreted quoting doubles every escape.
		got, ok := golang.RawQuote(`\d+`)
		if !ok || got != "`\\d+`" {
			t.Fatalf("RawQuote = %q, %v", got, ok)
		}
	})

	t.Run("a value holding a backtick cannot be raw-quoted", func(t *testing.T) {
		t.Parallel()
		// A raw literal admits no escapes, so the caller falls back.
		if _, ok := golang.RawQuote("a`b"); ok {
			t.Fatalf("RawQuote must refuse a value containing a backtick")
		}
	})

	t.Run("a carriage return cannot be raw-quoted", func(t *testing.T) {
		t.Parallel()
		// Go strips carriage returns from raw literals, so the value
		// would not survive the round trip.
		if _, ok := golang.RawQuote("a\rb"); ok {
			t.Fatalf("RawQuote must refuse a carriage return")
		}
	})
}

func TestValuesEdges(t *testing.T) {
	t.Parallel()

	t.Run("a defined type resolving to nothing yields no zero", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"x.A": &node.Alias{Name: "A", Package: "x", Target: namedTypeRef("x", "B")}}
		if _, ok := golang.ZeroLiteralFor(namedTypeRef("x", "A"), r); ok {
			t.Fatalf("an alias to an unresolvable type must yield no zero")
		}
	})

	t.Run("an alias with no target yields no sample", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"x.A": &node.Alias{Name: "A", Package: "x"}}
		if s, _ := golang.SampleFor(namedTypeRef("x", "A"), "F", r); s != "" {
			t.Fatalf("SampleFor = %q, want empty", s)
		}
	})

	t.Run("a non-alias declaration yields no sample", func(t *testing.T) {
		t.Parallel()
		// A struct's fields are the caller's to walk; this answers for
		// the underlying type of a defined one.
		r := mapResolver{"x.User": &node.Struct{Name: "User", Package: "x"}}
		if s, _ := golang.SampleFor(namedTypeRef("x", "User"), "F", r); s != "" {
			t.Fatalf("SampleFor = %q, want empty", s)
		}
	})

	t.Run("a resolver answering an unusable kind yields no zero", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"x.F": &node.Function{Name: "F", Package: "x"}}
		if _, ok := golang.ZeroLiteralFor(namedTypeRef("x", "F"), r); ok {
			t.Fatalf("ZeroLiteralFor must refuse a non-type declaration")
		}
	})

	t.Run("an array of an unnameable element yields no zero", func(t *testing.T) {
		t.Parallel()
		arr := &node.TypeRef{TypeKind: node.TypeRefArray, ArrayLen: 2}
		if _, ok := golang.ZeroLiteralFor(arr, mapResolver{}); ok {
			t.Fatalf("ZeroLiteralFor must refuse an array with no element")
		}
	})

	t.Run("an unterminated tag stops parsing rather than looping", func(t *testing.T) {
		t.Parallel()
		for _, raw := range []string{`json:"unterminated`, `json`, `:"v"`} {
			if got := golang.ParseTag(raw); len(got) != 0 {
				t.Errorf("ParseTag(%q) = %v, want empty", raw, got)
			}
		}
	})

	t.Run("an escaped quote inside a tag does not end it", func(t *testing.T) {
		t.Parallel()
		got := golang.ParseTag(`json:"a\"b" db:"c"`)
		if got["db"] != "c" {
			t.Fatalf("ParseTag = %v, want the second entry after an escape", got)
		}
	})
}
