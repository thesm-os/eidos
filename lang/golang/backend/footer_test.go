// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/plugin"
)

// TestFooter_TwoLineLayout pins the canonical two-line footer
// shape — brand-prefixed end-of-generated-content marker on one
// line, brand-prefixed provenance hash on the next. Splitting them
// lets tools grep each component without parsing a composite line.
func TestFooter_TwoLineLayout(t *testing.T) {
	t.Parallel()

	t.Run("standard footer is two lines: EOGC marker, then hash", func(t *testing.T) {
		t.Parallel()
		body := renderSingleFieldStruct(t, "F", emit.Builtin("int"))
		if !strings.Contains(body, "\n// eidos: end of generated content.\n// eidos:provenance ") {
			t.Fatalf("footer should be a two-line block; got:\n%s", body)
		}
	})
}

// TestFooter_Suffix covers FooterSuffix: extension lines render
// verbatim after the standard two-line block.
func TestFooter_Suffix(t *testing.T) {
	t.Parallel()

	t.Run("FooterSuffix entries trail the standard footer", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		ctx.FooterSuffix = []string{"// Signed-Off-By: release-bot"}
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "X", target,
			fieldSpec{name: "F", builtin: "int"},
		)))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		// Suffix should appear after the hash line.
		if !strings.HasSuffix(string(body), "// Signed-Off-By: release-bot\n") {
			t.Fatalf("FooterSuffix should be last in the file; got:\n%s", body)
		}
		if !strings.Contains(string(body), "// eidos:provenance ") {
			t.Fatalf("standard hash line should still appear; got:\n%s", body)
		}
	})
}

// TestFooter_HashOverBodyOnly pins the provenance-hash semantics:
// the hash is SHA-256 of the body bytes (items 2–8 of the canonical
// layout), with the header and the footer excluded. Two runs over
// the same input produce byte-identical files when the header
// carries no run-dependent fields — and the hash is equal regardless
// of what the header carries.
func TestFooter_HashOverBodyOnly(t *testing.T) {
	t.Parallel()

	t.Run("hash matches a SHA-256 of the body slice (header + footer excluded)", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
			"x", "X", target,
			fieldSpec{name: "F", builtin: "int"},
		)))
		body := assertRenderSucceeds(t, ctx, mem, d, target)
		actualHash := extractFooterHash(t, string(body))
		bodyBytes := extractBodyBytes(t, body)
		sum := sha256.Sum256(bodyBytes)
		expected := hex.EncodeToString(sum[:])
		if actualHash != expected {
			t.Fatalf("footer hash mismatch\nactual:   %s\nexpected: %s\nbody:\n%s", actualHash, expected, bodyBytes)
		}
	})

	t.Run("two runs over the same input produce equal hashes", func(t *testing.T) {
		t.Parallel()
		mkBody := func() []byte {
			ctx, mem, d := newBackendContext(t)
			target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
			addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
				"x", "X", target,
				fieldSpec{name: "ID", builtin: "int"},
				fieldSpec{name: "Name", builtin: "string"},
			)))
			return assertRenderSucceeds(t, ctx, mem, d, target)
		}
		first := mkBody()
		second := mkBody()
		if extractFooterHash(t, string(first)) != extractFooterHash(t, string(second)) {
			t.Fatalf("hash should be byte-stable across runs of the same input")
		}
		if !bytes.Equal(first, second) {
			t.Fatalf("entire file should be byte-identical across runs (no timestamp in header)")
		}
	})

	t.Run("changing the body changes the hash", func(t *testing.T) {
		t.Parallel()
		hashFor := func(fields ...fieldSpec) string {
			ctx, mem, d := newBackendContext(t)
			target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
			addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
				"x", "X", target, fields...,
			)))
			body := assertRenderSucceeds(t, ctx, mem, d, target)
			return extractFooterHash(t, string(body))
		}
		one := hashFor(fieldSpec{name: "A", builtin: "int"})
		two := hashFor(fieldSpec{name: "B", builtin: "int"})
		if one == two {
			t.Fatalf("distinct fields should produce distinct hashes; both = %s", one)
		}
	})

	t.Run("changing only header-affecting context preserves the hash", func(t *testing.T) {
		t.Parallel()
		// The hash covers items 2–8 only; changes to Command,
		// Plugins, or SourcesOverride alter the header but must
		// not alter the hash.
		hashFor := func(prep func(ctx *plugin.BackendContext)) string {
			ctx, mem, d := newBackendContext(t)
			prep(ctx)
			target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
			addEmitPackage(t, ctx, emitPackage("x", emitStructWithFields(
				"x", "X", target, fieldSpec{name: "F", builtin: "int"},
			)))
			body := assertRenderSucceeds(t, ctx, mem, d, target)
			return extractFooterHash(t, string(body))
		}
		bare := hashFor(func(*plugin.BackendContext) {})
		withCmd := hashFor(func(ctx *plugin.BackendContext) { ctx.Command = "different command" })
		withSrc := hashFor(func(ctx *plugin.BackendContext) { ctx.SourcesOverride = []string{"a.go"} })
		if bare != withCmd || bare != withSrc {
			t.Fatalf(
				"header-only changes must not perturb the body hash;\n"+
					"bare    = %s\nwithCmd = %s\nwithSrc = %s",
				bare, withCmd, withSrc,
			)
		}
	})
}

// extractFooterHash pulls the hex hash from the trailing footer
// line. Fails the test if the footer marker is absent. Uses the
// default-brand marker since callers in this file never override
// [plugin.BackendContext.Brand].
func extractFooterHash(t *testing.T, body string) string {
	t.Helper()
	const marker = "eidos:provenance "
	idx := strings.LastIndex(body, marker)
	if idx < 0 {
		t.Fatalf("footer marker %q absent; got:\n%s", marker, body)
	}
	rest := body[idx+len(marker):]
	if newline := strings.IndexByte(rest, '\n'); newline >= 0 {
		rest = rest[:newline]
	}
	return strings.TrimSpace(rest)
}

// extractBodyBytes slices out the body region of a finished file —
// everything between the trailing blank line after the header and
// the leading newline of the footer marker. This mirrors the
// production composeFile logic and is the slice the hash covers.
func extractBodyBytes(t *testing.T, file []byte) []byte {
	t.Helper()
	const marker = "\n// eidos: end of generated content."
	end := strings.LastIndex(string(file), marker)
	if end < 0 {
		t.Fatalf("footer absent; got:\n%s", file)
	}
	// Header ends with "DO NOT EDIT.\n" followed by 0+ info lines,
	// then a blank line. The body begins immediately after the
	// blank line. Find the first "\n\n" — that's the
	// header/body boundary.
	headerEnd := bytes.Index(file, []byte("\n\n"))
	if headerEnd < 0 {
		t.Fatalf("header/body boundary absent; got:\n%s", file)
	}
	bodyStart := headerEnd + 2 // skip the "\n\n"
	if bodyStart >= end {
		t.Fatalf("body region empty between header and footer; got:\n%s", file)
	}
	return file[bodyStart:end]
}
