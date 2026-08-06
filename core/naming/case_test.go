// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package naming_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/naming"
)

// styleCase is the shared shape for case-converter tables: a Caser
// (so individual rows can vary the initialism set), an input, and an
// expected output.
type styleCase struct {
	name  string
	caser *naming.Caser
	in    string
	want  string
}

// runStyleCases drives a single converter (selected by fn) over a
// slice of styleCase rows, each as its own subtest.
func runStyleCases(t *testing.T, fn func(*naming.Caser, string) string, cases []styleCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			caser := tc.caser
			if caser == nil {
				caser = naming.Default()
			}
			assertEqualString(t, fn(caser, tc.in), tc.want)
		})
	}
}

func TestCaser_Pascal(t *testing.T) {
	t.Parallel()

	runStyleCases(
		t,
		func(c *naming.Caser, s string) string { return c.Pascal(s) },
		[]styleCase{
			{name: "empty input returns empty", in: "", want: ""},
			{name: "snake_case becomes PascalCase", in: "hello_world", want: "HelloWorld"},
			{name: "camelCase becomes PascalCase", in: "helloWorld", want: "HelloWorld"},
			{name: "already PascalCase stays unchanged", in: "HelloWorld", want: "HelloWorld"},
			{name: "known initialism is upper-cased", in: "url_path", want: "URLPath"},
			{
				name:  "unknown acronym is title-cased without initialisms",
				caser: naming.New(),
				in:    "url_path",
				want:  "UrlPath",
			},
			{name: "preserves all-upper acronym in mixed input", in: "HTTPServer", want: "HTTPServer"},
			{name: "trailing initialism preserved", in: "user_id", want: "UserID"},
			{name: "all-upper non-initialism word is preserved", caser: naming.New(), in: "FOO", want: "FOO"},
		},
	)
}

func TestCaser_Camel(t *testing.T) {
	t.Parallel()

	runStyleCases(
		t,
		func(c *naming.Caser, s string) string { return c.Camel(s) },
		[]styleCase{
			{name: "empty input returns empty", in: "", want: ""},
			{name: "snake_case becomes camelCase", in: "hello_world", want: "helloWorld"},
			{name: "PascalCase becomes camelCase", in: "HelloWorld", want: "helloWorld"},
			{name: "first word fully lower-cased even when initialism", in: "URL_path", want: "urlPath"},
			{name: "trailing initialism preserved", in: "user_id", want: "userID"},
		},
	)
}

func TestCaser_Snake(t *testing.T) {
	t.Parallel()

	runStyleCases(
		t,
		func(c *naming.Caser, s string) string { return c.Snake(s) },
		[]styleCase{
			{name: "empty input returns empty", in: "", want: ""},
			{name: "PascalCase becomes snake_case", in: "HelloWorld", want: "hello_world"},
			{name: "acronym run is lower-cased", in: "HTTPServer", want: "http_server"},
			{name: "idempotent on snake_case input", in: "hello_world", want: "hello_world"},
		},
	)
}

func TestCaser_ScreamingSnake(t *testing.T) {
	t.Parallel()

	runStyleCases(
		t,
		func(c *naming.Caser, s string) string { return c.ScreamingSnake(s) },
		[]styleCase{
			{name: "empty input returns empty", in: "", want: ""},
			{name: "PascalCase becomes SCREAMING_SNAKE", in: "HelloWorld", want: "HELLO_WORLD"},
			{name: "acronym run preserved upper", in: "HTTPServer", want: "HTTP_SERVER"},
		},
	)
}

func TestCaser_Kebab(t *testing.T) {
	t.Parallel()

	runStyleCases(
		t,
		func(c *naming.Caser, s string) string { return c.Kebab(s) },
		[]styleCase{
			{name: "empty input returns empty", in: "", want: ""},
			{name: "PascalCase becomes kebab-case", in: "HelloWorld", want: "hello-world"},
			{name: "snake_case becomes kebab-case", in: "hello_world", want: "hello-world"},
		},
	)
}

func TestCaser_ScreamingKebab(t *testing.T) {
	t.Parallel()

	runStyleCases(
		t,
		func(c *naming.Caser, s string) string { return c.ScreamingKebab(s) },
		[]styleCase{
			{name: "empty input returns empty", in: "", want: ""},
			{name: "PascalCase becomes SCREAMING-KEBAB", in: "HelloWorld", want: "HELLO-WORLD"},
		},
	)
}

func TestCaser_Dot(t *testing.T) {
	t.Parallel()

	runStyleCases(
		t,
		func(c *naming.Caser, s string) string { return c.Dot(s) },
		[]styleCase{
			{name: "empty input returns empty", in: "", want: ""},
			{name: "PascalCase becomes dot.case", in: "HelloWorld", want: "hello.world"},
		},
	)
}

func TestCaser_Title(t *testing.T) {
	t.Parallel()

	runStyleCases(
		t,
		func(c *naming.Caser, s string) string { return c.Title(s) },
		[]styleCase{
			{name: "empty input returns empty", in: "", want: ""},
			{
				name: "snake_case becomes Title Case with initialism preserved",
				in:   "user_id_fetcher",
				want: "User ID Fetcher",
			},
			{name: "camelCase becomes Title Case", in: "helloWorld", want: "Hello World"},
		},
	)
}

// TestPackageLevelShorthands covers the package-level façade: each
// shorthand routes to the same converter on the default Caser. This is
// not table-driven because each row tests a different function rather
// than the same function with different inputs.
func TestPackageLevelShorthands(t *testing.T) {
	t.Parallel()

	t.Run("Pascal", func(t *testing.T) {
		t.Parallel()
		assertEqualString(t, naming.Pascal("hello_world"), "HelloWorld")
	})
	t.Run("Camel", func(t *testing.T) {
		t.Parallel()
		assertEqualString(t, naming.Camel("hello_world"), "helloWorld")
	})
	t.Run("Snake", func(t *testing.T) {
		t.Parallel()
		assertEqualString(t, naming.Snake("HelloWorld"), "hello_world")
	})
	t.Run("ScreamingSnake", func(t *testing.T) {
		t.Parallel()
		assertEqualString(t, naming.ScreamingSnake("HelloWorld"), "HELLO_WORLD")
	})
	t.Run("Kebab", func(t *testing.T) {
		t.Parallel()
		assertEqualString(t, naming.Kebab("HelloWorld"), "hello-world")
	})
	t.Run("ScreamingKebab", func(t *testing.T) {
		t.Parallel()
		assertEqualString(t, naming.ScreamingKebab("HelloWorld"), "HELLO-WORLD")
	})
	t.Run("Dot", func(t *testing.T) {
		t.Parallel()
		assertEqualString(t, naming.Dot("HelloWorld"), "hello.world")
	})
	t.Run("Title", func(t *testing.T) {
		t.Parallel()
		assertEqualString(t, naming.Title("hello_world"), "Hello World")
	})
}

// styleConverter pairs a converter with its name so a property can be
// asserted across all eight styles and still report which one broke.
type styleConverter struct {
	name    string
	convert func(string) string
}

// styleConverters returns c's eight style converters in the order the
// package documents them. Declared as a function rather than a package
// var so each caller gets a fresh slice bound to its own Caser.
func styleConverters(c *naming.Caser) []styleConverter {
	return []styleConverter{
		{"Pascal", c.Pascal},
		{"Camel", c.Camel},
		{"Snake", c.Snake},
		{"ScreamingSnake", c.ScreamingSnake},
		{"Kebab", c.Kebab},
		{"ScreamingKebab", c.ScreamingKebab},
		{"Dot", c.Dot},
		{"Title", c.Title},
	}
}

// FuzzCaser_Idempotence asserts that the style converters normalise
// rather than merely transform.
//
// A converter is used as a normaliser: a name may pass through Pascal
// in a frontend, again in a plugin, and again in a backend, and the
// generated identifier must not depend on how many layers touched it.
// Instability is invisible in a unit table, because a table only ever
// applies the converter once.
//
// Two properties, one universal and one domain-restricted:
//
//   - Every style reaches a fixed point by the second application, for
//     every input: f(f(f(x))) == f(f(x)). A style that never settles
//     would make a generated name a function of pipeline depth.
//   - Every style is strictly idempotent — f(f(x)) == f(x) — over
//     inputs drawn from ASCII letters and separators.
//
// The domain restriction on the second property is not a convenience.
// Three defects break strict idempotence outside that domain, and each
// is seeded below so the fuzzer keeps exercising the path:
//
//   - Both boundary rules in words.go require a cased predecessor, so
//     no boundary is ever inserted after a rune that is neither upper
//     nor lower case — a digit, punctuation, a symbol. Pascal("aA1") is
//     "AA1", which re-splits as the single word "AA1" and title-cases
//     to "Aa1". The same rule mangles ordinary Go identifiers on the
//     first pass, not only on the second: Pascal("UTF8Encoder") is
//     "Utf8encoder".
//   - strings.ToUpper is not total — ß has no single-rune upper-case
//     form — so ScreamingSnake and ScreamingKebab can emit a lower-case
//     rune inside an otherwise upper-case word. ScreamingSnake("ßa") is
//     "ßA", whose ß-to-A transition is a boundary on the next pass:
//     "ß_A".
//   - isAllUpperASCII rejects any word holding a non-ASCII rune, so an
//     all-upper non-ASCII word takes the title-casing path that an
//     all-upper ASCII word is spared. Pascal("aÉ") is "AÉ", which
//     lower-cases to "Aé" while Pascal("FOO") stays "FOO".
//
// Widening asciiLettersAndSeparators is the test-side signal that one
// of the three has been fixed.
func FuzzCaser_Idempotence(f *testing.F) {
	for _, seed := range []string{
		"",
		"_",
		"hello",
		"helloWorld",
		"HelloWorld",
		"hello_world",
		"HTTPServer",
		"url_path",
		"user_id_fetcher",
		"FOO",
		"a b/c.d-e_f",
		// Counterexamples to strict idempotence — see the doc comment.
		"aA1",
		"UTF8Encoder",
		"v2Handler",
		"! A",
		"ßa",
		"aÉ",
		"\xff",
	} {
		f.Add(seed)
	}

	c := naming.Default()

	f.Fuzz(func(t *testing.T, s string) {
		for _, style := range styleConverters(c) {
			once := style.convert(s)
			twice := style.convert(once)
			if thrice := style.convert(twice); thrice != twice {
				t.Fatalf("%s never settles: %q -> %q -> %q -> %q", style.name, s, once, twice, thrice)
			}
			if !asciiLettersAndSeparators(s) {
				continue
			}
			if once != twice {
				t.Fatalf("%s is not idempotent on %q: %q -> %q", style.name, s, once, twice)
			}
		}
	})
}

// BenchmarkCaser_Pascal measures one Pascal conversion of an identifier
// holding 24 words.
//
// Pascal is the dominant style in a Go code generator — every emitted
// type, method, and field name goes through it — and its per-word cost
// is the initialism decision, not the case change: titleWord builds an
// upper-case copy of every word with strings.ToUpper purely to probe
// the initialism map, and throws that copy away again on a miss.
//
// The two sub-benchmarks hold the word count fixed and vary only
// whether the words are registered initialisms, which isolates that
// cost: the hit path returns the copy it already built, the miss path
// pays for it and then rebuilds the title-cased form rune by rune. The
// gap between the two is the budget available to a cheaper probe.
//
// Inputs are built above the loop; only the conversion is timed. The
// post-loop check on the result is what proves the call was not folded
// away.
func BenchmarkCaser_Pascal(b *testing.B) {
	b.ReportAllocs()

	const words = 24
	c := naming.Default()
	cases := []struct {
		name string
		in   string
	}{
		{"initialism-hits", benchStyleInput(words, []string{"url", "http", "json", "id"})},
		{"initialism-misses", benchStyleInput(words, []string{"path", "server", "value", "node"})},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()

			var out string
			for b.Loop() {
				out = c.Pascal(tc.in)
			}
			if out == "" {
				b.Fatalf("Pascal(%q) returned empty", tc.in)
			}
		})
	}
}

// benchStyleInput builds a snake_case identifier of exactly n words by
// cycling vocab.
//
// snake_case is the input shape a style benchmark should use: the
// separator makes the word count exact and independent of the boundary
// rules, so the two sub-benchmarks it feeds differ in initialism
// membership alone.
func benchStyleInput(n int, vocab []string) string {
	parts := make([]string, n)
	for i := range n {
		parts[i] = vocab[i%len(vocab)]
	}
	return strings.Join(parts, "_")
}
