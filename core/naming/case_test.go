// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package naming_test

import (
	"strings"
	"testing"
	"unicode"

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

// The reference block below is a frozen copy of the case converters as
// they stood before the allocation work, kept so the rewrite can be
// checked against the behaviour it claims to preserve rather than
// against a restatement of its own rules.
//
// Every remedy in that change was "output-neutral by construction":
// the probe returns the same text, the Builder writers emit the same
// bytes, the Identifier pre-scan returns s only where the loop
// provably reproduced it. Each of those is a claim about equivalence,
// and the only honest way to hold a claim like that is to run both.
//
// Do not tidy these. Their value is that they are the old code.

func refTitleWord(initialisms map[string]string, w string) string {
	upper := strings.ToUpper(w)
	if _, ok := initialisms[upper]; ok {
		return upper
	}
	if refIsAllUpperASCII(w) {
		return w
	}
	runes := []rune(w)
	runes[0] = unicode.ToUpper(runes[0])
	for i := 1; i < len(runes); i++ {
		runes[i] = unicode.ToLower(runes[i])
	}
	return string(runes)
}

func refIsAllUpperASCII(s string) bool {
	for i := range len(s) {
		if c := s[i]; c < 'A' || c > 'Z' {
			return false
		}
	}
	return true
}

func refJoinWith(words []string, sep string, transform func(string) string) string {
	if len(words) == 0 {
		return ""
	}
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = transform(w)
	}
	return strings.Join(parts, sep)
}

func refPascal(initialisms map[string]string, words []string) string {
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	for _, w := range words {
		b.WriteString(refTitleWord(initialisms, w))
	}
	return b.String()
}

func refCamel(initialisms map[string]string, words []string) string {
	if len(words) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(strings.ToLower(words[0]))
	for _, w := range words[1:] {
		b.WriteString(refTitleWord(initialisms, w))
	}
	return b.String()
}

func refTitle(initialisms map[string]string, words []string) string {
	if len(words) == 0 {
		return ""
	}
	parts := make([]string, len(words))
	for i, w := range words {
		parts[i] = refTitleWord(initialisms, w)
	}
	return strings.Join(parts, " ")
}

func refIdentifier(s string) string {
	if s == "" {
		return "_"
	}
	var b strings.Builder
	b.Grow(len(s) + 1)
	for i, r := range s {
		switch {
		case r == '_' || unicode.IsLetter(r):
			b.WriteRune(r)
		case unicode.IsDigit(r):
			if i == 0 {
				b.WriteByte('_')
			}
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// refInitialisms rebuilds the default Caser's table in the shape the
// frozen converters expect. Derived from the live Caser rather than
// hardcoded so a change to CommonInitialisms cannot make the
// differential compare against a stale vocabulary.
func refInitialisms(c *naming.Caser) map[string]string {
	out := map[string]string{}
	for _, w := range c.Initialisms() {
		out[w] = w
	}
	return out
}

// FuzzCaser_StylesMatchReference is the equivalence barrier on the
// allocation work.
//
// The idempotence fuzz next to it proves each style is stable under
// repeated application, which a wrong-but-consistent rewrite would
// also satisfy. This proves the styles still produce what they
// produced before, for every input the fuzzer can find.
//
// Words is deliberately shared between the two sides: it has its own
// differential in words_test.go, and feeding both from it isolates
// this one to the case-mapping and joining layer.
func FuzzCaser_StylesMatchReference(f *testing.F) {
	for _, seed := range []string{
		"", "a", "id", "ctx", "http_client", "HTTPServer", "urlPath",
		"user_id_fetcher", "FOO", "a b/c.d-e_f", "___", "aA1",
		"UTF8Encoder", "v2Handler", "! A", "ßa", "aÉ", "\xff",
		"ıd", "ǅungla", "ΩMEGA", "a_1", "9lives", "_ok", "É",
	} {
		f.Add(seed)
	}

	c := naming.Default()
	table := refInitialisms(c)

	f.Fuzz(func(t *testing.T, s string) {
		words := c.Words(s)

		for _, tc := range []struct {
			name string
			got  string
			want string
		}{
			{"Pascal", c.Pascal(s), refPascal(table, words)},
			{"Camel", c.Camel(s), refCamel(table, words)},
			{"Title", c.Title(s), refTitle(table, words)},
			{"Snake", c.Snake(s), refJoinWith(words, "_", strings.ToLower)},
			{"ScreamingSnake", c.ScreamingSnake(s), refJoinWith(words, "_", strings.ToUpper)},
			{"Kebab", c.Kebab(s), refJoinWith(words, "-", strings.ToLower)},
			{"ScreamingKebab", c.ScreamingKebab(s), refJoinWith(words, "-", strings.ToUpper)},
			{"Dot", c.Dot(s), refJoinWith(words, ".", strings.ToLower)},
			{"Identifier", naming.Identifier(s), refIdentifier(s)},
		} {
			if tc.got != tc.want {
				t.Fatalf("%s(%q) = %q, reference = %q", tc.name, s, tc.got, tc.want)
			}
		}
	})
}

// TestCaser_InitialismProbeHandlesNonASCII pins the one case the
// stack-buffer probe must not get wrong.
//
// 'ı' (U+0131) upper-cases to 'I', so "ıd" normalises to "ID" and
// legitimately names a registered initialism. A fast path that
// refused non-ASCII and reported a miss — rather than falling through
// to strings.ToUpper — would silently stop recognising it, and the
// failure would surface as a quietly different generated identifier.
func TestCaser_InitialismProbeHandlesNonASCII(t *testing.T) {
	t.Parallel()

	c := naming.Default()
	table := refInitialisms(c)

	for _, in := range []string{"ıd", "İD", "ID", "id", "Id"} {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			if got, want := c.Pascal(in), refPascal(table, c.Words(in)); got != want {
				t.Fatalf("Pascal(%q) = %q, reference = %q", in, got, want)
			}
		})
	}
}

// TestIdentifier_FastPathReturnsInput pins that the pre-scan is a
// shortcut and not a second implementation: where it returns early the
// result must be the input itself, and where it declines the general
// loop must still run.
func TestIdentifier_FastPathReturnsInput(t *testing.T) {
	t.Parallel()

	t.Run("an already-valid identifier is returned unchanged", func(t *testing.T) {
		t.Parallel()
		for _, in := range []string{"ctx", "userID", "_x", "a1", "A_B_9"} {
			if got := naming.Identifier(in); got != in {
				t.Fatalf("Identifier(%q) = %q, want the input back", in, got)
			}
		}
	})

	t.Run("a leading digit still gains its underscore", func(t *testing.T) {
		t.Parallel()
		// The output differs from the input, so the pre-scan must
		// decline this even though every byte is otherwise legal.
		if got, want := naming.Identifier("9lives"), "_9lives"; got != want {
			t.Fatalf("Identifier(9lives) = %q, want %q", got, want)
		}
	})

	t.Run("non-ASCII letters still reach the general loop", func(t *testing.T) {
		t.Parallel()
		// unicode.IsLetter accepts 'é', so it survives; an ASCII-only
		// pre-scan must not decide that for itself.
		if got, want := naming.Identifier("éclair"), refIdentifier("éclair"); got != want {
			t.Fatalf("Identifier(éclair) = %q, want %q", got, want)
		}
	})

	t.Run("a non-ASCII non-letter is still replaced", func(t *testing.T) {
		t.Parallel()
		// The case that separates "the pre-scan declines non-ASCII"
		// from "the pre-scan accepts non-ASCII": a letter above 0x7F
		// survives either way, so only a rune that is neither letter
		// nor digit can tell the two apart. '→' must become '_'.
		if got, want := naming.Identifier("a→b"), "a_b"; got != want {
			t.Fatalf("Identifier(a→b) = %q, want %q", got, want)
		}
	})
}

// BenchmarkCaser_Camel and BenchmarkCaser_Snake cover the two shapes
// [BenchmarkCaser_Pascal] does not.
//
// Camel is the only style that treats its first word differently, and
// Snake is the representative of the five separator styles, which
// share one body. Neither had a benchmark before the allocation work,
// which meant the two remedies aimed at them — writing the leading
// word through the Builder instead of through strings.ToLower, and
// replacing the join helper's throwaway []string — could not be shown
// to have done anything at all.
//
// Same fixture as Pascal so the three are directly comparable: the
// hits vocabulary exercises the initialism probe, the misses
// vocabulary exercises the title-casing path underneath it.
func BenchmarkCaser_Camel(b *testing.B) {
	benchStyle(b, naming.Default().Camel)
}

func BenchmarkCaser_Snake(b *testing.B) {
	benchStyle(b, naming.Default().Snake)
}

// benchStyle runs convert over the hits and misses vocabularies.
func benchStyle(b *testing.B, convert func(string) string) {
	b.Helper()
	b.ReportAllocs()

	const words = 24
	for _, tc := range []struct {
		name string
		in   string
	}{
		{"initialism-hits", benchStyleInput(words, []string{"url", "http", "json", "id"})},
		{"initialism-misses", benchStyleInput(words, []string{"path", "server", "value", "node"})},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			var out string
			for b.Loop() {
				out = convert(tc.in)
			}
			if out == "" {
				b.Fatalf("convert(%q) returned empty", tc.in)
			}
		})
	}
}
