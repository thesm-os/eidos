// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package notfound_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/notfound"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, notfound.Mixin(), notfound.Name, notfound.Params)
	})
}

// build assembles a reader-shaped method host, optionally declaring
// the sentinel. The fixture mirrors the motivating corpus shape: a
// Get on an interface, with the sentinel a package-level var.
func build(sentinel string) (*sdk.Method, *sdk.Package) {
	kv := map[string]string{}
	if sentinel != "" {
		kv[notfound.ParamSentinel] = sentinel
	}
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
				mixintest.HostDirective(notfound.Name, kv),
			},
		},
	}
	reader := &sdk.Interface{Name: "Reader", Package: "x", Methods: []*sdk.Method{get}}
	get.Owner = reader
	return get, &sdk.Package{
		Name: "x", Path: "x",
		Interfaces: []*sdk.Interface{reader},
		Variables:  []*sdk.Variable{{Name: "ErrNotFound", Package: "x"}},
	}
}

// TestMixin_Sentinel covers the one param: a var, resolved through
// the package rather than the receiver, and required.
func TestMixin_Sentinel(t *testing.T) {
	t.Parallel()

	t.Run("the sentinel resolves against the package", func(t *testing.T) {
		t.Parallel()
		get, pkg := build("ErrNotFound")
		diags := mixintest.RunWithValidator(t, notfound.Mixin(), pkg)
		for _, d := range diags {
			if d.Severity == sdk.SeverityError {
				t.Fatalf("unexpected error diagnostic: %s", d.Message)
			}
		}

		got, _ := shape.MixinParamKey(notfound.Name, notfound.ParamSentinel).Get(get.Meta())
		if got != "x.ErrNotFound" {
			t.Fatalf("sentinel = %q, want %q", got, "x.ErrNotFound")
		}
	})

	t.Run("a bare attachment is reported", func(t *testing.T) {
		t.Parallel()
		// The sentinel is the mixin's entire content. A bare stamp can
		// only lower to "some error came back", which passes against
		// every implementation — the unfalsifiable check this
		// vocabulary keeps refusing to generate.
		_, pkg := build("")
		diags := mixintest.RunWithValidator(t, notfound.Mixin(), pkg)
		found := false
		for _, d := range diags {
			if d.Severity == sdk.SeverityError {
				found = true
			}
		}
		if !found {
			t.Fatalf("no error diagnostic for a sentinel-less notfound; got %d diags: %+v",
				len(diags), diags)
		}
	})

	t.Run("a sentinel naming no var is reported", func(t *testing.T) {
		t.Parallel()
		get, pkg := build("ErrMissing")
		diags := mixintest.RunWithValidator(t, notfound.Mixin(), pkg)
		found := false
		for _, d := range diags {
			if d.Severity == sdk.SeverityError {
				found = true
			}
		}
		if !found {
			t.Fatal("no error diagnostic for a sentinel naming no package var")
		}
		if got, _ := shape.MixinParamKey(notfound.Name, notfound.ParamSentinel).Get(get.Meta()); got != "ErrMissing" {
			t.Fatalf("sentinel = %q, want the raw name left unrewritten", got)
		}
	})
}
