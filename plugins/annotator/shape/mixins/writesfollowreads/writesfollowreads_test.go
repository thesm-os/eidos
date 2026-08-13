// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package writesfollowreads_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/writesfollowreads"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, writesfollowreads.Mixin(), writesfollowreads.Name, writesfollowreads.Params)
}

// TestMixin_VersionIsOpaque pins the param's kind, which the identity
// assertion cannot: it compares Mixin().Params against this package's
// own Params, so it proves the wiring and not the contents.
//
// The kind is behavioural. A version names a field or a zero-argument
// method of the value type, so declaring it resolvable would send the
// resolver looking for a callable of that name and report every
// correct directive as unresolved.
func TestMixin_VersionIsOpaque(t *testing.T) {
	t.Parallel()

	host := &sdk.Function{
		Name: "Get", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				mixintest.HostDirective(writesfollowreads.Name, map[string]string{
					writesfollowreads.ParamVersion: "Version",
				}),
			},
		},
	}
	diags := mixintest.RunWithValidator(t, writesfollowreads.Mixin(), &sdk.Package{
		Name: "x", Path: "x", Functions: []*sdk.Function{host},
	})
	for _, d := range diags {
		if d.Severity == sdk.SeverityError {
			t.Fatalf("unexpected diagnostic: %s", d.Message)
		}
	}

	got, _ := shape.MixinParamKey(writesfollowreads.Name, writesfollowreads.ParamVersion).Get(host.Meta())
	if got != "Version" {
		t.Fatalf("version = %q, want it left verbatim", got)
	}
}
