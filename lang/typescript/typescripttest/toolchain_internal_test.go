// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package typescripttest

import (
	"fmt"
	"strings"
	"testing"
)

// missingSpy records what [reportMissing] did instead of stopping the
// goroutine, which both of the real methods do.
//
// A second stand-in rather than the external tests' spy: that one
// lives in package typescripttest_test and [reportMissing] is
// unexported, so the two cannot see each other. [testing.TB] is
// sealed, so embedding it is the only way to hold one either way.
type missingSpy struct {
	testing.TB

	failed  bool
	skipped bool
	msg     string
}

func (s *missingSpy) Fatalf(format string, args ...any) {
	s.failed = true
	s.msg = fmt.Sprintf(format, args...)
}

func (s *missingSpy) Skip(args ...any) {
	s.skipped = true
	s.msg = fmt.Sprint(args...)
}

func TestReportMissing(t *testing.T) {
	t.Parallel()

	t.Run("an unset environment skips", func(t *testing.T) {
		t.Parallel()
		s := &missingSpy{TB: t}
		reportMissing(s, "", "no compiler answered", "install one")
		if !s.skipped || s.failed {
			t.Fatalf("skipped=%v failed=%v, want a skip: %s", s.skipped, s.failed, s.msg)
		}
	})

	t.Run("a set environment fails", func(t *testing.T) {
		t.Parallel()
		s := &missingSpy{TB: t}
		reportMissing(s, "required", "no compiler answered", "install one")
		if !s.failed || s.skipped {
			t.Fatalf("failed=%v skipped=%v, want a failure: %s", s.failed, s.skipped, s.msg)
		}
	})

	t.Run("a value the workflow did not spell still fails", func(t *testing.T) {
		t.Parallel()
		// The whole point of reading non-empty rather than one word: a
		// job that sets the variable to `1` or `true` meant to demand
		// the toolchain, and a spelling check would hand it a green
		// build that checked nothing.
		for _, setting := range []string{"1", "true", "yes", "REQUIRED"} {
			s := &missingSpy{TB: t}
			reportMissing(s, setting, "no compiler answered", "install one")
			if !s.failed {
				t.Errorf("%s=%q skipped, want a failure", toolchainEnv, setting)
			}
		}
	})

	t.Run("the failure names the variable and the fix", func(t *testing.T) {
		t.Parallel()
		// A job hitting this reads the log and nothing else. Without
		// the variable it cannot tell a demanded toolchain from a
		// generator that broke, and without the fix it cannot act.
		s := &missingSpy{TB: t}
		reportMissing(s, "required", "no compiler answered", "install one with `npm i -D typescript`")
		for _, want := range []string{toolchainEnv, "required", "no compiler answered", "npm i -D typescript"} {
			if !strings.Contains(s.msg, want) {
				t.Errorf("the failure does not mention %q: %s", want, s.msg)
			}
		}
	})
}

func TestIsVersionLine(t *testing.T) {
	t.Parallel()

	t.Run("what tsc prints is accepted", func(t *testing.T) {
		t.Parallel()
		for _, out := range []string{"Version 5.9.3", "Version 7.0.2\n", "  Version 6.0.0-dev  \n"} {
			if !isVersionLine(out) {
				t.Errorf("isVersionLine(%q) = false, want true", out)
			}
		}
	})

	t.Run("anything else is rejected", func(t *testing.T) {
		t.Parallel()
		// The second is what the `tsc` npm package — a 2016 stub, not
		// the compiler — answers with, and the reason the probe reads
		// the output at all.
		for _, out := range []string{
			"",
			"This is not the tsc command you are looking for",
			"Version",
			"Version x",
			"tsc 5.9.3",
		} {
			if isVersionLine(out) {
				t.Errorf("isVersionLine(%q) = true, want false", out)
			}
		}
	})
}
