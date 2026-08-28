// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package xsssafe_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/xsssafe"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, xsssafe.Mixin(), xsssafe.Name, xsssafe.Params)
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
					mixintest.HostDirective(xsssafe.Name, kv),
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
		host, pkg := build(map[string]string{xsssafe.ParamUnsafe: "Hostile"})
		mixintest.RunWithResolver(t, xsssafe.Mixin(), pkg)

		got, _ := shape.MixinParamKey(xsssafe.Name, xsssafe.ParamUnsafe).Get(host.Meta())
		if got != "x.Hostile" {
			t.Fatalf("unsafe = %q, want x.Hostile", got)
		}
	})

	t.Run("the bare form still classifies", func(t *testing.T) {
		t.Parallel()
		_, pkg := build(map[string]string{})
		for _, d := range mixintest.RunWithValidator(t, xsssafe.Mixin(), pkg) {
			if d.Severity == sdk.SeverityError {
				t.Fatalf("bare xsssafe was refused: %s", d.Message)
			}
		}
	})
}
