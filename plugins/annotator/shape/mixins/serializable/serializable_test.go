// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package serializable_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/serializable"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/snapshotisolation"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, serializable.Mixin(), serializable.Name, serializable.Params)
}

// TestMixin_DistinctFromSnapshotIsolation pins the two as separate
// claims.
//
// Snapshot isolation permits write skew and serializability does not,
// so a checker selected from the wrong one reddens a correct store. A
// single name carrying both — or a level param on one — would make
// that mistake unavoidable rather than merely possible.
func TestMixin_DistinctFromSnapshotIsolation(t *testing.T) {
	t.Parallel()
	if serializable.Name == snapshotisolation.Name {
		t.Fatal("the two isolation claims share a name")
	}
	// A level knob specifically, not any param: an observer names
	// what a check looks through and leaves the model alone, while a
	// level would let one name carry both and reintroduce exactly the
	// mistake two names exist to prevent.
	for _, param := range snapshotisolation.Mixin().Params {
		switch param.Key {
		case "level", "mode", "isolation":
			t.Errorf("snapshotisolation grew a %q param; a level knob makes its"+
				" name contradict its own documentation", param.Key)
		}
	}
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
				mixintest.HostDirective(serializable.Name, map[string]string{serializable.ParamRead: "Get"}),
			},
		},
	}
	fns := []*sdk.Function{
		host,
		{Name: "Get", Package: "x"},
	}
	mixintest.RunWithResolver(t, serializable.Mixin(), &sdk.Package{
		Name: "x", Path: "x", Functions: fns,
	})
	if got, _ := shape.MixinParamKey(serializable.Name, serializable.ParamRead).Get(host.Meta()); got != "x.Get" {
		t.Errorf("read = %q, want x.Get", got)
	}
}
