// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cli_test

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/cli"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/plugin"
)

// TestCheckCommand_NoDrift covers the happy path: when the
// on-disk file matches the pipeline's output, the command
// reports zero drift and exits ExitOK.
func TestCheckCommand_NoDrift(t *testing.T) {
	t.Parallel()

	t.Run("matching on-disk content reports no drift", func(t *testing.T) {
		t.Parallel()
		env, stdout, _ := freshEnv(t, "eidos")
		body := []byte("// generated\npackage x\n")
		tgt := emit.Target{Dir: "out", Filename: "hello.go", Package: "x"}
		// Seed the on-disk state to match what the pipeline will write.
		if err := os.MkdirAll(filepath.Join(env.Workdir, tgt.Dir), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(env.Workdir, tgt.Dir, tgt.Filename), body, 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		cmd := &cli.CheckCommand{Config: cli.CheckConfig{
			Plugins: []plugin.Plugin{
				stubFrontend{name: "fe"},
				writingBackend{name: "be", body: body, tgt: tgt},
			},
		}}
		if code := cmd.Execute(t.Context(), env); code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout.String(), "no drift detected") {
			t.Fatalf("expected no-drift line; got %q", stdout.String())
		}
	})
}

// TestCheckCommand_ContentDrift covers the drift path: when the
// on-disk file's bytes differ from the pipeline's output, the
// command exits ExitCheckDrift.
func TestCheckCommand_ContentDrift(t *testing.T) {
	t.Parallel()

	t.Run("different on-disk bytes report drift", func(t *testing.T) {
		t.Parallel()
		env, stdout, _ := freshEnv(t, "eidos")
		body := []byte("// generated\npackage x\n")
		stale := []byte("// stale\npackage x\n")
		tgt := emit.Target{Dir: "out", Filename: "hello.go", Package: "x"}
		if err := os.MkdirAll(filepath.Join(env.Workdir, tgt.Dir), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(env.Workdir, tgt.Dir, tgt.Filename), stale, 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		cmd := &cli.CheckCommand{Config: cli.CheckConfig{
			Plugins: []plugin.Plugin{
				stubFrontend{name: "fe"},
				writingBackend{name: "be", body: body, tgt: tgt},
			},
		}}
		if code := cmd.Execute(t.Context(), env); code != cli.ExitCheckDrift {
			t.Fatalf("Execute = %d, want ExitCheckDrift", code)
		}
		if !strings.Contains(stdout.String(), "content differs") {
			t.Fatalf("expected content-differs line; got %q", stdout.String())
		}
	})
}

// TestCheckCommand_MissingFile covers the missing-on-disk case:
// the pipeline produced a file that doesn't exist on disk — also
// drift, treated as such by CI gates.
func TestCheckCommand_MissingFile(t *testing.T) {
	t.Parallel()

	t.Run("file missing from disk is drift", func(t *testing.T) {
		t.Parallel()
		env, stdout, _ := freshEnv(t, "eidos")
		body := []byte("// generated\npackage x\n")
		tgt := emit.Target{Dir: "out", Filename: "hello.go", Package: "x"}
		// Don't seed the disk — the pipeline output has no counterpart.
		cmd := &cli.CheckCommand{Config: cli.CheckConfig{
			Plugins: []plugin.Plugin{
				stubFrontend{name: "fe"},
				writingBackend{name: "be", body: body, tgt: tgt},
			},
		}}
		if code := cmd.Execute(t.Context(), env); code != cli.ExitCheckDrift {
			t.Fatalf("Execute = %d, want ExitCheckDrift", code)
		}
		if !strings.Contains(stdout.String(), "missing from disk") {
			t.Fatalf("expected missing-from-disk line; got %q", stdout.String())
		}
	})
}

// TestCheckCommand_ProvenanceMismatch covers the body-edit drift
// surface: a file whose stamped provenance hash still matches the
// in-memory render's stamped hash counts as drift when the on-disk
// body bytes themselves no longer hash to that value — the
// drift-detection contract must catch manual edits between the
// header and the trailer even when the trailer stays untouched.
func TestCheckCommand_ProvenanceMismatch(t *testing.T) {
	t.Parallel()

	t.Run("body edited but trailer untouched is drift", func(t *testing.T) {
		t.Parallel()
		env, stdout, _ := freshEnv(t, "eidos")
		const cleanBody = "package x\n\nfunc Hello() {}\n"
		mem := framedFile(cleanBody)
		// Disk has tampered body bytes but keeps the original
		// stamped hash so the trailer agrees with the in-memory
		// render at the hash level.
		hash := sha256Hex(cleanBody)
		tampered := framedFileWith("test", "package x\n\nfunc Tampered() {}\n", hash)
		tgt := emit.Target{Dir: "out", Filename: "hello.go", Package: "x"}
		if err := os.MkdirAll(filepath.Join(env.Workdir, tgt.Dir), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(env.Workdir, tgt.Dir, tgt.Filename), tampered, 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		cmd := &cli.CheckCommand{Config: cli.CheckConfig{
			Plugins: []plugin.Plugin{
				stubFrontend{name: "fe"},
				writingBackend{name: "be", body: mem, tgt: tgt},
			},
		}}
		if code := cmd.Execute(t.Context(), env); code != cli.ExitCheckDrift {
			t.Fatalf("Execute = %d, want ExitCheckDrift", code)
		}
		if !strings.Contains(stdout.String(), "content differs") {
			t.Fatalf("expected content-differs line; got %q", stdout.String())
		}
	})
}

// TestCheckCommand_AppendedAfterTrailer covers the
// post-trailer-tampering drift surface: bytes appended to the
// file *after* the provenance line surface as drift, even though
// the stamped hash and the actual body bytes both stay coherent
// with the in-memory render.
func TestCheckCommand_AppendedAfterTrailer(t *testing.T) {
	t.Parallel()

	t.Run("bytes appended after the trailer are drift", func(t *testing.T) {
		t.Parallel()
		env, stdout, _ := freshEnv(t, "eidos")
		const cleanBody = "package x\n\nfunc Hello() {}\n"
		mem := framedFile(cleanBody)
		appended := append([]byte(nil), mem...)
		appended = append(appended, "// trailing junk\n"...)
		tgt := emit.Target{Dir: "out", Filename: "hello.go", Package: "x"}
		if err := os.MkdirAll(filepath.Join(env.Workdir, tgt.Dir), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(env.Workdir, tgt.Dir, tgt.Filename), appended, 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		cmd := &cli.CheckCommand{Config: cli.CheckConfig{
			Plugins: []plugin.Plugin{
				stubFrontend{name: "fe"},
				writingBackend{name: "be", body: mem, tgt: tgt},
			},
		}}
		if code := cmd.Execute(t.Context(), env); code != cli.ExitCheckDrift {
			t.Fatalf("Execute = %d, want ExitCheckDrift", code)
		}
		if !strings.Contains(stdout.String(), "content differs") {
			t.Fatalf("expected content-differs line; got %q", stdout.String())
		}
	})
}

// TestCheckCommand_HeaderOnlyDifference pins the spec contract
// that header lines varying between `eidos run` and `eidos check`
// (Command, Plugins, Source paths) do not surface as drift —
// provenance is over body bytes only, so the rendered files
// agree at the hash level even when the header text differs.
func TestCheckCommand_HeaderOnlyDifference(t *testing.T) {
	t.Parallel()

	t.Run("differing header lines with matching body are not drift", func(t *testing.T) {
		t.Parallel()
		env, stdout, _ := freshEnv(t, "eidos")
		const bodyText = "package x\n\nfunc Hello() {}\n"
		hash := sha256Hex(bodyText)
		mem := framedFileWith("check ./...", bodyText, hash)
		disk := framedFileWith("run ./...", bodyText, hash)
		tgt := emit.Target{Dir: "out", Filename: "hello.go", Package: "x"}
		if err := os.MkdirAll(filepath.Join(env.Workdir, tgt.Dir), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(env.Workdir, tgt.Dir, tgt.Filename), disk, 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		cmd := &cli.CheckCommand{Config: cli.CheckConfig{
			Plugins: []plugin.Plugin{
				stubFrontend{name: "fe"},
				writingBackend{name: "be", body: mem, tgt: tgt},
			},
		}}
		if code := cmd.Execute(t.Context(), env); code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK (header-only differences); got %s", code, stdout.String())
		}
		if !strings.Contains(stdout.String(), "no drift detected") {
			t.Fatalf("expected no-drift summary; got %q", stdout.String())
		}
	})
}

// framedFile wraps body in the canonical eidos header / footer
// envelope so the check command's provenance-hash comparison has
// a realistic input to parse. The stamped hash is computed over
// body's bytes so the result is a well-formed eidos file.
func framedFile(body string) []byte {
	return framedFileWith("test", body, sha256Hex(body))
}

// framedFileWith mirrors framedFile but lets callers pin the
// Command-line stamp and the trailer hash independently. Tests
// that exercise stale-stamp or post-trailer-tampering scenarios
// supply a mismatching hash or extend the returned bytes.
func framedFileWith(command, body, hash string) []byte {
	header := "// Code generated by eidos. DO NOT EDIT.\n" +
		"//\n" +
		"// Source:    test.go\n" +
		"// Plugins:   stub\n" +
		"// Command:   " + command + "\n\n"
	footer := "\n// eidos: end of generated content.\n" +
		"// eidos:provenance " + hash + "\n"
	return []byte(header + body + footer)
}

// sha256Hex returns the lowercase-hex SHA-256 of s — the body
// hash format every backend stamps into the provenance trailer.
func sha256Hex(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func TestCheckCommand_ConfigError(t *testing.T) {
	t.Parallel()

	t.Run("missing plugin exits ExitUserError", func(t *testing.T) {
		t.Parallel()
		env, _, stderr := freshEnv(t, "eidos")
		cfg := cli.DefaultConfig()
		cfg.Plugins = []cli.ConfigPlugin{{Name: "ghost"}}
		cmd := &cli.CheckCommand{Config: cli.CheckConfig{
			File: cfg,
			Plugins: []plugin.Plugin{
				stubFrontend{name: "fe"},
				stubBackend{name: "be", lang: "stub"},
			},
		}}
		if code := cmd.Execute(t.Context(), env); code != cli.ExitUserError {
			t.Fatalf("Execute = %d, want ExitUserError", code)
		}
		if !strings.Contains(stderr.String(), "ghost") {
			t.Fatalf("stderr should name the missing plugin; got %q", stderr.String())
		}
	})
}

// TestCheckCommand_EmptyPipelineNoDrift covers the no-output case:
// a pipeline with a backend that writes nothing reports zero
// outputs checked and zero drift.
func TestCheckCommand_EmptyPipelineNoDrift(t *testing.T) {
	t.Parallel()

	t.Run("pipeline with no sink writes reports no drift", func(t *testing.T) {
		t.Parallel()
		env, stdout, _ := freshEnv(t, "eidos")
		cmd := &cli.CheckCommand{Config: cli.CheckConfig{
			Plugins: []plugin.Plugin{
				stubFrontend{name: "fe"},
				stubBackend{name: "be", lang: "stub"},
			},
		}}
		if code := cmd.Execute(t.Context(), env); code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK", code)
		}
		if !strings.Contains(stdout.String(), "0 output") {
			t.Fatalf("expected zero-output summary; got %q", stdout.String())
		}
	})
}

// TestCheckCommand_RegisterFlags_Routing pins the routing-flag
// wiring on CheckCommand — parsed values land on
// [cli.CheckConfig.Routing].
func TestCheckCommand_RegisterFlags_Routing(t *testing.T) {
	t.Parallel()

	t.Run("Routing flags parse onto CheckCommand.Config.Routing", func(t *testing.T) {
		t.Parallel()
		var cmd cli.CheckCommand
		fs := flag.NewFlagSet("check", flag.ContinueOnError)
		cmd.RegisterFlags(fs)
		assertRoutingFlagsParse(t, fs, &cmd.Config.Routing)
	})
}

// TestCheckCommand_OrphanedSource covers the report that made this
// class of drift visible at all.
//
// Deleting a source package left an entry in the manifest that no
// command mentioned: prune could not classify it, and check only ever
// walked the targets the run emitted — which by definition excludes
// it. The manifest looked wrong for a reason nothing surfaced.
//
// check probes unconditionally where prune needs a flag: reporting
// costs nothing and deleting costs a file.
func TestCheckCommand_OrphanedSource(t *testing.T) {
	t.Parallel()

	// seedOrphan writes a module, a generated file, and a prior
	// manifest claiming it for an import path that has no directory.
	seedOrphan := func(t *testing.T, env *cli.Env, plugins []plugin.Plugin) {
		t.Helper()
		modBody := "module example.com/m\n\ngo 1.26\n"
		if err := os.WriteFile(filepath.Join(env.Workdir, "go.mod"), []byte(modBody), 0o600); err != nil {
			t.Fatalf("write go.mod: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(env.Workdir, "out"), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		body := []byte("// Code generated by " + env.Brand + ". DO NOT EDIT.\npackage x\n")
		if err := os.WriteFile(filepath.Join(env.Workdir, "out", "orphan.go"), body, 0o600); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
		writePrevManifest(t, env, derivePruneTestPipelineID(t, env, plugins), emit.Target{
			Dir: "out", Filename: "orphan.go", Package: "x",
			ImportPath: "example.com/m/gone",
		})
	}

	t.Run("an orphaned output is reported as drift", func(t *testing.T) {
		t.Parallel()
		env, stdout, _ := freshEnv(t, "eidos")
		plugins := []plugin.Plugin{
			stubFrontend{name: "fe"},
			stubBackend{name: "be", lang: "stub"},
		}
		seedOrphan(t, env, plugins)
		cmd := &cli.CheckCommand{Config: cli.CheckConfig{Plugins: plugins}}
		if code := cmd.Execute(t.Context(), env); code != cli.ExitCheckDrift {
			t.Fatalf("Execute = %d, want ExitCheckDrift", code)
		}
		if !strings.Contains(stdout.String(), "orphaned") {
			t.Fatalf("expected an orphan drift line; got %q", stdout.String())
		}
	})

	t.Run("the report names the remedy", func(t *testing.T) {
		t.Parallel()
		// An operator reading this line must not have to go looking
		// for the flag that acts on it.
		env, stdout, _ := freshEnv(t, "eidos")
		plugins := []plugin.Plugin{
			stubFrontend{name: "fe"},
			stubBackend{name: "be", lang: "stub"},
		}
		seedOrphan(t, env, plugins)
		cmd := &cli.CheckCommand{Config: cli.CheckConfig{Plugins: plugins}}
		_ = cmd.Execute(t.Context(), env)
		if !strings.Contains(stdout.String(), cli.FlagDeletedSources) {
			t.Fatalf("drift line should name the prune flag; got %q", stdout.String())
		}
	})

	t.Run("a package that still exists is not reported", func(t *testing.T) {
		t.Parallel()
		env, stdout, _ := freshEnv(t, "eidos")
		plugins := []plugin.Plugin{
			stubFrontend{name: "fe"},
			stubBackend{name: "be", lang: "stub"},
		}
		seedOrphan(t, env, plugins)
		// Give the "gone" package a directory: it is now merely
		// unloaded, which a narrow run does constantly.
		if err := os.MkdirAll(filepath.Join(env.Workdir, "gone"), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if code := runCheck(t, env, plugins); code != cli.ExitOK {
			t.Fatalf("Execute = %d, want ExitOK; stdout=%q", code, stdout.String())
		}
	})
}

// runCheck executes a CheckCommand against env with the supplied
// plugin universe and returns its exit code.
func runCheck(t *testing.T, env *cli.Env, plugins []plugin.Plugin) int {
	t.Helper()
	cmd := &cli.CheckCommand{Config: cli.CheckConfig{Plugins: plugins}}
	return cmd.Execute(t.Context(), env)
}
