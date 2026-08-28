// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package workflow_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/workflow"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract(t *testing.T) {
	t.Parallel()

	t.Run("identity", func(t *testing.T) {
		t.Parallel()
		contracttest.AssertIdentity(t, workflow.Contract(), workflow.Name, workflow.Roles)
	})

	t.Run("the graph resolves through the package's vars", func(t *testing.T) {
		t.Parallel()
		// The value names the declaration holding the graph, not an
		// encoding of it — so the stamp is a reference a generated
		// body can index, rather than a string it would have to parse.
		run, pkg := build(map[string]string{workflow.ParamTransitions: "Transitions"})
		contracttest.AssertNoErrorDiag(t, contracttest.RunPipeline(t, workflow.Contract(), pkg))

		got, _ := shape.ContractParamKey(workflow.Name, workflow.ParamTransitions).Get(run.Meta())
		if got != "x.Transitions" {
			t.Fatalf("transitions = %q, want x.Transitions", got)
		}
	})

	t.Run("the observation resolves through the callable scope", func(t *testing.T) {
		t.Parallel()
		// Without it a move can be permitted and not seen: the graph
		// says which moves are allowed and nothing said how to watch
		// one happen.
		run, pkg := build(map[string]string{workflow.ParamObserve: "State"})
		contracttest.AssertNoErrorDiag(t, contracttest.RunPipeline(t, workflow.Contract(), pkg))

		got, _ := shape.ContractParamKey(workflow.Name, workflow.ParamObserve).Get(run.Meta())
		if got != "x.Machine.State" {
			t.Fatalf("observe = %q, want x.Machine.State", got)
		}
	})

	t.Run("a name nothing declares is reported", func(t *testing.T) {
		t.Parallel()
		// What the opaque form could never do. An encoding stamped
		// clean whatever it said, including the two notations that
		// drifted apart under it.
		_, pkg := build(map[string]string{workflow.ParamTransitions: "Absent"})
		diags := contracttest.RunPipeline(t, workflow.Contract(), pkg)
		contracttest.AssertContainsDiag(t, diags, sdk.SeverityError, "Absent")
	})

	t.Run("the bare form still classifies", func(t *testing.T) {
		t.Parallel()
		_, pkg := build(map[string]string{})
		contracttest.AssertNoErrorDiag(t, contracttest.RunPipeline(t, workflow.Contract(), pkg))
	})
}

// build returns a workflow callable beside the graph it advances
// through and the read that observes it — the shape the directive
// describes.
func build(kv map[string]string) (*sdk.Method, *sdk.Package) {
	run := &sdk.Method{
		Name: "Run",
		Params: []*sdk.Param{
			{Name: "ctx", Type: &sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Name: "Context", Package: "context"}},
		},
		Returns: sdk.AnonReturns(&sdk.TypeRef{TypeKind: sdk.TypeRefNamed, Name: "error"}),
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(workflow.Name, workflow.RoleFn, kv),
			},
		},
	}
	state := &sdk.Method{Name: "State"}
	machine := &sdk.Interface{Name: "Machine", Package: "x", Methods: []*sdk.Method{run, state}}
	run.Owner, state.Owner = machine, machine
	return run, &sdk.Package{
		Name: "x", Path: "x",
		Interfaces: []*sdk.Interface{machine},
		Variables:  []*sdk.Variable{{Name: "Transitions", Package: "x"}},
	}
}
