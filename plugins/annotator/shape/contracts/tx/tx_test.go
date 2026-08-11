// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package tx_test

import (
	"maps"
	"reflect"
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
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

// TestContract_ClosedSentinel covers sentinel resolution.
//
// The law this contract selects compares against the error a finished
// transaction reports, so a binding with no sentinel leaves the field
// nil and errors.Is(err, nil) is false for every error a correct
// implementation returns — the law then fails every subject including
// the right ones. Naming it is what makes the law statable.
func TestContract_ClosedSentinel(t *testing.T) {
	t.Parallel()

	beginWith := func(partners map[string]string) *sdk.Function {
		return &sdk.Function{
			Name: "Begin", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(tx.Name, "begin", partners),
				},
			},
		}
	}
	pkgWith := func(host *sdk.Function, vars ...*sdk.Variable) *sdk.Package {
		return &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{
				host,
				{Name: "Commit", Package: "x"},
				{Name: "Rollback", Package: "x"},
			},
			Variables: vars,
		}
	}
	partners := func(extra map[string]string) map[string]string {
		out := map[string]string{"commit": "Commit", "rollback": "Rollback"}
		maps.Copy(out, extra)
		return out
	}

	t.Run("a declared sentinel is qualified", func(t *testing.T) {
		t.Parallel()
		host := beginWith(partners(map[string]string{tx.ParamClosed: "ErrTxClosed"}))
		diags := contracttest.RunPipeline(t, tx.Contract(),
			pkgWith(host, &sdk.Variable{Name: "ErrTxClosed", Package: "x"}))
		contracttest.AssertNoErrorDiag(t, diags)

		got, _ := shape.ContractParamKey(tx.Name, tx.ParamClosed).Get(host.Meta())
		if got != "x.ErrTxClosed" {
			t.Fatalf("closed = %q, want %q", got, "x.ErrTxClosed")
		}
	})

	t.Run("a sentinel naming no var is reported", func(t *testing.T) {
		t.Parallel()
		// The diagnostic the issue asks for, in place of a generator
		// qualifying a bare identifier by guessing.
		host := beginWith(partners(map[string]string{tx.ParamClosed: "ErrAbsent"}))
		diags := contracttest.RunPipeline(t, tx.Contract(), pkgWith(host))
		contracttest.AssertContainsDiag(t, diags, sdk.SeverityError, "names no package-level var")
	})

	t.Run("a sentinel is not resolved as a callable", func(t *testing.T) {
		t.Parallel()
		// Why SiblingVars is its own declaration: a function of the
		// same name must not satisfy it, or the resolver stops being
		// able to say what it expected.
		host := beginWith(partners(map[string]string{tx.ParamClosed: "ErrTxClosed"}))
		pkg := pkgWith(host)
		pkg.Functions = append(pkg.Functions, &sdk.Function{Name: "ErrTxClosed", Package: "x"})
		diags := contracttest.RunPipeline(t, tx.Contract(), pkg)
		contracttest.AssertContainsDiag(t, diags, sdk.SeverityError, "names no package-level var")
	})

	t.Run("an absent sentinel is not an error", func(t *testing.T) {
		t.Parallel()
		host := beginWith(partners(nil))
		diags := contracttest.RunPipeline(t, tx.Contract(), pkgWith(host))
		contracttest.AssertNoErrorDiag(t, diags)
	})
}
