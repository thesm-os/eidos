// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package batchwriter_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/batchwriter"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		contracttest.AssertIdentity(t, batchwriter.Contract(), batchwriter.Name, batchwriter.Roles)
	})

	t.Run("pipeline stamps mode param", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "WriteBatch", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(batchwriter.Name, "writer", map[string]string{
						"mode": "all-or-nothing",
					}),
				},
			},
		}
		pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{fn}}
		diags := contracttest.RunPipeline(t, batchwriter.Contract(), pkg)
		contracttest.AssertNoErrorDiag(t, diags)

		contracttest.AssertRole(t, fn.Meta(), batchwriter.Name, "writer")
		got, _ := shape.ContractParamKey(batchwriter.Name, "mode").Get(fn.Meta())
		if got != "all-or-nothing" {
			t.Fatalf("mode param = %q, want %q", got, "all-or-nothing")
		}
	})
}

// TestContract_ReaderRole covers the observation partner: optional,
// resolved and back-stamped like persister's, so a consumer holding
// the reader finds the batch-writer it confirms.
func TestContract_ReaderRole(t *testing.T) {
	t.Parallel()

	build := func(withReader bool) (*sdk.Function, *sdk.Function, *sdk.Package) {
		kv := map[string]string{"mode": "all-or-nothing"}
		if withReader {
			kv["reader"] = "Get"
		}
		write := &sdk.Function{
			Name: "WriteBatch", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(batchwriter.Name, "writer", kv),
				},
			},
		}
		get := &sdk.Function{Name: "Get", Package: "x"}
		pkg := &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{write, get},
		}
		return write, get, pkg
	}

	t.Run("the reader resolves and is back-stamped", func(t *testing.T) {
		t.Parallel()
		write, get, pkg := build(true)
		diags := contracttest.RunPipeline(t, batchwriter.Contract(), pkg)
		contracttest.AssertNoErrorDiag(t, diags)

		contracttest.AssertPartner(t, write.Meta(), batchwriter.Name, "reader", "x.Get")
		// The back-stamp is what a param could not do: walking
		// memberships from the reader's end reaches the contract.
		contracttest.AssertRole(t, get.Meta(), batchwriter.Name, "reader")
		contracttest.AssertPartner(t, get.Meta(), batchwriter.Name, "writer", "x.WriteBatch")
	})

	t.Run("a writer without a reader still classifies", func(t *testing.T) {
		t.Parallel()
		// Optional: the mode becomes a claim the consumer declines to
		// check, and a best-effort writer never needs the role.
		write, _, pkg := build(false)
		diags := contracttest.RunPipeline(t, batchwriter.Contract(), pkg)
		contracttest.AssertNoErrorDiag(t, diags)
		contracttest.AssertRole(t, write.Meta(), batchwriter.Name, "writer")

		got, ok := shape.ContractParamKey(batchwriter.Name, "mode").Get(write.Meta())
		if !ok || got != "all-or-nothing" {
			t.Fatalf("mode = %q (present=%v), want all-or-nothing", got, ok)
		}
	})
}

// TestContract_Refused covers the key that gives mode=atomic its
// first step: a value the writer turns down, which a derived draw —
// built to be accepted — can never be.
func TestContract_Refused(t *testing.T) {
	t.Parallel()

	build := func(role string, kv map[string]string) (*sdk.Function, *sdk.Package) {
		host := &sdk.Function{
			Name: "PutAll", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(batchwriter.Name, role, kv),
				},
			},
		}
		return host, &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{host},
			Variables: []*sdk.Variable{{Name: "BadEntry", Package: "x"}},
		}
	}

	t.Run("the refused value resolves through the package's vars", func(t *testing.T) {
		t.Parallel()
		host, pkg := build(batchwriter.RoleWriter,
			map[string]string{batchwriter.ParamRefused: "BadEntry"})
		diags := contracttest.RunPipeline(t, batchwriter.Contract(), pkg)
		contracttest.AssertNoErrorDiag(t, diags)

		got, _ := shape.ContractParamKey(batchwriter.Name, batchwriter.ParamRefused).Get(host.Meta())
		if got != "x.BadEntry" {
			t.Fatalf("refused = %q, want x.BadEntry", got)
		}
	})

	t.Run("the bare form still classifies", func(t *testing.T) {
		t.Parallel()
		// Optional: a consumer that cannot state the law without it
		// declines to state it, recorded rather than assumed.
		_, pkg := build(batchwriter.RoleWriter, map[string]string{})
		contracttest.AssertNoErrorDiag(t, contracttest.RunPipeline(t, batchwriter.Contract(), pkg))
	})
}
