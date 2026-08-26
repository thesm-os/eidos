// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk_test

import (
	"embed"
	"testing"

	"go.thesmos.sh/eidos/lang/typescript"
	tssdk "go.thesmos.sh/eidos/lang/typescript/sdk"
	"go.thesmos.sh/eidos/sdk"
)

// templates stands in for a plugin's embedded template tree.
//
//go:embed testdata/templates
var templates embed.FS

func TestSupport(t *testing.T) {
	t.Parallel()

	t.Run("declares the language and its read-side rules", func(t *testing.T) {
		t.Parallel()
		// The pair is what lets a plugin's language-neutral core spread
		// this straight into the builder and never name a language.
		lang, support := tssdk.Support(templates, sdk.Output{Suffix: ".types.ts"})

		if lang != typescript.Language {
			t.Fatalf("language = %q, want %q", lang, typescript.Language)
		}
		if support.Source == nil {
			t.Fatal("Support declares no read-side rules")
		}
		if support.Templates == nil {
			t.Fatal("Support carries no template tree")
		}
		if len(support.Outputs) != 1 || support.Outputs[0].Suffix != ".types.ts" {
			t.Fatalf("Outputs = %+v", support.Outputs)
		}
		if support.Builtin {
			t.Error("Support marked builtin, but it ships a tree")
		}
	})

	t.Run("Builtin declares an output rendered through the backend's own templates", func(t *testing.T) {
		t.Parallel()
		// The missing tree is deliberate rather than an omission,
		// which is what the flag says.
		lang, support := tssdk.Builtin(sdk.Output{Suffix: ".ts"})

		if lang != typescript.Language {
			t.Fatalf("language = %q", lang)
		}
		if !support.Builtin {
			t.Error("Builtin did not mark the support builtin")
		}
		if support.Templates != nil {
			t.Error("Builtin carries a template tree")
		}
		if support.Source == nil {
			t.Error("Builtin declares no read-side rules")
		}
	})

	t.Run("Reads declares the read side alone", func(t *testing.T) {
		t.Parallel()
		// A plugin that speaks TypeScript can read it whether or not
		// it also renders it; one left to declare the two separately
		// is one that can declare half.
		lang, support := tssdk.Reads()

		if lang != typescript.Language {
			t.Fatalf("language = %q", lang)
		}
		if support.Source == nil {
			t.Fatal("Reads declares no read-side rules")
		}
		if len(support.Outputs) != 0 || support.Templates != nil {
			t.Error("Reads declares something to emit")
		}
	})

	t.Run("every constructor shares one rules value", func(t *testing.T) {
		t.Parallel()
		// Held once as a package value rather than constructed per
		// call, so a plugin's declaration is comparable in a test.
		_, a := tssdk.Reads()
		_, b := tssdk.Builtin()
		_, c := tssdk.Support(templates)

		if a.Source != b.Source || b.Source != c.Source {
			t.Fatal("the constructors hand out different rules values")
		}
	})

	t.Run("the rules satisfy the SDK contract", func(t *testing.T) {
		t.Parallel()
		// The compile-time half is the assignment inside the package;
		// this is the runtime half, which catches an interface that
		// grew a method the language answers with a nil panic.
		_, support := tssdk.Reads()

		if got := support.Source.TypeName("user", "repo"); got != "UserRepo" {
			t.Errorf("TypeName through the façade = %q", got)
		}
		if got := support.Source.ConstructorName("User"); got != "createUser" {
			t.Errorf("ConstructorName through the façade = %q", got)
		}
		if got := support.Source.TypeArgs(nil); got != "" {
			t.Errorf("TypeArgs through the façade = %q", got)
		}
	})
}

// compile-time confirmation that the language's rules satisfy the
// contract, asserted from the one package that imports both.
//
// `lang/typescript` cannot state this itself: it sits below the SDK
// and importing it would invert the layering the package
// documentation describes.
var _ sdk.SourceRules = typescript.Source{}

func TestOptionalRulesAnswered(t *testing.T) {
	t.Parallel()

	// Asserted from the plugin's side rather than only at compile time
	// in the façade: a generator finds these by assertion, and one that
	// stopped being satisfied would generate its degraded form with no
	// build failure anywhere.
	_, s := tssdk.Reads()

	t.Run("the enumeration rules are answered", func(t *testing.T) {
		t.Parallel()
		if _, ok := s.Source.(sdk.EnumRules); !ok {
			t.Fatal("TypeScript answers no EnumRules")
		}
	})

	t.Run("the signature rules are answered", func(t *testing.T) {
		t.Parallel()
		if _, ok := s.Source.(sdk.SigRules); !ok {
			t.Fatal("TypeScript answers no SigRules")
		}
	})

	t.Run("the error rules are answered", func(t *testing.T) {
		t.Parallel()
		if _, ok := s.Source.(sdk.ErrorRules); !ok {
			t.Fatal("TypeScript answers no ErrorRules")
		}
	})
}
