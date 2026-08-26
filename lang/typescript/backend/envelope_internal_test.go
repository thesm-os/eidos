// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import "testing"

func TestFinalise(t *testing.T) {
	t.Parallel()

	t.Run("collapses a run of blank lines to one", func(t *testing.T) {
		t.Parallel()
		// Several blanks are an artefact of template whitespace
		// control rather than anything a template meant to say.
		if got := string(finalise("a\n\n\n\nb\n")); got != "a\n\nb\n" {
			t.Fatalf("finalise = %q", got)
		}
	})

	t.Run("strips trailing whitespace", func(t *testing.T) {
		t.Parallel()
		if got := string(finalise("a   \nb\t\n")); got != "a\nb\n" {
			t.Fatalf("finalise = %q", got)
		}
	})

	t.Run("ends the file with exactly one newline", func(t *testing.T) {
		t.Parallel()
		for _, in := range []string{"a", "a\n", "a\n\n\n"} {
			if got := string(finalise(in)); got != "a\n" {
				t.Errorf("finalise(%q) = %q, want a single trailing newline", in, got)
			}
		}
	})

	t.Run("an empty body writes no file", func(t *testing.T) {
		t.Parallel()
		for _, in := range []string{"", "\n", "   \n\n"} {
			if got := finalise(in); got != nil {
				t.Errorf("finalise(%q) = %q, want nothing", in, got)
			}
		}
	})
}
