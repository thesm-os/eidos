// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package ttl_test

import (
	"strings"
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

// TestMixin_Lifetime covers the key that lets a store whose entries
// each carry their own expiry say so.
//
// The gap it closes: every param the mixin had described a lifetime
// the directive fixes, so a cache giving each entry its own could
// only misdescribe itself with `duration=` or classify nothing.
func TestMixin_Lifetime(t *testing.T) {
	t.Parallel()

	// build returns a reader answering an Entry whose Lifetime field
	// is the one a `lifetime=` names — the shape the issue reported.
	build := func(kv map[string]string) (*sdk.Method, *sdk.Package) {
		entry := &sdk.Struct{
			Name: "Entry", Package: "x",
			Fields: []*sdk.Field{
				{Name: "Body", Type: &sdk.TypeRef{Name: "string"}},
				{Name: "Lifetime", Type: &sdk.TypeRef{Name: "Duration", Package: "time"}},
			},
		}
		read := &sdk.Method{
			Name: "Read",
			Params: []*sdk.Param{
				{Name: "ctx", Type: &sdk.TypeRef{Name: "Context", Package: "context"}},
				{Name: "key", Type: &sdk.TypeRef{Name: "string"}},
			},
			Returns: sdk.AnonReturns(
				&sdk.TypeRef{Name: "Entry", Package: "x"},
				&sdk.TypeRef{Name: "error"},
			),
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{mixintest.HostDirective(ttl.Name, kv)},
			},
		}
		store := &sdk.Interface{Name: "Store", Package: "x", Methods: []*sdk.Method{read}}
		read.Owner = store
		return read, &sdk.Package{
			Name: "x", Path: "x",
			Interfaces: []*sdk.Interface{store},
			Structs:    []*sdk.Struct{entry},
			Variables:  []*sdk.Variable{{Name: "ErrExpired", Package: "x"}},
		}
	}

	t.Run("the lifetime resolves against the answered value's fields", func(t *testing.T) {
		t.Parallel()
		// KindValueField, so the stamp lands qualified — which is what
		// lets a consumer read the member without re-deriving which
		// type it belongs to.
		read, pkg := build(map[string]string{
			ttl.ParamRead:     "Read",
			ttl.ParamLifetime: "Lifetime",
			ttl.ParamNotFound: "ErrExpired",
		})
		mixintest.RunWithResolver(t, ttl.Mixin(), pkg)

		got, _ := shape.MixinParamKey(ttl.Name, ttl.ParamLifetime).Get(read.Meta())
		if got != "x.Entry.Lifetime" {
			t.Fatalf("lifetime = %q, want x.Entry.Lifetime", got)
		}
	})

	t.Run("a lifetime and a duration together are refused", func(t *testing.T) {
		t.Parallel()
		// They answer the same question differently, so a directive
		// carrying both leaves a consumer to pick and makes the loser
		// a line the author wrote that does nothing.
		_, pkg := build(map[string]string{
			ttl.ParamRead:     "Read",
			ttl.ParamDuration: "1m",
			ttl.ParamLifetime: "Lifetime",
		})
		diags := mixintest.RunWithValidator(t, ttl.Mixin(), pkg)

		var found bool
		for _, d := range diags {
			if d.Severity == sdk.SeverityError && strings.Contains(d.Message, "not both") {
				found = true
			}
		}
		if !found {
			t.Fatalf("both keys were accepted; diagnostics = %+v", diags)
		}
	})

	t.Run("neither key is required", func(t *testing.T) {
		t.Parallel()
		// A bare ttl still classifies the pair as expiring, which is a
		// fact a reader wants even where no check can hold a call to a
		// clock.
		_, pkg := build(map[string]string{ttl.ParamRead: "Read"})
		for _, d := range mixintest.RunWithValidator(t, ttl.Mixin(), pkg) {
			if d.Severity == sdk.SeverityError {
				t.Fatalf("bare ttl was refused: %s", d.Message)
			}
		}
	})
}
