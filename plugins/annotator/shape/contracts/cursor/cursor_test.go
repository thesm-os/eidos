// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cursor_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/cursor"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t,
		cursor.Contract(),
		cursor.Name, cursor.Roles)
}

func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	next := &sdk.Function{
		Name: "Next", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(cursor.Name, "next", map[string]string{
					"close": "Close",
				}),
			},
		},
	}
	closeFn := &sdk.Function{Name: "Close", Package: "x"}
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{next, closeFn},
	}
	diags := contracttest.RunPipeline(t, cursor.Contract(), pkg)
	contracttest.AssertNoErrorDiag(t, diags)

	contracttest.AssertRole(t, next.Meta(), cursor.Name, "next")
	contracttest.AssertPartner(t, next.Meta(), cursor.Name, "close", "x.Close")
	contracttest.AssertRole(t, closeFn.Meta(), cursor.Name, "close")
	contracttest.AssertPartner(t, closeFn.Meta(), cursor.Name, "next", "x.Next")
}

// buildProducer assembles the producer arm: a Scan method on an
// interface, answering a Cursor whose Next and Close live on the
// returned type rather than beside the factory.
//
// withNext controls whether the directive names the reader, which is
// the one param the `open` arm requires.
func buildProducer(withNext bool) (*sdk.Method, *sdk.Package) {
	kv := map[string]string{
		cursor.ParamClose:    "Close",
		cursor.ParamSentinel: "ErrClosed",
	}
	if withNext {
		kv[cursor.ParamNext] = "Next"
	}
	scan := &sdk.Method{
		Name: "Scan",
		Params: []*sdk.Param{
			{Name: "ctx", Type: &sdk.TypeRef{Name: "Context", Package: "context"}},
		},
		Returns: sdk.AnonReturns(
			&sdk.TypeRef{Name: "Cursor", Package: "x"},
			&sdk.TypeRef{Name: "error"},
		),
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(cursor.Name, cursor.RoleOpen, kv),
			},
		},
	}
	store := &sdk.Interface{Name: "Store", Package: "x", Methods: []*sdk.Method{scan}}
	scan.Owner = store
	handle := &sdk.Interface{
		Name: "Cursor", Package: "x",
		Methods: []*sdk.Method{{Name: "Next"}, {Name: "Close"}},
	}
	for _, m := range handle.Methods {
		m.Owner = handle
	}
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Interfaces: []*sdk.Interface{store, handle},
		Variables:  []*sdk.Variable{{Name: "ErrClosed", Package: "x"}},
	}
	return scan, pkg
}

// TestContract_ProducerArm covers the `open` role — a factory
// answering a fresh cursor.
//
// The arm exists because `next` and `close` mean different things
// depending on which role hosts the directive: siblings of the reader
// on the method arms, members of the answered handle here. Resolving
// them through the host's own scope would report this correct
// directive as naming callables that are not there.
func TestContract_ProducerArm(t *testing.T) {
	t.Parallel()

	t.Run("handle members resolve against the answered type", func(t *testing.T) {
		t.Parallel()
		scan, pkg := buildProducer(true)
		diags := contracttest.RunPipeline(t, cursor.Contract(), pkg)
		contracttest.AssertNoErrorDiag(t, diags)

		next, _ := shape.ContractParamKey(cursor.Name, cursor.ParamNext).Get(scan.Meta())
		closed, _ := shape.ContractParamKey(cursor.Name, cursor.ParamClose).Get(scan.Meta())
		if next != "x.Cursor.Next" || closed != "x.Cursor.Close" {
			t.Fatalf("next/close = %q/%q, want x.Cursor.Next/x.Cursor.Close", next, closed)
		}
	})

	t.Run("the sentinel still resolves through the package", func(t *testing.T) {
		t.Parallel()
		// Unscoped, so it applies to every arm — and a sentinel is a
		// package-level var on the producer arm as much as on the
		// reader, since it is declared beside the type rather than on
		// the handle.
		scan, pkg := buildProducer(true)
		contracttest.RunPipeline(t, cursor.Contract(), pkg)

		got, _ := shape.ContractParamKey(cursor.Name, cursor.ParamSentinel).Get(scan.Meta())
		if got != "x.ErrClosed" {
			t.Fatalf("sentinel = %q, want x.ErrClosed", got)
		}
	})

	t.Run("no partner stamp is written for a member key", func(t *testing.T) {
		t.Parallel()
		// The routing half of the role scope. `close` is a declared
		// role, so a key that failed to route as a param would land as
		// a partner reference and resolve against the host's scope —
		// finding nothing, because Close is on the handle.
		scan, pkg := buildProducer(true)
		contracttest.RunPipeline(t, cursor.Contract(), pkg)

		if got, ok := shape.ContractPartnerKey(cursor.Name, "close").Get(scan.Meta()); ok {
			t.Fatalf("partner.close = %q, want no stamp: the key is a param on this arm", got)
		}
	})

	t.Run("an open host without next= is reported", func(t *testing.T) {
		t.Parallel()
		// The reader is the host on the method arms, so it cannot be
		// missing there. Here the factory is the host and `next=` is
		// the only thing naming what to read.
		_, pkg := buildProducer(false)
		diags := contracttest.RunPipeline(t, cursor.Contract(), pkg)
		contracttest.AssertContainsDiag(t, diags, sdk.SeverityError,
			`cursor role "open" requires next=`)
	})
}

// TestContract_KeyMeansTwoThings pins the collision the role scope
// exists to resolve: one key, two resolutions, chosen by the host's
// role.
func TestContract_KeyMeansTwoThings(t *testing.T) {
	t.Parallel()

	t.Run("close= is a sibling partner on the reader arm", func(t *testing.T) {
		t.Parallel()
		next := &sdk.Function{
			Name: "Next", Package: "x",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					contracttest.HostDirective(cursor.Name, "next", map[string]string{
						cursor.ParamClose: "Close",
					}),
				},
			},
		}
		closeFn := &sdk.Function{Name: "Close", Package: "x"}
		pkg := &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{next, closeFn},
		}
		contracttest.RunPipeline(t, cursor.Contract(), pkg)

		contracttest.AssertPartner(t, next.Meta(), cursor.Name, "close", "x.Close")
		if got, ok := shape.ContractParamKey(cursor.Name, cursor.ParamClose).Get(next.Meta()); ok {
			t.Fatalf("param.close = %q, want no stamp: the key is a partner on this arm", got)
		}
	})
}
