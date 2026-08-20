// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cli_test

import (
	"errors"
	"testing"

	"go.thesmos.sh/eidos/cli"
)

// TestExitCodes pins the stable integer values CI gates compare
// against. Changing any of these is a breaking change to the
// command-line contract.
func TestExitCodes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		got  int
		want int
	}{
		{"ExitOK", cli.ExitOK, 0},
		{"ExitUserError", cli.ExitUserError, 1},
		{"ExitPipelineError", cli.ExitPipelineError, 2},
		{"ExitInternalError", cli.ExitInternalError, 3},
		{"ExitCacheVerifyFailed", cli.ExitCacheVerifyFailed, 4},
		{"ExitCheckDrift", cli.ExitCheckDrift, 5},
	}
	for _, tc := range cases {
		t.Run(tc.name+" pins its value", func(t *testing.T) {
			t.Parallel()
			if tc.got != tc.want {
				t.Fatalf("%s = %d, want %d", tc.name, tc.got, tc.want)
			}
		})
	}
}

// TestDiagFormat_StringAndSet covers the flag.Value interface
// implementation: String round-trips through Set, unrecognised
// values return ErrInvalidDiagFormat, and the empty string is
// accepted as the text default.
func TestDiagFormat_StringAndSet(t *testing.T) {
	t.Parallel()

	t.Run("String returns the lower-case form", func(t *testing.T) {
		t.Parallel()
		if cli.DiagFormatText.String() != "text" {
			t.Fatalf("DiagFormatText.String = %q, want %q", cli.DiagFormatText.String(), "text")
		}
		if cli.DiagFormatJSON.String() != "json" {
			t.Fatalf("DiagFormatJSON.String = %q, want %q", cli.DiagFormatJSON.String(), "json")
		}
	})

	t.Run("Set parses text and json", func(t *testing.T) {
		t.Parallel()
		var d cli.DiagFormat
		if err := d.Set("text"); err != nil || d != cli.DiagFormatText {
			t.Fatalf("Set(\"text\") = (%v, %v), want (nil, DiagFormatText)", d, err)
		}
		if err := d.Set("json"); err != nil || d != cli.DiagFormatJSON {
			t.Fatalf("Set(\"json\") = (%v, %v), want (nil, DiagFormatJSON)", d, err)
		}
	})

	t.Run("Set with empty string defaults to text", func(t *testing.T) {
		t.Parallel()
		d := cli.DiagFormatJSON
		if err := d.Set(""); err != nil || d != cli.DiagFormatText {
			t.Fatalf("Set(\"\") should reset to text; got (%v, %v)", d, err)
		}
	})

	t.Run("Set rejects unknown values", func(t *testing.T) {
		t.Parallel()
		var d cli.DiagFormat
		if err := d.Set("xml"); !errors.Is(err, cli.ErrInvalidDiagFormat) {
			t.Fatalf("Set(\"xml\") = %v, want ErrInvalidDiagFormat", err)
		}
	})

	t.Run("String on an out-of-range value falls back to text", func(t *testing.T) {
		t.Parallel()
		d := cli.DiagFormat(99)
		if d.String() != "text" {
			t.Fatalf("DiagFormat(99).String = %q, want %q", d.String(), "text")
		}
	})
}

// TestConfigError covers the error type's three formatting paths
// (path+line, path-only, bare reason) and the errors.Is selector
// that lets callers detect the configuration-fault class without
// inspecting the message.
func TestConfigError(t *testing.T) {
	t.Parallel()

	t.Run("path + line + column renders all three", func(t *testing.T) {
		t.Parallel()
		ce := &cli.ConfigError{Path: ".eidos.yaml", Line: 12, Column: 4, Reason: "unknown key"}
		want := ".eidos.yaml:12:4: unknown key"
		if got := ce.Error(); got != want {
			t.Fatalf("Error = %q, want %q", got, want)
		}
	})

	t.Run("path only renders without line/column", func(t *testing.T) {
		t.Parallel()
		ce := &cli.ConfigError{Path: ".eidos.yaml", Reason: "config file not found"}
		want := ".eidos.yaml: config file not found"
		if got := ce.Error(); got != want {
			t.Fatalf("Error = %q, want %q", got, want)
		}
	})

	t.Run("bare reason renders without path", func(t *testing.T) {
		t.Parallel()
		ce := &cli.ConfigError{Reason: "Env.Brand is required"}
		if got := ce.Error(); got != "Env.Brand is required" {
			t.Fatalf("Error = %q, want %q", got, "Env.Brand is required")
		}
	})

	t.Run("Is detects ConfigError via errors.Is", func(t *testing.T) {
		t.Parallel()
		wrapped := errors.Join(&cli.ConfigError{Reason: "x"}, errors.New("other"))
		if _, ok := errors.AsType[*cli.ConfigError](wrapped); !ok {
			t.Fatalf("errors.As should find the embedded ConfigError")
		}
	})

	t.Run("Is(target) returns true for any *ConfigError", func(t *testing.T) {
		t.Parallel()
		err := &cli.ConfigError{Reason: "concrete"}
		other := &cli.ConfigError{Reason: "abstract"}
		// (*ConfigError).Is implements `Is(target error) bool` so
		// errors.Is matches any *ConfigError regardless of fields.
		if !errors.Is(err, other) {
			t.Fatalf("errors.Is should treat any two *ConfigError values as the same class")
		}
		if errors.Is(err, errors.New("not a config error")) {
			t.Fatalf("errors.Is should reject non-ConfigError targets")
		}
	})
}
