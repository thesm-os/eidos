// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package persister_test

import (
	"reflect"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/persister"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

//nolint:gochecknoglobals // test-side singleton mirroring plugin's lookup
var frontendMarker = sdk.EnsureKey("frontend", sdk.StringParser)

// TestContract_Identity pins the package-exported constants
// against the [shape.Contract] value the constructor returns.
func TestContract_Identity(t *testing.T) {
	t.Parallel()
	c := persister.Contract()
	if c.Name != persister.Name {
		t.Fatalf("Contract().Name = %q, want %q", c.Name, persister.Name)
	}
	if !reflect.DeepEqual(c.Roles, persister.Roles) {
		t.Fatalf("Contract().Roles = %v, want %v", c.Roles, persister.Roles)
	}
}

// TestContract_DirectiveStamping pins the end-to-end stamping
// flow — the writer-side directive lands the role and the raw
// partner reader name on the host callable. The test asserts
// only the writer-side stamps the umbrella plugin produces;
// resolver back-stamping on the partner is covered by the
// resolver's own tests.
func TestContract_DirectiveStamping(t *testing.T) {
	t.Parallel()

	save := &sdk.Function{
		Name: "Save", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				{
					Name: shape.ContractDirectiveName,
					Args: []string{persister.Name},
					KV: map[string]string{
						"role":   "writer",
						"reader": "GetByID",
					},
				},
			},
		},
	}
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{save},
	}
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")

	p := shape.New().Contracts(persister.Contract())
	ctx := &sdk.AnnotatorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   diag.New(),
	}
	if err := p.Annotate(ctx); err != nil {
		t.Fatalf("Annotate: %v", err)
	}

	if got := shape.Contracts(save.Meta()); !reflect.DeepEqual(got, []string{persister.Name}) {
		t.Fatalf("Contracts = %v, want [%q]", got, persister.Name)
	}
	if got, _ := shape.ContractRoleKey(persister.Name).Get(save.Meta()); got != "writer" {
		t.Fatalf("role = %q, want %q", got, "writer")
	}
	if got, _ := shape.ContractPartnerKey(persister.Name, "reader").Get(save.Meta()); got != "GetByID" {
		t.Fatalf("partner.reader = %q, want %q", got, "GetByID")
	}
}
