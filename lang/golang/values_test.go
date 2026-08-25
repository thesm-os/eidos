// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/store"
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

	t.Run("a type needing an import yields nothing", func(t *testing.T) {
		t.Parallel()
		// This used to compose the spelling with QName and return
		// `example.com/cfg.Weekday(42)` — not Go, and no import
		// registered. The old test hid it by using "x" as the package,
		// which is short enough to pass for a qualifier.
		r := mapResolver{"example.com/cfg.Weekday": &node.Alias{
			Name: "Weekday", Package: "example.com/cfg", Target: builtinRef("int"),
		}}
		s, a := golang.SampleFor(namedTypeRef("example.com/cfg", "Weekday"), "Day", r)
		if s != "" || a != "" {
			t.Fatalf("SampleFor = %q, %q; a string cannot spell a type that needs an import",
				s, a)
		}
	})

	t.Run("SampleRefFor answers what SampleFor cannot", func(t *testing.T) {
		t.Parallel()
		// The ref beside the text is what lets the backend spell the
		// type for the file it lands in and register the import.
		r := mapResolver{"example.com/cfg.Weekday": &node.Alias{
			Name: "Weekday", Package: "example.com/cfg", Target: builtinRef("int"),
		}}
		s, a := golang.SampleRefFor(namedTypeRef("example.com/cfg", "Weekday"), "Day", r)
		if !s.OK() || !a.OK() {
			t.Fatalf("SampleRefFor derived nothing for a resolvable defined type")
		}
		if s.Ref == nil {
			t.Errorf("sample carries no ref, so the import would go unregistered")
		}
		if s.Text != "42" || a.Text != "7" {
			t.Errorf("Text = %q, %q; want the underlying literals", s.Text, a.Text)
		}
		if s.Composite {
			t.Errorf("a defined type renders as a conversion, not a composite literal")
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

	t.Run("resolves a local defined numeric type", func(t *testing.T) {
		t.Parallel()
		// The answer ZeroLiteral refuses, available once the caller
		// supplies the graph.
		r := mapResolver{"Weekday": &node.Alias{
			Name: "Weekday", Target: builtinRef("int"),
		}}
		got, ok := golang.ZeroLiteralFor(namedTypeRef("", "Weekday"), r)
		if !ok || got != "Weekday(0)" {
			t.Fatalf("ZeroLiteralFor = %q, %v", got, ok)
		}
	})

	t.Run("a local struct zeroes to a composite literal", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"User": &node.Struct{Name: "User"}}
		got, ok := golang.ZeroLiteralFor(namedTypeRef("", "User"), r)
		if !ok || got != "User{}" {
			t.Fatalf("ZeroLiteralFor = %q, %v", got, ok)
		}
	})

	t.Run("a type needing an import is refused", func(t *testing.T) {
		t.Parallel()
		// Same defect as SampleFor's: the spelling depends on the file
		// the zero lands in, which a string cannot carry.
		r := mapResolver{"example.com/cfg.User": &node.Struct{
			Name: "User", Package: "example.com/cfg",
		}}
		if got, ok := golang.ZeroLiteralFor(namedTypeRef("example.com/cfg", "User"), r); ok {
			t.Fatalf("ZeroLiteralFor = %q, want a refusal", got)
		}
	})

	t.Run("ZeroRefFor answers it instead", func(t *testing.T) {
		t.Parallel()
		r := mapResolver{"example.com/cfg.User": &node.Struct{
			Name: "User", Package: "example.com/cfg",
		}}
		got, ok := golang.ZeroRefFor(namedTypeRef("example.com/cfg", "User"), r)
		if !ok || !got.OK() {
			t.Fatalf("ZeroRefFor derived nothing for a resolvable struct")
		}
		if got.Ref == nil || !got.Composite || got.Text != "{}" {
			t.Errorf("ZeroRefFor = %+v; want a composite literal carrying its ref", got)
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

// TestResolverPort_Store pins that the store's Reader satisfies the
// [golang.Resolver] port, against a real graph rather than a double.
//
// The assertion lives here rather than beside the Reader because the
// port is this package's: asserting it from the store made the root
// module depend on a language adapter that already depends on it,
// and a module cycle is a poor way to say "these two agree". This
// direction is the one that already existed.
//
// Worth its own test at all because until Reader.Resolve landed the
// port had no implementation outside a test double, so the exported
// functions taking one had never run against a loaded package.
func TestResolverPort_Store(t *testing.T) {
	t.Parallel()

	const pkg = "example.com/x"
	namedRef := func(name string) *node.TypeRef {
		return &node.TypeRef{TypeKind: node.TypeRefNamed, Package: pkg, Name: name}
	}
	readerOver := func(t *testing.T, p *node.Package) *store.Reader {
		t.Helper()
		s := store.New()
		if err := s.Nodes().AddPackage(p); err != nil {
			t.Fatalf("AddPackage: %v", err)
		}
		return store.NewReader(s)
	}

	t.Run("a defined type resolves to the builtin behind it", func(t *testing.T) {
		t.Parallel()
		// The assertion the port existed for: the zero of a
		// cross-package defined type is derivable at all. Passed as
		// the interface so the structural match is proven at a call
		// site, not only at a var declaration.
		r := readerOver(t, &node.Package{
			Name: "x", Path: pkg,
			Aliases: []*node.Alias{{
				Name: "Weekday", Package: pkg,
				Target: &node.TypeRef{TypeKind: node.TypeRefNamed, Name: "int"},
			}},
		})
		var port golang.Resolver = r
		got, ok := golang.ZeroRefFor(namedRef("Weekday"), port)
		if !ok || !got.OK() {
			t.Fatalf("ZeroRefFor through the store resolver returned no answer")
		}
		if got.Ref == nil {
			t.Errorf("the zero of a cross-package type must carry its ref, "+
				"or the generated file references a type it never imports; got %+v", got)
		}
	})

	t.Run("a type the run never loaded reports not found", func(t *testing.T) {
		t.Parallel()
		// The refusal half of the port's contract: a narrow run has no
		// declaration for a cross-package name, and a caller acts on
		// that by omitting a check rather than writing one against a
		// type it cannot see.
		var port golang.Resolver = readerOver(t, &node.Package{Name: "x", Path: pkg})
		if got, ok := port.Resolve(namedRef("Absent")); ok || got != nil {
			t.Errorf("Resolve of an unloaded type = (%v, %v), want (nil, false)", got, ok)
		}
	})
}
