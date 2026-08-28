// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package validates_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/validates"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, validates.Mixin(), validates.Name, validates.Params)
	})

	t.Run("resolver rewrites fn param to qualified name", func(t *testing.T) {
		t.Parallel()
		host := &sdk.Function{
			Name: "Save", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(validates.Name, map[string]string{
						"fn": "ValidateInput",
					}),
				},
			},
		}
		validator := &sdk.Function{Name: "ValidateInput", Package: "x"}
		mixintest.RunWithResolver(t, validates.Mixin(), &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{host, validator},
		})

		got, _ := shape.MixinParamKey(validates.Name, validates.ParamFn).Get(host.Meta())
		if got != "x.ValidateInput" {
			t.Fatalf("fn param = %q, want %q", got, "x.ValidateInput")
		}
	})
}

// TestMixin_Invalid covers the key that lets the forward law bind.
//
// The backwards reading — whatever the call accepted, the validator
// must accept too — binds without a refused value and cannot fail:
// the corpus proved it by deleting the subject's screening and
// watching the check stay green. The forward reading needs a value
// the validator refuses, and only the author knows what that is.
func TestMixin_Invalid(t *testing.T) {
	t.Parallel()

	build := func(kv map[string]string) (*sdk.Function, *sdk.Package) {
		host := &sdk.Function{
			Name: "Save", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(validates.Name, kv),
				},
			},
		}
		return host, &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{host, {Name: "ValidateInput", Package: "x"}},
			Variables: []*sdk.Variable{{Name: "BadPayload", Package: "x"}},
		}
	}

	t.Run("the invalid value resolves through the package's vars", func(t *testing.T) {
		t.Parallel()
		// KindVar: a refused value is not declared on the receiver, so
		// the scope is the package's — and the stamp lands qualified,
		// which is what a law needs to hand it to the call.
		host, pkg := build(map[string]string{
			validates.ParamFn:      "ValidateInput",
			validates.ParamInvalid: "BadPayload",
		})
		mixintest.RunWithResolver(t, validates.Mixin(), pkg)

		got, _ := shape.MixinParamKey(validates.Name, validates.ParamInvalid).Get(host.Meta())
		if got != "x.BadPayload" {
			t.Fatalf("invalid = %q, want x.BadPayload", got)
		}
	})

	t.Run("the bare form still classifies", func(t *testing.T) {
		t.Parallel()
		// Optional: a validated callable without a refused value is
		// still what the mixin names, and the forward law simply does
		// not bind.
		_, pkg := build(map[string]string{validates.ParamFn: "ValidateInput"})
		for _, d := range mixintest.RunWithValidator(t, validates.Mixin(), pkg) {
			if d.Severity == sdk.SeverityError {
				t.Fatalf("bare validates was refused: %s", d.Message)
			}
		}
	})
}
