// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package indexed_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/indexed"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/sdk"
)

// less builds the motivating shape: two integer parameters, both
// positions into a collection the subject sizes.
func less(params map[string]string) *sdk.Method {
	return &sdk.Method{
		Name: "Less",
		Params: []*sdk.Param{
			{Name: "i", Type: &sdk.TypeRef{Name: "int"}},
			{Name: "j", Type: &sdk.TypeRef{Name: "int"}},
		},
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				mixintest.HostDirective(indexed.Name, params),
			},
		},
	}
}

func sorter(host *sdk.Method, withLen bool) *sdk.Package {
	iface := &sdk.Interface{Name: "Sorter", Package: "x", Methods: []*sdk.Method{host}}
	host.Owner = iface
	if withLen {
		length := &sdk.Method{
			Name:     "Len",
			Returns:  sdk.AnonReturns(&sdk.TypeRef{Name: "int"}),
			BaseNode: sdk.BaseNode{},
		}
		length.Owner = iface
		iface.Methods = append(iface.Methods, length)
	}
	return &sdk.Package{Name: "x", Path: "x", Interfaces: []*sdk.Interface{iface}}
}

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, indexed.Mixin(), indexed.Name, indexed.Params)
	})

	t.Run("the sizing method resolves to a qualified name", func(t *testing.T) {
		t.Parallel()
		host := less(map[string]string{indexed.ParamBy: "Len"})
		mixintest.RunWithResolver(t, indexed.Mixin(), sorter(host, true))

		got, _ := shape.MixinParamKey(indexed.Name, indexed.ParamBy).Get(host.Meta())
		if got != "x.Sorter.Len" {
			t.Fatalf("by = %q, want %q", got, "x.Sorter.Len")
		}
	})

	t.Run("a sizing method naming nothing is reported", func(t *testing.T) {
		t.Parallel()
		// The whole reason `by` is a callable reference rather than a
		// literal: a misspelling fails here instead of as an
		// out-of-range index in generated code.
		host := less(map[string]string{indexed.ParamBy: "Length"})
		diags := mixintest.RunWithValidator(t, indexed.Mixin(), sorter(host, true))

		var reported bool
		for _, d := range diags {
			if d.Severity == sdk.SeverityError {
				reported = true
			}
		}
		if !reported {
			t.Fatal("a by= naming no sibling raised nothing")
		}
	})

	t.Run("an absent sizing method is not an error", func(t *testing.T) {
		t.Parallel()
		// The fact stands without the bound; a consumer that must size
		// the collection declines rather than guessing.
		host := less(map[string]string{})
		diags := mixintest.RunWithValidator(t, indexed.Mixin(), sorter(host, true))
		for _, d := range diags {
			if d.Severity == sdk.SeverityError {
				t.Fatalf("unexpected diagnostic: %s", d.Message)
			}
		}
	})
}
