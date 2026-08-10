// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pool_test

import (
	"reflect"
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/internal/contracttest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/pool"
	"go.thesmos.sh/eidos/sdk"
)

func TestContract_Identity(t *testing.T) {
	t.Parallel()
	contracttest.AssertIdentity(t,
		pool.Contract(),
		pool.Name, pool.Roles)
}

func TestContract_DeclaresRequiredAndValidate(t *testing.T) {
	t.Parallel()
	c := pool.Contract()
	wantRequired := map[string][]string{"get": {"put"}}
	if !reflect.DeepEqual(c.Required, wantRequired) {
		t.Fatalf("Required = %v, want %v", c.Required, wantRequired)
	}
	if c.Validate == nil {
		t.Fatalf("Validate hook missing")
	}
}

func TestContract_ValidateAcceptsExactlyOneEach(t *testing.T) {
	t.Parallel()
	c := pool.Contract()
	members := map[string][]shape.ContractMember{
		"get": {{Host: &sdk.Function{Name: "Get"}}},
		"put": {{Host: &sdk.Function{Name: "Put"}}},
	}
	if got := c.Validate(members); len(got) != 0 {
		t.Fatalf("Validate(one-each) = %+v; want no violations", got)
	}
}

// TestContract_ValidateScansEveryRole pins the role cascade in
// pool's Validate hook. The loop over [pool.Roles] must skip a
// compliant role and keep going, so the `continue` at pool.go:39
// cannot become a `break`: under that mutant a compliant `get`
// short-circuits the whole loop and a duplicated `put` is
// reported as no violation at all — a silently-dropped
// diagnostic of exactly the kind that shipped green for months
// in the detector cascade. Do not fold this into
// TestContract_ValidateAcceptsExactlyOneEach: the point is the
// ordering of a compliant role BEFORE a violating one, which a
// single-role fixture cannot express.
func TestContract_ValidateScansEveryRole(t *testing.T) {
	t.Parallel()
	c := pool.Contract()

	t.Run("a compliant get does not stop the put role from being validated", func(t *testing.T) {
		t.Parallel()
		putB := &sdk.Function{Name: "PutB"}
		members := map[string][]shape.ContractMember{
			"get": {{Host: &sdk.Function{Name: "Get"}}},
			"put": {{Host: &sdk.Function{Name: "PutA"}}, {Host: putB}},
		}
		const want = "pool requires exactly one put; got 2 callables"
		got := c.Validate(members)
		if len(got) != 1 {
			t.Fatalf("Validate(one compliant get, two puts) reported %d violations %q; "+
				"want exactly one, %q, against the surplus put",
				len(got), messages(got), want)
		}
		if got[0].Host != sdk.Node(putB) {
			t.Fatalf("violation hangs off a node other than the surplus put %q", putB.Name)
		}
		if got[0].Message != want {
			t.Fatalf("violation Message = %q, want %q", got[0].Message, want)
		}
	})
}

// messages projects violations onto their message bodies for
// failure output. A raw [shape.ContractViolation] dump renders its
// host as a pointer address, which tells the reader nothing about
// which role went unchecked.
func messages(violations []shape.ContractViolation) []string {
	out := make([]string, len(violations))
	for i, v := range violations {
		out[i] = v.Message
	}
	return out
}

// TestContract_PipelineRoundTrip exercises the happy path of one
// Get + one Put through umbrella → resolver → validator.
func TestContract_PipelineRoundTrip(t *testing.T) {
	t.Parallel()
	get := &sdk.Function{
		Name: "Get", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(pool.Name, "get", map[string]string{
					"put": "Put",
				}),
			},
		},
	}
	put := &sdk.Function{Name: "Put", Package: "x"}
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{get, put},
	}
	diags := contracttest.RunPipeline(t, pool.Contract(), pkg)
	contracttest.AssertNoErrorDiag(t, diags)

	contracttest.AssertRole(t, get.Meta(), pool.Name, "get")
	contracttest.AssertPartner(t, get.Meta(), pool.Name, "put", "x.Put")
	contracttest.AssertRole(t, put.Meta(), pool.Name, "put")
	contracttest.AssertPartner(t, put.Meta(), pool.Name, "get", "x.Get")
}

// TestContract_ValidatorFlagsDuplicateGet exercises the Validate
// hook through the actual [shape.Validator] annotator — two Get
// callables share one Put, and the validator must emit a
// diagnostic naming the duplicate.
func TestContract_ValidatorFlagsDuplicateGet(t *testing.T) {
	t.Parallel()
	getA := &sdk.Function{
		Name: "GetA", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(pool.Name, "get", map[string]string{
					"put": "Put",
				}),
			},
		},
	}
	getB := &sdk.Function{
		Name: "GetB", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				contracttest.HostDirective(pool.Name, "get", map[string]string{
					"put": "Put",
				}),
			},
		},
	}
	put := &sdk.Function{Name: "Put", Package: "x"}
	pkg := &sdk.Package{
		Name: "x", Path: "x",
		Functions: []*sdk.Function{getA, getB, put},
	}
	diags := contracttest.RunPipeline(t, pool.Contract(), pkg)
	contracttest.AssertContainsDiag(t, diags, sdk.SeverityError, "exactly one get")
}
