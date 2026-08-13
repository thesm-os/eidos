// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package ifabsent_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/ifabsent"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t, ifabsent.Contract(), ifabsent.Name, ifabsent.Roles)
}

// TestContract_ConflictSentinel pins the refusal's identity.
//
// The contract's whole claim is that a second write is refused, so
// without the sentinel a check asserts only that some error came back
// — and a store refusing every write passes it. That check cannot
// fail, which is worse than none.
func TestContract_ConflictSentinel(t *testing.T) {
	t.Parallel()

	host := func(partners map[string]string) *sdk.Function {
		return &sdk.Function{
			Name: "Create", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(ifabsent.Name, "writer", partners),
				},
			},
		}
	}

	t.Run("a declared sentinel is qualified", func(t *testing.T) {
		t.Parallel()
		fn := host(map[string]string{ifabsent.ParamConflict: "ErrExists"})
		diags := contracttest.RunPipeline(t, ifabsent.Contract(), &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{fn},
			Variables: []*sdk.Variable{{Name: "ErrExists", Package: "x"}},
		})
		contracttest.AssertNoErrorDiag(t, diags)

		got, _ := shape.ContractParamKey(ifabsent.Name, ifabsent.ParamConflict).Get(fn.Meta())
		if got != "x.ErrExists" {
			t.Fatalf("conflict = %q, want %q", got, "x.ErrExists")
		}
	})

	t.Run("a sentinel naming no var is reported", func(t *testing.T) {
		t.Parallel()
		fn := host(map[string]string{ifabsent.ParamConflict: "ErrAbsent"})
		diags := contracttest.RunPipeline(t, ifabsent.Contract(), &sdk.Package{
			Name: "x", Path: "x", Functions: []*sdk.Function{fn},
		})
		contracttest.AssertContainsDiag(t, diags, sdk.SeverityError, "names no package-level var")
	})

	t.Run("an absent sentinel is not an error", func(t *testing.T) {
		t.Parallel()
		diags := contracttest.RunPipeline(t, ifabsent.Contract(), &sdk.Package{
			Name: "x", Path: "x", Functions: []*sdk.Function{host(map[string]string{})},
		})
		contracttest.AssertNoErrorDiag(t, diags)
	})
}
