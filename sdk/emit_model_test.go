// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk_test

import (
	"errors"
	"testing"

	"go.thesmos.sh/eidos/core/kind"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/sdk"
)

// TestEmitModelAliasesPreserveIdentity pins every re-exported emit
// kind as a type alias. A generator builds these and hands them to
// the pipeline typed as [emit] values; a wrapper would break that
// handoff at the plugin boundary.
//
//nolint:staticcheck // intentional redundant typing — the redundancy is the test
func TestEmitModelAliasesPreserveIdentity(t *testing.T) {
	t.Parallel()

	t.Run("declaration kinds alias to the emit package", func(t *testing.T) {
		t.Parallel()
		var p1 *sdk.EmitPackage
		var p2 *emit.Package = p1
		_ = p2
		var f1 *sdk.EmitFile
		var f2 *emit.File = f1
		_ = f2
		var i1 *sdk.EmitImport
		var i2 *emit.Import = i1
		_ = i2
		var s1 *sdk.EmitStruct
		var s2 *emit.Struct = s1
		_ = s2
		var if1 *sdk.EmitInterface
		var if2 *emit.Interface = if1
		_ = if2
		var m1 *sdk.EmitMethod
		var m2 *emit.Method = m1
		_ = m2
		var fn1 *sdk.EmitFunction
		var fn2 *emit.Function = fn1
		_ = fn2
		var a1 *sdk.EmitAlias
		var a2 *emit.Alias = a1
		_ = a2
		var c1 *sdk.EmitConstant
		var c2 *emit.Constant = c1
		_ = c2
		var v1 *sdk.EmitVariable
		var v2 *emit.Variable = v1
		_ = v2
		var e1 *sdk.EmitEnum
		var e2 *emit.Enum = e1
		_ = e2
	})

	t.Run("declaration parts alias to the emit package", func(t *testing.T) {
		t.Parallel()
		var f1 *sdk.EmitField
		var f2 *emit.Field = f1
		_ = f2
		var p1 *sdk.EmitParam
		var p2 *emit.Param = p1
		_ = p2
		var r1 *sdk.EmitReturn
		var r2 *emit.Return = r1
		_ = r2
		var tp1 *sdk.EmitTypeParam
		var tp2 *emit.TypeParam = tp1
		_ = tp2
		var co1 *sdk.EmitConstraint
		var co2 *emit.Constraint = co1
		_ = co2
		var tr1 *sdk.EmitTypeRef
		var tr2 *emit.TypeRef = tr1
		_ = tr2
		var ev1 *sdk.EmitEnumVariant
		var ev2 *emit.EnumVariant = ev1
		_ = ev2
		var em1 *sdk.EmitEmbed
		var em2 *emit.Embed = em1
		_ = em2
	})

	t.Run("emit-only shapes alias to the emit package", func(t *testing.T) {
		t.Parallel()
		var s1 *sdk.Stmt
		var s2 *emit.Stmt = s1
		_ = s2
		var tg1 *sdk.Tag
		var tg2 *emit.Tag = tg1
		_ = tg2
		var b1 *sdk.BuiltinRef
		var b2 *emit.BuiltinRef = b1
		_ = b2
		var x1 *sdk.ExternalRef
		var x2 *emit.ExternalRef = x1
		_ = x2
		var c1 *sdk.CompositeRef
		var c2 *emit.CompositeRef = c1
		_ = c2
		var af1 sdk.AnonField
		var af2 emit.AnonField = af1
		_ = af2
		var ut1 sdk.UnionTerm
		var ut2 emit.UnionTerm = ut1
		_ = ut2
		var pv1 sdk.EmitProvenance
		var pv2 emit.Provenance = pv1
		_ = pv2
	})

	t.Run("traversal shapes alias to the emit package", func(t *testing.T) {
		t.Parallel()
		var v1 sdk.EmitVisitor
		var v2 emit.Visitor = v1
		_ = v2
		var vf1 sdk.EmitVisitorFunc
		var vf2 emit.VisitorFunc = vf1
		_ = vf2
	})
}

// TestEmitKindsMatchUnderlying pins the emit-side discriminators to
// emit's values and — the property the prefix exists for — as
// distinct from their source namesakes.
//
// The failure the prefix prevents is silent in both directions. A
// slot constrained on a source kind rejects every emit value with a
// kind mismatch the contributing plugin never sees; a directive
// scoped to an emit kind matches no source node, so the plugin
// simply never fires.
func TestEmitKindsMatchUnderlying(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		sdk  kind.Kind
		emit kind.Kind
	}{
		{"Package", sdk.EmitKindPackage, emit.KindPackage},
		{"File", sdk.EmitKindFile, emit.KindFile},
		{"Import", sdk.EmitKindImport, emit.KindImport},
		{"Struct", sdk.EmitKindStruct, emit.KindStruct},
		{"Interface", sdk.EmitKindInterface, emit.KindInterface},
		{"Method", sdk.EmitKindMethod, emit.KindMethod},
		{"Field", sdk.EmitKindField, emit.KindField},
		{"Function", sdk.EmitKindFunction, emit.KindFunction},
		{"Variable", sdk.EmitKindVariable, emit.KindVariable},
		{"Constant", sdk.EmitKindConstant, emit.KindConstant},
		{"Enum", sdk.EmitKindEnum, emit.KindEnum},
		{"EnumVariant", sdk.EmitKindEnumVariant, emit.KindEnumVariant},
		{"Alias", sdk.EmitKindAlias, emit.KindAlias},
		{"Embed", sdk.EmitKindEmbed, emit.KindEmbed},
		{"Param", sdk.EmitKindParam, emit.KindParam},
		{"Return", sdk.EmitKindReturn, emit.KindReturn},
		{"TypeParam", sdk.EmitKindTypeParam, emit.KindTypeParam},
		{"Tag", sdk.EmitKindTag, emit.KindTag},
		{"TypeRef", sdk.EmitKindTypeRef, emit.KindTypeRef},
		{"ExternalRef", sdk.EmitKindExternalRef, emit.KindExternalRef},
		{"BuiltinRef", sdk.EmitKindBuiltinRef, emit.KindBuiltinRef},
		{"CompositeRef", sdk.EmitKindCompositeRef, emit.KindCompositeRef},
		{"Stmt", sdk.EmitKindStmt, emit.KindStmt},
		{"Expr", sdk.EmitKindExpr, emit.KindExpr},
		{"Slot", sdk.EmitKindSlot, emit.KindSlot},
	}

	t.Run("each re-export equals its emit constant", func(t *testing.T) {
		t.Parallel()
		for _, tc := range cases {
			if tc.sdk != tc.emit {
				t.Errorf("EmitKind%s = %q, want %q", tc.name, tc.sdk, tc.emit)
			}
		}
	})

	t.Run("the emit set covers every kind emit declares", func(t *testing.T) {
		t.Parallel()
		// Counted from emit/kind.go rather than a literal, for the
		// same reason the source set is: a literal would need
		// updating by the very commit that forgets the re-export.
		declared := declaredKindsIn(t, "emit")
		if len(cases) != declared {
			t.Fatalf("re-exported %d emit kinds, emit/kind.go declares %d",
				len(cases), declared)
		}
	})

	t.Run("no emit kind collides with its source namesake", func(t *testing.T) {
		t.Parallel()
		sourceKinds := map[string]kind.Kind{
			"Package": sdk.NodeKindPackage, "File": sdk.NodeKindFile,
			"Import": sdk.NodeKindImport, "Struct": sdk.NodeKindStruct,
			"Interface": sdk.NodeKindInterface, "Method": sdk.NodeKindMethod,
			"Field": sdk.NodeKindField, "Function": sdk.NodeKindFunction,
			"Param": sdk.NodeKindParam, "Return": sdk.NodeKindReturn,
			"TypeParam": sdk.NodeKindTypeParam, "TypeRef": sdk.NodeKindTypeRef,
			"Alias": sdk.NodeKindAlias, "Constant": sdk.NodeKindConstant,
			"Variable": sdk.NodeKindVariable, "Enum": sdk.NodeKindEnum,
			"EnumVariant": sdk.NodeKindEnumVariant, "Embed": sdk.NodeKindEmbed,
		}
		for _, tc := range cases {
			sk, shared := sourceKinds[tc.name]
			if !shared {
				continue
			}
			if tc.sdk == sk {
				t.Errorf("EmitKind%s and NodeKind%s share the value %q; "+
					"the prefix stops being a distinction", tc.name, tc.name, sk)
			}
		}
	})
}

// TestEmitSentinelsAreDistinct pins the emit error sentinels as
// re-exported and as separate failure modes. A plugin appending
// into a slot it does not own is the caller that has to tell a
// kind mismatch from a missing contribution ID, and collapsing
// them would make one of the two unreportable.
func TestEmitSentinelsAreDistinct(t *testing.T) {
	t.Parallel()

	t.Run("each aliases its emit sentinel", func(t *testing.T) {
		t.Parallel()
		if !errors.Is(sdk.ErrSlotElementType, emit.ErrSlotElementType) {
			t.Error("sdk.ErrSlotElementType does not match emit.ErrSlotElementType")
		}
		if !errors.Is(sdk.ErrProvenanceNotFound, emit.ErrProvenanceNotFound) {
			t.Error("sdk.ErrProvenanceNotFound does not match emit.ErrProvenanceNotFound")
		}
		if !errors.Is(sdk.ErrMixedNamedReturns, emit.ErrMixedNamedReturns) {
			t.Error("sdk.ErrMixedNamedReturns does not match emit.ErrMixedNamedReturns")
		}
	})

	t.Run("the sentinels do not match one another", func(t *testing.T) {
		t.Parallel()
		if errors.Is(sdk.ErrSlotElementType, sdk.ErrProvenanceNotFound) {
			t.Error("ErrSlotElementType must not match ErrProvenanceNotFound")
		}
		if errors.Is(sdk.ErrMixedNamedReturns, sdk.ErrSlotElementType) {
			t.Error("ErrMixedNamedReturns must not match ErrSlotElementType")
		}
	})
}
