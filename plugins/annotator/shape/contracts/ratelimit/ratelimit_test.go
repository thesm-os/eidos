// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package ratelimit_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/ratelimit"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		contracttest.AssertIdentity(t, ratelimit.Contract(), ratelimit.Name, ratelimit.Roles)
	})

	t.Run("pipeline stamps rate and burst params", func(t *testing.T) {
		t.Parallel()
		fn := &sdk.Function{
			Name: "Call", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(ratelimit.Name, "fn", map[string]string{
						"rate":  "100",
						"burst": "10",
					}),
				},
			},
		}
		pkg := &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{fn}}
		diags := contracttest.RunPipeline(t, ratelimit.Contract(), pkg)
		contracttest.AssertNoErrorDiag(t, diags)

		if got, _ := shape.ContractParamKey(ratelimit.Name, "rate").Get(fn.Meta()); got != "100" {
			t.Fatalf("rate = %q, want %q", got, "100")
		}
		if got, _ := shape.ContractParamKey(ratelimit.Name, "burst").Get(fn.Meta()); got != "10" {
			t.Fatalf("burst = %q, want %q", got, "10")
		}
	})
}

// TestContract_Limited covers the key that makes the burst statable:
// without a named refusal, "the N+1st call is not admitted" is
// satisfied by any error at all.
func TestContract_Limited(t *testing.T) {
	t.Parallel()

	build := func(kv map[string]string) (*sdk.Function, *sdk.Package) {
		fn := &sdk.Function{
			Name: "Charge", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(ratelimit.Name, ratelimit.RoleFn, kv),
				},
			},
		}
		return fn, &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{fn},
			Variables: []*sdk.Variable{{Name: "ErrRateLimited", Package: "x"}},
		}
	}

	t.Run("the sentinel resolves through the package's vars", func(t *testing.T) {
		t.Parallel()
		// KindVar: a sentinel is not declared on the receiver, so the
		// scope is the package's — and the stamp lands qualified,
		// which is what a check needs to compare against.
		fn, pkg := build(map[string]string{
			ratelimit.ParamBurst:   "10",
			ratelimit.ParamLimited: "ErrRateLimited",
		})
		contracttest.AssertNoErrorDiag(t, contracttest.RunPipeline(t, ratelimit.Contract(), pkg))

		got, _ := shape.ContractParamKey(ratelimit.Name, ratelimit.ParamLimited).Get(fn.Meta())
		if got != "x.ErrRateLimited" {
			t.Fatalf("limited = %q, want x.ErrRateLimited", got)
		}
	})

	t.Run("the quantities stay verbatim beside it", func(t *testing.T) {
		t.Parallel()
		// rate= and burst= are opaque by design: reading one as a
		// sentinel would be parsing a number into an identifier.
		fn, pkg := build(map[string]string{
			ratelimit.ParamRate:    "100",
			ratelimit.ParamLimited: "ErrRateLimited",
		})
		contracttest.RunPipeline(t, ratelimit.Contract(), pkg)
		if got, _ := shape.ContractParamKey(ratelimit.Name, ratelimit.ParamRate).Get(fn.Meta()); got != "100" {
			t.Fatalf("rate = %q, want it untouched", got)
		}
	})

	t.Run("the bare form still classifies", func(t *testing.T) {
		t.Parallel()
		_, pkg := build(map[string]string{ratelimit.ParamBurst: "10"})
		contracttest.AssertNoErrorDiag(t, contracttest.RunPipeline(t, ratelimit.Contract(), pkg))
	})
}
