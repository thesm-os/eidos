// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package deleteremoves_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/deleteremoves"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, deleteremoves.Mixin(), deleteremoves.Name, deleteremoves.Params)
	})

	t.Run("resolver rewrites the read param to a qualified name", func(t *testing.T) {
		t.Parallel()
		// The point of declaring the param: an undeclared key stamps as
		// a bare name with no package and no owner, which a generator
		// cannot resolve to a callable — and the partner is the whole
		// content of this mixin.
		host := &sdk.Function{
			Name: "Delete", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(deleteremoves.Name, map[string]string{
						deleteremoves.ParamRead: "Get",
					}),
				},
			},
		}
		partner := &sdk.Function{Name: "Get", Package: "x"}
		mixintest.RunWithResolver(t, deleteremoves.Mixin(), &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{host, partner},
		})

		got, _ := shape.MixinParamKey(deleteremoves.Name, deleteremoves.ParamRead).Get(host.Meta())
		if got != "x.Get" {
			t.Fatalf("read param = %q, want %q", got, "x.Get")
		}
	})
}

// TestMixin_Sentinel covers sentinel resolution on a method host.
//
// The scope is the point: a partner callable is reached through the
// receiver's own method set, and a sentinel is not declared there. It
// resolves against the package instead, for a method exactly as for a
// function.
func TestMixin_Sentinel(t *testing.T) {
	t.Parallel()

	build := func(sentinel string, vars ...*sdk.Variable) (*sdk.Method, *sdk.Package) {
		del := &sdk.Method{
			Name: "Delete",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(deleteremoves.Name, map[string]string{
						deleteremoves.ParamRead:     "Get",
						deleteremoves.ParamSentinel: sentinel,
					}),
				},
			},
		}
		get := &sdk.Method{Name: "Get"}
		store := &sdk.Interface{
			Name: "Store", Package: "x",
			Methods: []*sdk.Method{del, get},
		}
		del.Owner, get.Owner = store, store
		return del, &sdk.Package{
			Name: "x", Path: "x",
			Interfaces: []*sdk.Interface{store},
			Variables:  vars,
		}
	}

	t.Run("a sentinel resolves against the package, not the receiver", func(t *testing.T) {
		t.Parallel()
		del, pkg := build("ErrNotFound", &sdk.Variable{Name: "ErrNotFound", Package: "x"})
		mixintest.RunWithResolver(t, deleteremoves.Mixin(), pkg)

		got, _ := shape.MixinParamKey(deleteremoves.Name, deleteremoves.ParamSentinel).Get(del.Meta())
		if got != "x.ErrNotFound" {
			t.Fatalf("sentinel = %q, want %q", got, "x.ErrNotFound")
		}
	})

	t.Run("the read partner still resolves through the receiver", func(t *testing.T) {
		t.Parallel()
		// The two scopes coexist on one directive: a callable through
		// the owner, a var through the package.
		del, pkg := build("ErrNotFound", &sdk.Variable{Name: "ErrNotFound", Package: "x"})
		mixintest.RunWithResolver(t, deleteremoves.Mixin(), pkg)

		got, _ := shape.MixinParamKey(deleteremoves.Name, deleteremoves.ParamRead).Get(del.Meta())
		if got != "x.Store.Get" {
			t.Fatalf("read = %q, want %q", got, "x.Store.Get")
		}
	})
}
