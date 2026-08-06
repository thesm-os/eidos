// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package saga_test

import (
	"fmt"
	"slices"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/saga"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t,
		saga.Contract(),
		saga.Name, saga.Roles)
}

// TestContract_Validate exercises the saga validator hook
// directly — accepts distinct compensations, flags shared
// compensations, and survives non-callable hosts via the
// stepLabel default branch.
func TestContract_Validate(t *testing.T) {
	t.Parallel()
	c := saga.Contract()

	t.Run("accepts distinct compensations", func(t *testing.T) {
		t.Parallel()
		members := map[string][]shape.ContractMember{
			"step": {
				{Host: &node.Function{Name: "Charge"}, Partners: map[string]string{"compensate": "x.Refund"}},
				{Host: &node.Function{Name: "Ship"}, Partners: map[string]string{"compensate": "x.Unship"}},
			},
			"compensate": {
				{Host: &node.Function{Name: "Refund"}},
				{Host: &node.Function{Name: "Unship"}},
			},
		}
		if got := c.Validate(members); len(got) != 0 {
			t.Fatalf("Validate(distinct) = %+v; want no violations", got)
		}
	})

	t.Run("handles non-callable host via stepLabel fallback", func(t *testing.T) {
		t.Parallel()
		// First member is a [*node.Struct] — neither Function
		// nor Method, so stepLabel returns the empty string for
		// the seen-map entry. The second step (a Function) then
		// tries to register the same compensate qname and gets
		// flagged.
		members := map[string][]shape.ContractMember{
			"step": {
				{Host: &node.Struct{Name: "S"}, Partners: map[string]string{"compensate": "x.A"}},
				{Host: &node.Function{Name: "Other"}, Partners: map[string]string{"compensate": "x.A"}},
			},
		}
		got := c.Validate(members)
		if len(got) != 1 {
			t.Fatalf("Validate(non-callable + dup) = %+v; want one violation on the second step", got)
		}
	})
}

// TestContract_ValidateScansEveryStep pins the step cascade in
// saga's Validate hook: every step after a skipped or a flagged
// one must still be examined. It kills two `continue -> break`
// mutants that the rest of this file leaves alive, because no
// other fixture here has a THIRD step behind a declining second
// one:
//
//   - saga.go:50 (`continue` after recording a duplicate) — under
//     `break` only the first duplicate is ever reported, so a
//     saga with three duplicate compensations reports one and the
//     author fixes one third of the bug.
//   - saga.go:44 (`continue` on an unpaired step) — under `break`
//     a single step without a compensate partner silently
//     disables uniqueness checking for every step below it.
//
// Both mutants make the validator under-report, which is the
// failure mode a green suite cannot see. Assertions therefore pin
// which hosts were flagged, not merely how many.
func TestContract_ValidateScansEveryStep(t *testing.T) {
	t.Parallel()
	c := saga.Contract()

	t.Run("every duplicate compensation is reported, not only the first", func(t *testing.T) {
		t.Parallel()
		ship := &node.Function{Name: "Ship"}
		pack := &node.Function{Name: "Pack"}
		bill := &node.Function{Name: "Bill"}
		members := map[string][]shape.ContractMember{
			"step": {
				{Host: &node.Function{Name: "Charge"}, Partners: map[string]string{"compensate": "x.Refund"}},
				{Host: ship, Partners: map[string]string{"compensate": "x.Refund"}},
				{Host: pack, Partners: map[string]string{"compensate": "x.Refund"}},
				{Host: bill, Partners: map[string]string{"compensate": "x.Refund"}},
			},
		}
		assertFlagged(t, c.Validate(members),
			[]*node.Function{ship, pack, bill},
			"saga: compensation x.Refund is already paired with step Charge")
	})

	t.Run("a step with no compensation does not stop the steps below it", func(t *testing.T) {
		t.Parallel()
		ship := &node.Function{Name: "Ship"}
		members := map[string][]shape.ContractMember{
			"step": {
				{Host: &node.Function{Name: "Audit"}},
				{Host: &node.Function{Name: "Charge"}, Partners: map[string]string{"compensate": "x.Refund"}},
				{Host: ship, Partners: map[string]string{"compensate": "x.Refund"}},
			},
		}
		assertFlagged(t, c.Validate(members),
			[]*node.Function{ship},
			"saga: compensation x.Refund is already paired with step Charge")
	})
}

// assertFlagged fails unless got flags exactly the steps in want,
// in order, each carrying message. Identity is the assertion that
// matters: a count-only check still passes for a validator that
// flags the wrong step, and it reports "1 violation" for both a
// correct single duplicate and a truncated cascade. Failures name
// the steps, since a raw [shape.ContractViolation] dump renders
// its host as a pointer address.
func assertFlagged(t *testing.T, got []shape.ContractViolation, want []*node.Function, message string) {
	t.Helper()
	gotSteps := make([]string, len(got))
	for i, v := range got {
		gotSteps[i] = hostName(v.Host)
	}
	wantSteps := make([]string, len(want))
	for i, w := range want {
		wantSteps[i] = w.Name
	}
	if !slices.Equal(gotSteps, wantSteps) {
		t.Fatalf("Validate flagged steps %v; want %v", gotSteps, wantSteps)
	}
	for i, w := range want {
		if got[i].Host != node.Node(w) {
			t.Fatalf("violation %d names step %q but hangs off a different node", i, w.Name)
		}
		if got[i].Message != message {
			t.Fatalf("violation on step %q: Message = %q, want %q", w.Name, got[i].Message, message)
		}
	}
}

// hostName renders a violation host as the step name a reader can
// match against the fixture; non-callable hosts fall back to their
// type so the output never degrades to a pointer address.
func hostName(n node.Node) string {
	if fn, ok := n.(*node.Function); ok {
		return fn.Name
	}
	return fmt.Sprintf("%T", n)
}

// TestContract_PipelineRoundTrip exercises the happy path of one
// step + one compensate through umbrella → resolver → validator.
func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	step := &node.Function{
		Name: "Charge", Package: "x",
		BaseNode: node.BaseNode{
			DirectiveList: []*directive.Directive{
				contracttest.HostDirective(saga.Name, "step", map[string]string{
					"compensate": "Refund",
				}),
			},
		},
	}
	refund := &node.Function{Name: "Refund", Package: "x"}
	pkg := &node.Package{
		Name: "x", Path: "x",
		Functions: []*node.Function{step, refund},
	}
	diags := contracttest.RunPipeline(t, saga.Contract(), pkg)
	contracttest.AssertNoErrorDiag(t, diags)

	contracttest.AssertRole(t, step.Meta(), saga.Name, "step")
	contracttest.AssertPartner(t, step.Meta(), saga.Name, "compensate", "x.Refund")
	contracttest.AssertRole(t, refund.Meta(), saga.Name, "compensate")
}

// TestContract_ValidatorFlagsSharedCompensate exercises the
// Validate hook through the actual [shape.Validator] annotator
// — two steps pointing at the same compensate produces a
// "already paired with step" diagnostic the validator must
// surface.
func TestContract_ValidatorFlagsSharedCompensate(t *testing.T) {
	t.Parallel()
	stepA := &node.Function{
		Name: "Charge", Package: "x",
		BaseNode: node.BaseNode{
			DirectiveList: []*directive.Directive{
				contracttest.HostDirective(saga.Name, "step", map[string]string{
					"compensate": "Refund",
				}),
			},
		},
	}
	stepB := &node.Function{
		Name: "Ship", Package: "x",
		BaseNode: node.BaseNode{
			DirectiveList: []*directive.Directive{
				contracttest.HostDirective(saga.Name, "step", map[string]string{
					"compensate": "Refund",
				}),
			},
		},
	}
	refund := &node.Function{Name: "Refund", Package: "x"}
	pkg := &node.Package{
		Name: "x", Path: "x",
		Functions: []*node.Function{stepA, stepB, refund},
	}
	diags := contracttest.RunPipeline(t, saga.Contract(), pkg)
	contracttest.AssertContainsDiag(t, diags, diag.Error, "already paired")
}
