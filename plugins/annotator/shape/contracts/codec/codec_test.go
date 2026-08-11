// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package codec_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/codec"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/sdk"
)

func encodeWith(partners map[string]string) *sdk.Function {
	return &sdk.Function{
		Name: "Encode", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(codec.Name, codec.RoleForward, partners),
			},
		},
	}
}

func TestContract(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		contracttest.AssertIdentity(t, codec.Contract(), codec.Name, codec.Roles)
	})

	t.Run("the inverse is qualified and back-stamped", func(t *testing.T) {
		t.Parallel()
		fwd := encodeWith(map[string]string{codec.RoleInverse: "Decode"})
		inv := &sdk.Function{Name: "Decode", Package: "x"}
		diags := contracttest.RunPipeline(t, codec.Contract(), &sdk.Package{
			Name: "x", Path: "x", Functions: []*sdk.Function{fwd, inv},
		})
		contracttest.AssertNoErrorDiag(t, diags)

		contracttest.AssertPartner(t, fwd.Meta(), codec.Name, codec.RoleInverse, "x.Decode")
		contracttest.AssertRole(t, inv.Meta(), codec.Name, codec.RoleInverse)
		contracttest.AssertPartner(t, inv.Meta(), codec.Name, codec.RoleForward, "x.Encode")
	})

	t.Run("a forward with no inverse is reported", func(t *testing.T) {
		t.Parallel()
		// The property is unstatable without both halves, so the
		// omission fails at the directive rather than producing a suite
		// that asserts nothing.
		fwd := encodeWith(map[string]string{})
		diags := contracttest.RunPipeline(t, codec.Contract(), &sdk.Package{
			Name: "x", Path: "x", Functions: []*sdk.Function{fwd},
		})
		contracttest.AssertContainsDiag(t, diags, sdk.SeverityError, codec.RoleInverse)
	})

	t.Run("fidelity is carried verbatim", func(t *testing.T) {
		t.Parallel()
		// It names neither a callable nor a parameter — only which of
		// two laws applies — so the resolver must not touch it.
		fwd := encodeWith(map[string]string{
			codec.RoleInverse:   "Decode",
			codec.ParamFidelity: codec.FidelityLossy,
		})
		inv := &sdk.Function{Name: "Decode", Package: "x"}
		diags := contracttest.RunPipeline(t, codec.Contract(), &sdk.Package{
			Name: "x", Path: "x", Functions: []*sdk.Function{fwd, inv},
		})
		contracttest.AssertNoErrorDiag(t, diags)

		got, _ := shape.ContractParamKey(codec.Name, codec.ParamFidelity).Get(fwd.Meta())
		if got != codec.FidelityLossy {
			t.Fatalf("fidelity = %q, want %q", got, codec.FidelityLossy)
		}
	})
}
