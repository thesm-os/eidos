// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tx_test

import (
	"reflect"
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/tx"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t,
		tx.Contract(),
		tx.Name, tx.Roles)
}

func TestContract_RequiresCommitAndRollback(t *testing.T) {
	t.Parallel()
	got := tx.Contract().Required
	want := map[string][]string{"begin": {"commit", "rollback"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Required = %v, want %v", got, want)
	}
}

// TestContract_PipelineRoundTrip exercises the happy path of
// begin + commit + rollback through umbrella → resolver →
// validator.
func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	begin := &sdk.Function{
		Name: "Begin", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(tx.Name, "begin", map[string]string{
					"commit":   "Commit",
					"rollback": "Rollback",
				}),
			},
		},
	}
	commit := &sdk.Function{Name: "Commit", Package: "x"}
	rollback := &sdk.Function{Name: "Rollback", Package: "x"}
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{begin, commit, rollback},
	}
	diags := contracttest.RunPipeline(t, tx.Contract(), pkg)
	contracttest.AssertNoErrorDiag(t, diags)

	contracttest.AssertRole(t, begin.Meta(), tx.Name, "begin")
	contracttest.AssertPartner(t, begin.Meta(), tx.Name, "commit", "x.Commit")
	contracttest.AssertPartner(t, begin.Meta(), tx.Name, "rollback", "x.Rollback")
	contracttest.AssertRole(t, commit.Meta(), tx.Name, "commit")
	contracttest.AssertRole(t, rollback.Meta(), tx.Name, "rollback")
}

// TestContract_ValidatorFlagsMissingPartner exercises the
// Required check through the actual [shape.Validator]
// annotator — begin declares only commit, omitting the
// rollback partner, and the validator must emit a diagnostic
// naming the missing role.
func TestContract_ValidatorFlagsMissingPartner(t *testing.T) {
	t.Parallel()
	begin := &sdk.Function{
		Name: "Begin", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(tx.Name, "begin", map[string]string{
					"commit": "Commit",
				}),
			},
		},
	}
	commit := &sdk.Function{Name: "Commit", Package: "x"}
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{begin, commit},
	}
	diags := contracttest.RunPipeline(t, tx.Contract(), pkg)
	contracttest.AssertContainsDiag(t, diags, sdk.SeverityError, "rollback")
}
