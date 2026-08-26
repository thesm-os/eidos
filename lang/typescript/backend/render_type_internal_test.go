// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"errors"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/lang/typescript"
)

func TestRenderTypeFailures(t *testing.T) {
	t.Parallel()

	t.Run("a nil ref renders no annotation", func(t *testing.T) {
		t.Parallel()
		// A declaration with no declared type is legal TypeScript —
		// the compiler infers it.
		s := exprState(t)
		if got, err := s.renderType(nil); got != "" || err != nil {
			t.Fatalf("renderType(nil) = %q, %v", got, err)
		}
	})

	t.Run("a type nested past the budget is refused", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		ref := emit.Ref(emit.Builtin("string"))
		for range maxRefDepth + 2 {
			ref = emit.SliceOf(ref)
		}
		if _, err := s.renderType(ref); !errors.Is(err, ErrUnsupportedRef) {
			t.Fatalf("renderType = %v, want ErrUnsupportedRef", err)
		}
	})

	t.Run("a composite with no element is refused", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		for _, ref := range []emit.Ref{
			&emit.CompositeRef{Shape: emit.ShapePointer},
			&emit.CompositeRef{Shape: emit.ShapeSlice},
			&emit.CompositeRef{Shape: emit.ShapeMap},
		} {
			if _, err := s.renderType(ref); !errors.Is(err, ErrUnsupportedRef) {
				t.Errorf("renderType(%v) = %v, want ErrUnsupportedRef", ref, err)
			}
		}
	})

	t.Run("an empty union is never", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		got, err := s.renderType(emit.Union())
		if err != nil {
			t.Fatalf("renderType: %v", err)
		}
		if got != typescript.TypeNever {
			t.Fatalf("renderType(empty union) = %q, want never", got)
		}
	})

	t.Run("a function type names its parameters", func(t *testing.T) {
		t.Parallel()
		// `(string) => void` declares a parameter named string with an
		// inferred type, which is a different signature.
		s := exprState(t)
		got, err := s.renderType(emit.FuncOf(
			[]emit.Ref{emit.Builtin("string")},
			[]emit.Ref{emit.Builtin("bool")},
		))
		if err != nil {
			t.Fatalf("renderType: %v", err)
		}
		if got != "(arg0: string) => boolean" {
			t.Fatalf("renderType = %q", got)
		}
	})

	t.Run("a function with several returns renders a tuple", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		got, _ := s.renderType(emit.FuncOf(nil, []emit.Ref{
			emit.Builtin("string"), emit.Builtin("error"),
		}))
		if got != "() => [string, Error]" {
			t.Fatalf("renderType = %q", got)
		}
	})

	t.Run("a function with no return is void", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		got, _ := s.renderType(emit.FuncOf(nil, nil))
		if got != "() => void" {
			t.Fatalf("renderType = %q", got)
		}
	})

	t.Run("an unknown builtin passes through", func(t *testing.T) {
		t.Parallel()
		// A generator that already said `string` gets `string`; one
		// naming a type this mapping has never heard of gets its own
		// name rather than a guess.
		s := exprState(t)
		got, _ := s.renderType(emit.Builtin("MyOwnType"))
		if got != "MyOwnType" {
			t.Fatalf("renderType = %q", got)
		}
	})

	t.Run("an unrenderable ref kind is refused", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		unknown := &emit.CompositeRef{Shape: emit.CompositeShape(99)}

		_, err := s.renderType(unknown)
		if !errors.Is(err, ErrUnsupportedRef) {
			t.Fatalf("renderType = %v, want ErrUnsupportedRef", err)
		}
	})
}

func TestExternalRefRendering(t *testing.T) {
	t.Parallel()

	t.Run("an external type registers a named import", func(t *testing.T) {
		t.Parallel()
		// Spelling a type is what registers its import, which is why
		// the import block is assembled after the body.
		s := exprState(t)
		got, err := s.renderType(emit.External("./models", "Person"))
		if err != nil {
			t.Fatalf("renderType: %v", err)
		}
		if got != "Person" {
			t.Fatalf("renderType = %q, want the bare local name", got)
		}

		stmts := s.imports.Imports()
		if len(stmts) != 1 || !strings.Contains(stmts[0], "./models") {
			t.Fatalf("imports = %v", stmts)
		}
	})

	t.Run("a generic external carries its arguments", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		ref := emit.External("./box", "Box")
		ref.TypeArgs = []emit.Ref{emit.Builtin("string")}

		got, err := s.renderType(ref)
		if err != nil {
			t.Fatalf("renderType: %v", err)
		}
		if got != "Box<string>" {
			t.Fatalf("renderType = %q", got)
		}
	})

	t.Run("an external naming no type is refused", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		if _, err := s.renderType(emit.External("./m", "")); !errors.Is(err, ErrUnsupportedRef) {
			t.Fatalf("renderType = %v, want ErrUnsupportedRef", err)
		}
	})

	t.Run("a type from the rendered module renders bare", func(t *testing.T) {
		t.Parallel()
		// A module cannot import itself; emitting the import would
		// make the file fail to load.
		s := exprState(t)
		got, err := s.renderType(emit.External("./self", "Local"))
		if err != nil {
			t.Fatalf("renderType: %v", err)
		}
		if got != "Local" {
			t.Fatalf("renderType = %q", got)
		}
		if s.imports.Len() != 0 {
			t.Fatal("a self-import was recorded")
		}
	})
}

func TestInternalRefRendering(t *testing.T) {
	t.Parallel()

	t.Run("a reference to another declaration in this run resolves", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		target := &emit.Interface{
			Name:   "User",
			Target: emit.Target{Dir: "out", Filename: "user.ts", ImportPath: "./out/user"},
		}

		got, err := s.renderType(&emit.TypeRef{Target: target})
		if err != nil {
			t.Fatalf("renderType: %v", err)
		}
		if got != "User" {
			t.Fatalf("renderType = %q", got)
		}
		if s.imports.Len() != 1 {
			t.Fatal("a cross-module reference registered no import")
		}
	})

	t.Run("a reference into the rendered module records no import", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		target := &emit.Struct{Name: "Local", Target: emit.Target{ImportPath: "./self"}}

		got, _ := s.renderType(&emit.TypeRef{Target: target})
		if got != "Local" {
			t.Fatalf("renderType = %q", got)
		}
		if s.imports.Len() != 0 {
			t.Fatal("a same-module reference registered an import")
		}
	})

	t.Run("a reference naming no target is refused", func(t *testing.T) {
		t.Parallel()
		s := exprState(t)
		if _, err := s.renderType(&emit.TypeRef{}); !errors.Is(err, ErrUnsupportedRef) {
			t.Fatalf("renderType = %v, want ErrUnsupportedRef", err)
		}
	})

	t.Run("every declaration kind reports its own module", func(t *testing.T) {
		t.Parallel()
		// An alias names its file in File rather than Target — its
		// Target field is the type it aliases — so the lookup is
		// per-kind rather than through one accessor.
		tgt := emit.Target{ImportPath: "./m"}
		for name, n := range map[string]emit.Node{
			"struct":    &emit.Struct{Name: "S", Target: tgt},
			"interface": &emit.Interface{Name: "I", Target: tgt},
			"enum":      &emit.Enum{Name: "E", Target: tgt},
			"alias":     &emit.Alias{Name: "A", File: tgt},
			"function":  &emit.Function{Name: "F", Target: tgt},
			"variable":  &emit.Variable{Name: "V", Target: tgt},
			"constant":  &emit.Constant{Name: "K", Target: tgt},
		} {
			if got := targetModule(n); got != "./m" {
				t.Errorf("%s: targetModule = %q, want ./m", name, got)
			}
		}
	})

	t.Run("an unknown kind reports no module", func(t *testing.T) {
		t.Parallel()
		// Which renders the reference bare — right for a target in the
		// same file, and visible as a missing import for one that is
		// not.
		if got := targetModule(&emit.Method{Name: "m"}); got != "" {
			t.Fatalf("targetModule = %q, want empty", got)
		}
	})
}

// badRef is a ref no arm of the type walk can spell.
func badRef() emit.Ref { return &emit.CompositeRef{Shape: emit.CompositeShape(99)} }

func TestRefFailurePropagates(t *testing.T) {
	t.Parallel()

	// A ref that cannot be spelled has to surface from wherever it
	// sits. Swallowed at any of these positions it renders as the
	// empty string, which produces `Box<>` or `Record<string, >` —
	// output that fails in the consumer's build rather than here.
	external := emit.External("./m", "Box")
	external.TypeArgs = []emit.Ref{badRef()}

	positions := map[string]emit.Ref{
		"a pointer's element": &emit.CompositeRef{Shape: emit.ShapePointer, Elem: badRef()},
		"a slice's element":   &emit.CompositeRef{Shape: emit.ShapeSlice, Elem: badRef()},
		"a map's key": &emit.CompositeRef{
			Shape: emit.ShapeMap, MapKey: badRef(), Elem: emit.Builtin("string"),
		},
		"a map's value": &emit.CompositeRef{
			Shape: emit.ShapeMap, MapKey: emit.Builtin("string"), Elem: badRef(),
		},
		"a function's parameter": emit.FuncOf([]emit.Ref{badRef()}, nil),
		"a function's return":    emit.FuncOf(nil, []emit.Ref{badRef()}),
		"one of several returns": emit.FuncOf(nil, []emit.Ref{
			emit.Builtin("string"), badRef(),
		}),
		"a union's member": emit.Union(
			emit.UnionTerm{Type: emit.Builtin("string")},
			emit.UnionTerm{Type: badRef()},
		),
		"an external's type arg": external,
		"an internal's type arg": &emit.TypeRef{
			Target:   &emit.Interface{Name: "Box", Target: emit.Target{ImportPath: "./m"}},
			TypeArgs: []emit.Ref{badRef()},
		},
	}

	for name, ref := range positions {
		t.Run(name+" surfaces", func(t *testing.T) {
			t.Parallel()
			s := exprState(t)
			got, err := s.renderType(ref)
			if !errors.Is(err, ErrUnsupportedRef) {
				t.Fatalf("renderType = %q, %v, want ErrUnsupportedRef", got, err)
			}
		})
	}
}
