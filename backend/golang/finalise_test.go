// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"bytes"
	"go/format"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/emit"
)

// TestFinalise_HappyPath covers the format + goimports chain on
// clean input: the body passes gofmt, the imports block is
// regrouped per Go convention, and no untracked-import warnings
// surface.
func TestFinalise_HappyPath(t *testing.T) {
	t.Parallel()

	t.Run("rendered struct is gofmt-stable", func(t *testing.T) {
		t.Parallel()
		body := renderSingleFieldStruct(t, "Name", emit.Builtin("string"))
		formatted, err := format.Source([]byte(body))
		if err != nil {
			t.Fatalf("format.Source rejected backend output: %v\noutput:\n%s", err, body)
		}
		if !bytes.Equal(formatted, []byte(body)) {
			t.Fatalf("backend output is not gofmt-stable\n--- got ---\n%s\n--- gofmt ---\n%s", body, formatted)
		}
	})

	t.Run("repeated runs of the same fixture produce byte-identical output", func(t *testing.T) {
		t.Parallel()
		first := renderSingleFieldStruct(t, "ID", emit.Builtin("int"))
		second := renderSingleFieldStruct(t, "ID", emit.Builtin("int"))
		if first != second {
			t.Fatalf("output is not byte-identical across runs\n--- first ---\n%s\n--- second ---\n%s", first, second)
		}
	})
}

// TestFinalise_FormatFailureIsAnError covers the contract for a
// body gofmt cannot parse: the bytes still reach the sink so the
// defect can be read, and the failure is an Error so the run does
// not exit successfully having written Go that does not compile.
//
// This was a Warn, which meant a template bug surfaced later at
// `go build` inside generated code with nothing pointing back at
// the generator that produced it.
func TestFinalise_FormatFailureIsAnError(t *testing.T) {
	t.Parallel()

	t.Run("invalid Go body errors and writes unformatted bytes", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		// Field names with whitespace produce invalid Go syntax —
		// format.Source rejects, and the backend's contract is to
		// report-and-emit-unformatted, not abort the render loop.
		addEmitPackage(t, ctx, emitPackage("x", &emit.Struct{
			Name: "X", Package: "x", Target: target,
			Fields: []*emit.Field{{Name: "Not A Valid Name", Type: emit.Builtin("int")}},
		}))
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render should not abort on a format failure: %v", err)
		}
		body, ok := mem.Get(target)
		if !ok {
			t.Fatalf("sink must still receive the unformatted body")
		}
		if !strings.Contains(string(body), "Not A Valid Name") {
			t.Fatalf("body should retain unformatted content; got:\n%s", body)
		}
		if !diagnosticsContain(d, diag.Error, "format.Source failed") {
			t.Fatalf("expected Error from format.Source failure; got %+v", d.Diagnostics())
		}
	})
}

// TestFinalise_GoimportsRegroupsImports covers the canonical
// regrouping behaviour: stdlib imports go before external ones with
// a blank-line separator per Go convention.
func TestFinalise_GoimportsRegroupsImports(t *testing.T) {
	t.Parallel()

	t.Run("stdlib and external imports are split by a blank line", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		// One stdlib + one external import; verify the blank-line
		// separator between the two groups in goimports output.
		addEmitPackage(t, ctx, emitPackage("x", &emit.Struct{
			Name: "X", Package: "x", Target: target,
			Fields: []*emit.Field{
				{Name: "Ctx", Type: emit.External("context", "Context")},
				{Name: "User", Type: emit.External("github.com/example/users", "User")},
			},
		}))
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if d.HasErrors() {
			t.Fatalf("unexpected error diagnostics: %+v", d.Diagnostics())
		}
		body, _ := mem.Get(target)
		// Find the import block; the regroup adds a blank line
		// between stdlib (context) and external (users).
		idxContext := bytes.Index(body, []byte("\"context\""))
		idxUsers := bytes.Index(body, []byte("\"github.com/example/users\""))
		if idxContext < 0 || idxUsers < 0 {
			t.Fatalf("both imports must appear; got:\n%s", body)
		}
		if idxContext > idxUsers {
			t.Fatalf("stdlib import should precede external; got:\n%s", body)
		}
		between := body[idxContext:idxUsers]
		if !bytes.Contains(between, []byte("\n\n")) {
			t.Fatalf("stdlib/external groups must be separated by a blank line; between:\n%s", between)
		}
	})
}

// TestFinalise_AliasCollision covers the deterministic suffix
// discipline `writer.ImportSet` provides. Two distinct external
// packages whose default-derived alias collides ("users") produce
// "users" and "users2".
func TestFinalise_AliasCollision(t *testing.T) {
	t.Parallel()

	t.Run("collision produces suffix-2 alias for the second path", func(t *testing.T) {
		t.Parallel()
		ctx, mem, d := newBackendContext(t)
		target := emit.Target{Dir: "x", Filename: "x.go", Package: "x"}
		addEmitPackage(t, ctx, emitPackage("x", &emit.Struct{
			Name: "X", Package: "x", Target: target,
			Fields: []*emit.Field{
				{Name: "A", Type: emit.External("github.com/example/users", "User")},
				{Name: "B", Type: emit.External("github.com/other/users", "User")},
			},
		}))
		if err := mustNew(t).Render(ctx); err != nil {
			t.Fatalf("Render: %v", err)
		}
		if d.HasErrors() {
			t.Fatalf("unexpected error diagnostics: %+v", d.Diagnostics())
		}
		body, _ := mem.Get(target)
		if !strings.Contains(string(body), "A users.User") {
			t.Fatalf("first collision should keep base alias; got:\n%s", body)
		}
		if !strings.Contains(string(body), "B users2.User") {
			t.Fatalf("second collision should suffix '2'; got:\n%s", body)
		}
		// The aliased import line should declare the explicit alias.
		if !strings.Contains(string(body), "users2 \"github.com/other/users\"") {
			t.Fatalf("body should declare 'users2' alias; got:\n%s", body)
		}
	})
}
