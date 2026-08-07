// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package-internal tests for the pre-flight workspace check.
//
// Every helper here is unexported and the only blackbox surface is a
// notice on stderr that the command emits before doing any real
// work. Pinning "did the parser resolve this path" through that
// single string would make each case depend on the whole pipeline
// starting up, so the parsing and path-resolution decisions are made
// here and the command-level wiring stays in the blackbox tests.
package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFileAt creates dir (and parents) and writes body to
// dir/name, returning the directory.
func writeFileAt(t *testing.T, dir, name, body string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return dir
}

func TestStripComment(t *testing.T) {
	t.Parallel()

	t.Run("a line with no comment is returned unchanged", func(t *testing.T) {
		t.Parallel()
		if got := stripComment("./backend/golang"); got != "./backend/golang" {
			t.Fatalf("stripComment = %q, want the line unchanged", got)
		}
	})

	t.Run("a trailing comment is dropped", func(t *testing.T) {
		t.Parallel()
		if got := stripComment("./cli // the command surface"); got != "./cli" {
			t.Fatalf("stripComment = %q, want ./cli", got)
		}
	})

	t.Run("a comment-only line becomes empty", func(t *testing.T) {
		t.Parallel()
		if got := stripComment("// just a note"); got != "" {
			t.Fatalf("stripComment = %q, want empty", got)
		}
	})

	t.Run("whitespace around the surviving text is trimmed", func(t *testing.T) {
		t.Parallel()
		if got := stripComment("  ./cli   // note"); got != "./cli" {
			t.Fatalf("stripComment = %q, want ./cli", got)
		}
	})
}

func TestPathMatches(t *testing.T) {
	t.Parallel()

	t.Run("a relative entry resolves against the workspace directory", func(t *testing.T) {
		t.Parallel()
		if !pathMatches("/w", "./cli", "/w/cli") {
			t.Fatalf("./cli under /w must match /w/cli")
		}
	})

	t.Run("an absolute entry is used as-is", func(t *testing.T) {
		t.Parallel()
		if !pathMatches("/w", "/elsewhere/cli", "/elsewhere/cli") {
			t.Fatalf("an absolute entry must not be re-rooted at the workspace")
		}
	})

	t.Run("a quoted entry is unquoted before matching", func(t *testing.T) {
		t.Parallel()
		if !pathMatches("/w", `"./cli"`, "/w/cli") {
			t.Fatalf("a quoted use entry must still match")
		}
	})

	t.Run("surrounding whitespace is tolerated", func(t *testing.T) {
		t.Parallel()
		if !pathMatches("/w", "   ./cli  ", "/w/cli") {
			t.Fatalf("a padded use entry must still match")
		}
	})

	t.Run("an unrelated entry does not match", func(t *testing.T) {
		t.Parallel()
		if pathMatches("/w", "./backend", "/w/cli") {
			t.Fatalf("./backend must not match /w/cli")
		}
	})

	t.Run("an empty entry does not match", func(t *testing.T) {
		t.Parallel()
		if pathMatches("/w", "", "/w/cli") {
			t.Fatalf("an empty entry must never match")
		}
	})

	t.Run("an uncleaned entry matches its cleaned form", func(t *testing.T) {
		t.Parallel()
		// go.work files are hand-edited, so `./cli/.` and
		// `./x/../cli` both appear in the wild and name the same
		// module as `./cli`.
		if !pathMatches("/w", "./x/../cli", "/w/cli") {
			t.Fatalf("an uncleaned entry must resolve to the same module")
		}
	})
}

func TestFindUp(t *testing.T) {
	t.Parallel()

	t.Run("finds the file in the starting directory", func(t *testing.T) {
		t.Parallel()
		root := writeFileAt(t, t.TempDir(), "go.mod", "module x\n")
		got, ok := findUp(root, "go.mod")
		if !ok || got != root {
			t.Fatalf("findUp = (%q, %v), want (%q, true)", got, ok, root)
		}
	})

	t.Run("walks up to an ancestor directory", func(t *testing.T) {
		t.Parallel()
		root := writeFileAt(t, t.TempDir(), "go.mod", "module x\n")
		deep := filepath.Join(root, "a", "b")
		if err := os.MkdirAll(deep, 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		got, ok := findUp(deep, "go.mod")
		if !ok || got != root {
			t.Fatalf("findUp = (%q, %v), want (%q, true)", got, ok, root)
		}
	})

	t.Run("returns the full path for go.work rather than its directory", func(t *testing.T) {
		t.Parallel()
		// The asymmetry is load-bearing: the caller passes the
		// result straight to the workfile parser, while the go.mod
		// result is compared against use-block entries as a
		// directory.
		root := writeFileAt(t, t.TempDir(), "go.work", "go 1.26\n")
		got, ok := findUp(root, "go.work")
		if !ok || got != filepath.Join(root, "go.work") {
			t.Fatalf("findUp = (%q, %v), want the go.work file path", got, ok)
		}
	})

	t.Run("reports not-found when no ancestor carries the file", func(t *testing.T) {
		t.Parallel()
		if _, ok := findUp(t.TempDir(), "definitely-absent.txt"); ok {
			t.Fatalf("findUp must report not-found rather than walking to the root forever")
		}
	})

	t.Run("an empty start reports not-found", func(t *testing.T) {
		t.Parallel()
		if _, ok := findUp("", "go.mod"); ok {
			t.Fatalf("an empty start must report not-found")
		}
	})
}

func TestModuleInWorkspaceUse(t *testing.T) {
	t.Parallel()

	// workspaceWith writes a go.work carrying body plus a `cli`
	// module directory, and returns (workPath, modDir).
	workspaceWith := func(t *testing.T, body string) (string, string) {
		t.Helper()
		root := t.TempDir()
		writeFileAt(t, root, "go.work", body)
		mod := writeFileAt(t, filepath.Join(root, "cli"), "go.mod", "module x/cli\n")
		return filepath.Join(root, "go.work"), mod
	}

	t.Run("a module listed in a use block is found", func(t *testing.T) {
		t.Parallel()
		work, mod := workspaceWith(t, "go 1.26\n\nuse (\n\t./cli\n\t./backend\n)\n")
		listed, ok := moduleInWorkspaceUse(work, mod)
		if !ok || !listed {
			t.Fatalf("moduleInWorkspaceUse = (%v, %v), want (true, true)", listed, ok)
		}
	})

	t.Run("a module absent from the use block is not found", func(t *testing.T) {
		t.Parallel()
		work, mod := workspaceWith(t, "go 1.26\n\nuse (\n\t./backend\n)\n")
		listed, ok := moduleInWorkspaceUse(work, mod)
		if !ok || listed {
			t.Fatalf("moduleInWorkspaceUse = (%v, %v), want (false, true)", listed, ok)
		}
	})

	t.Run("the single-line use form is recognised", func(t *testing.T) {
		t.Parallel()
		work, mod := workspaceWith(t, "go 1.26\n\nuse ./cli\n")
		listed, ok := moduleInWorkspaceUse(work, mod)
		if !ok || !listed {
			t.Fatalf("the bare `use <path>` form must be recognised")
		}
	})

	t.Run("a commented-out entry does not count as listed", func(t *testing.T) {
		t.Parallel()
		// Commenting a module out of the workspace is exactly how a
		// user reaches the state this notice exists to explain, so
		// reading through the comment would suppress the one warning
		// they need.
		work, mod := workspaceWith(t, "go 1.26\n\nuse (\n\t// ./cli\n\t./backend\n)\n")
		listed, ok := moduleInWorkspaceUse(work, mod)
		if !ok || listed {
			t.Fatalf("a commented entry must not read as listed")
		}
	})

	t.Run("a trailing comment on a listed entry is ignored", func(t *testing.T) {
		t.Parallel()
		work, mod := workspaceWith(t, "go 1.26\n\nuse (\n\t./cli // the command surface\n)\n")
		listed, ok := moduleInWorkspaceUse(work, mod)
		if !ok || !listed {
			t.Fatalf("a trailing comment must not hide the entry")
		}
	})

	t.Run("entries after a closed block are not treated as uses", func(t *testing.T) {
		t.Parallel()
		work, mod := workspaceWith(t, "go 1.26\n\nuse (\n\t./backend\n)\n\nreplace ./cli => ./other\n")
		listed, ok := moduleInWorkspaceUse(work, mod)
		if !ok || listed {
			t.Fatalf("a path outside the use block must not count as listed")
		}
	})

	t.Run("an unreadable workfile reports unknown rather than absent", func(t *testing.T) {
		t.Parallel()
		// Unknown suppresses the notice; reporting "absent" would
		// warn every user whose workfile happens to be unreadable.
		missing := filepath.Join(t.TempDir(), "go.work")
		listed, ok := moduleInWorkspaceUse(missing, t.TempDir())
		if ok || listed {
			t.Fatalf("moduleInWorkspaceUse = (%v, %v), want (false, false)", listed, ok)
		}
	})
}

func TestEmitWorkspaceNotice(t *testing.T) {
	t.Parallel()

	// notice renders the notice for a module nested under a
	// workspace and returns the stderr text.
	notice := func(t *testing.T) string {
		t.Helper()
		root := t.TempDir()
		mod := filepath.Join(root, "cli")
		stderr := &bytes.Buffer{}
		env := &Env{Brand: "eidos", Workdir: mod, Stderr: stderr}
		emitWorkspaceNotice(env, mod, filepath.Join(root, "go.work"))
		return stderr.String()
	}

	t.Run("names the brand so the notice is attributable", func(t *testing.T) {
		t.Parallel()
		if !strings.HasPrefix(notice(t), "eidos: notice:") {
			t.Fatalf("notice must be attributable to the binary; got %q", notice(t))
		}
	})

	t.Run("quotes the loader error the user is about to hit", func(t *testing.T) {
		t.Parallel()
		if !strings.Contains(notice(t), "does not contain modules listed in go.work") {
			t.Fatalf("the notice exists to pre-empt that error; got %q", notice(t))
		}
	})

	t.Run("names all three fixes", func(t *testing.T) {
		t.Parallel()
		out := notice(t)
		for _, fix := range []string{"GOWORK=off", "ignore_workspace", "use() block"} {
			if !strings.Contains(out, fix) {
				t.Errorf("notice omitted the %q fix; got %q", fix, out)
			}
		}
	})

	t.Run("renders the workspace path relative to the module", func(t *testing.T) {
		t.Parallel()
		if !strings.Contains(notice(t), filepath.Join("..", "go.work")) {
			t.Fatalf("expected a module-relative workspace path; got %q", notice(t))
		}
	})
}

// TestPreflightWorkspaceCheck covers the orchestration: which
// directory layouts warrant a notice and which are silent. A false
// positive here trains users to ignore the one message that explains
// an otherwise cryptic loader failure.
//
// Not parallel: the check consults GOWORK, and t.Setenv is
// incompatible with t.Parallel.
//
//nolint:paralleltest // t.Setenv pins GOWORK; see above.
func TestPreflightWorkspaceCheck(t *testing.T) {
	// run builds a layout, invokes the check, and returns stderr.
	run := func(t *testing.T, gowork string, build func(t *testing.T, root string) string) string {
		t.Helper()
		t.Setenv("GOWORK", gowork)
		root := t.TempDir()
		workdir := build(t, root)
		stderr := &bytes.Buffer{}
		preflightWorkspaceCheck(&Env{Brand: "eidos", Workdir: workdir, Stderr: stderr})
		return stderr.String()
	}

	// nestedUnlisted is the layout the notice exists for: a module
	// parked inside a project whose workspace does not list it.
	nestedUnlisted := func(t *testing.T, root string) string {
		t.Helper()
		writeFileAt(t, root, "go.work", "go 1.26\n\nuse (\n\t./other\n)\n")
		return writeFileAt(t, filepath.Join(root, "fixture"), "go.mod", "module f\n")
	}

	t.Run("warns for a module the enclosing workspace does not list", func(t *testing.T) {
		if out := run(t, "", nestedUnlisted); !strings.Contains(out, "not listed in the workspace") {
			t.Fatalf("expected the notice; got %q", out)
		}
	})

	t.Run("stays silent when GOWORK is off", func(t *testing.T) {
		// The loader will not consult go.work at all, so there is no
		// conflict to report.
		if out := run(t, "off", nestedUnlisted); out != "" {
			t.Fatalf("GOWORK=off must suppress the notice; got %q", out)
		}
	})

	t.Run("stays silent when the module is listed", func(t *testing.T) {
		out := run(t, "", func(t *testing.T, root string) string {
			t.Helper()
			writeFileAt(t, root, "go.work", "go 1.26\n\nuse (\n\t./fixture\n)\n")
			return writeFileAt(t, filepath.Join(root, "fixture"), "go.mod", "module f\n")
		})
		if out != "" {
			t.Fatalf("a listed module must not warn; got %q", out)
		}
	})

	t.Run("stays silent when the workspace is the module's own", func(t *testing.T) {
		out := run(t, "", func(t *testing.T, root string) string {
			t.Helper()
			writeFileAt(t, root, "go.work", "go 1.26\n")
			return writeFileAt(t, root, "go.mod", "module f\n")
		})
		if out != "" {
			t.Fatalf("a module owning its workspace must not warn; got %q", out)
		}
	})

	t.Run("stays silent when there is no enclosing workspace", func(t *testing.T) {
		out := run(t, "", func(t *testing.T, root string) string {
			t.Helper()
			return writeFileAt(t, root, "go.mod", "module f\n")
		})
		if out != "" {
			t.Fatalf("a standalone module must not warn; got %q", out)
		}
	})

	t.Run("stays silent when there is no module at all", func(t *testing.T) {
		out := run(t, "", func(t *testing.T, root string) string {
			t.Helper()
			return root
		})
		if out != "" {
			t.Fatalf("a directory outside any module must not warn; got %q", out)
		}
	})
}
