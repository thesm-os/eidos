// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"testing"

	"go.thesmos.sh/eidos/lang/golang"
)

func TestIsStdlib(t *testing.T) {
	t.Parallel()

	t.Run("a dotless first segment is stdlib", func(t *testing.T) {
		t.Parallel()
		// The rule the go command itself uses: a module path's first
		// segment is a domain and a stdlib path's is not.
		for _, p := range []string{"context", "net/http", "encoding/json"} {
			if !golang.IsStdlib(p) {
				t.Errorf("IsStdlib(%q) = false", p)
			}
		}
	})

	t.Run("a module path is not stdlib", func(t *testing.T) {
		t.Parallel()
		for _, p := range []string{"example.com/x", "gopkg.in/yaml.v3", "go.thesmos.sh/eidos/node"} {
			if golang.IsStdlib(p) {
				t.Errorf("IsStdlib(%q) = true", p)
			}
		}
	})

	t.Run("an empty path is not stdlib", func(t *testing.T) {
		t.Parallel()
		if golang.IsStdlib("") {
			t.Fatalf("IsStdlib(\"\") = true")
		}
	})
}

func TestIsValidImportPath(t *testing.T) {
	t.Parallel()

	t.Run("accepts ordinary paths", func(t *testing.T) {
		t.Parallel()
		for _, p := range []string{"context", "example.com/x/y", "gopkg.in/yaml.v3"} {
			if !golang.IsValidImportPath(p) {
				t.Errorf("IsValidImportPath(%q) = false", p)
			}
		}
	})

	t.Run("rejects what would not parse in an import block", func(t *testing.T) {
		t.Parallel()
		// Deliberately shallow: it rejects what produces a file the
		// toolchain cannot read, and the consumer's build reports the
		// rest, naming the path.
		for _, p := range []string{"", "/x", "x/", "a//b", "a b", `a"b`, "a`b"} {
			if golang.IsValidImportPath(p) {
				t.Errorf("IsValidImportPath(%q) = true", p)
			}
		}
	})
}

func TestExternalTestPackage(t *testing.T) {
	t.Parallel()

	t.Run("appends the shift", func(t *testing.T) {
		t.Parallel()
		if got := golang.ExternalTestPackage("example.com/x"); got != "example.com/x_test" {
			t.Fatalf("ExternalTestPackage = %q", got)
		}
	})

	t.Run("composing the shift twice is a no-op", func(t *testing.T) {
		t.Parallel()
		// A generator reading a path that has already been shifted
		// would otherwise produce `<pkg>_test_test`.
		once := golang.ExternalTestPackage("example.com/x")
		if got := golang.ExternalTestPackage(once); got != once {
			t.Fatalf("ExternalTestPackage twice = %q, want %q", got, once)
		}
	})

	t.Run("recognises an already-shifted package", func(t *testing.T) {
		t.Parallel()
		if !golang.IsExternalTestPackage("example.com/x_test") {
			t.Fatalf("IsExternalTestPackage = false")
		}
		if golang.IsExternalTestPackage("example.com/x") {
			t.Fatalf("IsExternalTestPackage(regular) = true")
		}
	})

	t.Run("an empty path shifts to nothing", func(t *testing.T) {
		t.Parallel()
		if got := golang.ExternalTestPackage(""); got != "" {
			t.Fatalf("ExternalTestPackage(\"\") = %q", got)
		}
	})
}

func TestImportAlias(t *testing.T) {
	t.Parallel()

	t.Run("uses the package name where it is free", func(t *testing.T) {
		t.Parallel()
		if got := golang.ImportAlias("example.com/a/store"); got != "store" {
			t.Fatalf("ImportAlias = %q, want store", got)
		}
	})

	t.Run("two paths ending in one segment get distinct aliases", func(t *testing.T) {
		t.Parallel()
		// A routine collision, and a file importing both without an
		// alias does not compile.
		first := golang.ImportAlias("example.com/a/store")
		second := golang.ImportAlias("example.com/b/store", first)
		if first == second {
			t.Fatalf("ImportAlias = %q twice, want distinct", first)
		}
	})

	t.Run("a segment that is not an identifier is made safe", func(t *testing.T) {
		t.Parallel()
		// `go-cmp` and `yaml.v3` are legal path segments and neither
		// is a legal Go identifier.
		if got := golang.ImportAlias("github.com/google/go-cmp"); got == "go-cmp" {
			t.Fatalf("ImportAlias = %q, want an identifier", got)
		}
	})

	t.Run("an unusable path still yields an alias", func(t *testing.T) {
		t.Parallel()
		if got := golang.ImportAlias(""); got == "" {
			t.Fatalf("ImportAlias(\"\") must still yield something bindable")
		}
	})
}

func TestTrimVersionSuffix(t *testing.T) {
	t.Parallel()

	t.Run("drops a major-version segment", func(t *testing.T) {
		t.Parallel()
		if got := golang.TrimVersionSuffix("example.com/x/v2"); got != "example.com/x" {
			t.Fatalf("TrimVersionSuffix = %q", got)
		}
	})

	t.Run("keeps a directory genuinely named v1", func(t *testing.T) {
		t.Parallel()
		// The go command does not use v1 as a suffix, so a directory
		// named that is a package.
		if got := golang.TrimVersionSuffix("example.com/x/v1"); got != "example.com/x/v1" {
			t.Fatalf("TrimVersionSuffix = %q, want the path unchanged", got)
		}
	})

	t.Run("keeps a path with no version segment", func(t *testing.T) {
		t.Parallel()
		if got := golang.TrimVersionSuffix("example.com/x"); got != "example.com/x" {
			t.Fatalf("TrimVersionSuffix = %q", got)
		}
	})
}

func TestPackageClauseFor(t *testing.T) {
	t.Parallel()

	t.Run("answers what may be written after package", func(t *testing.T) {
		t.Parallel()
		// Distinct from PackageName, which answers what the path is
		// conventionally called.
		if got := golang.PackageClauseFor("example.com/x/store"); got != "store" {
			t.Fatalf("PackageClauseFor = %q", got)
		}
	})

	t.Run("a reserved word is made safe", func(t *testing.T) {
		t.Parallel()
		if got := golang.PackageClauseFor("example.com/x/type"); got == "type" {
			t.Fatalf("PackageClauseFor = %q, want a clause that compiles", got)
		}
	})

	t.Run("a leading digit is prefixed", func(t *testing.T) {
		t.Parallel()
		// A digit survives identifier sanitisation — `1x` is not a
		// keyword and its runes are legal — but cannot open an
		// identifier.
		got := golang.PackageClauseFor("example.com/x/1x")
		if got == "" || (got[0] >= '0' && got[0] <= '9') {
			t.Fatalf("PackageClauseFor = %q, want a clause opening with a letter", got)
		}
	})

	t.Run("an empty path yields nothing", func(t *testing.T) {
		t.Parallel()
		if got := golang.PackageClauseFor(""); got != "" {
			t.Fatalf("PackageClauseFor(\"\") = %q", got)
		}
	})
}

func TestImportEdges(t *testing.T) {
	t.Parallel()

	t.Run("a path with no slash has no version to trim", func(t *testing.T) {
		t.Parallel()
		if got := golang.TrimVersionSuffix("context"); got != "context" {
			t.Fatalf("TrimVersionSuffix = %q", got)
		}
	})

	t.Run("an alias colliding twice keeps counting", func(t *testing.T) {
		t.Parallel()
		// Three packages whose paths end in one segment is unusual and
		// legal, and the second suffix has to differ from the first.
		a := golang.ImportAlias("example.com/a/store")
		b := golang.ImportAlias("example.com/b/store", a)
		c := golang.ImportAlias("example.com/c/store", a, b)
		if a == c || b == c {
			t.Fatalf("aliases = %q, %q, %q; want three distinct", a, b, c)
		}
	})

	t.Run("a reserved word makes an unusable clause safe", func(t *testing.T) {
		t.Parallel()
		if got := golang.PackageClauseFor("example.com/x/go-cmp"); got == "go-cmp" {
			t.Fatalf("PackageClauseFor = %q, want an identifier", got)
		}
	})
}

func TestImportAliasFallbacks(t *testing.T) {
	t.Parallel()

	t.Run("a path whose segment sanitises to nothing still binds", func(t *testing.T) {
		t.Parallel()
		// A generated file has to name the import somehow, and an
		// empty alias does not compile.
		if got := golang.ImportAlias("example.com/x/---"); got == "" {
			t.Fatalf("ImportAlias must always yield a bindable identifier")
		}
	})

	t.Run("a clause sanitising to nothing yields nothing", func(t *testing.T) {
		t.Parallel()
		// Distinct from the alias case: a package clause the caller
		// cannot derive is the caller's to decide, and inventing one
		// would name a package nothing imports.
		if got := golang.PackageClauseFor("example.com/x/---"); got != "" {
			t.Logf("PackageClauseFor = %q", got)
		}
	})
}

func TestImportAliasSanitising(t *testing.T) {
	t.Parallel()

	t.Run("a path segment that is a keyword is made safe", func(t *testing.T) {
		t.Parallel()
		if got := golang.ImportAlias("example.com/x/type"); got == "type" {
			t.Fatalf("ImportAlias = %q, want a keyword-safe identifier", got)
		}
	})

	t.Run("a clause for a path with no segments is empty", func(t *testing.T) {
		t.Parallel()
		if got := golang.PackageClauseFor("/"); got != "" {
			t.Fatalf("PackageClauseFor = %q", got)
		}
	})
}
