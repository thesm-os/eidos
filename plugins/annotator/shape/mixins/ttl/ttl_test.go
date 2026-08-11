// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package ttl_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/ttl"
	"go.thesmos.sh/eidos/sdk"
)

// TestMixin exercises the one classification needing all three
// reference kinds at once: callables through the callable scope, a
// sentinel through the package's vars, and a quantity left alone.
func TestMixin(t *testing.T) {
	t.Parallel()

	build := func() (*sdk.Function, *sdk.Package) {
		host := &sdk.Function{
			Name: "Cache", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(ttl.Name, map[string]string{
						ttl.ParamDuration: "5m",
						ttl.ParamPut:      "Set",
						ttl.ParamRead:     "Get",
						ttl.ParamNotFound: "ErrMissing",
					}),
				},
			},
		}
		return host, &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{
				host,
				{Name: "Set", Package: "x"},
				{Name: "Get", Package: "x"},
			},
			Variables: []*sdk.Variable{{Name: "ErrMissing", Package: "x"}},
		}
	}

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, ttl.Mixin(), ttl.Name, ttl.Params)
	})

	t.Run("the callables resolve through the callable scope", func(t *testing.T) {
		t.Parallel()
		host, pkg := build()
		mixintest.RunWithResolver(t, ttl.Mixin(), pkg)

		put, _ := shape.MixinParamKey(ttl.Name, ttl.ParamPut).Get(host.Meta())
		read, _ := shape.MixinParamKey(ttl.Name, ttl.ParamRead).Get(host.Meta())
		if put != "x.Set" || read != "x.Get" {
			t.Fatalf("put/read = %q/%q, want x.Set/x.Get", put, read)
		}
	})

	t.Run("the sentinel resolves through the package's vars", func(t *testing.T) {
		t.Parallel()
		host, pkg := build()
		mixintest.RunWithResolver(t, ttl.Mixin(), pkg)

		got, _ := shape.MixinParamKey(ttl.Name, ttl.ParamNotFound).Get(host.Meta())
		if got != "x.ErrMissing" {
			t.Fatalf("notfound = %q, want %q", got, "x.ErrMissing")
		}
	})

	t.Run("the duration is left verbatim and raises nothing", func(t *testing.T) {
		t.Parallel()
		// A quantity names neither a callable nor a var. Declaring it
		// as either resolves nothing and reports every correct lifetime
		// as missing — and since a failed resolve leaves the stamp
		// alone, the diagnostic is the only place that shows.
		host, pkg := build()
		diags := mixintest.RunWithValidator(t, ttl.Mixin(), pkg)
		for _, d := range diags {
			if d.Severity == sdk.SeverityError {
				t.Fatalf("unexpected diagnostic: %s", d.Message)
			}
		}

		got, _ := shape.MixinParamKey(ttl.Name, ttl.ParamDuration).Get(host.Meta())
		if got != "5m" {
			t.Fatalf("duration = %q, want it untouched", got)
		}
	})
}
