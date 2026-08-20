// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package scope_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/scope"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, scope.Mixin(), scope.Name, scope.Params)
	})

	t.Run("pipeline stamps name param", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Begin", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(scope.Name, map[string]string{
						"name": "request",
					}),
				},
			},
		}
		bag := mixintest.RunPipeline(t, scope.Mixin(), fn)
		mixintest.AssertAttached(t, bag, scope.Name)
		mixintest.AssertParam(t, bag, scope.Name, "name", "request")
	})
}

// TestMixin_Axis covers the axis param: partition's reasoning on this
// mixin, since a misspelled axis stamps like any other opaque value
// and the check derived from it varies nothing.
func TestMixin_Axis(t *testing.T) {
	t.Parallel()

	build := func(axis string) *sdk.Package {
		kv := map[string]string{scope.ParamName: "tenant"}
		if axis != "" {
			kv[scope.ParamAxis] = axis
		}
		fn := &sdk.Function{
			Name: "Put", Package: "x",
			Params: []*sdk.Param{
				{Name: "tenantID", Type: &sdk.TypeRef{Name: "string"}},
				{Name: "v", Type: &sdk.TypeRef{Name: "string"}},
			},
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(scope.Name, kv),
				},
			},
		}
		return &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{fn}}
	}

	hasError := func(diags []sdk.Diag) bool {
		for _, d := range diags {
			if d.Severity == sdk.SeverityError {
				return true
			}
		}
		return false
	}

	t.Run("an axis naming a host parameter passes", func(t *testing.T) {
		t.Parallel()
		if diags := mixintest.RunWithValidator(t, scope.Mixin(), build("tenantID")); hasError(diags) {
			t.Fatalf("unexpected error diagnostics: %+v", diags)
		}
	})

	t.Run("an axis naming no host parameter is reported", func(t *testing.T) {
		t.Parallel()
		if diags := mixintest.RunWithValidator(t, scope.Mixin(), build("tenantId")); !hasError(diags) {
			t.Fatal("no error diagnostic for a misspelled axis, which varies nothing")
		}
	})

	t.Run("the bare form passes", func(t *testing.T) {
		t.Parallel()
		// name= alone classifies; the isolation check is declined
		// rather than derived from a guessed parameter.
		if diags := mixintest.RunWithValidator(t, scope.Mixin(), build("")); hasError(diags) {
			t.Fatalf("unexpected error diagnostics: %+v", diags)
		}
	})
}
