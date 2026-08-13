// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package orderafter_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/orderafter"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, orderafter.Mixin(), orderafter.Name, orderafter.Params)
	})

	t.Run("resolver rewrites fn param to qualified name", func(t *testing.T) {
		t.Parallel()
		host := &sdk.Function{
			Name: "DoWork", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(orderafter.Name, map[string]string{
						"fn": "Initialise",
					}),
				},
			},
		}
		initFn := &sdk.Function{Name: "Initialise", Package: "x"}
		mixintest.RunWithResolver(t, orderafter.Mixin(), &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{host, initFn},
		})

		got, _ := shape.MixinParamKey(orderafter.Name, orderafter.ParamFn).Get(host.Meta())
		if got != "x.Initialise" {
			t.Fatalf("fn param = %q, want %q", got, "x.Initialise")
		}
	})
}

// TestMixin_UnreadySentinel pins the early-call refusal's identity.
//
// Without it "calling early fails" is a bare non-nil check, which an
// implementation failing for an unrelated reason — a nil map, a
// refused connection — passes as ordering enforcement.
func TestMixin_UnreadySentinel(t *testing.T) {
	t.Parallel()

	host := &sdk.Function{
		Name: "DoWork", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				mixintest.HostDirective(orderafter.Name, map[string]string{
					orderafter.ParamFn:      "Initialise",
					orderafter.ParamUnready: "ErrNotReady",
				}),
			},
		},
	}
	mixintest.RunWithResolver(t, orderafter.Mixin(), &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{host, {Name: "Initialise", Package: "x"}},
		Variables: []*sdk.Variable{{Name: "ErrNotReady", Package: "x"}},
	})

	// The callable through the callable scope, the sentinel through
	// the package's vars — both on one directive.
	fn, _ := shape.MixinParamKey(orderafter.Name, orderafter.ParamFn).Get(host.Meta())
	unready, _ := shape.MixinParamKey(orderafter.Name, orderafter.ParamUnready).Get(host.Meta())
	if fn != "x.Initialise" {
		t.Errorf("fn = %q, want x.Initialise", fn)
	}
	if unready != "x.ErrNotReady" {
		t.Errorf("unready = %q, want x.ErrNotReady", unready)
	}
}
