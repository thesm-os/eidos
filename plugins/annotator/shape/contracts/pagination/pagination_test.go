// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pagination_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/pagination"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t, pagination.Contract(), pagination.Name, pagination.Roles)
}

// build returns a reader answering a Page whose Cursor field is the
// one a `cursor=` names — the shape a continuation claim reads.
func build(t *testing.T, cursor string) (*sdk.Method, *sdk.Package) {
	t.Helper()
	page := &sdk.Struct{
		Name: "Page", Package: "x",
		Fields: []*sdk.Field{
			{Name: "Items", Type: &sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Name: "string"}},
			{Name: "Cursor", Type: &sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Name: "string"}},
		},
	}
	list := &sdk.Method{
		Name: "List",
		Params: []*sdk.Param{
			{Name: "ctx", Type: &sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Name: "Context", Package: "context"}},
		},
		Returns: sdk.AnonReturns(
			&sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Name: "Page", Package: "x"},
			&sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Name: "error"},
		),
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(pagination.Name, pagination.RoleReader,
					map[string]string{pagination.ParamCursor: cursor}),
			},
		},
	}
	reader := &sdk.Interface{Name: "Reader", Package: "x", Methods: []*sdk.Method{list}}
	list.Owner = reader
	return list, &sdk.Package{
		Name: "x", Path: "x",
		Interfaces: []*sdk.Interface{reader},
		Structs:    []*sdk.Struct{page},
	}
}

func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	list, pkg := build(t, "Cursor")
	diags := contracttest.RunPipeline(t, pagination.Contract(), pkg)
	contracttest.AssertNoErrorDiag(t, diags)

	contracttest.AssertRole(t, list.Meta(), pagination.Name, pagination.RoleReader)

	// Qualified, not verbatim: the promotion from KindOpaque is what
	// lets a consumer select the member rather than only read its
	// name — step two of the continuation check.
	got, ok := shape.ContractParamKey(pagination.Name, pagination.ParamCursor).Get(list.Meta())
	if !ok || got != "x.Page.Cursor" {
		t.Fatalf("param.cursor = %q (present=%v); want x.Page.Cursor", got, ok)
	}
}

// TestContract_CursorIsChecked covers what the opaque form could
// never do: report a name the answered page does not carry.
func TestContract_CursorIsChecked(t *testing.T) {
	t.Parallel()
	_, pkg := build(t, "Nonesuch")
	diags := contracttest.RunPipeline(t, pagination.Contract(), pkg)
	contracttest.AssertContainsDiag(t, diags, sdk.SeverityError, "Nonesuch")
}
