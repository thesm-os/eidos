// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package retrysucceeds_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/retrysucceeds"
	"go.thesmos.sh/eidos/sdk"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, retrysucceeds.Mixin(), retrysucceeds.Name, retrysucceeds.Params)
}

// build assembles a host with the mixin attached, optionally carrying
// an attempts bound.
func build(attempts string) *sdk.Package {
	kv := map[string]string{}
	if attempts != "" {
		kv[retrysucceeds.ParamAttempts] = attempts
	}
	fn := &sdk.Function{
		Name: "Send", Package: "x",
		BaseNode: sdk.BaseNode{
			DirectiveList: []*sdk.Directive{
				mixintest.HostDirective(retrysucceeds.Name, kv),
			},
		},
	}
	return &sdk.Package{Name: "x", Path: "x", Functions: []*sdk.Function{fn}}
}

// TestMixin_Attempts covers the bound: part of what the author
// asserts, so it is validated rather than trusted — but optional,
// because the bare form still separates "considered retryable" from
// "nobody considered it".
func TestMixin_Attempts(t *testing.T) {
	t.Parallel()

	assertErrDiag := func(t *testing.T, diags []sdk.Diag, want bool) {
		t.Helper()
		found := false
		for _, d := range diags {
			if d.Severity == sdk.SeverityError {
				found = true
			}
		}
		if found != want {
			t.Fatalf("error diagnostic present = %v, want %v; diags: %+v", found, want, diags)
		}
	}

	t.Run("a bound of at least 2 passes", func(t *testing.T) {
		t.Parallel()
		diags := mixintest.RunWithValidator(t, retrysucceeds.Mixin(), build("3"))
		assertErrDiag(t, diags, false)
	})

	t.Run("the bare form passes", func(t *testing.T) {
		t.Parallel()
		// Deliberate in the consumer's corpus: classification without
		// a bound, the law declined until an author declares one.
		diags := mixintest.RunWithValidator(t, retrysucceeds.Mixin(), build(""))
		assertErrDiag(t, diags, false)
	})

	t.Run("one attempt is reported", func(t *testing.T) {
		t.Parallel()
		// attempts=1 is "succeeds first try": the smoke check under a
		// number, wrong for a subject whose first attempts must fail.
		diags := mixintest.RunWithValidator(t, retrysucceeds.Mixin(), build("1"))
		assertErrDiag(t, diags, true)
	})

	t.Run("a non-integer bound is reported", func(t *testing.T) {
		t.Parallel()
		diags := mixintest.RunWithValidator(t, retrysucceeds.Mixin(), build("thrice"))
		assertErrDiag(t, diags, true)
	})
}
