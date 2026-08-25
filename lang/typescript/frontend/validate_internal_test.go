// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
)

// diagnose parses src and returns the diagnostics reporting its
// syntax errors.
func diagnose(t *testing.T, src string) []diag.Diag {
	t.Helper()
	p, err := parseFile("broken.ts", []byte(src))
	if err != nil {
		t.Fatalf("parseFile: %v", err)
	}
	t.Cleanup(p.close)

	sink := diag.New()
	c := newConv(p, "./x", sink.For(FrontendName), directive.DefaultParser())
	c.reportSyntaxErrors()
	return sink.Diagnostics()
}

func TestReportSyntaxErrors(t *testing.T) {
	t.Parallel()

	t.Run("a well-formed file reports nothing", func(t *testing.T) {
		t.Parallel()
		if got := diagnose(t, "export interface A { id: string; }\n"); len(got) != 0 {
			t.Fatalf("diagnostics = %+v, want none", got)
		}
	})

	t.Run("a syntax error is reported with a position", func(t *testing.T) {
		t.Parallel()
		// Warn rather than Error: tree-sitter recovers, so
		// declarations either side of the error still convert and
		// dropping the file would discard them.
		got := diagnose(t, "export interface A { id: \nexport interface B {}\n")
		if len(got) == 0 {
			t.Fatal("a malformed file produced no diagnostic")
		}
		for _, d := range got {
			if d.Severity != diag.Warn {
				t.Errorf("severity = %v, want Warn", d.Severity)
			}
			if d.Pos.IsZero() {
				t.Error("a diagnostic carried no position")
			}
		}
	})

	t.Run("a missing token names what is missing", func(t *testing.T) {
		t.Parallel()
		got := diagnose(t, "class C { m( {} }\n")
		if len(got) == 0 {
			t.Fatal("no diagnostic for a missing token")
		}
		messages := make([]string, 0, len(got))
		for _, d := range got {
			messages = append(messages, d.Message)
		}
		joined := strings.Join(messages, "\n")
		if !strings.Contains(joined, "missing") && !strings.Contains(joined, "unexpected") {
			t.Fatalf("diagnostics do not describe the problem:\n%s", joined)
		}
	})

	t.Run("a cascade is bounded rather than reported per line", func(t *testing.T) {
		t.Parallel()
		// One missing brace early leaves everything after it
		// mis-parsed. The first few reports name the real problem; the
		// rest are its shadow.
		var b strings.Builder
		b.WriteString("class C {\n")
		for range 40 {
			b.WriteString("  ] [ } {\n")
		}
		got := diagnose(t, b.String())

		if len(got) == 0 {
			t.Fatal("a cascading failure produced no diagnostic")
		}
		if len(got) > maxSyntaxReports+1 {
			t.Fatalf("diagnostics = %d, want at most %d plus the suppression notice",
				len(got), maxSyntaxReports)
		}
	})

	t.Run("long unparseable input is truncated in the message", func(t *testing.T) {
		t.Parallel()
		got := diagnose(t, "class C { "+strings.Repeat("@", 120)+" }\n")
		for _, d := range got {
			if len(d.Message) > 200 {
				t.Fatalf("a diagnostic message ran to %d characters", len(d.Message))
			}
		}
	})
}
