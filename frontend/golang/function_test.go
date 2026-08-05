// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang_test

import (
	"slices"
	"testing"
)

// TestConvertFunction covers the top-level Function path: name,
// package, params, returns, generic params, variadic semantics.
func TestConvertFunction(t *testing.T) {
	t.Parallel()
	t.Run("top-level function lands in Functions slice", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nfunc Do() {}\n",
		})
		fn := pkg.FunctionByName("Do")
		if fn == nil {
			t.Fatalf("Do missing")
		}
	})

	t.Run("params and returns are recorded with names and types", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nimport \"context\"\n\nfunc Do(ctx context.Context, key string) (bool, error) { return false, nil }\n",
		})
		fn := pkg.FunctionByName("Do")
		if len(fn.Params) != 2 || len(fn.Returns) != 2 {
			t.Fatalf("expected 2 params + 2 returns, got %d/%d", len(fn.Params), len(fn.Returns))
		}
		names := []string{fn.Params[0].Name, fn.Params[1].Name}
		if !slices.Equal(names, []string{"ctx", "key"}) {
			t.Fatalf("param names = %v", names)
		}
	})

	t.Run("variadic param flagged and element-typed", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nfunc Concat(parts ...string) string { return \"\" }\n",
		})
		fn := pkg.FunctionByName("Concat")
		last := fn.Params[len(fn.Params)-1]
		if !last.Variadic || last.Type.Name != "string" {
			t.Fatalf("expected variadic string, got Variadic=%v Type=%+v", last.Variadic, last.Type)
		}
	})

	t.Run("generic function captures type parameters", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nfunc Map[T any](v []T) []T { return v }\n",
		})
		fn := pkg.FunctionByName("Map")
		if len(fn.TypeParams) != 1 || fn.TypeParams[0].Name != "T" {
			t.Fatalf("expected 1 type-param T, got %+v", fn.TypeParams)
		}
	})
}

// TestConvertMethod covers receivers (value + pointer), receiver
// names, generic params on methods, and the attachMethods bridge.
func TestConvertMethod(t *testing.T) {
	t.Parallel()
	t.Run("value receiver records the variable name", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{}\n\nfunc (s S) Do() {}\n",
		})
		m := pkg.StructByName("S").Methods[0]
		if m.ReceiverName != "s" {
			t.Fatalf("receiver name = %q, want s", m.ReceiverName)
		}
		if m.Receiver == nil || m.Receiver.IsPointer() {
			t.Fatalf("expected value receiver ref, got %+v", m.Receiver)
		}
	})

	t.Run("pointer receiver is normalised to its named type", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{}\n\nfunc (s *S) Touch() {}\n",
		})
		s := pkg.StructByName("S")
		if len(s.Methods) != 1 {
			t.Fatalf("expected 1 method, got %d", len(s.Methods))
		}
		m := s.Methods[0]
		if m.Receiver == nil || !m.Receiver.IsPointer() {
			t.Fatalf("expected pointer receiver, got %+v", m.Receiver)
		}
	})

	t.Run("directive on the line before a param attaches to that param", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nfunc Do(\n\t// +gen:nonzero\n\tid string,\n) {}\n",
		})
		fn := pkg.FunctionByName("Do")
		if fn == nil || len(fn.Params) != 1 {
			t.Fatalf("Do not loaded with 1 param: %+v", fn)
		}
		p := fn.Params[0]
		if len(p.DirectiveList) != 1 || p.DirectiveList[0].Name != "nonzero" {
			t.Fatalf("expected one +gen:nonzero directive on param, got %+v", p.DirectiveList)
		}
	})

	t.Run("directive on the same line after a param attaches to that param", func(t *testing.T) {
		t.Parallel()
		// Trailing comment on the param line: Go's CommentMap
		// associates it with the param field.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nfunc Do(\n\tid string, // +gen:nonzero\n) {}\n",
		})
		fn := pkg.FunctionByName("Do")
		if fn == nil || len(fn.Params) != 1 {
			t.Fatalf("Do not loaded with 1 param: %+v", fn)
		}
		p := fn.Params[0]
		if len(p.DirectiveList) != 1 || p.DirectiveList[0].Name != "nonzero" {
			t.Fatalf("expected one +gen:nonzero directive on param, got %+v", p.DirectiveList)
		}
	})

	t.Run("multi-name params share the directive list from a shared field", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nfunc Do(\n\t// +gen:nonzero\n\tx, y int,\n) {}\n",
		})
		fn := pkg.FunctionByName("Do")
		if fn == nil || len(fn.Params) != 2 {
			t.Fatalf("expected 2 params, got %+v", fn)
		}
		for _, p := range fn.Params {
			if len(p.DirectiveList) != 1 {
				t.Fatalf("expected each multi-name param to share the directive, got %+v", p.DirectiveList)
			}
		}
	})

	t.Run("anonymous receiver leaves ReceiverName empty", func(t *testing.T) {
		t.Parallel()
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\ntype S struct{}\n\nfunc (*S) Touch() {}\n",
		})
		m := pkg.StructByName("S").Methods[0]
		if m.ReceiverName != "" {
			t.Fatalf("anonymous receiver must leave ReceiverName empty, got %q", m.ReceiverName)
		}
	})

	t.Run("function with an unresolvable parameter type does not crash the converter", func(t *testing.T) {
		t.Parallel()
		// Drives convertFuncDecl's broken-source defensive guards.
		_, d := loadFromSource(t, map[string]string{
			"a.go": "package a\n\nfunc F(x Missing) {}\n",
		})
		if !d.HasErrors() {
			t.Fatalf("expected an Error diagnostic for unresolved function param")
		}
	})

	t.Run("function with no return clause carries an empty Returns slice", func(t *testing.T) {
		t.Parallel()
		// `func F() {}` parses with FuncType.Results == nil;
		// overlayReturnTypePos's early-return branch fires when
		// returnsFromSignature happens to invoke it with an empty
		// signature result tuple.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nfunc F() {}\n",
		})
		fn := pkg.FunctionByName("F")
		if fn == nil {
			t.Fatalf("F missing")
		}
		if len(fn.Returns) != 0 {
			t.Fatalf("expected zero returns, got %d", len(fn.Returns))
		}
	})

	t.Run("function with anonymous (unnamed) parameters surfaces each as a Param", func(t *testing.T) {
		t.Parallel()
		// `func F(int, string) {}` declares two anonymous params;
		// overlayParamDocsAndTypePos's len(field.Names)==0 branch
		// runs once per field, expanding count to 1 so the AST and
		// signature run in lock-step over a single positional entry.
		pkg := requirePackage(t, map[string]string{
			"a.go": "package a\n\nfunc F(int, string) {}\n",
		})
		fn := pkg.FunctionByName("F")
		if fn == nil {
			t.Fatalf("F missing")
		}
		if len(fn.Params) != 2 {
			t.Fatalf("expected 2 anonymous params, got %d", len(fn.Params))
		}
		if fn.Params[0].Name != "" || fn.Params[1].Name != "" {
			t.Fatalf("anonymous params must have empty names, got %q / %q",
				fn.Params[0].Name, fn.Params[1].Name)
		}
		if fn.Params[0].Type == nil || fn.Params[0].Type.Name != "int" {
			t.Fatalf("expected first param type int, got %+v", fn.Params[0].Type)
		}
		if fn.Params[1].Type == nil || fn.Params[1].Type.Name != "string" {
			t.Fatalf("expected second param type string, got %+v", fn.Params[1].Type)
		}
	})
}

// TestConvertFuncDecl_RedeclaredNameDoesNotPanic pins that a func
// the type-checker rejects degrades to a diagnostic rather than
// taking the converter down. See
// [TestConvertConstSpec_RedeclaredNameDoesNotPanic] for the shared
// mechanism — go/types leaves a nil Defs entry on the losing side of
// a redeclaration, which an unconditional type assertion converts
// into a panic mid-walk.
func TestConvertFuncDecl_RedeclaredNameDoesNotPanic(t *testing.T) {
	t.Parallel()

	// The func sits in the later-sorting file so it is the losing
	// declaration.
	cases := map[string]map[string]string{
		"func losing to a func": {
			"a.go": "package p\n\nfunc Dup() {}\n",
			"b.go": "package p\n\nfunc Dup() {}\n",
		},
		"func losing to a type": {
			"a.go": "package p\n\ntype Dup struct{}\n",
			"b.go": "package p\n\nfunc Dup() {}\n",
		},
	}

	for name, src := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assertRedeclarationReported(t, src)
		})
	}
}
