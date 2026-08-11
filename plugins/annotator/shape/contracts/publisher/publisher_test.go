// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package publisher_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/publisher"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t,
		publisher.Contract(),
		publisher.Name, publisher.Roles)
}

func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	pub := &sdk.Function{
		Name: "Publish", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(publisher.Name, "publish", map[string]string{
					"subscribe": "Subscribe",
				}),
			},
		},
	}
	sub := &sdk.Function{Name: "Subscribe", Package: "x"}
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{pub, sub},
	}
	diags := contracttest.RunPipeline(t, publisher.Contract(), pkg)
	contracttest.AssertNoErrorDiag(t, diags)

	contracttest.AssertRole(t, pub.Meta(), publisher.Name, "publish")
	contracttest.AssertPartner(t, pub.Meta(), publisher.Name, "subscribe", "x.Subscribe")
	contracttest.AssertRole(t, sub.Meta(), publisher.Name, "subscribe")
	contracttest.AssertPartner(t, sub.Meta(), publisher.Name, "publish", "x.Publish")
}

// TestContract_DeliveryMode pins the guarantee param.
//
// The three modes imply different assertions — duplicates permitted,
// loss permitted, or neither — so a publisher that states none must
// not be read as stating a default, and the value must reach the
// consumer verbatim rather than being resolved as a partner.
func TestContract_DeliveryMode(t *testing.T) {
	t.Parallel()

	t.Run("the mode is carried verbatim", func(t *testing.T) {
		t.Parallel()
		pub := &sdk.Function{
			Name: "Publish", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(publisher.Name, "publish", map[string]string{
						"subscribe":         "Subscribe",
						publisher.ParamMode: publisher.ModeExactlyOnce,
					}),
				},
			},
		}
		sub := &sdk.Function{Name: "Subscribe", Package: "x"}
		diags := contracttest.RunPipeline(t, publisher.Contract(), &sdk.Package{
			Name: "x", Path: "x", Functions: []*sdk.Function{pub, sub},
		})
		contracttest.AssertNoErrorDiag(t, diags)

		got, _ := shape.ContractParamKey(publisher.Name, publisher.ParamMode).Get(pub.Meta())
		if got != publisher.ModeExactlyOnce {
			t.Fatalf("mode = %q, want %q", got, publisher.ModeExactlyOnce)
		}
	})

	t.Run("an absent mode stamps nothing", func(t *testing.T) {
		t.Parallel()
		// Unstated rather than defaulted: picking a bound for a
		// publisher that did not say produces a check that fails on
		// correct code.
		pub := &sdk.Function{
			Name: "Publish", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(publisher.Name, "publish", map[string]string{
						"subscribe": "Subscribe",
					}),
				},
			},
		}
		sub := &sdk.Function{Name: "Subscribe", Package: "x"}
		contracttest.RunPipeline(t, publisher.Contract(), &sdk.Package{
			Name: "x", Path: "x", Functions: []*sdk.Function{pub, sub},
		})
		if got, ok := shape.ContractParamKey(publisher.Name, publisher.ParamMode).Get(pub.Meta()); ok {
			t.Fatalf("mode = %q, want unstamped", got)
		}
	})
}
