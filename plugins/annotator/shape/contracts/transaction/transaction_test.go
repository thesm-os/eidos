// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package transaction_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/transaction"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t,
		transaction.Contract(),
		transaction.Name, transaction.Roles)
}

// TestContract_PipelineRoundTrip exercises the umbrella →
// resolver → validator sequence for a real `+gen:contract
// transaction` directive — proving the package's [Contract]
// value plugs into the framework correctly.
func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	fn := &sdk.Function{
		Name: "Charge", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(transaction.Name, "fn", nil),
			},
		},
	}
	pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{fn}}
	diags := contracttest.RunPipeline(t, transaction.Contract(), pkg)

	contracttest.AssertRole(t, fn.Meta(), transaction.Name, "fn")
	contracttest.AssertNoErrorDiag(t, diags)
}

// TestContract_Partners covers the pair that reaches the state
// notfound= is about.
//
// Neither is derivable: the scope's whole signature is the body it
// runs, so a walk for "the establishing write" finds every
// error-answering sibling equally qualified — and guessing aims the
// read at state the body never touched, where a correct subject and
// a broken one report the same nothing.
func TestContract_Partners(t *testing.T) {
	t.Parallel()

	build := func(kv map[string]string) (*sdk.Method, *sdk.Package) {
		run := &sdk.Method{
			Name: "Run",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(transaction.Name, transaction.RoleFn, kv),
				},
			},
		}
		put := &sdk.Method{Name: "Put"}
		get := &sdk.Method{Name: "Get"}
		store := &sdk.Interface{
			Name: "Store", Package: "x",
			Methods: []*sdk.Method{run, put, get},
		}
		run.Owner, put.Owner, get.Owner = store, store, store
		return run, &sdk.Package{
			Name: "x", Path: "x",
			Interfaces: []*sdk.Interface{store},
			Variables:  []*sdk.Variable{{Name: "ErrNotFound", Package: "x"}},
		}
	}

	t.Run("both partners resolve through the host's scope", func(t *testing.T) {
		t.Parallel()
		// KindCallable on a method host: the owner's method set, not
		// the package's functions — the same scope atomic's read=
		// resolves through.
		run, pkg := build(map[string]string{
			transaction.ParamWrite:    "Put",
			transaction.ParamRead:     "Get",
			transaction.ParamNotFound: "ErrNotFound",
		})
		diags := contracttest.RunPipeline(t, transaction.Contract(), pkg)
		contracttest.AssertNoErrorDiag(t, diags)

		for key, want := range map[string]string{
			transaction.ParamWrite:    "x.Store.Put",
			transaction.ParamRead:     "x.Store.Get",
			transaction.ParamNotFound: "x.ErrNotFound",
		} {
			got, _ := shape.ContractParamKey(transaction.Name, key).Get(run.Meta())
			if got != want {
				t.Errorf("%s = %q, want %q", key, got, want)
			}
		}
	})

	t.Run("a partner naming nothing in scope is reported", func(t *testing.T) {
		t.Parallel()
		// The diagnostic an opaque key could never produce: a typo
		// caught where the author is, rather than a check driving a
		// sibling that does not exist.
		_, pkg := build(map[string]string{transaction.ParamWrite: "Absent"})
		diags := contracttest.RunPipeline(t, transaction.Contract(), pkg)
		contracttest.AssertContainsDiag(t, diags, sdk.SeverityError, "names no callable in scope")
	})

	t.Run("the bare form still classifies", func(t *testing.T) {
		t.Parallel()
		// All three optional: a consumer that cannot state the law
		// without them declines to state it.
		_, pkg := build(map[string]string{})
		contracttest.AssertNoErrorDiag(t, contracttest.RunPipeline(t, transaction.Contract(), pkg))
	})
}
