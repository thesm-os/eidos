// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pointintime_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/pointintime"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, pointintime.Mixin(), pointintime.Name, pointintime.Params)
}

// TestMixin_Write covers the key that makes the law witnessable.
//
// Two reads agreeing across a write is the whole claim, so the write
// has to be nameable — before this key the directive stated the
// property and no derivation could find the partner, because which
// write matters is the author's choice rather than a fact of the
// signature.
func TestMixin_Write(t *testing.T) {
	t.Parallel()

	build := func(kv map[string]string) (*sdk.Method, *sdk.Package) {
		get := &sdk.Method{
			Name: "Get",
			Params: []*sdk.Param{
				{Name: "ctx", Type: &sdk.TypeRef{Name: "Context", Package: "context"}},
				{Name: "key", Type: &sdk.TypeRef{Name: "string"}},
			},
			Returns: sdk.AnonReturns(
				&sdk.TypeRef{Name: "Value", Package: "x"},
				&sdk.TypeRef{Name: "error"},
			),
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(pointintime.Name, kv),
				},
			},
		}
		store := &sdk.Method{
			Name: "Store",
			Params: []*sdk.Param{
				{Name: "ctx", Type: &sdk.TypeRef{Name: "Context", Package: "context"}},
				{Name: "v", Type: &sdk.TypeRef{Name: "Value", Package: "x"}},
			},
			Returns: sdk.AnonReturns(&sdk.TypeRef{Name: "error"}),
		}
		snap := &sdk.Interface{Name: "Snap", Package: "x", Methods: []*sdk.Method{get, store}}
		get.Owner, store.Owner = snap, snap
		return get, &sdk.Package{Name: "x", Path: "x", Interfaces: []*sdk.Interface{snap}}
	}

	t.Run("the write resolves through the callable scope", func(t *testing.T) {
		t.Parallel()
		// A sibling method, so it resolves through the owner's method
		// set rather than the package's functions — and the stamp
		// lands qualified, which is what a law needs to call it.
		get, pkg := build(map[string]string{pointintime.ParamWrite: "Store"})
		mixintest.RunWithResolver(t, pointintime.Mixin(), pkg)

		got, _ := shape.MixinParamKey(pointintime.Name, pointintime.ParamWrite).Get(get.Meta())
		if got != "x.Snap.Store" {
			t.Fatalf("write = %q, want x.Snap.Store", got)
		}
	})

	t.Run("the bare form still classifies", func(t *testing.T) {
		t.Parallel()
		// Optional on the terms pool's stats role is: a read answering
		// a consistent snapshot is what this names either way, and
		// requiring the key would retire the classification for every
		// subject already carrying it.
		_, pkg := build(map[string]string{})
		for _, d := range mixintest.RunWithValidator(t, pointintime.Mixin(), pkg) {
			if d.Severity == sdk.SeverityError {
				t.Fatalf("bare pointintime was refused: %s", d.Message)
			}
		}
	})

	t.Run("a write nothing declares is reported", func(t *testing.T) {
		t.Parallel()
		// KindCallable, so the resolver checks the name against the
		// host's scope: a typo is caught where the author is rather
		// than surfacing as a law that calls nothing.
		_, pkg := build(map[string]string{pointintime.ParamWrite: "Nonesuch"})
		var found bool
		for _, d := range mixintest.RunWithValidator(t, pointintime.Mixin(), pkg) {
			if d.Severity == sdk.SeverityError {
				found = true
			}
		}
		if !found {
			t.Fatal("an unresolvable write was accepted")
		}
	})
}
