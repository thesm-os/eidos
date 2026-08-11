// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"slices"
	"strings"
	"unicode"
)

// Import-path rules: what a path means, and what a generator may
// conclude from one.
//
// [PackageName] and [IsInternal] live in idents.go with the other
// identifier rules. What is here is the rest of what a generator
// asks of a path before emitting a reference through it.

// TestPackageSuffix is appended to a package's name and path to
// form its external test package.
//
// Exported because the shift is observable: a generated file ending
// `_test.go` lands in `<pkg>_test`, so every reference back into the
// regular package qualifies, and a generator that assumed otherwise
// emits bare identifiers that bind to nothing.
const TestPackageSuffix = "_test"

// IsStdlib reports whether an import path names a standard-library
// package.
//
// Decided by the absence of a dot in the first segment, which is
// the rule the go command itself uses: a module path's first
// segment is a domain and a stdlib path's is not. `context`,
// `net/http` and `encoding/json` are stdlib; `example.com/x` and
// `gopkg.in/yaml.v3` are not.
//
// Worth asking because a stdlib reference needs no module
// requirement — a generator emitting one into a consumer's module
// cannot make their build fail for a missing dependency, and one
// emitting a third-party reference can.
func IsStdlib(importPath string) bool {
	if importPath == "" {
		return false
	}
	first, _, _ := strings.Cut(importPath, "/")
	return !strings.Contains(first, ".")
}

// IsValidImportPath reports whether p is well-formed enough to
// render.
//
// Deliberately shallow: it rejects what would produce a file the Go
// toolchain cannot parse — an empty path, a leading or trailing
// slash, an empty segment, a space, a quote — and passes everything
// else. A full validation would reimplement the module resolver's
// rules against a path this package cannot resolve anyway, and the
// consumer's build reports what remains, naming the path.
func IsValidImportPath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") {
		return false
	}
	if slices.Contains(strings.Split(p, "/"), "") {
		return false
	}
	return !strings.ContainsFunc(p, func(r rune) bool {
		return unicode.IsSpace(r) || r == '"' || r == '`' || r == '\\' ||
			!unicode.IsPrint(r)
	})
}

// ExternalTestPackage returns the external test package a
// `_test.go` file declares for the given package.
//
// Both the name and the path take the suffix, and both matter: the
// name is the package clause, the path is the import identity that
// makes same-package elision stay inert so references back into the
// regular package qualify.
func ExternalTestPackage(pkg string) string {
	if pkg == "" || IsExternalTestPackage(pkg) {
		return pkg
	}
	return pkg + TestPackageSuffix
}

// IsExternalTestPackage reports whether pkg is already an external
// test package.
//
// Checked before appending, so composing the shift twice is a
// no-op: a generator reading a path that has already been shifted
// would otherwise produce `<pkg>_test_test`.
func IsExternalTestPackage(pkg string) bool {
	return strings.HasSuffix(pkg, TestPackageSuffix)
}

// ImportAlias returns the local identifier a file binds an import
// to, avoiding everything in taken.
//
// The path's package name where that is free, and a numbered
// variant otherwise. Two packages whose paths end in the same
// segment — `example.com/a/store` and `example.com/b/store` — are a
// routine collision, and a file importing both without an alias
// does not compile.
//
// The result is made safe as an identifier, because a path segment
// need not be one: `go-cmp` and `yaml.v3` are legal segments and
// neither is a legal Go identifier.
func ImportAlias(importPath string, taken ...string) string {
	// No empty-base guard: [SafeIdent] sanitises through
	// [naming.Identifier], which answers "_" for empty input, so a path
	// with no usable segment aliases to an underscore rather than to
	// nothing.
	return UniqueIdent(SafeIdent(PackageName(importPath)), taken...)
}

// TrimVersionSuffix removes a module path's major-version suffix.
//
// `example.com/x/v2` is the module `example.com/x` at v2, and its
// package clause is `x` rather than `v2`. [PackageName] applies
// this already; it is exported for a caller comparing two paths
// that differ only by major version.
func TrimVersionSuffix(importPath string) string {
	i := strings.LastIndex(importPath, "/")
	if i < 0 {
		return importPath
	}
	if !isMajorVersion(importPath[i+1:]) {
		return importPath
	}
	return importPath[:i]
}

// PackageClauseFor returns the package clause a file at the given
// import path declares, made safe as an identifier.
//
// Distinct from [PackageName], which answers what the path is
// conventionally called. This answers what may actually be written
// after `package`, so a path whose last segment is `go-cmp` or a
// reserved word yields something that compiles.
func PackageClauseFor(importPath string) string {
	name := PackageName(importPath)
	if name == "" {
		return ""
	}
	// No leading-digit guard: [naming.Identifier] prefixes one with an
	// underscore, so `2fa` arrives as `_2fa` and there is nothing left
	// to adjust.
	return SafeIdent(name)
}
