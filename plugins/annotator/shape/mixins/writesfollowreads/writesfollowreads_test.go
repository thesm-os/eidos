// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package writesfollowreads_test

import (
	"strings"
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

// TestMixin_VersionResolvesAgainstTheValue pins the param's kind and
// what it buys, which the identity assertion cannot: it compares
// Mixin().Params against this package's own Params, so it proves the
// wiring and not the contents.
//
// [shape.KindValueField]: the resolver checks the name against the
// answered value's fields and rewrites a hit into the qualified form,
// so a typo is reported where the author is rather than in whatever a
// consumer derives from the stamp.
func TestMixin_VersionResolvesAgainstTheValue(t *testing.T) {
	t.Parallel()

	build := func(field string) (*sdk.Function, *sdk.Package) {
		host := &sdk.Function{
			Name: "Get", Package: "x",
			Returns: sdk.AnonReturns(&sdk.TypeRef{Name: "Value", Package: "x"}),
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					mixintest.HostDirective(writesfollowreads.Name, map[string]string{
						writesfollowreads.ParamVersion: field,
					}),
				},
			},
		}
		value := &sdk.Struct{
			Name: "Value", Package: "x",
			Fields: []*sdk.Field{{Name: "Version", Type: &sdk.TypeRef{Name: "int"}}},
		}
		return host, &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{host}, Structs: []*sdk.Struct{value},
		}
	}

	t.Run("a declared field rewrites qualified", func(t *testing.T) {
		t.Parallel()
		host, pkg := build("Version")
		for _, d := range mixintest.RunWithValidator(t, writesfollowreads.Mixin(), pkg) {
			if d.Severity == sdk.SeverityError {
				t.Fatalf("unexpected diagnostic: %s", d.Message)
			}
		}
		got, _ := shape.MixinParamKey(writesfollowreads.Name, writesfollowreads.ParamVersion).Get(host.Meta())
		if got != "x.Value.Version" {
			t.Fatalf("version = %q, want the qualified field", got)
		}
	})

	t.Run("a field the value does not declare is reported", func(t *testing.T) {
		t.Parallel()
		// The failure this key sat opaque over: a directive that
		// stamps clean and produces a claim about nothing.
		_, pkg := build("Missing")
		diags := mixintest.RunWithValidator(t, writesfollowreads.Mixin(), pkg)
		if len(diags) != 1 || !strings.Contains(diags[0].Message, "names no field of its value type") {
			t.Fatalf("diagnostics = %+v, want the typo reported where the author is", diags)
		}
	})
}
