// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package naming_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"go.thesmos.sh/eidos/core/naming"
)

func TestCaser_Words(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"empty input returns nil", "", nil},
		{"separator-only input returns nil", "___---...", nil},
		{"single lowercase word", "hello", []string{"hello"}},
		{"camelCase splits at lower-to-upper", "helloWorld", []string{"hello", "World"}},
		{"PascalCase splits into multiple words", "HelloWorld", []string{"Hello", "World"}},
		{"acronym preceding word splits at boundary", "HTTPServer", []string{"HTTP", "Server"}},
		{"trailing acronym stays whole", "userID", []string{"user", "ID"}},
		{"all-uppercase identifier stays whole", "HTTP", []string{"HTTP"}},
		{"snake_case splits at underscore", "hello_world", []string{"hello", "world"}},
		{"kebab-case splits at hyphen", "hello-world", []string{"hello", "world"}},
		{"dot.case splits at dot", "hello.world", []string{"hello", "world"}},
		{"space splits at space", "hello world", []string{"hello", "world"}},
		{"tab splits at tab", "hello\tworld", []string{"hello", "world"}},
		{"forward slash splits", "hello/world", []string{"hello", "world"}},
		{"repeated mixed separators collapse", "__hello-_-world__", []string{"hello", "world"}},
		{"digits attach to surrounding word", "Version2", []string{"Version2"}},
		{"complex mixed input", "URLPath_v2 helloWorld", []string{"URL", "Path", "v2", "hello", "World"}},
	}

	c := naming.Default()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertEqualSlices(t, c.Words(tc.in), tc.want)
		})
	}
}

func TestWords_packageLevel(t *testing.T) {
	t.Parallel()
	assertEqualSlices(t, naming.Words("HelloWorld"), []string{"Hello", "World"})
}

// FuzzCaser_Words drives the splitter over arbitrary input.
//
// Words is the primitive every style converter is built on: each style
// renders exactly the words this function returns, so a splitter that
// invents, drops, or reorders content corrupts every identifier the
// framework generates — silently, and in a way no table of expected
// splits is broad enough to catch. The properties asserted here are
// therefore about conservation and stability rather than about any
// particular split, checked against a deliberately naive reference
// (stripSeparators) that knows one rule and nothing about boundaries.
//
// The seeds cover each branch the splitter takes: both boundary rules
// (lower-to-upper, acronym-then-word), every separator rune, the digit
// rule that deliberately does not break, and the degenerate inputs
// around each — empty, separator-only, repeated separators, a leading
// separator, non-ASCII case pairs, and invalid UTF-8.
func FuzzCaser_Words(f *testing.F) {
	for _, seed := range []string{
		"",
		"hello",
		"helloWorld",
		"HelloWorld",
		"HTTPServer",
		"userID",
		"HTTP",
		"hello_world",
		"hello-world",
		"hello.world",
		"hello world",
		"hello\tworld",
		"hello/world",
		"___---...",
		"__hello-_-world__",
		"Version2",
		"URLPath_v2 helloWorld",
		"ßa",
		"aÉ",
		"\xff\xfe",
	} {
		f.Add(seed)
	}

	c := naming.Default()

	f.Fuzz(func(t *testing.T, s string) {
		words := c.Words(s)

		// Conservation: the splitter is only ever allowed to remove
		// separators. Any other difference means a rune was invented,
		// dropped, or reordered, which every downstream converter
		// inherits verbatim.
		if got, want := strings.Join(words, ""), stripSeparators(s); got != want {
			t.Fatalf("Words(%q) concatenates to %q, want %q", s, got, want)
		}

		for i, w := range words {
			// An empty word renders as nothing but still contributes a
			// joiner in the separator styles, so it would surface as
			// "a__b" from Snake rather than as a visible split bug.
			if w == "" {
				t.Fatalf("Words(%q) returned an empty word at index %d (%q)", s, i, words)
			}
			// A separator surviving inside a word would be re-split on
			// the next pass through any converter, so the converters
			// would not be stable under repeated application.
			if strings.ContainsAny(w, separators) {
				t.Fatalf("Words(%q) returned word %q containing a separator", s, w)
			}
			// Splitting an already-split word is the identity. The
			// boundary rules read one rune either side of each
			// position, so a word that split further would mean the
			// split depended on the context the word was lifted from —
			// exactly the bug that makes a converter's output differ
			// from its own re-parse.
			if again := c.Words(w); len(again) != 1 || again[0] != w {
				t.Fatalf("re-splitting word %q of %q yielded %q, want [%q]", w, s, again, w)
			}
		}
	})
}

// BenchmarkCaser_Words measures one split of an identifier holding n
// words.
//
// Words runs at least once per generated name and once more per style
// conversion layered on top of it, so its per-call cost multiplies
// across a run. The scaling axis is the word count because that is what
// drives the two costs the implementation cannot avoid: the per-rune
// boundary test, and the growth of both the accumulating rune buffer
// and the words slice, neither of which is pre-sized. Super-linear
// growth here would mean the append strategy, not the scan, dominates.
//
// Input construction is hoisted above each sub-benchmark; only the
// split itself is timed. The post-loop word-count assertion is what
// proves the benchmark measured a split of the intended size rather
// than a call the compiler folded away.
func BenchmarkCaser_Words(b *testing.B) {
	b.ReportAllocs()

	c := naming.Default()
	for _, n := range []int{1, 10, 100, 1000} {
		in := benchIdentifier(n)
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()

			var words []string
			for b.Loop() {
				words = c.Words(in)
			}
			if len(words) != n {
				b.Fatalf("Words returned %d words, want %d", len(words), n)
			}
		})
	}
}

// benchIdentifier builds a PascalCase identifier of exactly n words by
// cycling a fixed vocabulary.
//
// Every vocabulary entry is one capital followed by lower-case runes,
// so each contributes exactly one word and the word count is the
// benchmark's size parameter rather than an artefact of the text. The
// acronym branch is deliberately absent here — it is measured by
// [BenchmarkCaser_Pascal], where the initialism lookup is the subject.
func benchIdentifier(n int) string {
	vocab := []string{"Http", "Server", "Config", "Handler", "Node"}
	var b strings.Builder
	for i := range n {
		b.WriteString(vocab[i%len(vocab)])
	}
	return b.String()
}

// wordsReference is a frozen copy of the rune-accumulating splitter
// that [naming.Caser.Words] replaced.
//
// It exists so the rewrite can be checked against the behaviour it
// claims to preserve, rather than against a re-statement of the new
// rules. Kept verbatim — including the []rune conversion, which is the
// source of the one behavioural subtlety the substring splitter has to
// reproduce by hand: it folds every invalid byte to U+FFFD, where
// slicing the input would preserve the raw byte.
//
// Do not "tidy" this function. Its value is that it is the old code.
func wordsReference(s string) []string {
	if s == "" {
		return nil
	}
	var words []string
	var current []rune
	flush := func() {
		if len(current) > 0 {
			words = append(words, string(current))
			current = nil
		}
	}
	runes := []rune(s)
	for i, r := range runes {
		if refIsSeparator(r) {
			flush()
			continue
		}
		if i > 0 && refShouldBreakBefore(runes, i) {
			flush()
		}
		current = append(current, r)
	}
	flush()
	return words
}

// refShouldBreakBefore is the frozen boundary rule.
func refShouldBreakBefore(runes []rune, i int) bool {
	cur := runes[i]
	prev := runes[i-1]
	if unicode.IsLower(prev) && unicode.IsUpper(cur) {
		return true
	}
	if unicode.IsUpper(prev) && unicode.IsUpper(cur) && i+1 < len(runes) && unicode.IsLower(runes[i+1]) {
		return true
	}
	return false
}

// refIsSeparator is the frozen separator set.
func refIsSeparator(r rune) bool {
	switch r {
	case '_', '-', '.', ' ', '\t', '/':
		return true
	default:
		return false
	}
}

// FuzzCaser_WordsMatchesReference is the differential barrier on the
// byte-offset rewrite.
//
// The conservation oracle in [FuzzCaser_Words] catches a naive
// substring split — it compares against a helper that also iterates
// runes, so a preserved invalid byte fails it — but it only proves the
// concatenation is right. This proves the split is, word for word,
// against the implementation that defined the behaviour.
//
// The seeds carry the two shapes that separate the implementations:
// invalid UTF-8, where the rune conversion substitutes and slicing
// does not, and acronym runs, where the boundary needs a lookahead the
// substring form has to do by decoding rather than by indexing.
func FuzzCaser_WordsMatchesReference(f *testing.F) {
	for _, seed := range []string{
		"", "a", "hello", "helloWorld", "HelloWorld", "HTTPServer",
		"URLPath", "Version2", "a_b-c.d e/f\tg", "___", "ÉclairÖvre",
		"\xbe", "\xff\xfe", "a\xffb", "A\xffB", "HTTP\xffServer",
		"aB", "AB", "ABc", "aBc", "ßeta", "ΩΩmega",
	} {
		f.Add(seed)
	}

	c := naming.Default()

	f.Fuzz(func(t *testing.T, s string) {
		got, want := c.Words(s), wordsReference(s)
		if !slices.Equal(got, want) {
			t.Fatalf("Words(%q) = %q, reference = %q", s, got, want)
		}
	})
}

// TestCaser_WordsInvalidUTF8 pins the substitution the rewrite has to
// perform by hand.
//
// Without it the splitter would return the raw byte, which is a
// different string, fails the package's conservation oracle, and
// silently changes every identifier a generator derives from
// mis-encoded source. A table case rather than fuzz-only coverage so
// deleting the branch fails a named test instead of a seed corpus.
func TestCaser_WordsInvalidUTF8(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"a lone invalid byte becomes the replacement rune", "\xbe", []string{"�"}},
		{"consecutive invalid bytes each substitute", "\xff\xfe", []string{"��"}},
		{"an invalid byte inside a word substitutes in place", "a\xffb", []string{"a�b"}},
		{"a valid word beside an invalid one is still sliced", "ok_a\xffb", []string{"ok", "a�b"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := naming.Default().Words(tc.in); !slices.Equal(got, tc.want) {
				t.Fatalf("Words(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestCaser_WordsAllocations is the host-independent regression
// barrier. The nanosecond figures for this function moved by as much
// as 28% between machines; the allocation count did not move at all.
//
// One allocation is the floor: Words returns a non-empty slice and is
// far too large to inline, so the slice header is a heap allocation no
// caller can elide. What the rewrite removed was everything else — the
// []rune conversion, a growth ladder restarted per word, and a string
// copy per word, which together came to 3.6 allocations per word, flat
// at every input size.
//
//nolint:paralleltest // testing.AllocsPerRun panics in a parallel test.
func TestCaser_WordsAllocations(t *testing.T) {
	c := naming.Default()

	for _, tc := range []struct {
		words int
		want  float64
	}{
		{1, 1},
		{24, 1},
		{1000, 1},
	} {
		in := benchIdentifier(tc.words)
		got := testing.AllocsPerRun(50, func() { _ = c.Words(in) })
		if got > tc.want {
			t.Fatalf("Words over %d words allocated %v times, want at most %v", tc.words, got, tc.want)
		}
	}
}
