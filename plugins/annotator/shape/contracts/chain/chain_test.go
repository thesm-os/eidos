// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package chain_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/chain"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/sdk"
)

func appendWith(partners map[string]string) *sdk.Function {
	return &sdk.Function{
		Name: "Append", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(chain.Name, chain.RoleAppend, partners),
			},
		},
	}
}

func TestContract(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		contracttest.AssertIdentity(t, chain.Contract(), chain.Name, chain.Roles)
	})

	t.Run("the replay is qualified and back-stamped", func(t *testing.T) {
		t.Parallel()
		app := appendWith(map[string]string{chain.RoleReplay: "Replay"})
		replay := &sdk.Function{Name: "Replay", Package: "x"}
		diags := contracttest.RunPipeline(t, chain.Contract(), &sdk.Package{
			Name: "x", Path: "x", Functions: []*sdk.Function{app, replay},
		})
		contracttest.AssertNoErrorDiag(t, diags)

		contracttest.AssertPartner(t, app.Meta(), chain.Name, chain.RoleReplay, "x.Replay")
		contracttest.AssertRole(t, replay.Meta(), chain.Name, chain.RoleReplay)
		contracttest.AssertPartner(t, replay.Meta(), chain.Name, chain.RoleAppend, "x.Append")
	})

	t.Run("an append with no replay is reported", func(t *testing.T) {
		t.Parallel()
		// Append-only is a claim about history, so without a way to read
		// history every property of this contract is unobservable.
		diags := contracttest.RunPipeline(t, chain.Contract(), &sdk.Package{
			Name: "x", Path: "x", Functions: []*sdk.Function{appendWith(map[string]string{})},
		})
		contracttest.AssertContainsDiag(t, diags, sdk.SeverityError, chain.RoleReplay)
	})

	t.Run("verify is optional", func(t *testing.T) {
		t.Parallel()
		// A chain reporting corruption through a poison accessor is
		// checkable without one, and requiring it would rule out the
		// commoner Go spelling.
		app := appendWith(map[string]string{chain.RoleReplay: "Replay"})
		replay := &sdk.Function{Name: "Replay", Package: "x"}
		diags := contracttest.RunPipeline(t, chain.Contract(), &sdk.Package{
			Name: "x", Path: "x", Functions: []*sdk.Function{app, replay},
		})
		contracttest.AssertNoErrorDiag(t, diags)
	})

	t.Run("verify resolves when declared", func(t *testing.T) {
		t.Parallel()
		app := appendWith(map[string]string{
			chain.RoleReplay: "Replay",
			chain.RoleVerify: "Verify",
		})
		replay := &sdk.Function{Name: "Replay", Package: "x"}
		verify := &sdk.Function{Name: "Verify", Package: "x"}
		diags := contracttest.RunPipeline(t, chain.Contract(), &sdk.Package{
			Name: "x", Path: "x", Functions: []*sdk.Function{app, replay, verify},
		})
		contracttest.AssertNoErrorDiag(t, diags)
		contracttest.AssertPartner(t, app.Meta(), chain.Name, chain.RoleVerify, "x.Verify")
	})
}
