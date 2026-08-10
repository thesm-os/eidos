// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package writethroughcache_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/writethroughcache"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		contracttest.AssertIdentity(t, writethroughcache.Contract(),
			writethroughcache.Name, writethroughcache.Roles)
	})

	t.Run("pipeline round-trip back-stamps backing partner", func(t *testing.T) {
		t.Parallel()
		cache := &sdk.Function{
			Name: "Get", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(writethroughcache.Name, "cache", map[string]string{
						"backing": "GetFromDB",
					}),
				},
			},
		}
		backing := &sdk.Function{Name: "GetFromDB", Package: "x"}
		pkg := &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{cache, backing},
		}
		diags := contracttest.RunPipeline(t, writethroughcache.Contract(), pkg)
		contracttest.AssertNoErrorDiag(t, diags)

		contracttest.AssertRole(t, cache.Meta(), writethroughcache.Name, "cache")
		contracttest.AssertPartner(t, cache.Meta(), writethroughcache.Name, "backing", "x.GetFromDB")
		contracttest.AssertRole(t, backing.Meta(), writethroughcache.Name, "backing")
	})
}
