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
		for _, p := range partition.Params {
			if p.Key == partition.ParamAxis && p.Kind != shape.KindOpaque {
				t.Errorf("%q resolves as %s; a parameter of the callable has no"+
					" qualified name and every correct axis would report missing",
					partition.ParamAxis, p.Kind)
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

// ifacePair builds the corpus shape: a writer and a reader declared on
// one interface, so the validator can reach the partner through the
// host's Owner.
func ifacePair(t *testing.T, axis string, readParams []string) []sdk.Diag {
	t.Helper()
	params := func(names ...string) []*sdk.Param {
		out := make([]*sdk.Param, 0, len(names))
		for _, n := range names {
			out = append(out, &sdk.Param{Name: n, Type: &sdk.TypeRef{Name: "string"}})
		}
		return out
	}
	put := &sdk.Method{
		Name: "Put", Params: params("partition", "key", "value"),
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				mixintest.HostDirective(partition.Name, map[string]string{
					partition.ParamRead: "Read",
					partition.ParamAxis: axis,
				}),
			},
		},
	}
	read := &sdk.Method{Name: "Read", Params: params(readParams...)}
	store := &sdk.Interface{Name: "Store", Package: "x", Methods: []*sdk.Method{put, read}}
	put.Owner, read.Owner = store, store

	return mixintest.RunWithValidator(t, partition.Mixin(), &sdk.Package{
		Name: "x", Path: "x", Interfaces: []*sdk.Interface{store},
	})
}

// TestMixin_AxisCorrespondence covers the half that makes the axis
// usable rather than merely present.
//
// A check writes through the host and reads through the partner, so it
// carries one partition value across both calls. The axis names a
// parameter of the host only, so a generator matching the two by name
// is guessing — unless the pair is checked, which is what this does.
func TestMixin_AxisCorrespondence(t *testing.T) {
	t.Parallel()

	t.Run("an axis both halves declare raises nothing", func(t *testing.T) {
		t.Parallel()
		got := ifacePair(t, "partition", []string{"partition", "key"})
		if len(got) != 0 {
			t.Fatalf("diagnostics = %+v, want none", got)
		}
	})

	t.Run("an axis the reader does not declare is reported", func(t *testing.T) {
		t.Parallel()
		// Read(ctx, tenant, key) cannot be handed the host's partition
		// by name, so the pair does not compose and a generator has
		// nothing sound to emit.
		got := ifacePair(t, "partition", []string{"tenant", "key"})
		if len(got) != 1 {
			t.Fatalf("diagnostics = %+v, want one", got)
		}
		if !strings.Contains(got[0].Message, "Read") {
			t.Errorf("message = %q, want it to name the read partner", got[0].Message)
		}
	})

	t.Run("a host-side miss is reported once, not twice", func(t *testing.T) {
		t.Parallel()
		// The pair check is unreachable when the axis is not on the
		// host at all; reporting both would name the reader for a
		// mistake made on the writer.
		got := ifacePair(t, "shard", []string{"partition", "key"})
		if len(got) != 1 {
			t.Fatalf("diagnostics = %+v, want one", got)
		}
		if !strings.Contains(got[0].Message, "annotated callable") {
			t.Errorf("message = %q, want the host-side diagnostic", got[0].Message)
		}
	})
}

// TestMixin_UnresolvableRead pins the no-double-report rule.
//
// A read naming nothing in scope is already the resolver's
// diagnostic, and the stamp stays a bare name — so the partner cannot
// be reached and the axis cannot be checked against it. Adding a
// second violation there would report the same mistake twice and
// blame the axis for the read's error.
func TestMixin_UnresolvableRead(t *testing.T) {
	t.Parallel()

	put := &sdk.Method{
		Name: "Put",
		Params: []*sdk.Param{
			{Name: "partition", Type: &sdk.TypeRef{Name: "string"}},
			{Name: "key", Type: &sdk.TypeRef{Name: "string"}},
		},
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				mixintest.HostDirective(partition.Name, map[string]string{
					partition.ParamRead: "Absent",
					partition.ParamAxis: "partition",
				}),
			},
		},
	}
	store := &sdk.Interface{Name: "Store", Package: "x", Methods: []*sdk.Method{put}}
	put.Owner = store

	got := mixintest.RunWithValidator(t, partition.Mixin(), &sdk.Package{
		Name: "x", Path: "x", Interfaces: []*sdk.Interface{store},
	})
	if len(got) != 1 {
		t.Fatalf("diagnostics = %+v, want only the resolver's", got)
	}
	if !strings.Contains(got[0].Message, "not found in scope") {
		t.Errorf("message = %q, want the resolver's sibling diagnostic", got[0].Message)
	}
}
