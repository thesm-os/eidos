// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package partition_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/partition"
	"go.thesmos.sh/eidos/sdk"
)

// put builds the callable the corpus annotates: two strings of the
// same type, either of which could be the partition, which is why the
// axis is not derivable from the signature.
func put(params map[string]string) *sdk.Function {
	return &sdk.Function{
		Name: "Put", Package: "x",
		Params: []*sdk.Param{
			{Name: "partition", Type: &sdk.TypeRef{Name: "string"}},
			{Name: "key", Type: &sdk.TypeRef{Name: "string"}},
			{Name: "value", Type: &sdk.TypeRef{Name: "string"}},
		},
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				mixintest.HostDirective(partition.Name, params),
			},
		},
	}
}

func pkgWith(host *sdk.Function) *sdk.Package {
	return &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{host, {Name: "Read", Package: "x"}},
	}
}

func TestMixin(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		mixintest.AssertIdentity(t, partition.Mixin(), partition.Name, partition.Params)
	})

	t.Run("the axis is not a sibling param", func(t *testing.T) {
		t.Parallel()
		// A parameter has no qualified name, so asking the resolver to
		// look one up in scope would report every correct axis as not
		// found.
		for _, p := range partition.SiblingParams {
			if p == partition.ParamAxis {
				t.Errorf("%q is declared as a sibling param", partition.ParamAxis)
			}
		}
	})

	t.Run("resolver rewrites the read param to a qualified name", func(t *testing.T) {
		t.Parallel()
		host := put(map[string]string{partition.ParamRead: "Read"})
		mixintest.RunWithResolver(t, partition.Mixin(), pkgWith(host))

		got, _ := shape.MixinParamKey(partition.Name, partition.ParamRead).Get(host.Meta())
		if got != "x.Read" {
			t.Fatalf("read param = %q, want %q", got, "x.Read")
		}
	})

	t.Run("the axis is stamped verbatim, not qualified", func(t *testing.T) {
		t.Parallel()
		// It names a parameter of this callable, so a package
		// qualification would spell something that does not exist.
		host := put(map[string]string{
			partition.ParamRead: "Read",
			partition.ParamAxis: "partition",
		})
		mixintest.RunWithResolver(t, partition.Mixin(), pkgWith(host))

		got, _ := shape.MixinParamKey(partition.Name, partition.ParamAxis).Get(host.Meta())
		if got != "partition" {
			t.Fatalf("axis param = %q, want it left verbatim", got)
		}
	})
}

// TestMixin_AxisValidation covers the half the resolver cannot do.
//
// A misspelled axis stamps like any other value, and a generator
// trusting it varies nothing — producing a check that writes two
// distinct keys and passes against an implementation ignoring
// partitions entirely. That check cannot fail, which is worse than
// generating none.
func TestMixin_AxisValidation(t *testing.T) {
	t.Parallel()

	t.Run("an axis naming a real parameter raises nothing", func(t *testing.T) {
		t.Parallel()
		host := put(map[string]string{
			partition.ParamRead: "Read",
			partition.ParamAxis: "partition",
		})
		if got := mixintest.RunWithValidator(t, partition.Mixin(), pkgWith(host)); len(got) != 0 {
			t.Fatalf("diagnostics = %+v, want none", got)
		}
	})

	t.Run("an axis naming no parameter is reported", func(t *testing.T) {
		t.Parallel()
		host := put(map[string]string{
			partition.ParamRead: "Read",
			partition.ParamAxis: "tenant",
		})
		got := mixintest.RunWithValidator(t, partition.Mixin(), pkgWith(host))
		if len(got) != 1 {
			t.Fatalf("diagnostics = %+v, want one", got)
		}
		if !strings.Contains(got[0].Message, "tenant") {
			t.Errorf("message = %q, want it to name the offending axis", got[0].Message)
		}
	})

	t.Run("an absent axis is not a violation", func(t *testing.T) {
		t.Parallel()
		// The bare form stays a classification. Erroring here would
		// break every directive the package documents, and whether a
		// check is worth emitting belongs to the generator.
		host := put(map[string]string{partition.ParamRead: "Read"})
		if got := mixintest.RunWithValidator(t, partition.Mixin(), pkgWith(host)); len(got) != 0 {
			t.Fatalf("diagnostics = %+v, want none", got)
		}
	})
}
