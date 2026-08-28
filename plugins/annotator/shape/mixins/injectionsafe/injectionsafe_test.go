// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package injectionsafe_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/injectionsafe"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, injectionsafe.Mixin(), injectionsafe.Name, injectionsafe.Params)
}

// TestMixin_ObserverResolves pins the partner params: the claim is
// about state a check has to look at, and a bare name is not
// something a generated check can call.
func TestMixin_ObserverResolves(t *testing.T) {
	t.Parallel()
	host := &sdk.Function{
		Name: "Host", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				mixintest.HostDirective(injectionsafe.Name, map[string]string{injectionsafe.ParamRead: "Load"}),
			},
		},
	}
	fns := []*sdk.Function{
		host,
		{Name: "Load", Package: "x"},
	}
	mixintest.RunWithResolver(t, injectionsafe.Mixin(), &sdk.Package{
		Name: "x", Path: "x", Functions: fns,
	})
	if got, _ := shape.MixinParamKey(injectionsafe.Name, injectionsafe.ParamRead).Get(host.Meta()); got != "x.Load" {
		t.Errorf("read = %q, want x.Load", got)
	}
}

// TestMixin_DeclaredInput covers the key that lets the law bind in
// the direction that can fail: only the author knows which input
// their subject must refuse to pass through intact, so the directive
// names a declared value rather than deriving one — the trap the
// validates mixin's invalid= closed, three classifications wide.
func TestMixin_DeclaredInput(t *testing.T) {
	t.Parallel()

	build := func(kv map[string]string) (*sdk.Function, *sdk.Package) {
		host := &sdk.Function{
			Name: "Handle", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(injectionsafe.Name, kv),
				},
			},
		}
		return host, &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{host},
			Variables: []*sdk.Variable{{Name: "Hostile", Package: "x"}},
		}
	}

	t.Run("the declared input resolves through the package's vars", func(t *testing.T) {
		t.Parallel()
		host, pkg := build(map[string]string{injectionsafe.ParamUnsafe: "Hostile"})
		mixintest.RunWithResolver(t, injectionsafe.Mixin(), pkg)

		got, _ := shape.MixinParamKey(injectionsafe.Name, injectionsafe.ParamUnsafe).Get(host.Meta())
		if got != "x.Hostile" {
			t.Fatalf("unsafe = %q, want x.Hostile", got)
		}
	})

	t.Run("the bare form still classifies", func(t *testing.T) {
		t.Parallel()
		_, pkg := build(map[string]string{})
		for _, d := range mixintest.RunWithValidator(t, injectionsafe.Mixin(), pkg) {
			if d.Severity == sdk.SeverityError {
				t.Fatalf("bare injectionsafe was refused: %s", d.Message)
			}
		}
	})
}
