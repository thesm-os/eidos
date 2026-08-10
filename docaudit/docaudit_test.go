// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package docaudit_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/docaudit"
)

// TestAssertEveryMetaKeyDocumented_AllPresent covers the happy
// path: every meta-key literal under the package directory
// appears in doc.go.
func TestAssertEveryMetaKeyDocumented_AllPresent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "doc.go"), `// Package foo documents foo.bar and foo.baz.
package foo
`)
	writeFile(t, filepath.Join(dir, "meta.go"), `package foo

import "go.thesmos.sh/eidos/core/meta"

var Bar = meta.NewKey("foo.bar", meta.StringParser)
var Baz = meta.EnsureKey("foo.baz", meta.StringParser)
`)
	docaudit.AssertEveryMetaKeyDocumented(t, dir)
}

// TestAssertEveryMetaKeyDocumented_MissingKeyFails pins the
// failure mode: a meta-key literal not mentioned in doc.go
// surfaces as a t.Errorf entry naming the missing key.
func TestAssertEveryMetaKeyDocumented_MissingKeyFails(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "doc.go"), `// Package foo documents foo.bar only.
package foo
`)
	writeFile(t, filepath.Join(dir, "meta.go"), `package foo

import "go.thesmos.sh/eidos/core/meta"

var Bar = meta.NewKey("foo.bar", meta.StringParser)
var Missing = meta.NewKey("foo.missing", meta.StringParser)
`)
	fake := newFake()
	docaudit.AssertEveryMetaKeyDocumented(fake, dir)
	if !fake.failed {
		t.Fatalf("expected failure for undocumented key")
	}
	joined := strings.Join(fake.errs, "\n")
	if !strings.Contains(joined, "foo.missing") {
		t.Fatalf("error should name the missing key; got %q", joined)
	}
}

// TestAssertEveryMetaKeyDocumented_SkipsDynamicNames covers the
// dynamic-name skip rule: a meta.EnsureKey call whose first
// argument is not a literal string is omitted from the audit,
// since the key resolves through a runtime-composed name the
// caller documents under a namespace prefix.
func TestAssertEveryMetaKeyDocumented_SkipsDynamicNames(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "doc.go"), `// Package foo documents foo.literal under namespace foo.prefix.
package foo
`)
	writeFile(t, filepath.Join(dir, "meta.go"), `package foo

import "go.thesmos.sh/eidos/core/meta"

const prefix = "foo.prefix."

var Literal = meta.NewKey("foo.literal", meta.StringParser)

func DynamicKey(name string) meta.Key[string] {
	return meta.EnsureKey(prefix+name, meta.StringParser)
}
`)
	docaudit.AssertEveryMetaKeyDocumented(t, dir)
}

// writeFile is a tiny helper that writes path with body bytes
// and fails the test on error.
func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// fakeT is a minimal stand-in for [testing.T] that captures
// Errorf calls without affecting the host test's pass/fail. The
// audit-failure subtest uses it to drive the helper and inspect
// what it would have reported.
type fakeT struct {
	failed bool
	errs   []string
}

// newFake returns a fresh fakeT.
func newFake() *fakeT { return &fakeT{} }

// Helper satisfies the helper-marker on [testing.TB].
func (*fakeT) Helper() {}

// Errorf records a non-fatal failure entry.
func (f *fakeT) Errorf(format string, args ...any) {
	f.failed = true
	f.errs = append(f.errs, fmt.Sprintf(format, args...))
}

// Fatalf records a fatal failure. The audit helper's positive-
// failure subtest never reaches Fatalf in the missing-key path
// (every missing key is a non-fatal Errorf); a Fatalf here is a
// wiring error worth surfacing.
func (f *fakeT) Fatalf(format string, args ...any) {
	f.failed = true
	f.errs = append(f.errs, "FATAL: "+fmt.Sprintf(format, args...))
}

// pkgWith writes a single-file Go package into a fresh temp dir
// and returns the dir. The body is only ever parsed, never
// compiled, so a fixture may use shapes that would not typecheck
// — which is what lets the generic-instantiation cases below be
// expressed without declaring the generic functions.
func pkgWith(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "src.go"), body)
	return dir
}

// TestMetaKeys covers the exported collection entry point. It is
// the same walk the documentation audit runs, surfaced for callers
// that pin a cache-invalidating version constant to the stamping
// surface — so the set it returns has to be exactly the literal
// keys, sorted and de-duplicated, with every non-literal form
// skipped rather than guessed at.
func TestMetaKeys(t *testing.T) {
	t.Parallel()

	t.Run("returns literal keys sorted", func(t *testing.T) {
		t.Parallel()
		dir := pkgWith(t, "package p\n"+
			`var _ = meta.NewKey("z.last", nil)`+"\n"+
			`var _ = meta.NewKey("a.first", nil)`+"\n")
		got, err := docaudit.MetaKeys(dir)
		if err != nil {
			t.Fatalf("MetaKeys: %v", err)
		}
		if len(got) != 2 || got[0] != "a.first" || got[1] != "z.last" {
			t.Fatalf("MetaKeys = %v, want [a.first z.last]", got)
		}
	})

	t.Run("de-duplicates a key declared twice", func(t *testing.T) {
		t.Parallel()
		dir := pkgWith(t, "package p\n"+
			`var _ = meta.NewKey("go.same", nil)`+"\n"+
			`var _ = meta.EnsureKey("go.same", nil)`+"\n")
		got, err := docaudit.MetaKeys(dir)
		if err != nil {
			t.Fatalf("MetaKeys: %v", err)
		}
		if len(got) != 1 || got[0] != "go.same" {
			t.Fatalf("MetaKeys = %v, want [go.same]", got)
		}
	})

	t.Run("collects EnsureKey alongside NewKey", func(t *testing.T) {
		t.Parallel()
		dir := pkgWith(t, "package p\n"+`var _ = meta.EnsureKey("go.ensured", nil)`+"\n")
		got, err := docaudit.MetaKeys(dir)
		if err != nil || len(got) != 1 || got[0] != "go.ensured" {
			t.Fatalf("MetaKeys = %v (err %v), want [go.ensured]", got, err)
		}
	})

	t.Run("collects a key declared through the sdk façade", func(t *testing.T) {
		t.Parallel()
		// A plugin names sdk.NewKey / sdk.EnsureKey, which are thin
		// generic wrappers returning the identical key. Matching only
		// the `meta` qualifier exempted every façade-spelled key from
		// the audit while the gate stayed green.
		dir := pkgWith(t, "package p\n"+
			`var _ = sdk.NewKey("sdk.new", nil)`+"\n"+
			`var _ = sdk.EnsureKey("sdk.ensured", nil)`+"\n")
		got, err := docaudit.MetaKeys(dir)
		if err != nil || len(got) != 2 || got[0] != "sdk.ensured" || got[1] != "sdk.new" {
			t.Fatalf("MetaKeys = %v (err %v), want [sdk.ensured sdk.new]", got, err)
		}
	})

	t.Run("collects a key declared with one explicit type argument", func(t *testing.T) {
		t.Parallel()
		// meta.NewKey[bool]("k") parses as an IndexExpr wrapping the
		// selector. Matching only the bare selector exempted this
		// form from the audit in silence — the exact failure the
		// package exists to prevent.
		dir := pkgWith(t, "package p\n"+`var _ = meta.NewKey[bool]("go.explicit", nil)`+"\n")
		got, err := docaudit.MetaKeys(dir)
		if err != nil || len(got) != 1 || got[0] != "go.explicit" {
			t.Fatalf("MetaKeys = %v (err %v), want [go.explicit]", got, err)
		}
	})

	t.Run("collects a key declared with two explicit type arguments", func(t *testing.T) {
		t.Parallel()
		// Two or more type arguments parse as an IndexListExpr.
		dir := pkgWith(t, "package p\n"+`var _ = meta.NewKey[bool, int]("go.listexplicit", nil)`+"\n")
		got, err := docaudit.MetaKeys(dir)
		if err != nil || len(got) != 1 || got[0] != "go.listexplicit" {
			t.Fatalf("MetaKeys = %v (err %v), want [go.listexplicit]", got, err)
		}
	})

	t.Run("skips a call on a package other than meta", func(t *testing.T) {
		t.Parallel()
		dir := pkgWith(t, "package p\n"+
			`var _ = other.NewKey("not.meta", nil)`+"\n"+
			`var _ = meta.NewKey("is.meta", nil)`+"\n")
		got, _ := docaudit.MetaKeys(dir)
		if len(got) != 1 || got[0] != "is.meta" {
			t.Fatalf("MetaKeys = %v, want only [is.meta]", got)
		}
	})

	t.Run("skips a meta call that is not a key constructor", func(t *testing.T) {
		t.Parallel()
		dir := pkgWith(t, "package p\n"+`var _ = meta.Something("not.a.key", nil)`+"\n"+
			`var _ = meta.NewKey("real.key", nil)`+"\n")
		got, _ := docaudit.MetaKeys(dir)
		if len(got) != 1 || got[0] != "real.key" {
			t.Fatalf("MetaKeys = %v, want only [real.key]", got)
		}
	})

	t.Run("skips a qualified callee whose receiver is not a bare identifier", func(t *testing.T) {
		t.Parallel()
		dir := pkgWith(t, "package p\n"+`var _ = pkg.meta.NewKey("nested.recv", nil)`+"\n"+
			`var _ = meta.NewKey("real.key", nil)`+"\n")
		got, _ := docaudit.MetaKeys(dir)
		if len(got) != 1 || got[0] != "real.key" {
			t.Fatalf("MetaKeys = %v, want only [real.key]", got)
		}
	})

	t.Run("skips an unqualified call", func(t *testing.T) {
		t.Parallel()
		dir := pkgWith(t, "package p\n"+`var _ = NewKey("bare.call", nil)`+"\n"+
			`var _ = meta.NewKey("real.key", nil)`+"\n")
		got, _ := docaudit.MetaKeys(dir)
		if len(got) != 1 || got[0] != "real.key" {
			t.Fatalf("MetaKeys = %v, want only [real.key]", got)
		}
	})

	t.Run("skips a constructor call carrying no arguments", func(t *testing.T) {
		t.Parallel()
		dir := pkgWith(t, "package p\n"+`var _ = meta.NewKey()`+"\n"+
			`var _ = meta.NewKey("real.key", nil)`+"\n")
		got, _ := docaudit.MetaKeys(dir)
		if len(got) != 1 || got[0] != "real.key" {
			t.Fatalf("MetaKeys = %v, want only [real.key]", got)
		}
	})

	t.Run("skips a raw-string key literal", func(t *testing.T) {
		t.Parallel()
		// The grammar is deliberately narrow: only double-quoted
		// literals count, so a backtick form is skipped rather than
		// silently mis-unquoted.
		dir := pkgWith(t, "package p\n"+"var _ = meta.NewKey(`raw.key`, nil)\n"+
			`var _ = meta.NewKey("real.key", nil)`+"\n")
		got, _ := docaudit.MetaKeys(dir)
		if len(got) != 1 || got[0] != "real.key" {
			t.Fatalf("MetaKeys = %v, want only [real.key]", got)
		}
	})

	t.Run("ignores _test.go sources", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "src.go"), "package p\n"+`var _ = meta.NewKey("in.src", nil)`+"\n")
		writeFile(t, filepath.Join(dir, "src_test.go"), "package p\n"+`var _ = meta.NewKey("in.test", nil)`+"\n")
		got, _ := docaudit.MetaKeys(dir)
		if len(got) != 1 || got[0] != "in.src" {
			t.Fatalf("MetaKeys = %v, want only [in.src]", got)
		}
	})

	t.Run("ignores a nested directory", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "src.go"), "package p\n"+`var _ = meta.NewKey("top.key", nil)`+"\n")
		if err := os.Mkdir(filepath.Join(dir, "sub"), 0o750); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		got, _ := docaudit.MetaKeys(dir)
		if len(got) != 1 || got[0] != "top.key" {
			t.Fatalf("MetaKeys = %v, want only [top.key]", got)
		}
	})

	t.Run("returns ErrEmptyDirectory when no Go source parses", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "README.md"), "not go\n")
		if _, err := docaudit.MetaKeys(dir); !errors.Is(err, docaudit.ErrEmptyDirectory) {
			t.Fatalf("MetaKeys = %v, want ErrEmptyDirectory", err)
		}
	})

	t.Run("surfaces a parse error", func(t *testing.T) {
		t.Parallel()
		dir := pkgWith(t, "package p\nthis is not go\n")
		if _, err := docaudit.MetaKeys(dir); err == nil {
			t.Fatalf("MetaKeys on unparseable source = nil error, want a parse error")
		}
	})

	t.Run("surfaces a directory-read error", func(t *testing.T) {
		t.Parallel()
		missing := filepath.Join(t.TempDir(), "absent")
		if _, err := docaudit.MetaKeys(missing); err == nil {
			t.Fatalf("MetaKeys on a missing directory = nil error, want a read error")
		}
	})
}

// TestAssertEveryMetaKeyDocumented_KeyBoundary covers the
// boundary rule that keeps a mention from passing coincidentally.
// A gate that passes by accident is worse than an absent one, so
// each case here is a way the plain substring test used to pass.
func TestAssertEveryMetaKeyDocumented_KeyBoundary(t *testing.T) {
	t.Parallel()

	// auditWith writes a package declaring key plus a doc.go
	// containing doc, and reports whether the audit passed.
	auditWith := func(t *testing.T, key, doc string) bool {
		t.Helper()
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "src.go"),
			"package p\n"+fmt.Sprintf("var _ = meta.NewKey(%q, nil)\n", key))
		writeFile(t, filepath.Join(dir, "doc.go"), "// "+doc+"\npackage p\n")
		f := newFake()
		docaudit.AssertEveryMetaKeyDocumented(f, dir)
		return !f.failed
	}

	t.Run("a key terminated by a full stop is a mention", func(t *testing.T) {
		t.Parallel()
		if !auditWith(t, "go.chanDir", "the go.chanDir key.") {
			t.Fatalf("a sentence-final dot must not read as key continuation")
		}
	})

	t.Run("a key that is a prefix of a longer key is not a mention", func(t *testing.T) {
		t.Parallel()
		if auditWith(t, "go.isIterSeq", "documents go.isIterSeq2 only") {
			t.Fatalf("a prefix must not ride on its sibling")
		}
	})

	t.Run("a key followed by a dotted segment is not a mention", func(t *testing.T) {
		t.Parallel()
		if auditWith(t, "shape.key", "documents shape.key_type only") {
			t.Fatalf("an underscore must continue the key name")
		}
	})

	t.Run("a key followed by a further dotted segment is not a mention", func(t *testing.T) {
		t.Parallel()
		if auditWith(t, "shape.key", "documents shape.key.more only") {
			t.Fatalf("a dot followed by an identifier must continue the key name")
		}
	})

	t.Run("a key followed by an upper-case letter is not a mention", func(t *testing.T) {
		t.Parallel()
		if auditWith(t, "go.iter", "documents go.iterValue only") {
			t.Fatalf("an upper-case letter must continue the key name")
		}
	})

	t.Run("a key followed by a digit is not a mention", func(t *testing.T) {
		t.Parallel()
		if auditWith(t, "go.seq", "documents go.seq2 only") {
			t.Fatalf("a digit must continue the key name")
		}
	})

	t.Run("a later unambiguous mention rescues an earlier ambiguous one", func(t *testing.T) {
		t.Parallel()
		if !auditWith(t, "go.seq", "go.seq2 is derived from go.seq itself") {
			t.Fatalf("the scan must continue past a continued occurrence")
		}
	})

	t.Run("a key ending the document is a mention", func(t *testing.T) {
		t.Parallel()
		if !auditWith(t, "go.tail", "the final key is go.tail") {
			t.Fatalf("end-of-document must terminate a key")
		}
	})

	t.Run("a key whose trailing dot ends the file is a mention", func(t *testing.T) {
		t.Parallel()
		// The dot-lookahead has to cope with the dot being the last
		// byte in the document: there is no following character to
		// classify, so the key ends there. Written without a final
		// newline deliberately — doc.go is also parsed for keys, so
		// it stays valid Go, but nothing may follow the comment.
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "src.go"),
			"package p\n"+`var _ = meta.NewKey("go.tail", nil)`+"\n")
		writeFile(t, filepath.Join(dir, "doc.go"), "package p\n\n// the final key is go.tail.")
		f := newFake()
		docaudit.AssertEveryMetaKeyDocumented(f, dir)
		if f.failed {
			t.Fatalf("a dot at end-of-document must terminate the key, not continue it")
		}
	})

	t.Run("a key ending the file with nothing after it is a mention", func(t *testing.T) {
		t.Parallel()
		// The key's last character is the document's last byte, so
		// there is no following character to classify at all. Distinct
		// from the trailing-dot case above, which still has a byte to
		// look at.
		dir := t.TempDir()
		writeFile(t, filepath.Join(dir, "src.go"),
			"package p\n"+`var _ = meta.NewKey("go.eof", nil)`+"\n")
		writeFile(t, filepath.Join(dir, "doc.go"), "package p\n\n// the final key is go.eof")
		f := newFake()
		docaudit.AssertEveryMetaKeyDocumented(f, dir)
		if f.failed {
			t.Fatalf("end-of-document must terminate the key")
		}
	})

	t.Run("a key mentioned nowhere fails the audit", func(t *testing.T) {
		t.Parallel()
		if auditWith(t, "go.absent", "documents nothing relevant") {
			t.Fatalf("an undocumented key must fail")
		}
	})
}

// TestAssertEveryMetaKeyDocumented_WiringErrors covers the three
// ways the audit refuses to render a verdict at all. Each is a
// Fatalf rather than an Errorf because a mis-wired audit that
// reports "pass" is the outcome this package exists to prevent.
func TestAssertEveryMetaKeyDocumented_WiringErrors(t *testing.T) {
	t.Parallel()

	t.Run("an unreadable package directory is fatal", func(t *testing.T) {
		t.Parallel()
		f := newFake()
		docaudit.AssertEveryMetaKeyDocumented(f, filepath.Join(t.TempDir(), "absent"))
		if !f.failed {
			t.Fatalf("a missing package directory must fail the audit")
		}
	})

	t.Run("a package declaring no keys is fatal", func(t *testing.T) {
		t.Parallel()
		dir := pkgWith(t, "package p\n\nvar x = 1\n")
		f := newFake()
		docaudit.AssertEveryMetaKeyDocumented(f, dir)
		if !f.failed {
			t.Fatalf("discovering no keys means the audit is mis-wired and must fail")
		}
	})

	t.Run("a missing doc.go is fatal", func(t *testing.T) {
		t.Parallel()
		dir := pkgWith(t, "package p\n"+`var _ = meta.NewKey("go.key", nil)`+"\n")
		f := newFake()
		docaudit.AssertEveryMetaKeyDocumented(f, dir)
		if !f.failed {
			t.Fatalf("an absent doc.go must fail the audit")
		}
	})
}
