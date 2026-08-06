// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package directive_test

import (
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
)

func TestNewParser(t *testing.T) {
	t.Parallel()

	t.Run("returns a parser exposing the configured prefix", func(t *testing.T) {
		t.Parallel()
		p, err := directive.NewParser("gen")
		assertNoError(t, err, "NewParser")
		assertEqualString(t, p.Prefix(), "gen")
	})

	t.Run("rejects an empty prefix with ErrInvalidPrefix", func(t *testing.T) {
		t.Parallel()
		_, err := directive.NewParser("")
		if !errors.Is(err, directive.ErrInvalidPrefix) {
			t.Fatalf("err = %v, want ErrInvalidPrefix", err)
		}
	})

	t.Run("rejects prefixes containing reserved characters", func(t *testing.T) {
		t.Parallel()
		cases := []string{":", "gen:", "+gen", "-gen", "ge n", "gen\t"}
		for _, in := range cases {
			t.Run(in, func(t *testing.T) {
				t.Parallel()
				_, err := directive.NewParser(in)
				if !errors.Is(err, directive.ErrInvalidPrefix) {
					t.Fatalf("NewParser(%q) err = %v, want ErrInvalidPrefix", in, err)
				}
			})
		}
	})
}

func TestParser_Parse(t *testing.T) {
	t.Parallel()

	parser, err := directive.NewParser("gen")
	assertNoError(t, err, "NewParser")
	pos := position.At("a.go", 1, 1)

	t.Run("parses the neutral form", func(t *testing.T) {
		t.Parallel()
		d := parseFirst(t, parser, "gen:mock", pos)
		if d.Name != "mock" {
			t.Fatalf("Name = %q, want mock", d.Name)
		}
		if d.Negated {
			t.Fatalf("Negated should be false for the neutral form")
		}
	})

	t.Run("parses the explicit set form", func(t *testing.T) {
		t.Parallel()
		d := parseFirst(t, parser, "+gen:mock", pos)
		if d.Negated {
			t.Fatalf("Negated should be false for the +gen: form")
		}
	})

	t.Run("parses the negated form", func(t *testing.T) {
		t.Parallel()
		d := parseFirst(t, parser, "-gen:mock", pos)
		if !d.Negated {
			t.Fatalf("Negated should be true for the -gen: form")
		}
	})

	t.Run("populates positional args", func(t *testing.T) {
		t.Parallel()
		d := parseFirst(t, parser, "gen:shape writer paginator", pos)
		if len(d.Args) != 2 || d.Args[0] != "writer" || d.Args[1] != "paginator" {
			t.Fatalf("Args = %v", d.Args)
		}
	})

	t.Run("populates KV args from key=value pairs", func(t *testing.T) {
		t.Parallel()
		d := parseFirst(t, parser, "gen:mock target=Repository out=mocks/", pos)
		if d.Value("target") != "Repository" {
			t.Fatalf("target = %q", d.Value("target"))
		}
		if d.Value("out") != "mocks/" {
			t.Fatalf("out = %q", d.Value("out"))
		}
	})

	t.Run("mixes positional and KV in any order", func(t *testing.T) {
		t.Parallel()
		d := parseFirst(t, parser, "gen:shape writer method=Write buffer=buf", pos)
		if d.Arg(0) != "writer" || d.Value("method") != "Write" || d.Value("buffer") != "buf" {
			t.Fatalf("parse mismatch: args=%v kv=%v", d.Args, d.KV)
		}
	})

	t.Run("supports quoted string values with embedded spaces", func(t *testing.T) {
		t.Parallel()
		d := parseFirst(t, parser, `gen:foo desc="hello world"`, pos)
		assertEqualString(t, d.Value("desc"), "hello world")
	})

	t.Run("supports escape sequences in quoted values", func(t *testing.T) {
		t.Parallel()
		d := parseFirst(t, parser, `gen:foo desc="say \"hi\" then \\ and a \n line"`, pos)
		assertEqualString(t, d.Value("desc"), "say \"hi\" then \\ and a \n line")
	})

	t.Run("preserves tab escape in quoted values", func(t *testing.T) {
		t.Parallel()
		d := parseFirst(t, parser, `gen:foo desc="col1\tcol2"`, pos)
		assertEqualString(t, d.Value("desc"), "col1\tcol2")
	})

	t.Run("records the directive position", func(t *testing.T) {
		t.Parallel()
		d := parseFirst(t, parser, "gen:mock", pos)
		if d.Pos != pos {
			t.Fatalf("Pos = %+v, want %+v", d.Pos, pos)
		}
	})

	t.Run("records the raw body after stripping the prefix", func(t *testing.T) {
		t.Parallel()
		d := parseFirst(t, parser, "gen:mock target=Repo", pos)
		assertEqualString(t, d.Raw, "mock target=Repo")
	})

	t.Run("rejects input without a recognised prefix", func(t *testing.T) {
		t.Parallel()
		_, err := parser.Parse("nope:mock", pos)
		if !errors.Is(err, directive.ErrMalformedDirective) {
			t.Fatalf("err = %v, want ErrMalformedDirective", err)
		}
	})

	t.Run("rejects an empty directive name", func(t *testing.T) {
		t.Parallel()
		_, err := parser.Parse("gen:", pos)
		if !errors.Is(err, directive.ErrMalformedDirective) {
			t.Fatalf("err = %v, want ErrMalformedDirective", err)
		}
	})

	t.Run("rejects a name that starts with a non-letter", func(t *testing.T) {
		t.Parallel()
		_, err := parser.Parse("gen:9mock", pos)
		if !errors.Is(err, directive.ErrMalformedDirective) {
			t.Fatalf("err = %v, want ErrMalformedDirective", err)
		}
	})

	t.Run("rejects a name that contains a non-name rune after the first byte", func(t *testing.T) {
		t.Parallel()
		_, err := parser.Parse("gen:mock.bad", pos)
		if !errors.Is(err, directive.ErrMalformedDirective) {
			t.Fatalf("err = %v, want ErrMalformedDirective", err)
		}
	})

	t.Run("rejects an unterminated quoted value", func(t *testing.T) {
		t.Parallel()
		_, err := parser.Parse(`gen:foo desc="hello`, pos)
		if !errors.Is(err, directive.ErrMalformedDirective) {
			t.Fatalf("err = %v, want ErrMalformedDirective", err)
		}
	})

	t.Run("rejects an unknown escape in a quoted value", func(t *testing.T) {
		t.Parallel()
		_, err := parser.Parse(`gen:foo desc="oops \x"`, pos)
		if !errors.Is(err, directive.ErrMalformedDirective) {
			t.Fatalf("err = %v, want ErrMalformedDirective", err)
		}
	})

	t.Run("rejects an unterminated escape inside quotes", func(t *testing.T) {
		t.Parallel()
		_, err := parser.Parse(`gen:foo desc="oops \`, pos)
		if !errors.Is(err, directive.ErrMalformedDirective) {
			t.Fatalf("err = %v, want ErrMalformedDirective", err)
		}
	})

	t.Run("tolerates leading whitespace before the prefix", func(t *testing.T) {
		t.Parallel()
		d := parseFirst(t, parser, "   gen:mock", pos)
		if d.Name != "mock" {
			t.Fatalf("Name = %q, want mock", d.Name)
		}
	})

	t.Run("rejects a whitespace-only input as malformed", func(t *testing.T) {
		t.Parallel()
		// The lexer trims leading whitespace and immediately hits
		// EOF; the loop exits with no directive collected, so Parse
		// surfaces ErrMalformedDirective rather than returning an
		// empty slice.
		_, err := parser.Parse("   \t  ", pos)
		if !errors.Is(err, directive.ErrMalformedDirective) {
			t.Fatalf("err = %v, want ErrMalformedDirective", err)
		}
	})
}

// TestParser_Parse_MultiDirective covers the greedy multi-directive
// split: when a single comment body holds multiple directives, the
// parser closes the current directive's arg list at the next
// recognised prefix token and starts a new directive. Each form
// (neutral / explicit-set / negated) participates.
func TestParser_Parse_MultiDirective(t *testing.T) {
	t.Parallel()

	parser, err := directive.NewParser("gen")
	assertNoError(t, err, "NewParser")
	pos := position.At("a.go", 1, 1)

	t.Run("splits a meta line carrying four directives", func(t *testing.T) {
		t.Parallel()
		ds, err := parser.Parse("gen:meta +gen:out users.go +gen:repo +gen:builder", pos)
		assertNoError(t, err, "Parse")
		if len(ds) != 4 {
			t.Fatalf("expected 4 directives; got %d (%+v)", len(ds), ds)
		}
		if ds[0].Name != "meta" || ds[1].Name != "out" || ds[2].Name != "repo" || ds[3].Name != "builder" {
			t.Fatalf("names = [%q %q %q %q]", ds[0].Name, ds[1].Name, ds[2].Name, ds[3].Name)
		}
		if len(ds[0].Args) != 0 || len(ds[1].Args) != 1 || ds[1].Args[0] != "users.go" {
			t.Fatalf("out should consume one positional arg; got args[1]=%v", ds[1].Args)
		}
	})

	t.Run("preserves Negated per directive when sigils mix", func(t *testing.T) {
		t.Parallel()
		ds, err := parser.Parse("gen:repo +gen:mock -gen:audit", pos)
		assertNoError(t, err, "Parse")
		if len(ds) != 3 {
			t.Fatalf("expected 3 directives; got %d", len(ds))
		}
		if ds[0].Negated || ds[1].Negated || !ds[2].Negated {
			t.Fatalf("Negated flags = [%v %v %v], want [false false true]",
				ds[0].Negated, ds[1].Negated, ds[2].Negated)
		}
	})

	t.Run("KV pairs stay attached to the preceding directive", func(t *testing.T) {
		t.Parallel()
		ds, err := parser.Parse("gen:mock target=Repo out=mocks/ +gen:builder suffix=Bld", pos)
		assertNoError(t, err, "Parse")
		if len(ds) != 2 {
			t.Fatalf("expected 2 directives; got %d", len(ds))
		}
		if ds[0].Value("target") != "Repo" || ds[0].Value("out") != "mocks/" {
			t.Fatalf("mock KV mismatch: %+v", ds[0].KV)
		}
		if ds[1].Value("suffix") != "Bld" {
			t.Fatalf("builder KV mismatch: %+v", ds[1].KV)
		}
	})

	t.Run("Raw of each directive excludes the trailing whitespace before the next", func(t *testing.T) {
		t.Parallel()
		ds, err := parser.Parse("gen:meta +gen:out users.go +gen:repo", pos)
		assertNoError(t, err, "Parse")
		assertEqualString(t, ds[0].Raw, "meta")
		assertEqualString(t, ds[1].Raw, "out users.go")
		assertEqualString(t, ds[2].Raw, "repo")
	})

	t.Run("a quoted KV value with whitespace and embedded sigil is held together", func(t *testing.T) {
		t.Parallel()
		// `+gen:` inside a quoted string should NOT trigger a new
		// directive — the lexer reads the quoted body whole and
		// returns to the boundary check after the closing quote.
		ds, err := parser.Parse(`gen:meta desc="see +gen:repo notes" +gen:repo`, pos)
		assertNoError(t, err, "Parse")
		if len(ds) != 2 {
			t.Fatalf("expected 2 directives; got %d (%+v)", len(ds), ds)
		}
		assertEqualString(t, ds[0].Value("desc"), "see +gen:repo notes")
		if ds[1].Name != "repo" {
			t.Fatalf("second directive name = %q, want repo", ds[1].Name)
		}
	})
}

func TestParser_Parse_CustomPrefix(t *testing.T) {
	t.Parallel()

	t.Run("recognises a non-default prefix", func(t *testing.T) {
		t.Parallel()
		parser, err := directive.NewParser("testkit")
		assertNoError(t, err, "NewParser")
		d := parseFirst(t, parser, "testkit:fixture target=DB", position.At("a.go", 1, 1))
		if d.Name != "fixture" || d.Value("target") != "DB" {
			t.Fatalf("custom-prefix parse mismatch: %+v", d)
		}
	})

	t.Run("rejects the default prefix when configured for another", func(t *testing.T) {
		t.Parallel()
		parser, err := directive.NewParser("testkit")
		assertNoError(t, err, "NewParser")
		_, err = parser.Parse("gen:mock", position.At("a.go", 1, 1))
		if !errors.Is(err, directive.ErrMalformedDirective) {
			t.Fatalf("err = %v, want ErrMalformedDirective", err)
		}
	})
}

func TestParser_ParseComment(t *testing.T) {
	t.Parallel()

	parser, err := directive.NewParser("gen")
	assertNoError(t, err, "NewParser")
	pos := position.At("a.go", 1, 1)

	t.Run("strips the // line-comment marker (no space)", func(t *testing.T) {
		t.Parallel()
		d := parseFirstComment(t, parser, "//gen:mock", pos)
		if d.Name != "mock" {
			t.Fatalf("ParseComment returned %+v", d)
		}
	})

	t.Run("tolerates whitespace between marker and prefix", func(t *testing.T) {
		t.Parallel()
		d := parseFirstComment(t, parser, "// gen:mock", pos)
		if d.Name != "mock" {
			t.Fatalf("ParseComment returned %+v", d)
		}
	})

	t.Run("strips block-comment markers", func(t *testing.T) {
		t.Parallel()
		d := parseFirstComment(t, parser, "/* +gen:mock target=Repo */", pos)
		if d.Name != "mock" || d.Value("target") != "Repo" {
			t.Fatalf("ParseComment returned %+v", d)
		}
	})

	t.Run("returns nil, nil for empty comments", func(t *testing.T) {
		t.Parallel()
		ds, err := parser.ParseComment("// ", pos)
		assertNoError(t, err, "ParseComment")
		if len(ds) != 0 {
			t.Fatalf("empty comment should return no directives; got %+v", ds)
		}
	})

	t.Run("returns nil, nil for non-directive comments", func(t *testing.T) {
		t.Parallel()
		ds, err := parser.ParseComment("// regular comment", pos)
		assertNoError(t, err, "ParseComment")
		if len(ds) != 0 {
			t.Fatalf("non-directive comment should return no directives; got %+v", ds)
		}
	})

	t.Run("propagates errors from a malformed directive comment", func(t *testing.T) {
		t.Parallel()
		_, err := parser.ParseComment("// gen:", pos)
		if !errors.Is(err, directive.ErrMalformedDirective) {
			t.Fatalf("err = %v, want ErrMalformedDirective", err)
		}
	})

	t.Run("parses a no-space //gen: line carrying multiple directives", func(t *testing.T) {
		t.Parallel()
		ds, err := parser.ParseComment("//gen:meta +gen:out users.go +gen:repo +gen:builder", pos)
		assertNoError(t, err, "ParseComment")
		if len(ds) != 4 {
			t.Fatalf("expected 4 directives; got %d (%+v)", len(ds), ds)
		}
		if ds[0].Name != "meta" || ds[1].Name != "out" || ds[2].Name != "repo" || ds[3].Name != "builder" {
			t.Fatalf("names = [%q %q %q %q]", ds[0].Name, ds[1].Name, ds[2].Name, ds[3].Name)
		}
		if len(ds[1].Args) != 1 || ds[1].Args[0] != "users.go" {
			t.Fatalf("out's positional arg = %v, want [users.go]", ds[1].Args)
		}
	})

	t.Run("parses a //gen: comment with no space and KV args", func(t *testing.T) {
		t.Parallel()
		d := parseFirstComment(t, parser, "//gen:mock target=Repo", pos)
		if d.Name != "mock" || d.Value("target") != "Repo" {
			t.Fatalf("ParseComment returned %+v", d)
		}
	})

	t.Run("parses a /* */ block comment carrying multiple directives", func(t *testing.T) {
		t.Parallel()
		ds, err := parser.ParseComment("/* gen:meta +gen:out users.go */", pos)
		assertNoError(t, err, "ParseComment")
		if len(ds) != 2 {
			t.Fatalf("expected 2 directives; got %d (%+v)", len(ds), ds)
		}
	})

	t.Run("recognises the -gen: negated prefix in a comment", func(t *testing.T) {
		t.Parallel()
		d := parseFirstComment(t, parser, "// -gen:mock", pos)
		if d.Name != "mock" {
			t.Fatalf("Name = %q, want mock", d.Name)
		}
		if !d.Negated {
			t.Fatalf("Negated should be true for the -gen: comment form")
		}
	})
}

// FuzzParser_Parse drives the hand-rolled directive lexer over
// arbitrary input.
//
// Parse is the user-facing syntax and is written by hand — quoting,
// escapes, a name grammar, and a greedy scan that keeps consuming
// after each directive ends. Its dangerous failure is a silent
// misparse rather than a crash, so the properties asserted here are
// about the shape of what comes back, not merely that nothing
// exploded.
//
// The seeds cover the forms the grammar branches on: bare names,
// negation, positionals, key/value pairs, quoting, escapes, several
// directives in one comment, and the degenerate inputs around each.
func FuzzParser_Parse(f *testing.F) {
	for _, seed := range []string{
		"",
		"+gen:stub",
		"-gen:stub",
		"+gen:stub out=testkit/ pkg=storetest",
		`+gen:value "quoted arg"`,
		`+gen:value "escaped \" quote"`,
		"+gen:mixin idempotent concurrent atomic",
		"+gen:a +gen:b +gen:c",
		"+gen:",
		"+gen:x=",
		"+gen:x=y=z",
		`+gen:x="unterminated`,
		"+gen:x \t \n +gen:y",
		"not a directive at all",
	} {
		f.Add(seed)
	}

	p, err := directive.NewParser("gen")
	if err != nil {
		f.Fatalf("NewParser: %v", err)
	}

	f.Fuzz(func(t *testing.T, text string) {
		got, err := p.Parse(text, position.Pos{})
		if err != nil {
			// A rejected input must yield nothing. Returning both a
			// partial parse and an error invites callers to use the
			// half-built result.
			if got != nil {
				t.Fatalf("Parse returned %d directives alongside error %v", len(got), err)
			}
			return
		}
		for i, d := range got {
			// An accepted directive with no name cannot be matched
			// against any schema, so it can only ever be a silent
			// no-op for the consumer.
			if d.Name == "" {
				t.Fatalf("directive %d parsed with an empty name from %q", i, text)
			}
			for k := range d.KV {
				// An empty key is unreadable by any consumer and
				// unmatchable by Schema.AllowedKeys, so accepting one
				// silently discards the value at exactly the point
				// the author expected it to take effect.
				if k == "" {
					t.Fatalf("directive %d parsed an empty KV key from %q", i, text)
				}
				// The map and the accessor must agree, or a consumer
				// reading through Value sees a different directive
				// than the one that parsed.
				if got := d.Value(k); got != d.KV[k] {
					t.Fatalf("Value(%q) = %q, KV[%q] = %q", k, got, k, d.KV[k])
				}
			}

			// Round-trip: Raw is the directive's own body, so
			// re-parsing it under the same prefix must reproduce the
			// directive. This is the property that catches a silent
			// misparse — the failure mode a crash-only target misses
			// entirely, since a lexer that drops an argument still
			// returns cleanly.
			// The sign is part of the prefix, not of Raw, so it has
			// to be restored to reconstruct the original text.
			sign := "+"
			if d.Negated {
				sign = "-"
			}
			again, err := p.Parse(sign+"gen:"+d.Raw, position.Pos{})
			if err != nil {
				t.Fatalf("re-parsing Raw %q of %q failed: %v", d.Raw, text, err)
			}
			if len(again) != 1 {
				t.Fatalf("re-parsing Raw %q yielded %d directives, want 1", d.Raw, len(again))
			}
			r := again[0]
			if r.Name != d.Name || r.Negated != d.Negated {
				t.Fatalf("round-trip changed identity: %q/%v became %q/%v",
					d.Name, d.Negated, r.Name, r.Negated)
			}
			if !slices.Equal(r.Args, d.Args) {
				t.Fatalf("round-trip changed args: %q became %q", d.Args, r.Args)
			}
			if !maps.Equal(r.KV, d.KV) {
				t.Fatalf("round-trip changed kv: %v became %v", d.KV, r.KV)
			}
		}
	})
}

// BenchmarkParser_Parse measures one Parse of a comment body carrying
// n directives.
//
// Parse is the per-comment cost of a run: the pipeline hands it every
// comment in every source file it scans, so the number here is
// multiplied by the comment count of the whole tree, not by the
// directive count. That makes even a small constant factor visible on
// large inputs, and it is why the parser is a hand-rolled lexer rather
// than a regexp.
//
// The scaling axis is directives-per-comment because that is the axis
// on which the greedy scan could go quadratic: parseOne re-tests all
// three prefix forms at every whitespace boundary of every argument
// list, and the sub-slicing that feeds those tests has to stay O(1).
// Growth worse than linear across the sizes below is the signal that
// it has stopped being.
//
// The Parser is constructed once, and each body is built above its own
// sub-benchmark; only Parse is timed. Parse allocates a Directive and a
// KV map per directive, so allocations are expected to track n — a
// flat alloc count across sizes would mean the loop body was folded
// away. The post-loop count assertion guards the same thing.
func BenchmarkParser_Parse(b *testing.B) {
	b.ReportAllocs()

	p, err := directive.NewParser("gen")
	if err != nil {
		b.Fatalf("NewParser: %v", err)
	}

	for _, n := range []int{1, 10, 100, 1000} {
		text := benchDirectiveLine(n)
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()

			var ds []*directive.Directive
			for b.Loop() {
				got, parseErr := p.Parse(text, position.Pos{})
				if parseErr != nil {
					b.Fatalf("Parse: %v", parseErr)
				}
				ds = got
			}
			if len(ds) != n {
				b.Fatalf("Parse returned %d directives, want %d", len(ds), n)
			}
		})
	}
}

// benchDirectiveLine builds a comment body carrying exactly n
// directives, each with one positional argument and one key/value pair.
//
// The shape mirrors what a real annotation looks like rather than the
// cheapest thing the parser accepts: a bare `+gen:x` would skip the
// argument loop entirely, which is where the per-boundary prefix
// re-testing this benchmark exists to measure actually happens. Names
// are distinct so no accidental interning flatters the numbers.
func benchDirectiveLine(n int) string {
	var b strings.Builder
	for i := range n {
		if i > 0 {
			b.WriteByte(' ')
		}
		id := strconv.Itoa(i)
		b.WriteString("+gen:stub")
		b.WriteString(id)
		b.WriteString(" positional")
		b.WriteString(id)
		b.WriteString(" out=file")
		b.WriteString(id)
		b.WriteString(".go")
	}
	return b.String()
}

// BenchmarkParser_ParseComment measures the entry point the pipeline
// actually calls per line.
//
// [Parser.Parse] takes a comment body already known to be a directive
// candidate; ParseComment takes a raw comment line and decides. Every
// doc-comment line on every converted declaration reaches it, and a
// grep of this tree finds 51 lines that get past it — so the reject
// path is the hot one by three orders of magnitude, and it is the one
// with no benchmark and no budget.
//
// The budget is zero allocations. The early-out is currently a prefix
// test and nothing more; a future regexp, ToLower or TrimSpace that
// allocated would be invisible in every other number this package
// records.
func BenchmarkParser_ParseComment(b *testing.B) {
	p := directive.DefaultParser()

	for _, tc := range []struct {
		name string
		line string
	}{
		{"plain_doc_line", "// Config is the runtime configuration for the service."},
		{"leading_whitespace", "//    indented prose that is not a directive"},
		{"block_comment", "/* a block comment body */"},
		{"near_miss", "// generated by something else, not a directive"},
		{"directive", "// +gen:builder"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				//nolint:errcheck // the reject path returns no error; the accept path is not asserted here
				_, _ = p.ParseComment(tc.line, position.Pos{})
			}
		})
	}
}

// TestParseComment_RejectPathDoesNotAllocate enforces the budget
// BenchmarkParser_ParseComment records.
//
// A benchmark fails no build. This is the hot path of the whole
// frontend — every doc-comment line of every converted declaration
// reaches it and almost none get past — so an allocation introduced
// here would be multiplied by the comment count of the entire tree
// and would show up in no other number this package records.
//
// Not parallel: testing.AllocsPerRun panics in a parallel test.
//
//nolint:paralleltest // testing.AllocsPerRun panics in a parallel test.
func TestParseComment_RejectPathDoesNotAllocate(t *testing.T) {
	p := directive.DefaultParser()

	for _, line := range []string{
		"// Config is the runtime configuration for the service.",
		"//    indented prose that is not a directive",
		"/* a block comment body */",
		"// generated by something else, not a directive",
	} {
		got := testing.AllocsPerRun(50, func() {
			//nolint:errcheck // the reject path returns no error
			_, _ = p.ParseComment(line, position.Pos{})
		})
		if got != 0 {
			t.Fatalf("ParseComment(%q) allocated %v times per op, want 0", line, got)
		}
	}
}
