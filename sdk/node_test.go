// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk_test

import (
	"testing"

	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/sdk"
)

// TestSourceModelAliasesPreserveIdentity pins every re-exported
// source kind as a Go type alias rather than a wrapper.
//
// The property matters more here than anywhere else in the façade:
// a plugin receives its nodes from the pipeline, typed as [node]
// values, and names them as [sdk] values. A wrapper would make
// that assignment fail to compile — and worse, a wrapper that
// happened to be structurally identical would make the same node
// two incompatible types across a plugin boundary.
//
// The deliberately-redundant `var x node.X = sdkAlias` pattern is
// the only compile-time spelling of the assertion.
//
//nolint:staticcheck // intentional redundant typing — the redundancy is the test
func TestSourceModelAliasesPreserveIdentity(t *testing.T) {
	t.Parallel()

	t.Run("declaration kinds alias to the node package", func(t *testing.T) {
		t.Parallel()
		var n1 sdk.Node
		var n2 node.Node = n1
		_ = n2
		var b1 *sdk.BaseNode
		var b2 *node.BaseNode = b1
		_ = b2
		var p1 *sdk.Package
		var p2 *node.Package = p1
		_ = p2
		var f1 *sdk.File
		var f2 *node.File = f1
		_ = f2
		var i1 *sdk.Import
		var i2 *node.Import = i1
		_ = i2
		var s1 *sdk.Struct
		var s2 *node.Struct = s1
		_ = s2
		var if1 *sdk.Interface
		var if2 *node.Interface = if1
		_ = if2
		var m1 *sdk.Method
		var m2 *node.Method = m1
		_ = m2
		var fn1 *sdk.Function
		var fn2 *node.Function = fn1
		_ = fn2
		var a1 *sdk.Alias
		var a2 *node.Alias = a1
		_ = a2
		var c1 *sdk.Constant
		var c2 *node.Constant = c1
		_ = c2
		var v1 *sdk.Variable
		var v2 *node.Variable = v1
		_ = v2
		var e1 *sdk.Enum
		var e2 *node.Enum = e1
		_ = e2
	})

	t.Run("declaration parts alias to the node package", func(t *testing.T) {
		t.Parallel()
		var f1 *sdk.Field
		var f2 *node.Field = f1
		_ = f2
		var p1 *sdk.Param
		var p2 *node.Param = p1
		_ = p2
		var r1 *sdk.Return
		var r2 *node.Return = r1
		_ = r2
		var tp1 *sdk.TypeParam
		var tp2 *node.TypeParam = tp1
		_ = tp2
		var co1 *sdk.Constraint
		var co2 *node.Constraint = co1
		_ = co2
		var tr1 *sdk.TypeRef
		var tr2 *node.TypeRef = tr1
		_ = tr2
		var ev1 *sdk.EnumVariant
		var ev2 *node.EnumVariant = ev1
		_ = ev2
		var em1 *sdk.Embed
		var em2 *node.Embed = em1
		_ = em2
	})

	t.Run("method-set shapes alias to the node package", func(t *testing.T) {
		t.Parallel()
		var r1 sdk.MethodSetResult
		var r2 node.MethodSetResult = r1
		_ = r2
		var e1 sdk.MethodSetEntry
		var e2 node.MethodSetEntry = e1
		_ = e2
		var i1 sdk.MethodSetIssue
		var i2 node.MethodSetIssue = i1
		_ = i2
		var rs1 sdk.MethodSetReason
		var rs2 node.MethodSetReason = rs1
		_ = rs2
		var ir1 sdk.InterfaceResolver
		var ir2 node.InterfaceResolver = ir1
		_ = ir2
	})

	t.Run("traversal shapes alias to the node package", func(t *testing.T) {
		t.Parallel()
		var v1 sdk.NodeVisitor
		var v2 node.Visitor = v1
		_ = v2
		var vf1 sdk.NodeVisitorFunc
		var vf2 node.VisitorFunc = vf1
		_ = vf2
		var k1 sdk.TypeRefKind
		var k2 node.TypeRefKind = k1
		_ = k2
	})
}

// TestTypeRefKindsMatchUnderlying pins the variant discriminators
// to node's values. These are iota constants, so a re-export that
// drifted by one would still compile everywhere and silently
// route every pointer type down the slice branch of a generator's
// switch.
func TestTypeRefKindsMatchUnderlying(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		sdk  sdk.TypeRefKind
		node node.TypeRefKind
	}{
		{"Named", sdk.TypeRefNamed, node.TypeRefNamed},
		{"Pointer", sdk.TypeRefPointer, node.TypeRefPointer},
		{"Slice", sdk.TypeRefSlice, node.TypeRefSlice},
		{"Array", sdk.TypeRefArray, node.TypeRefArray},
		{"Map", sdk.TypeRefMap, node.TypeRefMap},
		{"Func", sdk.TypeRefFunc, node.TypeRefFunc},
		{"TypeParam", sdk.TypeRefTypeParam, node.TypeRefTypeParam},
		{"AnonStruct", sdk.TypeRefAnonStruct, node.TypeRefAnonStruct},
		{"AnonInterface", sdk.TypeRefAnonInterface, node.TypeRefAnonInterface},
	}

	t.Run("each re-export equals its node constant", func(t *testing.T) {
		t.Parallel()
		for _, tc := range cases {
			if tc.sdk != tc.node {
				t.Errorf("TypeRef%s = %d, want %d", tc.name, tc.sdk, tc.node)
			}
		}
	})

	t.Run("the variants stay distinct from one another", func(t *testing.T) {
		t.Parallel()
		// Two constants collapsed onto one value is the failure a
		// per-constant equality check cannot see: both sides would
		// match their (equally wrong) source.
		seen := make(map[sdk.TypeRefKind]string, len(cases))
		for _, tc := range cases {
			if prev, dup := seen[tc.sdk]; dup {
				t.Errorf("TypeRef%s and TypeRef%s share the value %d",
					tc.name, prev, tc.sdk)
			}
			seen[tc.sdk] = tc.name
		}
	})
}

// TestMethodSetReasonsMatchUnderlying pins the failure
// classification. A generator decides whether to emit a partial
// double from this value; a drifted constant turns "the run was
// narrow" into "the source is broken" and vice versa.
func TestMethodSetReasonsMatchUnderlying(t *testing.T) {
	t.Parallel()
	pairs := []struct {
		name string
		got  sdk.MethodSetReason
		want node.MethodSetReason
	}{
		{"Unresolved", sdk.ReasonUnresolved, node.ReasonUnresolved},
		{"NonInterface", sdk.ReasonNonInterface, node.ReasonNonInterface},
		{"Cyclic", sdk.ReasonCyclic, node.ReasonCyclic},
		{"Generic", sdk.ReasonGeneric, node.ReasonGeneric},
	}
	for _, pair := range pairs {
		if pair.got != pair.want {
			t.Errorf("sdk.Reason%s = %d, want %d", pair.name, pair.got, pair.want)
		}
	}
}

// TestNodeHelpersProxyUnderlying drives the re-exported lookup
// helpers against a hand-built method slice. The alias test proves
// the names resolve; this proves each one is bound to the function
// its doc comment claims, which a `var` re-export can get wrong
// without anything failing to compile.
//
//nolint:staticcheck // intentional redundant typing — the redundancy is the test
func TestNodeHelpersProxyUnderlying(t *testing.T) {
	t.Parallel()

	methods := []*sdk.Method{
		{Name: "Get", Receiver: &sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Name: "Repo"}},
		{Name: "Set", Receiver: &sdk.TypeRef{
			TypeKind: sdk.TypeRefPointer,
			Elem:     &sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Name: "Repo"},
		}},
	}

	t.Run("MethodByName finds the named method", func(t *testing.T) {
		t.Parallel()
		got := sdk.MethodByName(methods, "Set")
		if got == nil || got.Name != "Set" {
			t.Fatalf("MethodByName(Set) = %v, want the Set method", got)
		}
	})

	t.Run("Declares answers presence", func(t *testing.T) {
		t.Parallel()
		if !sdk.Declares(methods, "Get") {
			t.Error("Declares(Get) = false, want true")
		}
		if sdk.Declares(methods, "Missing") {
			t.Error("Declares(Missing) = true, want false")
		}
	})

	t.Run("PointerReceiver distinguishes the receivers", func(t *testing.T) {
		t.Parallel()
		if sdk.PointerReceiver(methods, "Get") {
			t.Error("PointerReceiver(Get) = true, want false")
		}
		if !sdk.PointerReceiver(methods, "Set") {
			t.Error("PointerReceiver(Set) = false, want true")
		}
	})

	t.Run("LocalName strips the package qualifier", func(t *testing.T) {
		t.Parallel()
		if got := sdk.LocalName("pkg.Name"); got != "Name" {
			t.Errorf("LocalName(pkg.Name) = %q, want Name", got)
		}
	})

	t.Run("IsExportedName answers the case question", func(t *testing.T) {
		t.Parallel()
		if !sdk.IsExportedName("Exported") {
			t.Error("IsExportedName(Exported) = false, want true")
		}
		if sdk.IsExportedName("unexported") {
			t.Error("IsExportedName(unexported) = true, want false")
		}
	})

	t.Run("AnonReturns builds source-side returns", func(t *testing.T) {
		t.Parallel()
		// Pinned against the emit-side namesake: the two carry
		// different element types, and the bare name must be the
		// source one.
		var got []*node.Return = sdk.AnonReturns(&sdk.TypeRef{Name: "error"})
		if len(got) != 1 {
			t.Fatalf("AnonReturns returned %d entries, want 1", len(got))
		}
	})
}
