// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape_test

import (
	"reflect"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/meta"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// atomicMixin is the canonical zero-param test mixin used by the
// presence-only stamping tests below.
func atomicMixin() shape.Mixin {
	return shape.Mixin{Name: "atomic"}
}

// rateLimitedMixin is the canonical multi-param test mixin used
// by the parameter-stamping tests below.
func rateLimitedMixin() shape.Mixin {
	return shape.Mixin{
		Name:   "rate-limited",
		Params: []string{"limit", "burst"},
	}
}

// TestMixin_DirectiveStamping covers the umbrella plugin's
// mixin stamping: each non-negated `+gen:mixin` directive on a
// callable appends to the mixins list and stamps its per-param
// keys without interfering with structural shape stamps.
func TestMixin_DirectiveStamping(t *testing.T) {
	t.Parallel()

	t.Run("stamps a parameter-less mixin", func(t *testing.T) {
		t.Parallel()
		fn := mixinFn(
			"Save",
			&directive.Directive{
				Name: shape.MixinDirectiveName,
				Args: []string{"atomic"},
			},
		)
		runAnnotate(t, shape.New().Mixins(atomicMixin()), pkgWithFunction(fn))

		assertMixins(t, fn.Meta(), []string{"atomic"})
	})

	t.Run("stamps parameter values under the mixin's namespace", func(t *testing.T) {
		t.Parallel()
		fn := mixinFn(
			"Charge",
			&directive.Directive{
				Name: shape.MixinDirectiveName,
				Args: []string{"rate-limited"},
				KV:   map[string]string{"limit": "100", "burst": "10"},
			},
		)
		runAnnotate(t, shape.New().Mixins(rateLimitedMixin()), pkgWithFunction(fn))

		assertMixins(t, fn.Meta(), []string{"rate-limited"})
		assertMeta(t, fn.Meta(), shape.MixinParamKey("rate-limited", "limit"), "100")
		assertMeta(t, fn.Meta(), shape.MixinParamKey("rate-limited", "burst"), "10")
	})

	t.Run("multiple mixins on one callable stack in declaration order", func(t *testing.T) {
		t.Parallel()
		fn := mixinFn(
			"Save",
			&directive.Directive{Name: shape.MixinDirectiveName, Args: []string{"atomic"}},
			&directive.Directive{
				Name: shape.MixinDirectiveName,
				Args: []string{"rate-limited"},
				KV:   map[string]string{"limit": "50"},
			},
		)
		runAnnotate(
			t,
			shape.New().Mixins(atomicMixin(), rateLimitedMixin()),
			pkgWithFunction(fn),
		)

		assertMixins(t, fn.Meta(), []string{"atomic", "rate-limited"})
		assertMeta(t, fn.Meta(), shape.MixinParamKey("rate-limited", "limit"), "50")
	})

	t.Run("mixin stamps alongside contract membership and structural shape", func(t *testing.T) {
		t.Parallel()
		// Reader-shaped callable, with both a contract membership
		// and a mixin attached. All three stamps must land.
		fn := readerFunc("Find")
		fn.DirectiveList = []*directive.Directive{
			{
				Name: shape.ContractDirectiveName,
				Args: []string{"tx"},
				KV:   map[string]string{"role": "begin"},
			},
			{Name: shape.MixinDirectiveName, Args: []string{"atomic"}},
		}
		runAnnotate(
			t,
			shape.New().
				Detectors(testReaderDetector()).
				Contracts(txContract()).
				Mixins(atomicMixin()),
			pkgWithFunction(fn),
		)

		assertShape(t, fn.Meta(), "reader")
		assertContracts(t, fn.Meta(), []string{"tx"})
		assertMixins(t, fn.Meta(), []string{"atomic"})
	})

	t.Run("unknown mixin name is silently skipped", func(t *testing.T) {
		t.Parallel()
		fn := mixinFn(
			"X",
			&directive.Directive{
				Name: shape.MixinDirectiveName,
				Args: []string{"never-registered"},
			},
		)
		runAnnotate(t, shape.New(), pkgWithFunction(fn))
		if got := shape.Mixins(fn.Meta()); len(got) != 0 {
			t.Fatalf("expected no mixin stamps; got %v", got)
		}
	})

	t.Run("negated directive is ignored", func(t *testing.T) {
		t.Parallel()
		fn := mixinFn(
			"Save",
			&directive.Directive{
				Name:    shape.MixinDirectiveName,
				Args:    []string{"atomic"},
				Negated: true,
			},
		)
		runAnnotate(t, shape.New().Mixins(atomicMixin()), pkgWithFunction(fn))
		if got := shape.Mixins(fn.Meta()); len(got) != 0 {
			t.Fatalf("expected negated directive to be ignored; got %v", got)
		}
	})

	t.Run("empty parameter values are skipped", func(t *testing.T) {
		t.Parallel()
		fn := mixinFn(
			"Charge",
			&directive.Directive{
				Name: shape.MixinDirectiveName,
				Args: []string{"rate-limited"},
				KV:   map[string]string{"limit": "100", "burst": ""},
			},
		)
		runAnnotate(t, shape.New().Mixins(rateLimitedMixin()), pkgWithFunction(fn))

		assertMeta(t, fn.Meta(), shape.MixinParamKey("rate-limited", "limit"), "100")
		if _, ok := shape.MixinParamKey("rate-limited", "burst").Get(fn.Meta()); ok {
			t.Fatalf("expected empty parameter value to be unstamped")
		}
	})

	t.Run("duplicate mixin directive does not duplicate the list entry", func(t *testing.T) {
		t.Parallel()
		fn := mixinFn(
			"Save",
			&directive.Directive{Name: shape.MixinDirectiveName, Args: []string{"atomic"}},
			&directive.Directive{Name: shape.MixinDirectiveName, Args: []string{"atomic"}},
		)
		runAnnotate(t, shape.New().Mixins(atomicMixin()), pkgWithFunction(fn))
		assertMixins(t, fn.Meta(), []string{"atomic"})
	})

	t.Run("method-bound mixins stamp the same as free functions", func(t *testing.T) {
		t.Parallel()
		m := &node.Method{
			Name: "Save",
			BaseNode: node.BaseNode{
				DirectiveList: []*directive.Directive{
					{Name: shape.MixinDirectiveName, Args: []string{"atomic"}},
				},
			},
		}
		s := &node.Struct{Name: "Repo", Package: "x", Methods: []*node.Method{m}}
		runAnnotate(t, shape.New().Mixins(atomicMixin()), pkgWithStruct(s))

		assertMixins(t, m.Meta(), []string{"atomic"})
	})

	t.Run("Mixins helper returns nil for an unstamped bag", func(t *testing.T) {
		t.Parallel()
		if got := shape.Mixins(nil); got != nil {
			t.Fatalf("Mixins(nil) = %v, want nil", got)
		}
		if got := shape.Mixins(meta.NewBag()); got != nil {
			t.Fatalf("Mixins(empty) = %v, want nil", got)
		}
	})
}

// mixinFn returns a free-function node carrying the supplied
// directives — used by every test that exercises directive-driven
// mixin stamping.
func mixinFn(name string, dirs ...*directive.Directive) *node.Function {
	return &node.Function{
		Name: name, Package: "x",
		BaseNode: node.BaseNode{DirectiveList: dirs},
	}
}

// assertMixins fails the test when the mixin list stamped on bag
// does not deep-equal want.
func assertMixins(t *testing.T, bag *meta.Bag, want []string) {
	t.Helper()
	got := shape.Mixins(bag)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Mixins = %v, want %v", got, want)
	}
}

// TestMixin_SiblingResolution covers the resolver rewriting
// declared [shape.Mixin.SiblingParams] values from raw names to
// qualified names — exercising the per-mixin sibling-resolution
// pass added alongside the contract resolver.
func TestMixin_SiblingResolution(t *testing.T) {
	t.Parallel()
	rafw := shape.Mixin{
		Name:          "readafterwrite",
		Params:        []string{"write"},
		SiblingParams: []string{"write"},
	}
	find := mixinFn("Find", &directive.Directive{
		Name: shape.MixinDirectiveName,
		Args: []string{"readafterwrite"},
		KV:   map[string]string{"write": "Save"},
	})
	save := &node.Function{Name: "Save", Package: "x"}
	pkg := &node.Package{
		Name: "x", Path: "x",
		Functions: []*node.Function{find, save},
	}

	umbrella := shape.New().Mixins(rafw)
	ctx := contracttestCtxForMixin(t, pkg)
	if err := umbrella.Annotate(ctx); err != nil {
		t.Fatalf("umbrella.Annotate: %v", err)
	}
	if err := umbrella.Resolver().Annotate(ctx); err != nil {
		t.Fatalf("resolver.Annotate: %v", err)
	}

	got, _ := shape.MixinParamKey("readafterwrite", "write").Get(find.Meta())
	if got != "x.Save" {
		t.Fatalf("mixin sibling param = %q, want %q", got, "x.Save")
	}
}

// TestMixin_Validate covers the validator invoking the
// [shape.Mixin.Validate] hook after sibling resolution. The
// flagging mixin emits one violation per attachment; the
// validator surfaces it as a positioned diagnostic.
func TestMixin_Validate(t *testing.T) {
	t.Parallel()
	flagging := shape.Mixin{
		Name: "flagging",
		Validate: func(attachments []shape.MixinAttachment) []shape.MixinViolation {
			out := make([]shape.MixinViolation, 0, len(attachments))
			for _, a := range attachments {
				out = append(out, shape.MixinViolation{
					Host: a.Host, Message: "synthetic flag",
				})
			}
			return out
		},
	}
	fn := mixinFn("X", &directive.Directive{
		Name: shape.MixinDirectiveName,
		Args: []string{"flagging"},
	})
	pkg := &node.Package{Name: "x", Path: "x", Functions: []*node.Function{fn}}
	ctx := contracttestCtxForMixin(t, pkg)
	umbrella := shape.New().Mixins(flagging)
	if err := umbrella.Annotate(ctx); err != nil {
		t.Fatalf("umbrella.Annotate: %v", err)
	}
	if err := umbrella.Resolver().Annotate(ctx); err != nil {
		t.Fatalf("resolver.Annotate: %v", err)
	}
	if err := umbrella.Validator().Annotate(ctx); err != nil {
		t.Fatalf("validator.Annotate: %v", err)
	}
	assertContainsDiag(t, ctx.Diag.Diagnostics(), diag.Error, "synthetic flag")
}

// contracttestCtxForMixin builds an annotator context backed by a
// fresh store seeded with pkg and stamped with the "golang"
// frontend marker. Used by the mixin pipeline tests above.
func contracttestCtxForMixin(t *testing.T, pkg *node.Package) *sdk.AnnotatorContext {
	t.Helper()
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")
	return &sdk.AnnotatorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   diag.New(),
	}
}

// annotateCapturing runs p over a package holding fn and returns
// the diagnostic sink, so tests can assert on what the mixin
// stamping pass reported as well as what it stamped.
func annotateCapturing(t *testing.T, p *shape.Plugin, fn *node.Function) *diag.Sink {
	t.Helper()
	pkg := pkgWithFunction(fn)
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")
	ctx := newAnnotatorContext(t, s)
	if err := p.Annotate(ctx); err != nil {
		t.Fatalf("Annotate: %v", err)
	}
	return ctx.Diag
}

func TestMixin_MultipleNamesPerDirective(t *testing.T) {
	t.Parallel()

	t.Run("all names on one directive are stamped in written order", func(t *testing.T) {
		t.Parallel()
		fn := mixinFn("Put", &directive.Directive{
			Name: shape.MixinDirectiveName,
			Args: []string{"rate-limited", "atomic"},
		})
		sink := annotateCapturing(t, shape.New().Mixins(atomicMixin(), rateLimitedMixin()), fn)

		assertMixins(t, fn.Meta(), []string{"rate-limited", "atomic"})
		assertNoErrors(t, sink)
	})

	t.Run("an unregistered name does not discard the names after it", func(t *testing.T) {
		t.Parallel()
		// Regression: the pre-variadic handler skipped the whole
		// directive on an unregistered name. Nested under a per-name
		// loop that would silently drop every later name on the line.
		fn := mixinFn("Put", &directive.Directive{
			Name: shape.MixinDirectiveName,
			Args: []string{"atomic", "no-such-mixin", "rate-limited"},
		})
		annotateCapturing(t, shape.New().Mixins(atomicMixin(), rateLimitedMixin()), fn)

		assertMixins(t, fn.Meta(), []string{"atomic", "rate-limited"})
	})

	t.Run("repeated names collapse to one entry", func(t *testing.T) {
		t.Parallel()
		fn := mixinFn(
			"Put",
			&directive.Directive{
				Name: shape.MixinDirectiveName,
				Args: []string{"atomic", "atomic"},
			},
			&directive.Directive{
				Name: shape.MixinDirectiveName,
				Args: []string{"atomic", "rate-limited"},
			},
		)
		annotateCapturing(t, shape.New().Mixins(atomicMixin(), rateLimitedMixin()), fn)

		assertMixins(t, fn.Meta(), []string{"atomic", "rate-limited"})
	})

	t.Run("a single name still carries its parameters", func(t *testing.T) {
		t.Parallel()
		fn := mixinFn("Charge", &directive.Directive{
			Name: shape.MixinDirectiveName,
			Args: []string{"rate-limited"},
			KV:   map[string]string{"limit": "100"},
		})
		sink := annotateCapturing(t, shape.New().Mixins(rateLimitedMixin()), fn)

		assertMixins(t, fn.Meta(), []string{"rate-limited"})
		assertMeta(t, fn.Meta(), shape.MixinParamKey("rate-limited", "limit"), "100")
		assertNoErrors(t, sink)
	})
}

func TestMixin_ParametersWithSeveralNamesAreRejected(t *testing.T) {
	t.Parallel()

	// KV ownership is undefined once several names share a line, so
	// the names attach and the parameters are dropped with an error
	// rather than being guessed onto an arbitrary owner.
	newFn := func() *node.Function {
		return mixinFn("Put", &directive.Directive{
			Name: shape.MixinDirectiveName,
			Args: []string{"rate-limited", "atomic"},
			KV:   map[string]string{"limit": "100"},
		})
	}

	t.Run("reports an error naming the offending keys and mixins", func(t *testing.T) {
		t.Parallel()
		fn := newFn()
		sink := annotateCapturing(t, shape.New().Mixins(atomicMixin(), rateLimitedMixin()), fn)

		if !sink.HasErrors() {
			t.Fatalf("parameters with several mixin names should be an error")
		}
		msg := sink.Diagnostics()[0].Message
		for _, want := range []string{"limit", "rate-limited", "atomic"} {
			if !strings.Contains(msg, want) {
				t.Errorf("diagnostic should mention %q so the author can act on it; got %q", want, msg)
			}
		}
	})

	t.Run("the names are still attached", func(t *testing.T) {
		t.Parallel()
		fn := newFn()
		annotateCapturing(t, shape.New().Mixins(atomicMixin(), rateLimitedMixin()), fn)

		assertMixins(t, fn.Meta(), []string{"rate-limited", "atomic"})
	})

	t.Run("no parameter is stamped under any of the named mixins", func(t *testing.T) {
		t.Parallel()
		fn := newFn()
		annotateCapturing(t, shape.New().Mixins(atomicMixin(), rateLimitedMixin()), fn)

		for _, name := range []string{"rate-limited", "atomic"} {
			if got, ok := shape.MixinParamKey(name, "limit").Get(fn.Meta()); ok {
				t.Errorf("parameter was stamped under %q as %q; ambiguous parameters must be dropped", name, got)
			}
		}
	})
}

// assertNoErrors fails when sink recorded any Error diagnostic.
func assertNoErrors(t *testing.T, sink *diag.Sink) {
	t.Helper()
	if sink.HasErrors() {
		t.Fatalf("unexpected diagnostics: %+v", sink.Diagnostics())
	}
}

// validateAgainstSchema runs d through the framework validator with
// the plugin's own declared schemas registered, which is the gate a
// real source directive passes before any stamping happens. The
// stamping tests above build Directive values directly and so never
// exercise it.
func validateAgainstSchema(t *testing.T, d *directive.Directive) *diag.Sink {
	t.Helper()
	reg := directive.NewRegistry()
	for _, s := range shape.New().Directives() {
		if err := reg.Register(s); err != nil {
			t.Fatalf("Register %q: %v", s.Name, err)
		}
	}
	sink := diag.New()
	directive.Validate([]*directive.Directive{d}, node.KindFunction, reg, sink.For("test"))
	return sink
}

func TestMixin_SchemaAcceptsSeveralPositionals(t *testing.T) {
	t.Parallel()

	t.Run("several mixin names pass validation", func(t *testing.T) {
		t.Parallel()
		sink := validateAgainstSchema(t, &directive.Directive{
			Name: shape.MixinDirectiveName,
			Args: []string{"idempotent", "concurrent", "atomic", "bounded"},
		})
		if sink.HasErrors() {
			t.Fatalf("multi-name +gen:mixin should validate; got %+v", sink.Diagnostics())
		}
	})

	t.Run("a single mixin name still passes validation", func(t *testing.T) {
		t.Parallel()
		sink := validateAgainstSchema(t, &directive.Directive{
			Name: shape.MixinDirectiveName,
			Args: []string{"idempotent"},
		})
		if sink.HasErrors() {
			t.Fatalf("single-name +gen:mixin should validate; got %+v", sink.Diagnostics())
		}
	})

	t.Run("a mixin directive with no name is rejected", func(t *testing.T) {
		t.Parallel()
		// AllowExtraPositional widens the upper bound only. The name
		// slot is now marked Required: previously a bare +gen:mixin
		// passed validation and was then silently dropped by the
		// stamping pass, so the mistake had no surface at all.
		sink := validateAgainstSchema(t, &directive.Directive{
			Name: shape.MixinDirectiveName,
		})
		if !sink.HasErrors() {
			t.Fatalf("+gen:mixin with no name should be rejected")
		}
	})

	t.Run("the negated form is rejected", func(t *testing.T) {
		t.Parallel()
		// Mixins attach only through an explicit +gen:mixin, so there
		// is nothing for -gen:mixin to remove. It previously parsed
		// and did nothing, which reads as a working suppression.
		sink := validateAgainstSchema(t, &directive.Directive{
			Name:    shape.MixinDirectiveName,
			Args:    []string{"atomic"},
			Negated: true,
		})
		if !sink.HasErrors() {
			t.Fatalf("-gen:mixin should be rejected rather than silently ignored")
		}
	})

	t.Run("contract is deliberately left single-name", func(t *testing.T) {
		t.Parallel()
		// A role binds to exactly one contract, so batching contract
		// names would be ambiguous in a way mixin names are not.
		sink := validateAgainstSchema(t, &directive.Directive{
			Name: shape.ContractDirectiveName,
			Args: []string{"outbox", "saga"},
			KV:   map[string]string{"role": "writer"},
		})
		if !sink.HasErrors() {
			t.Fatalf("+gen:contract should still reject several names")
		}
	})
}

func TestMixin_UnregisteredNameIsReported(t *testing.T) {
	t.Parallel()

	t.Run("a name with no registered mixin is an error", func(t *testing.T) {
		t.Parallel()
		fn := mixinFn("Put", &directive.Directive{
			Name: shape.MixinDirectiveName,
			Args: []string{"idempotant"}, // typo
		})
		sink := annotateCapturing(t, shape.New().Mixins(atomicMixin()), fn)

		if !sink.HasErrors() {
			t.Fatalf("an unregistered mixin name should be reported, not silently dropped")
		}
		if got := sink.Diagnostics()[0].Message; !strings.Contains(got, "idempotant") {
			t.Errorf("diagnostic should name the offending mixin; got %q", got)
		}
		assertMixins(t, fn.Meta(), nil)
	})

	t.Run("a positional written as a parameter surfaces as an unknown name", func(t *testing.T) {
		t.Parallel()
		// Mixin parameters are KV-only. Batching means every bare
		// token is a name, so `bounded 100` reads as two mixins.
		// Before batching this failed as an arity error; the
		// unregistered-name report is what keeps it loud.
		fn := mixinFn("Put", &directive.Directive{
			Name: shape.MixinDirectiveName,
			Args: []string{"rate-limited", "100"},
		})
		sink := annotateCapturing(t, shape.New().Mixins(rateLimitedMixin()), fn)

		if !sink.HasErrors() {
			t.Fatalf("a stray positional should be reported rather than silently ignored")
		}
		msg := sink.Diagnostics()[0].Message
		if !strings.Contains(msg, `"100"`) {
			t.Errorf("diagnostic should quote the stray token; got %q", msg)
		}
		// The hint has to mention the KV form, since the author's
		// actual mistake is not "unknown mixin" but "wrong syntax".
		if !strings.Contains(msg, "key=value") {
			t.Errorf("diagnostic should point at the key=value form; got %q", msg)
		}
		// The legitimate name on the line still attaches.
		assertMixins(t, fn.Meta(), []string{"rate-limited"})
	})

	t.Run("the stray positional is not stamped as a mixin", func(t *testing.T) {
		t.Parallel()
		fn := mixinFn("Put", &directive.Directive{
			Name: shape.MixinDirectiveName,
			Args: []string{"rate-limited", "100"},
		})
		annotateCapturing(t, shape.New().Mixins(rateLimitedMixin()), fn)

		for _, m := range shape.Mixins(fn.Meta()) {
			if m == "100" {
				t.Fatalf("an unregistered name must not reach MetaMixins")
			}
		}
	})
}

func TestMixin_NamelessDirectiveIsIgnoredByStamping(t *testing.T) {
	t.Parallel()

	// The schema marks the name Required, so this cannot arrive from
	// parsed source. It can arrive from a caller that builds
	// Directive values directly — which is how plugins are unit
	// tested — so the stamping pass guards rather than indexing
	// Args[0] blindly.
	t.Run("a directive with no name stamps nothing and does not panic", func(t *testing.T) {
		t.Parallel()
		fn := mixinFn("Put", &directive.Directive{Name: shape.MixinDirectiveName})
		sink := annotateCapturing(t, shape.New().Mixins(atomicMixin()), fn)

		assertMixins(t, fn.Meta(), nil)
		assertNoErrors(t, sink)
	})
}

// retryingMixin is the many-parameter fixture for the blank-value
// skip below. Six declared parameters give a single directive room
// to interleave populated and blank values, which is what makes the
// skip observable no matter which order the parameter map is walked
// in.
func retryingMixin() shape.Mixin {
	return shape.Mixin{
		Name:   "retrying",
		Params: []string{"attempts", "backoff", "jitter", "budget", "ceiling", "floor"},
	}
}

// TestMixin_SkippedEntryDoesNotStopTheCascade pins the two skip
// points in the mixin stamping pass that a mutation audit found
// unguarded: the `continue` on an argument-less directive, and the
// `continue` on a blank parameter value. Rewriting either to
// `break` survived the whole suite, because every fixture above
// exercises the skip with nothing behind it — and a one-entry
// fixture cannot tell `continue` from `break`.
//
// Each subtest therefore puts a skipped entry AHEAD of an entry
// that must still be processed, and names the survivor it expects.
// That ordering is the entire assertion; do not fold these cases
// back into the single-entry tests above, and do not delete them as
// duplicates of the "nameless directive" or "empty parameter
// values" cases, neither of which constrains what happens next.
//
// The defect this guards is not hypothetical: the reader detector
// went months never winning dispatch because a sibling swallowed
// the cascade, under a green suite.
func TestMixin_SkippedEntryDoesNotStopTheCascade(t *testing.T) {
	t.Parallel()

	t.Run("an argument-less directive does not discard the mixins declared after it", func(t *testing.T) {
		t.Parallel()
		// The argument-less line is skipped, not fatal: both
		// directives written below it still have to stamp, values
		// included.
		fn := mixinFn(
			"Put",
			&directive.Directive{Name: shape.MixinDirectiveName},
			&directive.Directive{Name: shape.MixinDirectiveName, Args: []string{"atomic"}},
			&directive.Directive{
				Name: shape.MixinDirectiveName,
				Args: []string{"rate-limited"},
				KV:   map[string]string{"limit": "100"},
			},
		)
		sink := annotateCapturing(t, shape.New().Mixins(atomicMixin(), rateLimitedMixin()), fn)

		assertMixins(t, fn.Meta(), []string{"atomic", "rate-limited"})
		assertMeta(t, fn.Meta(), shape.MixinParamKey("rate-limited", "limit"), "100")
		assertNoErrors(t, sink)
	})

	t.Run("a blank parameter value does not discard the parameters after it", func(t *testing.T) {
		t.Parallel()
		// Parameters arrive in a map, so the pass walks them in an
		// order the runtime deliberately randomises. Interleaving the
		// blanks between the populated keys is what makes the
		// assertion sound rather than lucky: no walk order reaches all
		// three populated keys before meeting a blank, so aborting on
		// the first blank always loses at least one.
		//
		// The pass is still repeated, because that argument leans on
		// how small maps happen to be ordered today. Repetition holds
		// the assertion up even if that ordering becomes a true
		// shuffle, where one pass would leak with probability 3!·3!/6!.
		const passes = 16

		blanks := []string{"backoff", "budget", "floor"}
		for range passes {
			fn := mixinFn("Charge", &directive.Directive{
				Name: shape.MixinDirectiveName,
				Args: []string{"retrying"},
				KV: map[string]string{
					"attempts": "3",
					"backoff":  "",
					"jitter":   "0.2",
					"budget":   "",
					"ceiling":  "30s",
					"floor":    "",
				},
			})
			annotateCapturing(t, shape.New().Mixins(retryingMixin()), fn)

			assertMeta(t, fn.Meta(), shape.MixinParamKey("retrying", "attempts"), "3")
			assertMeta(t, fn.Meta(), shape.MixinParamKey("retrying", "jitter"), "0.2")
			assertMeta(t, fn.Meta(), shape.MixinParamKey("retrying", "ceiling"), "30s")
			for _, blank := range blanks {
				if got, ok := shape.MixinParamKey("retrying", blank).Get(fn.Meta()); ok {
					t.Fatalf("blank parameter %q was stamped as %q; a blank value must be skipped, not stamped",
						blank, got)
				}
			}
		}
	})
}
