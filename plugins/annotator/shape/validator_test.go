// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape_test

import (
	"fmt"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
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
		if got, want := shape.New().Validator().Priority(), sdk.AnnotatorValidation; got != want {
			t.Fatalf("Priority() = %v, want %v", got, want)
		}
	})

	t.Run("satisfies sdk.Annotator", func(t *testing.T) {
		t.Parallel()
		var _ sdk.Annotator = shape.New().Validator()
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
			&sdk.Directive{
				Name: shape.ContractDirectiveName,
				Args: []string{"tx"},
				KV:   map[string]string{"role": "begin", "commit": "Commit"},
			},
			// A standalone commit function so the resolver succeeds
			// for the one partner that is provided.
		)
		commit := &sdk.Function{Name: "Commit", Package: "x"}
		pkg := &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{fn, commit},
		}
		diags := runFullPipeline(t, pkg, spec)
		assertContainsDiag(t, diags, sdk.SeverityError, "rollback")
	})

	t.Run("all required partners present emits no diagnostic", func(t *testing.T) {
		t.Parallel()
		spec := shape.Contract{
			Name:     "tx",
			Roles:    []string{"begin", "commit"},
			Required: map[string][]string{"begin": {"commit"}},
		}
		fn := contractFn("Begin",
			&sdk.Directive{
				Name: shape.ContractDirectiveName,
				Args: []string{"tx"},
				KV:   map[string]string{"role": "begin", "commit": "Commit"},
			},
		)
		commit := &sdk.Function{Name: "Commit", Package: "x"}
		pkg := &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{fn, commit},
		}
		for _, d := range runFullPipeline(t, pkg, spec) {
			if d.Severity >= sdk.SeverityError {
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
			&sdk.Directive{
				Name: shape.ContractDirectiveName,
				Args: []string{"tx"},
				KV:   map[string]string{"role": "begin", "commit": "Commit"},
			},
		)
		commit := &sdk.Function{Name: "Commit", Package: "x"}
		pkg := &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{begin, commit},
		}
		_ = runFullPipeline(t, pkg, spec)

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
			&sdk.Directive{
				Name: shape.ContractDirectiveName,
				Args: []string{"tx"},
				KV:   map[string]string{"role": "begin", "commit": "Commit"},
			},
		)
		commit := &sdk.Function{Name: "Commit", Package: "x"}
		pkg := &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{begin, commit},
		}
		diags := runFullPipeline(t, pkg, spec)
		assertContainsDiag(t, diags, sdk.SeverityError, "synthetic invariant breach")
	})

	t.Run("nil Validate hook is a permissive no-op", func(t *testing.T) {
		t.Parallel()
		spec := shape.Contract{
			Name:  "tx",
			Roles: []string{"begin", "commit"},
		}
		fn := contractFn("Begin",
			&sdk.Directive{
				Name: shape.ContractDirectiveName,
				Args: []string{"tx"},
				KV:   map[string]string{"role": "begin", "commit": "Commit"},
			},
		)
		commit := &sdk.Function{Name: "Commit", Package: "x"}
		pkg := &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{fn, commit},
		}
		for _, d := range runFullPipeline(t, pkg, spec) {
			if d.Severity >= sdk.SeverityError {
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
		begin := &sdk.Method{
			Name: "Begin",
			BaseNode: sdk.BaseNode{
				DirectiveList: []*sdk.Directive{
					{
						Name: shape.ContractDirectiveName,
						Args: []string{"tx"},
						KV:   map[string]string{"role": "begin", "commit": "Commit"},
					},
				},
			},
		}
		commit := &sdk.Method{Name: "Commit"}
		s := &sdk.Struct{
			Name: "Repo", Package: "x",
			Methods: []*sdk.Method{begin, commit},
		}
		pkg := &sdk.Package{
			Name: "x", Path: "x",
			Structs: []*sdk.Struct{s},
		}
		diags := runFullPipeline(t, pkg, spec)
		assertContainsDiag(t, diags, sdk.SeverityError, "rollback")
	})
}

// cascadeRuns is how many times
// [TestValidator_ContractValidateCascade] re-runs the pipeline.
// [Validator.AfterNodes] ranges over a map, so every run draws a
// fresh random contract order; 64 runs put the odds of a `break`
// mutant never being observed at 2^-64. The body is a couple of
// in-memory nodes, so the repetition costs microseconds.
const cascadeRuns = 64

// TestValidator_ContractValidateCascade pins that a contract with
// no Validate hook does not silence the contracts registered
// beside it.
//
// Kills the `continue` → `break` mutant on the
// `spec.Validate == nil` guard in [Validator.AfterNodes]. Under
// `break` the first contract lacking a Validate hook aborts the
// whole loop, so every contract the walk had not yet reached
// stops being validated — violations vanish wholesale while the
// suite stays green. Do not delete this as redundant with "nil
// Validate hook is a permissive no-op": that one proves a lone
// nil hook emits nothing, this one proves it does not suppress
// its neighbours, and only the second shape is what shipped
// broken in the reader detector's dispatch cascade.
func TestValidator_ContractValidateCascade(t *testing.T) {
	t.Parallel()

	t.Run("a contract with no Validate hook does not silence the ones beside it", func(t *testing.T) {
		t.Parallel()

		// audit declines — no Validate hook — and is therefore the
		// entry that ends the walk under the mutant. tx is the
		// neighbour whose violation must survive that decline.
		audit := shape.Contract{Name: "audit", Roles: []string{"record"}}
		tx := shape.Contract{
			Name:  "tx",
			Roles: []string{"begin", "rollback"},
			Validate: func(members map[string][]shape.ContractMember) []shape.ContractViolation {
				begins := members["begin"]
				if len(begins) != 1 {
					return []shape.ContractViolation{
						{Message: fmt.Sprintf("want 1 begin member, got %d", len(begins))},
					}
				}
				host := "<not a function>"
				if fn, ok := begins[0].Host.(*sdk.Function); ok {
					host = fn.Name
				}
				return []shape.ContractViolation{
					{Host: begins[0].Host, Message: "begin member " + host + " has no rollback partner"},
				}
			},
		}

		// Fresh nodes per run: the umbrella stamps metadata onto
		// the bags it walks, so reusing them would let run N see
		// run N-1's stamps.
		newPkg := func() *sdk.Package {
			record := contractFn("Record", &sdk.Directive{
				Name: shape.ContractDirectiveName,
				Args: []string{"audit"},
				KV:   map[string]string{"role": "record"},
			})
			begin := contractFn("Begin", &sdk.Directive{
				Name: shape.ContractDirectiveName,
				Args: []string{"tx"},
				KV:   map[string]string{"role": "begin"},
			})
			return &sdk.Package{
				Name: "x", Path: "x",
				Functions: []*sdk.Function{record, begin},
			}
		}

		// Named in full: the assertion has to pin which contract
		// was validated and which member its hook saw, not merely
		// that some error was emitted.
		const want = `shape.contract "tx": begin member Begin has no rollback partner`
		for run := range cascadeRuns {
			diags := runFullPipeline(t, newPkg(), audit, tx)
			if !hasDiag(diags, sdk.SeverityError, shape.ValidatorName, want) {
				t.Fatalf("run %d/%d: contract %q never reached its Validate hook: "+
					"want %v from %q with message %q; got %d diags: %+v",
					run+1, cascadeRuns, "tx", sdk.SeverityError, shape.ValidatorName, want, len(diags), diags)
			}
		}
	})
}

// hasDiag reports whether diags holds a diagnostic matching sev,
// plugin and msg exactly. Exact rather than the substring match
// [assertContainsDiag] performs, because the cascade assertion
// above distinguishes *which* contract produced the diagnostic
// from the mere fact that one appeared.
func hasDiag(diags []sdk.Diag, sev sdk.Severity, plugin, msg string) bool {
	for _, d := range diags {
		if d.Severity == sev && d.Plugin == plugin && d.Message == msg {
			return true
		}
	}
	return false
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
			Params: []shape.Param{{Key: "tag"}},
			Validate: func(attachments []shape.MixinAttachment) []shape.MixinViolation {
				captured = attachments
				return nil
			},
		}
		fn := contractFn("X",
			&sdk.Directive{
				Name: shape.MixinDirectiveName,
				Args: []string{"tagged"},
				KV:   map[string]string{"tag": "important"},
			},
		)
		pkg := &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{fn},
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

// TestValidator_RequiredParams covers the declarative half of what
// [shape.Mixin.Validate] could only enforce by hand: a directive that
// omits a key its classification's own sentence needs is reported,
// and a complete declaration is not — however it was assembled.
func TestValidator_RequiredParams(t *testing.T) {
	t.Parallel()

	// ordered stands in for orderafter: two required keys, one
	// resolvable kind each is irrelevant here since nothing resolves.
	ordered := shape.Mixin{
		Name: "ordered",
		Params: []shape.Param{
			{Key: "fn", Required: true},
			{Key: "unready", Required: true},
		},
	}

	t.Run("a directive omitting a required key is reported", func(t *testing.T) {
		t.Parallel()
		fn := contractFn("X", &sdk.Directive{
			Name: shape.MixinDirectiveName,
			Args: []string{"ordered"},
			KV:   map[string]string{"fn": "Initialise"},
		})
		diags := runMixinPipeline(t, ordered, &sdk.Package{
			Name: "x", Path: "x", Functions: []*sdk.Function{fn},
		})
		assertHasError(t, diags, `shape.mixin "ordered": requires unready=`)
	})

	t.Run("a declaration split across lines is judged whole", func(t *testing.T) {
		t.Parallel()
		// The reason the check reads folded stamps rather than any one
		// directive: each line alone is incomplete, and the pair is one
		// legitimate attachment.
		fn := contractFn("X",
			&sdk.Directive{
				Name: shape.MixinDirectiveName,
				Args: []string{"ordered"},
				KV:   map[string]string{"fn": "Initialise"},
			},
			&sdk.Directive{
				Name: shape.MixinDirectiveName,
				Args: []string{"ordered"},
				KV:   map[string]string{"unready": "ErrNotReady"},
			},
		)
		diags := runMixinPipeline(t, ordered, &sdk.Package{
			Name: "x", Path: "x", Functions: []*sdk.Function{fn},
		})
		assertNoError(t, diags, "requires")
	})

	t.Run("an empty value counts as absent", func(t *testing.T) {
		t.Parallel()
		// The stamping pass never stamps one, so `unready=` with no
		// value and no `unready=` at all are one folded state — and
		// reporting only the second would let the first state the
		// classification without its sentence.
		fn := contractFn("X", &sdk.Directive{
			Name: shape.MixinDirectiveName,
			Args: []string{"ordered"},
			KV:   map[string]string{"fn": "Initialise", "unready": ""},
		})
		diags := runMixinPipeline(t, ordered, &sdk.Package{
			Name: "x", Path: "x", Functions: []*sdk.Function{fn},
		})
		assertHasError(t, diags, `shape.mixin "ordered": requires unready=`)
	})

	t.Run("a required contract param binds only on its role", func(t *testing.T) {
		t.Parallel()
		// The producer arm must name the handle's reader; the reader
		// arm IS the reader, and a required key leaking across roles
		// would report every correct method-arm directive.
		spec := shape.Contract{
			Name:  "walker",
			Roles: []string{"step", "open"},
			Params: []shape.Param{
				{Key: "step", Role: "open", Required: true},
			},
		}
		hostedBy := func(role string) *sdk.Function {
			return contractFn("On"+role, &sdk.Directive{
				Name: shape.ContractDirectiveName,
				Args: []string{"walker"},
				KV:   map[string]string{"role": role},
			})
		}

		diags := runFullPipeline(t, &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{hostedBy("open")},
		}, spec)
		assertHasError(t, diags, `shape.contract "walker": role "open" requires step=`)

		diags = runFullPipeline(t, &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{hostedBy("step")},
		}, spec)
		assertNoError(t, diags, "requires step=")
	})

	t.Run("an unrequired key stays optional", func(t *testing.T) {
		t.Parallel()
		relaxed := shape.Mixin{
			Name:   "relaxed",
			Params: []shape.Param{{Key: "hint"}},
		}
		fn := contractFn("X", &sdk.Directive{
			Name: shape.MixinDirectiveName,
			Args: []string{"relaxed"},
		})
		diags := runMixinPipeline(t, relaxed, &sdk.Package{
			Name: "x", Path: "x", Functions: []*sdk.Function{fn},
		})
		assertNoError(t, diags, "requires")
	})
}

// assertHasError fails unless an error diagnostic contains want.
func assertHasError(t *testing.T, diags []sdk.Diag, want string) {
	t.Helper()
	for _, d := range diags {
		if d.Severity == sdk.SeverityError && strings.Contains(d.Message, want) {
			return
		}
	}
	t.Fatalf("no error diagnostic containing %q; got %+v", want, diags)
}

// assertNoError fails when any error diagnostic contains fragment.
func assertNoError(t *testing.T, diags []sdk.Diag, fragment string) {
	t.Helper()
	for _, d := range diags {
		if d.Severity == sdk.SeverityError && strings.Contains(d.Message, fragment) {
			t.Fatalf("unexpected diagnostic %q", d.Message)
		}
	}
}

// runMixinPipeline runs the umbrella → resolver → validator
// sequence with m as the sole registered mixin. Mirrors
// [runFullPipeline] but takes a [shape.Mixin] instead of a
// [shape.Contract].
func runMixinPipeline(t *testing.T, m shape.Mixin, pkg *sdk.Package) []sdk.Diag {
	t.Helper()
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")

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
// contracts registered on all three. Returns the accumulated
// diagnostic snapshot.
//
// Variadic because [Validator.AfterNodes] walks the accumulated
// member sets as a cascade: proving one contract does not
// suppress another needs at least two registered at once.
func runFullPipeline(t *testing.T, pkg *sdk.Package, cs ...shape.Contract) []sdk.Diag {
	t.Helper()
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")

	umbrella := shape.New().Contracts(cs...)
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
// of sdk.CapabilityProvider — satisfying nothing, so the pipeline
// ignored the declared ordering and ran all three in the default
// bucket, in registration order. The suite's completeness check
// catches that shape directly; wiring the plugins into the suite is
// what makes the check reach them.
func TestValidatorConformance(t *testing.T) {
	t.Parallel()
	plugintest.RunSuite(t, shape.New().Validator())
}
