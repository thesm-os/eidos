// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/node"
)

// onlyImport returns the single import the source declares.
func onlyImport(t *testing.T, src string) *node.Import {
	t.Helper()
	d, ok := onlyDecl(t, src).(*node.Import)
	if !ok {
		t.Fatalf("expected *node.Import, got %T", onlyDecl(t, src))
	}
	return d
}

func TestImportDecl(t *testing.T) {
	t.Parallel()

	t.Run("records the specifier exactly as written", func(t *testing.T) {
		t.Parallel()
		// `./user` and `user` resolve differently, so a normalisation
		// that lost the leading `./` would turn a relative import into
		// a package one.
		for _, spec := range []string{"./user", "../lib/user", "@scope/pkg", "pkg"} {
			imp := onlyImport(t, `import { X } from '`+spec+`';`)
			if imp.Path != spec {
				t.Errorf("Path = %q, want %q", imp.Path, spec)
			}
			got, _ := typescript.MetaModuleSpecifier.Get(imp.Meta())
			if got != spec {
				t.Errorf("moduleSpecifier = %q, want %q", got, spec)
			}
		}
	})

	t.Run("a default import binds the module under its local name", func(t *testing.T) {
		t.Parallel()
		if got := onlyImport(t, `import D from './d';`).Alias; got != "D" {
			t.Fatalf("Alias = %q, want D", got)
		}
	})

	t.Run("a namespace import binds the module under its local name", func(t *testing.T) {
		t.Parallel()
		if got := onlyImport(t, `import * as NS from './ns';`).Alias; got != "NS" {
			t.Fatalf("Alias = %q, want NS", got)
		}
	})

	t.Run("a named import binds no single module name", func(t *testing.T) {
		t.Parallel()
		// It binds several names and none of them is the module's;
		// picking one would misreport the others.
		if got := onlyImport(t, `import { X, Y as Z } from './m';`).Alias; got != "" {
			t.Fatalf("Alias = %q, want empty", got)
		}
	})

	t.Run("a side-effect import records the dependency", func(t *testing.T) {
		t.Parallel()
		imp := onlyImport(t, `import './polyfill';`)
		if imp.Path != "./polyfill" {
			t.Fatalf("Path = %q", imp.Path)
		}
	})

	t.Run("a type-only import is marked", func(t *testing.T) {
		t.Parallel()
		plain := onlyImport(t, `import { T } from './t';`)
		typed := onlyImport(t, `import type { T } from './t';`)

		if typescript.MetaTypeOnly.Has(plain.Meta()) {
			t.Error("a value import was marked type-only")
		}
		if got, _ := typescript.MetaTypeOnly.Get(typed.Meta()); !got {
			t.Error("a type-only import was not marked")
		}
	})

	t.Run("the CommonJS require form records the dependency", func(t *testing.T) {
		t.Parallel()
		// `import x = require('y')` is how a TypeScript file consuming
		// a CommonJS module has to write it with esModuleInterop off.
		// Skipping it would leave such a file looking as though it
		// imports nothing.
		imp := onlyImport(t, `import x = require('y');`)

		if imp.Path != "y" {
			t.Fatalf("Path = %q, want y", imp.Path)
		}
		if imp.Alias != "x" {
			t.Fatalf("Alias = %q, want x", imp.Alias)
		}
		got, _ := typescript.MetaModuleSpecifier.Get(imp.Meta())
		if got != "y" {
			t.Fatalf("moduleSpecifier = %q", got)
		}
	})
}

func TestStringValue(t *testing.T) {
	t.Parallel()

	t.Run("strips every quote form TypeScript admits", func(t *testing.T) {
		t.Parallel()
		for raw, want := range map[string]string{
			`'./a'`: "./a",
			`"./b"`: "./b",
			"`./c`": "./c",
		} {
			if got := stringValue(raw); got != want {
				t.Errorf("stringValue(%s) = %q, want %q", raw, got, want)
			}
		}
	})

	t.Run("leaves unquoted text alone", func(t *testing.T) {
		t.Parallel()
		if got := stringValue("bare"); got != "bare" {
			t.Fatalf("stringValue(bare) = %q", got)
		}
	})

	t.Run("an unterminated quote yields nothing", func(t *testing.T) {
		t.Parallel()
		for _, raw := range []string{"", "'", `"`} {
			if got := stringValue(raw); got != "" {
				t.Errorf("stringValue(%q) = %q, want empty", raw, got)
			}
		}
	})

	t.Run("a one-character unquoted name survives", func(t *testing.T) {
		t.Parallel()
		// `namespace N` arrives here as a bare identifier. A length
		// guard applied before the quote check swallowed it, and the
		// namespace was recorded as having no name.
		if got := stringValue("N"); got != "N" {
			t.Fatalf("stringValue(N) = %q, want N", got)
		}
	})
}
