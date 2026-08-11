// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package ifmatch_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/ifmatch"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		contracttest.AssertIdentity(t, ifmatch.Contract(), ifmatch.Name, ifmatch.Roles)
	})

	t.Run("pipeline stamps pred param", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Update", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(ifmatch.Name, "writer", map[string]string{
						"pred": "Version==Expected",
					}),
				},
			},
		}
		pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{fn}}
		diags := contracttest.RunPipeline(t, ifmatch.Contract(), pkg)
		contracttest.AssertNoErrorDiag(t, diags)

		got, _ := shape.ContractParamKey(ifmatch.Name, "pred").Get(fn.Meta())
		if got != "Version==Expected" {
			t.Fatalf("pred = %q, want %q", got, "Version==Expected")
		}
	})
}

// TestContract_MatchRole covers the callable spelling of the
// predicate.
//
// The param form is opaque by design, so a consumer naming a method
// through it got no qualification, no diagnostic when the method did
// not exist, and nothing on the predicate to find the writer from.
// The role gets all three, which is what the resolver is for.
func TestContract_MatchRole(t *testing.T) {
	t.Parallel()

	t.Run("the match partner is qualified and back-stamped", func(t *testing.T) {
		t.Parallel()
		write := &sdk.Function{
			Name: "Update", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(ifmatch.Name, ifmatch.RoleWriter,
						map[string]string{ifmatch.RoleMatch: "Match"}),
				},
			},
		}
		match := &sdk.Function{Name: "Match", Package: "x"}
		diags := contracttest.RunPipeline(t, ifmatch.Contract(), &sdk.Package{
			Name: "x", Path: "x", Functions: []*sdk.Function{write, match},
		})
		contracttest.AssertNoErrorDiag(t, diags)

		contracttest.AssertPartner(t, write.Meta(), ifmatch.Name, ifmatch.RoleMatch, "x.Match")
		contracttest.AssertRole(t, match.Meta(), ifmatch.Name, ifmatch.RoleMatch)
		contracttest.AssertPartner(t, match.Meta(), ifmatch.Name, ifmatch.RoleWriter, "x.Update")
	})

	t.Run("a match naming nothing in scope is reported", func(t *testing.T) {
		t.Parallel()
		// The diagnostic the param form could never produce: as a Param
		// a missing predicate stamps exactly like a present one.
		write := &sdk.Function{
			Name: "Update", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(ifmatch.Name, ifmatch.RoleWriter,
						map[string]string{ifmatch.RoleMatch: "Absent"}),
				},
			},
		}
		diags := contracttest.RunPipeline(t, ifmatch.Contract(), &sdk.Package{
			Name: "x", Path: "x", Functions: []*sdk.Function{write},
		})
		contracttest.AssertContainsDiag(t, diags, sdk.SeverityError, "not found in scope")
	})

	t.Run("the expression form is still left verbatim", func(t *testing.T) {
		t.Parallel()
		// The reason the two are separate keys: an expression must not
		// be handed to the resolver, which would report every correct
		// one as unresolved.
		write := &sdk.Function{
			Name: "Update", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(ifmatch.Name, ifmatch.RoleWriter,
						map[string]string{ifmatch.ParamPred: "Version==Expected"}),
				},
			},
		}
		diags := contracttest.RunPipeline(t, ifmatch.Contract(), &sdk.Package{
			Name: "x", Path: "x", Functions: []*sdk.Function{write},
		})
		contracttest.AssertNoErrorDiag(t, diags)

		got, _ := shape.ContractParamKey(ifmatch.Name, ifmatch.ParamPred).Get(write.Meta())
		if got != "Version==Expected" {
			t.Fatalf("pred = %q, want it untouched", got)
		}
	})
}
