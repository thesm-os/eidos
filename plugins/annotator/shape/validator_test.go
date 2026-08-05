// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/priority"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// TestValidator_Contract pins the role / capability surface the
// framework asserts at registration / build time.
func TestValidator_Contract(t *testing.T) {
	t.Parallel()

	t.Run("name is stable", func(t *testing.T) {
		t.Parallel()
		v := shape.New().Validator()
		first := v.Name()
		second := v.Name()
		if first == "" || first != second {
			t.Fatalf("Name() unstable or empty: first=%q second=%q", first, second)
		}
	})

	t.Run("priority is AnnotatorValidation", func(t *testing.T) {
		t.Parallel()
		if got, want := shape.New().Validator().Priority(), priority.AnnotatorValidation; got != want {
			t.Fatalf("Priority() = %v, want %v", got, want)
		}
	})

	t.Run("satisfies plugin.Annotator", func(t *testing.T) {
		t.Parallel()
		var _ plugin.Annotator = shape.New().Validator()
	})

	t.Run("Annotate on empty store does not panic", func(t *testing.T) {
		t.Parallel()
		ctx := newAnnotatorContext(t, store.New())
		if err := shape.New().Validator().Annotate(ctx); err != nil {
			t.Fatalf("Annotate(empty): %v", err)
		}
	})
}

// TestValidator_RequiredPartners covers the per-role
// required-partner check: the validator emits a positioned
// diagnostic when a role declared in [Contract.Required] is
// missing a partner stamp after the resolver has run.
func TestValidator_RequiredPartners(t *testing.T) {
	t.Parallel()

	t.Run("missing required partner surfaces a diagnostic", func(t *testing.T) {
		t.Parallel()
		spec := shape.Contract{
			Name:     "tx",
			Roles:    []string{"begin", "commit", "rollback"},
			Required: map[string][]string{"begin": {"commit", "rollback"}},
		}
		// Directive declares only `commit=`; the validator must
		// flag the missing `rollback=` partner.
		fn := contractFn("Begin",
			&directive.Directive{
				Name: shape.ContractDirectiveName,
				Args: []string{"tx"},
				KV:   map[string]string{"role": "begin", "commit": "Commit"},
			},
			// A standalone commit function so the resolver succeeds
			// for the one partner that is provided.
		)
		commit := &node.Function{Name: "Commit", Package: "x"}
		pkg := &node.Package{
			Name: "x", Path: "x",
			Functions: []*node.Function{fn, commit},
		}
		diags := runFullPipeline(t, spec, pkg)
		assertContainsDiag(t, diags, diag.Error, "rollback")
	})

	t.Run("all required partners present emits no diagnostic", func(t *testing.T) {
		t.Parallel()
		spec := shape.Contract{
			Name:     "tx",
			Roles:    []string{"begin", "commit"},
			Required: map[string][]string{"begin": {"commit"}},
		}
		fn := contractFn("Begin",
			&directive.Directive{
				Name: shape.ContractDirectiveName,
				Args: []string{"tx"},
				KV:   map[string]string{"role": "begin", "commit": "Commit"},
			},
		)
		commit := &node.Function{Name: "Commit", Package: "x"}
		pkg := &node.Package{
			Name: "x", Path: "x",
			Functions: []*node.Function{fn, commit},
		}
		for _, d := range runFullPipeline(t, spec, pkg) {
			if d.Severity >= diag.Error {
				t.Fatalf("unexpected error diagnostic: %+v", d)
			}
		}
	})
}

// TestValidator_ContractValidate covers the per-contract
// invariant hook — [Contract.Validate] receives the resolved
// member set and may emit violations the validator surfaces as
// positioned diagnostics.
func TestValidator_ContractValidate(t *testing.T) {
	t.Parallel()

	t.Run("validator hook receives the member set keyed by role", func(t *testing.T) {
		t.Parallel()
		var captured map[string][]shape.ContractMember
		spec := shape.Contract{
			Name:  "tx",
			Roles: []string{"begin", "commit"},
			Validate: func(members map[string][]shape.ContractMember) []shape.ContractViolation {
				captured = members
				return nil
			},
		}
		begin := contractFn("Begin",
			&directive.Directive{
				Name: shape.ContractDirectiveName,
				Args: []string{"tx"},
				KV:   map[string]string{"role": "begin", "commit": "Commit"},
			},
		)
		commit := &node.Function{Name: "Commit", Package: "x"}
		pkg := &node.Package{
			Name: "x", Path: "x",
			Functions: []*node.Function{begin, commit},
		}
		_ = runFullPipeline(t, spec, pkg)

		if got := len(captured["begin"]); got != 1 {
			t.Fatalf("members[begin] = %d nodes, want 1", got)
		}
		if got := len(captured["commit"]); got != 1 {
			t.Fatalf("members[commit] = %d nodes, want 1", got)
		}
	})

	t.Run("violations surface as positioned diagnostics", func(t *testing.T) {
		t.Parallel()
		spec := shape.Contract{
			Name:  "tx",
			Roles: []string{"begin", "commit"},
			Validate: func(members map[string][]shape.ContractMember) []shape.ContractViolation {
				return []shape.ContractViolation{
					{
						Host:    members["begin"][0].Host,
						Message: "tx: synthetic invariant breach",
					},
				}
			},
		}
		begin := contractFn("Begin",
			&directive.Directive{
				Name: shape.ContractDirectiveName,
				Args: []string{"tx"},
				KV:   map[string]string{"role": "begin", "commit": "Commit"},
			},
		)
		commit := &node.Function{Name: "Commit", Package: "x"}
		pkg := &node.Package{
			Name: "x", Path: "x",
			Functions: []*node.Function{begin, commit},
		}
		diags := runFullPipeline(t, spec, pkg)
		assertContainsDiag(t, diags, diag.Error, "synthetic invariant breach")
	})

	t.Run("nil Validate hook is a permissive no-op", func(t *testing.T) {
		t.Parallel()
		spec := shape.Contract{
			Name:  "tx",
			Roles: []string{"begin", "commit"},
		}
		fn := contractFn("Begin",
			&directive.Directive{
				Name: shape.ContractDirectiveName,
				Args: []string{"tx"},
				KV:   map[string]string{"role": "begin", "commit": "Commit"},
			},
		)
		commit := &node.Function{Name: "Commit", Package: "x"}
		pkg := &node.Package{
			Name: "x", Path: "x",
			Functions: []*node.Function{fn, commit},
		}
		for _, d := range runFullPipeline(t, spec, pkg) {
			if d.Severity >= diag.Error {
				t.Fatalf("nil Validate must not produce errors; got %+v", d)
			}
		}
	})

	t.Run("validator visits methods alongside free functions", func(t *testing.T) {
		t.Parallel()
		// Exercises [Validator.OnMethod] (vs the free-function
		// path the other tests cover). Begin lives on a struct;
		// the Required check still fires for missing rollback.
		spec := shape.Contract{
			Name:     "tx",
			Roles:    []string{"begin", "commit", "rollback"},
			Required: map[string][]string{"begin": {"commit", "rollback"}},
		}
		begin := &node.Method{
			Name: "Begin",
			BaseNode: node.BaseNode{
				DirectiveList: []*directive.Directive{
					{
						Name: shape.ContractDirectiveName,
						Args: []string{"tx"},
						KV:   map[string]string{"role": "begin", "commit": "Commit"},
					},
				},
			},
		}
		commit := &node.Method{Name: "Commit"}
		s := &node.Struct{
			Name: "Repo", Package: "x",
			Methods: []*node.Method{begin, commit},
		}
		pkg := &node.Package{
			Name: "x", Path: "x",
			Structs: []*node.Struct{s},
		}
		diags := runFullPipeline(t, spec, pkg)
		assertContainsDiag(t, diags, diag.Error, "rollback")
	})
}

// TestValidator_MixinValidate covers the validator's
// [Mixin.Validate] path — accumulated [MixinAttachment] values
// carry the param snapshot the directive supplied, and emitted
// violations land on ctx.Diag with the validator's plugin
// attribution.
func TestValidator_MixinValidate(t *testing.T) {
	t.Parallel()

	t.Run("attachment captures declared params from the directive", func(t *testing.T) {
		t.Parallel()
		var captured []shape.MixinAttachment
		spec := shape.Mixin{
			Name:   "tagged",
			Params: []string{"tag"},
			Validate: func(attachments []shape.MixinAttachment) []shape.MixinViolation {
				captured = attachments
				return nil
			},
		}
		fn := contractFn("X",
			&directive.Directive{
				Name: shape.MixinDirectiveName,
				Args: []string{"tagged"},
				KV:   map[string]string{"tag": "important"},
			},
		)
		pkg := &node.Package{
			Name: "x", Path: "x",
			Functions: []*node.Function{fn},
		}
		runMixinPipeline(t, spec, pkg)

		if len(captured) != 1 {
			t.Fatalf("attachments = %d, want 1", len(captured))
		}
		if got := captured[0].Params["tag"]; got != "important" {
			t.Fatalf("attachments[0].Params[tag] = %q, want %q", got, "important")
		}
	})
}

// runMixinPipeline runs the umbrella → resolver → validator
// sequence with m as the sole registered mixin. Mirrors
// [runFullPipeline] but takes a [shape.Mixin] instead of a
// [shape.Contract].
func runMixinPipeline(t *testing.T, m shape.Mixin, pkg *node.Package) []diag.Diag {
	t.Helper()
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.Meta(), "golang", "test")

	umbrella := shape.New().Mixins(m)
	sink := diag.New()
	ctx := &sdk.AnnotatorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   sink,
	}
	if err := umbrella.Annotate(ctx); err != nil {
		t.Fatalf("umbrella.Annotate: %v", err)
	}
	if err := umbrella.Resolver().Annotate(ctx); err != nil {
		t.Fatalf("resolver.Annotate: %v", err)
	}
	if err := umbrella.Validator().Annotate(ctx); err != nil {
		t.Fatalf("validator.Annotate: %v", err)
	}
	return sink.Diagnostics()
}

// runFullPipeline wires pkg into a fresh store and runs the
// umbrella → resolver → validator sequence with the supplied
// contract registered on all three. Returns the accumulated
// diagnostic snapshot.
func runFullPipeline(t *testing.T, c shape.Contract, pkg *node.Package) []diag.Diag {
	t.Helper()
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.Meta(), "golang", "test")

	umbrella := shape.New().Contracts(c)
	sink := diag.New()
	ctx := &sdk.AnnotatorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   sink,
	}
	if err := umbrella.Annotate(ctx); err != nil {
		t.Fatalf("umbrella.Annotate: %v", err)
	}
	if err := umbrella.Resolver().Annotate(ctx); err != nil {
		t.Fatalf("resolver.Annotate: %v", err)
	}
	if err := umbrella.Validator().Annotate(ctx); err != nil {
		t.Fatalf("validator.Annotate: %v", err)
	}
	return sink.Diagnostics()
}

// TestValidatorConformance runs the framework conformance suite over
// the contract validator.
//
// The three shape plugins previously ran no conformance suite at
// all, which is how each came to declare Priority() without the rest
// of plugin.CapabilityProvider — satisfying nothing, so the pipeline
// ignored the declared ordering and ran all three in the default
// bucket, in registration order. The suite's completeness check
// catches that shape directly; wiring the plugins into the suite is
// what makes the check reach them.
func TestValidatorConformance(t *testing.T) {
	t.Parallel()
	plugintest.RunSuite(t, shape.New().Validator())
}
