// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package leaderelection_test

import (
	"reflect"
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/leaderelection"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t,
		leaderelection.Contract(),
		leaderelection.Name, leaderelection.Roles)
}

func TestContract_RequiresResignAndIsLeader(t *testing.T) {
	t.Parallel()
	got := leaderelection.Contract().Required
	want := map[string][]string{"campaign": {"resign", "isleader"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Required = %v, want %v", got, want)
	}
}

// TestContract_PipelineRoundTrip exercises the happy path of
// campaign + resign + isleader through umbrella → resolver →
// validator.
func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	campaign := &sdk.Function{
		Name: "Campaign", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(leaderelection.Name, "campaign", map[string]string{
					"resign":   "Resign",
					"isleader": "IsLeader",
				}),
			},
		},
	}
	resign := &sdk.Function{Name: "Resign", Package: "x"}
	isleader := &sdk.Function{Name: "IsLeader", Package: "x"}
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{campaign, resign, isleader},
	}
	diags := contracttest.RunPipeline(t, leaderelection.Contract(), pkg)
	contracttest.AssertNoErrorDiag(t, diags)

	contracttest.AssertRole(t, campaign.Meta(), leaderelection.Name, "campaign")
	contracttest.AssertPartner(t, campaign.Meta(), leaderelection.Name, "resign", "x.Resign")
	contracttest.AssertPartner(t, campaign.Meta(), leaderelection.Name, "isleader", "x.IsLeader")
	contracttest.AssertRole(t, resign.Meta(), leaderelection.Name, "resign")
	contracttest.AssertRole(t, isleader.Meta(), leaderelection.Name, "isleader")
}

// TestContract_ValidatorFlagsMissingPartner exercises the
// Required check through the actual [shape.Validator]
// annotator — campaign declares only resign, omitting
// isleader, and the validator must emit a diagnostic naming
// the missing role.
func TestContract_ValidatorFlagsMissingPartner(t *testing.T) {
	t.Parallel()
	campaign := &sdk.Function{
		Name: "Campaign", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(leaderelection.Name, "campaign", map[string]string{
					"resign": "Resign",
				}),
			},
		},
	}
	resign := &sdk.Function{Name: "Resign", Package: "x"}
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{campaign, resign},
	}
	diags := contracttest.RunPipeline(t, leaderelection.Contract(), pkg)
	contracttest.AssertContainsDiag(t, diags, sdk.SeverityError, "isleader")
}
