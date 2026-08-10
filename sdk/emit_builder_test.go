// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk_test

import (
	"errors"
	"testing"

	"go.thesmos.sh/eidos/emit"
	emitbuilder "go.thesmos.sh/eidos/emit/builder"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/sdk"
)

// TestBuilderAliasesPreserveIdentity pins every re-exported fluent
// builder to a Go type alias.
//
// Identity is load-bearing here in a way it is not for a leaf
// type: the builders are only ever reached as callback parameters
// handed out by the framework's own [emitbuilder.Context]. A
// wrapper would make `func(*sdk.StructBuilder)` a type the
// framework cannot call, so the failure would land in every plugin
// rather than in this package.
//
//nolint:staticcheck // intentional redundant typing — the redundancy is the test
func TestBuilderAliasesPreserveIdentity(t *testing.T) {
	t.Parallel()

	t.Run("package and file builders alias through", func(t *testing.T) {
		t.Parallel()
		var p1 *sdk.PackageBuilder
		var p2 *emitbuilder.PackageBuilder = p1
		_ = p2
		var f1 *sdk.FileBuilder
		var f2 *emitbuilder.FileBuilder = f1
		_ = f2
	})

	t.Run("declaration builders alias through", func(t *testing.T) {
		t.Parallel()
		var s1 *sdk.StructBuilder
		var s2 *emitbuilder.StructBuilder = s1
		_ = s2
		var i1 *sdk.InterfaceBuilder
		var i2 *emitbuilder.InterfaceBuilder = i1
		_ = i2
		var fn1 *sdk.FunctionBuilder
		var fn2 *emitbuilder.FunctionBuilder = fn1
		_ = fn2
		var e1 *sdk.EnumBuilder
		var e2 *emitbuilder.EnumBuilder = e1
		_ = e2
		var a1 *sdk.AliasBuilder
		var a2 *emitbuilder.AliasBuilder = a1
		_ = a2
		var c1 *sdk.ConstantBuilder
		var c2 *emitbuilder.ConstantBuilder = c1
		_ = c2
		var v1 *sdk.VariableBuilder
		var v2 *emitbuilder.VariableBuilder = v1
		_ = v2
		var im1 *sdk.ImportBuilder
		var im2 *emitbuilder.ImportBuilder = im1
		_ = im2
	})

	t.Run("member builders alias through", func(t *testing.T) {
		t.Parallel()
		var m1 *sdk.MethodBuilder
		var m2 *emitbuilder.MethodBuilder = m1
		_ = m2
		var f1 *sdk.FieldBuilder
		var f2 *emitbuilder.FieldBuilder = f1
		_ = f2
		var p1 *sdk.ParamBuilder
		var p2 *emitbuilder.ParamBuilder = p1
		_ = p2
		var tp1 *sdk.TypeParamBuilder
		var tp2 *emitbuilder.TypeParamBuilder = tp1
		_ = tp2
		var em1 *sdk.EmbedBuilder
		var em2 *emitbuilder.EmbedBuilder = em1
		_ = em2
		var ev1 *sdk.EnumVariantBuilder
		var ev2 *emitbuilder.EnumVariantBuilder = ev1
		_ = ev2
	})

	t.Run("chain and insert position alias through", func(t *testing.T) {
		t.Parallel()
		var c1 *sdk.ChainBuilder
		var c2 *emitbuilder.ChainBuilder = c1
		_ = c2
		var i1 sdk.InsertPos
		var i2 emitbuilder.InsertPos = i1
		_ = i2
	})
}

// TestBuilderCallbacksAcceptFacadeSpellings drives one nesting of
// the construction chain with every callback parameter spelled
// through the façade.
//
// The identity assertions above prove the aliases are aliases; this
// proves the aliased names are the ones the chain actually hands
// out, which is what a plugin author discovers first and cannot
// work around.
func TestBuilderCallbacksAcceptFacadeSpellings(t *testing.T) {
	t.Parallel()

	var sawParam bool
	pkg, err := sdk.NewProvenance("t").
		Package("users", "example.com/users").
		Struct("Repo", func(s *sdk.StructBuilder) {
			s.Field("DB", sdk.Builtin("string"), func(f *sdk.FieldBuilder) {
				f.Docs("the handle")
			})
			s.Method("Get", func(m *sdk.MethodBuilder) {
				m.Receiver("r", sdk.Ptr(sdk.Internal(s.Node())))
				m.Param("id", sdk.Builtin("string"), func(p *sdk.ParamBuilder) {
					sawParam = true
					p.Variadic()
				})
				m.Return(sdk.Builtin("error"))
			})
		}).
		Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if !sawParam {
		t.Fatal("param callback never ran")
	}
	if len(pkg.Structs) != 1 {
		t.Fatalf("Structs = %d, want 1", len(pkg.Structs))
	}
	if got := len(pkg.Structs[0].Methods); got != 1 {
		t.Fatalf("Methods = %d, want 1", got)
	}
}

// TestInsertPositionsAreDistinct pins the four renamed position
// constructors to distinct values.
//
// The rename from builder's bare Prepend / At / Before / After is
// the only place this façade renames a whole family, so a binding
// crossed during the rename — [sdk.InsertBefore] wired to After —
// would compile everywhere and surface only as a contribution
// landing on the wrong side of its anchor in a generated file.
func TestInsertPositionsAreDistinct(t *testing.T) {
	t.Parallel()

	t.Run("each constructor differs from the others", func(t *testing.T) {
		t.Parallel()
		positions := map[string]sdk.InsertPos{
			"append":  {},
			"prepend": sdk.InsertPrepend(),
			"at":      sdk.InsertAt(3),
			"before":  sdk.InsertBefore("anchor"),
			"after":   sdk.InsertAfter("anchor"),
		}
		for nameA, a := range positions {
			for nameB, b := range positions {
				if nameA != nameB && a == b {
					t.Errorf("%s and %s produce the same InsertPos", nameA, nameB)
				}
			}
		}
	})

	t.Run("bindings match the underlying constructors", func(t *testing.T) {
		t.Parallel()
		cases := map[string][2]sdk.InsertPos{
			"Prepend": {sdk.InsertPrepend(), emitbuilder.Prepend()},
			"At":      {sdk.InsertAt(3), emitbuilder.At(3)},
			"Before":  {sdk.InsertBefore("a"), emitbuilder.Before("a")},
			"After":   {sdk.InsertAfter("a"), emitbuilder.After("a")},
		}
		for name, pair := range cases {
			if pair[0] != pair[1] {
				t.Errorf("Insert%s is not bound to builder.%s", name, name)
			}
		}
	})
}

// TestTypeArgHelpersLiftBothModels pins the two lifters to the
// model each names.
//
// Both take a slice of pointers to a type spelled TypeParam and
// return the same []Ref, so a caller passing the wrong one gets a
// compile error only because the element types differ — and the
// bindings themselves could be swapped here without any caller
// noticing.
func TestTypeArgHelpersLiftBothModels(t *testing.T) {
	t.Parallel()

	t.Run("node params lift to bare-name refs", func(t *testing.T) {
		t.Parallel()
		args := sdk.TypeArgsFromNodeParams([]*node.TypeParam{{Name: "T"}})
		if len(args) != 1 {
			t.Fatalf("len(args) = %d, want 1", len(args))
		}
		b, ok := args[0].(*emit.BuiltinRef)
		if !ok || b.Name != "T" {
			t.Fatalf("args[0] = %v, want a builtin ref named T", args[0])
		}
	})

	t.Run("emit params lift to bare-name refs", func(t *testing.T) {
		t.Parallel()
		args := sdk.TypeArgsFromEmitParams([]*emit.TypeParam{{Name: "K"}})
		if len(args) != 1 {
			t.Fatalf("len(args) = %d, want 1", len(args))
		}
		b, ok := args[0].(*emit.BuiltinRef)
		if !ok || b.Name != "K" {
			t.Fatalf("args[0] = %v, want a builtin ref named K", args[0])
		}
	})

	t.Run("empty input lifts to nothing", func(t *testing.T) {
		t.Parallel()
		if got := sdk.TypeArgsFromNodeParams(nil); got != nil {
			t.Errorf("TypeArgsFromNodeParams(nil) = %v, want nil", got)
		}
		if got := sdk.TypeArgsFromEmitParams(nil); got != nil {
			t.Errorf("TypeArgsFromEmitParams(nil) = %v, want nil", got)
		}
	})

	t.Run("apply instantiates an external ref", func(t *testing.T) {
		t.Parallel()
		ref := sdk.External("example.com/users", "Repo")
		got, ok := sdk.ApplyTypeArgs(ref, sdk.TypeArgsFromNodeParams(
			[]*node.TypeParam{{Name: "T"}},
		)).(*emit.ExternalRef)
		if !ok {
			t.Fatal("ApplyTypeArgs did not return an external ref")
		}
		if len(got.TypeArgs) != 1 {
			t.Fatalf("TypeArgs = %d, want 1", len(got.TypeArgs))
		}
	})

	t.Run("apply passes an uninstantiated ref through", func(t *testing.T) {
		t.Parallel()
		ref := sdk.External("example.com/users", "Repo")
		if got := sdk.ApplyTypeArgs(ref, nil); got != sdk.Ref(ref) {
			t.Errorf("ApplyTypeArgs(ref, nil) = %v, want the input ref", got)
		}
	})
}

// TestBuilderSentinelsAreDistinct pins each re-exported
// construction sentinel to its own error value, so a plugin
// branching on one with `errors.Is` cannot be caught by another.
func TestBuilderSentinelsAreDistinct(t *testing.T) {
	t.Parallel()

	sentinels := map[string]error{
		"ErrNilHost":              sdk.ErrNilHost,
		"ErrUnsupportedHost":      sdk.ErrUnsupportedHost,
		"ErrNilOrigin":            sdk.ErrNilOrigin,
		"ErrAliasMethodForbidden": sdk.ErrAliasMethodForbidden,
		"ErrUnknownInsertPos":     sdk.ErrUnknownInsertPos,
	}
	for nameA, a := range sentinels {
		if a == nil {
			t.Errorf("%s is nil", nameA)
			continue
		}
		for nameB, b := range sentinels {
			if nameA != nameB && errors.Is(a, b) {
				t.Errorf("%s matches %s under errors.Is", nameA, nameB)
			}
		}
	}
}
