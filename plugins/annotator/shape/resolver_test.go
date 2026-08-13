// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package shape_test

import (
	"maps"
	"slices"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/eidostest/plugintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/sdk"
	"go.thesmos.sh/eidos/store"
)

// TestResolver_Contract pins the role / capability / hook surface
// the framework asserts at registration / build time.
func TestResolver_Contract(t *testing.T) {
	t.Parallel()

	t.Run("name is stable", func(t *testing.T) {
		t.Parallel()
		r := shape.New().Resolver()
		if got := r.Name(); got == "" {
			t.Fatalf("Name() returned empty string")
		}
		first := r.Name()
		second := r.Name()
		if first != second {
			t.Fatalf("Name() flapped: first=%q, second=%q", first, second)
		}
	})

	t.Run("priority is AnnotatorRefinement", func(t *testing.T) {
		t.Parallel()
		if got, want := shape.New().Resolver().Priority(), sdk.AnnotatorRefinement; got != want {
			t.Fatalf("Priority() = %v, want %v", got, want)
		}
	})

	t.Run("satisfies sdk.Annotator", func(t *testing.T) {
		t.Parallel()
		var _ sdk.Annotator = shape.New().Resolver()
	})

	t.Run("Annotate on empty store does not panic", func(t *testing.T) {
		t.Parallel()
		ctx := newAnnotatorContext(t, store.New())
		if err := shape.New().Resolver().Annotate(ctx); err != nil {
			t.Fatalf("Annotate(empty): %v", err)
		}
	})
}

// TestResolver_NameRewrite covers the resolver's primary job —
// rewriting raw partner names into qualified names sourced from
// the same scope as the host callable.
func TestResolver_NameRewrite(t *testing.T) {
	t.Parallel()

	t.Run("same-struct method partner resolves to ownerQName.method", func(t *testing.T) {
		t.Parallel()
		begin := contractMethod(
			"Begin",
			contractDirective("tx", "begin", map[string]string{"commit": "Commit"}),
		)
		commit := &sdk.Method{Name: "Commit"}
		s := &sdk.Struct{
			Name: "Repo", Package: "x",
			Methods: []*sdk.Method{begin, commit},
		}
		runWithResolver(t, txContract(), pkgWithStruct(s))

		assertMeta(t, begin.Meta(),
			shape.ContractPartnerKey("tx", "commit"),
			"x.Repo.Commit")
	})

	t.Run("same-interface method partner resolves to ownerQName.method", func(t *testing.T) {
		t.Parallel()
		begin := contractMethod(
			"Begin",
			contractDirective("tx", "begin", map[string]string{"commit": "Commit"}),
		)
		commit := &sdk.Method{Name: "Commit"}
		i := &sdk.Interface{
			Name: "Repo", Package: "x",
			Methods: []*sdk.Method{begin, commit},
		}
		runWithResolver(t, txContract(), pkgWithInterface(i))

		assertMeta(t, begin.Meta(),
			shape.ContractPartnerKey("tx", "commit"),
			"x.Repo.Commit")
	})

	t.Run("same-package free-function partner resolves to package.function", func(t *testing.T) {
		t.Parallel()
		begin := contractFn(
			"Begin",
			contractDirective("tx", "begin", map[string]string{"commit": "Commit"}),
		)
		commit := &sdk.Function{Name: "Commit", Package: "x"}
		pkg := &sdk.Package{
			Name: "x", Path: "x",
			Functions: []*sdk.Function{begin, commit},
		}
		runWithResolver(t, txContract(), pkg)

		assertMeta(t, begin.Meta(),
			shape.ContractPartnerKey("tx", "commit"),
			"x.Commit")
	})

	t.Run("partner that does not exist surfaces a diagnostic", func(t *testing.T) {
		t.Parallel()
		begin := contractMethod(
			"Begin",
			contractDirective("tx", "begin", map[string]string{"commit": "NonExistent"}),
		)
		s := &sdk.Struct{Name: "Repo", Package: "x", Methods: []*sdk.Method{begin}}

		diags := runWithResolverDiags(t, txContract(), pkgWithStruct(s))
		assertContainsDiag(t, diags, sdk.SeverityError, "NonExistent")
	})

	t.Run("rewrite is idempotent: second pass leaves qname unchanged", func(t *testing.T) {
		t.Parallel()
		begin := contractMethod(
			"Begin",
			contractDirective("tx", "begin", map[string]string{"commit": "Commit"}),
		)
		commit := &sdk.Method{Name: "Commit"}
		s := &sdk.Struct{Name: "Repo", Package: "x", Methods: []*sdk.Method{begin, commit}}

		s2, p := setupResolverPipeline(t, txContract(), pkgWithStruct(s))
		runPlugins(t, s2, p)
		first, _ := shape.ContractPartnerKey("tx", "commit").Get(begin.Meta())
		runPlugins(t, s2, p)
		second, _ := shape.ContractPartnerKey("tx", "commit").Get(begin.Meta())
		if first != second {
			t.Fatalf("partner rewrite not idempotent: first=%q second=%q", first, second)
		}
	})

	t.Run("qualified partner names are accepted as-is and back-stamped cross-scope", func(t *testing.T) {
		t.Parallel()
		begin := contractMethod(
			"Begin",
			contractDirective("tx", "begin", map[string]string{
				"commit": "x.Other.Commit",
			}),
		)
		other := &sdk.Struct{
			Name: "Other", Package: "x",
			Methods: []*sdk.Method{{Name: "Commit"}},
		}
		repo := &sdk.Struct{
			Name: "Repo", Package: "x",
			Methods: []*sdk.Method{begin},
		}
		runWithResolver(t, txContract(), &sdk.Package{
			Name: "x", Path: "x",
			Structs: []*sdk.Struct{repo, other},
		})

		// Host stamp is preserved verbatim (no rewriting).
		assertMeta(t, begin.Meta(),
			shape.ContractPartnerKey("tx", "commit"),
			"x.Other.Commit")
		// And the cross-scope partner gets back-stamped.
		assertMeta(t, other.Methods[0].Meta(),
			shape.ContractRoleKey("tx"), "commit")
	})
}

// TestResolver_BackStamp covers the resolver's second job —
// stamping the contract membership and the back-pointer onto the
// resolved partner callable.
func TestResolver_BackStamp(t *testing.T) {
	t.Parallel()

	t.Run("partner gets contract membership and its own role", func(t *testing.T) {
		t.Parallel()
		begin := contractMethod(
			"Begin",
			contractDirective("tx", "begin", map[string]string{"commit": "Commit"}),
		)
		commit := &sdk.Method{Name: "Commit"}
		s := &sdk.Struct{Name: "Repo", Package: "x", Methods: []*sdk.Method{begin, commit}}
		runWithResolver(t, txContract(), pkgWithStruct(s))

		if got := shape.Contracts(commit.Meta()); len(got) != 1 || got[0] != "tx" {
			t.Fatalf("partner Contracts = %v, want [tx]", got)
		}
		assertMeta(t, commit.Meta(), shape.ContractRoleKey("tx"), "commit")
	})

	t.Run("partner gets reverse partner pointer to the originating callable", func(t *testing.T) {
		t.Parallel()
		begin := contractMethod(
			"Begin",
			contractDirective("tx", "begin", map[string]string{"commit": "Commit"}),
		)
		commit := &sdk.Method{Name: "Commit"}
		s := &sdk.Struct{Name: "Repo", Package: "x", Methods: []*sdk.Method{begin, commit}}
		runWithResolver(t, txContract(), pkgWithStruct(s))

		assertMeta(t, commit.Meta(),
			shape.ContractPartnerKey("tx", "begin"),
			"x.Repo.Begin")
	})

	t.Run("partner that self-stamped its role is not overwritten", func(t *testing.T) {
		t.Parallel()
		begin := contractMethod(
			"Begin",
			contractDirective("tx", "begin", map[string]string{"commit": "Commit"}),
		)
		// Commit self-stamps with role=commit AND its own partner
		// pointer; the resolver must preserve those stamps.
		commit := contractMethod(
			"Commit",
			contractDirective("tx", "commit", map[string]string{"begin": "Begin"}),
		)
		s := &sdk.Struct{Name: "Repo", Package: "x", Methods: []*sdk.Method{begin, commit}}
		runWithResolver(t, txContract(), pkgWithStruct(s))

		assertMeta(t, commit.Meta(), shape.ContractRoleKey("tx"), "commit")
		assertMeta(t, commit.Meta(),
			shape.ContractPartnerKey("tx", "begin"),
			"x.Repo.Begin")
		// And the Begin's commit partner is the qname of Commit
		assertMeta(t, begin.Meta(),
			shape.ContractPartnerKey("tx", "commit"),
			"x.Repo.Commit")
	})
}

// TestResolver_Diagnostics pins the validation failures that
// surface as positioned diagnostics rather than panics or silent
// misbehaviour.
func TestResolver_Diagnostics(t *testing.T) {
	t.Parallel()

	t.Run("unknown self-role surfaces a diagnostic", func(t *testing.T) {
		t.Parallel()
		fn := contractFn(
			"X",
			contractDirective("tx", "no-such-role", nil),
		)
		diags := runWithResolverDiags(t, txContract(), pkgWithFunction(fn))
		assertContainsDiag(t, diags, sdk.SeverityError, "no-such-role")
	})

	t.Run("unknown partner role surfaces a diagnostic", func(t *testing.T) {
		t.Parallel()
		begin := contractMethod(
			"Begin",
			contractDirective("tx", "begin", map[string]string{"nonsense": "Foo"}),
		)
		s := &sdk.Struct{Name: "Repo", Package: "x", Methods: []*sdk.Method{begin}}
		diags := runWithResolverDiags(t, txContract(), pkgWithStruct(s))
		assertContainsDiag(t, diags, sdk.SeverityError, "nonsense")
	})

	t.Run("unregistered contract surfaces a diagnostic", func(t *testing.T) {
		t.Parallel()
		// The umbrella plugin silently skips unregistered contracts
		// (no stamp lands). To exercise the resolver's own diag
		// path we register the contract with the umbrella but a
		// different empty plugin with the resolver — the resolver
		// then sees a stamped contract name it cannot resolve.
		// In practice this happens when the umbrella plugin and
		// the resolver are configured asymmetrically (a
		// configuration error).
		fn := contractFn(
			"Begin",
			contractDirective("tx", "begin", nil),
		)
		s := store.New()
		pkg := pkgWithFunction(fn)
		if err := s.Nodes().AddPackage(pkg); err != nil {
			t.Fatalf("AddPackage: %v", err)
		}
		frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")

		umbrella := shape.New().Contracts(txContract())
		resolverOnly := shape.New() // resolver knows no contracts
		ctx := newAnnotatorContext(t, s)
		if err := umbrella.Annotate(ctx); err != nil {
			t.Fatalf("umbrella.Annotate: %v", err)
		}
		if err := resolverOnly.Resolver().Annotate(ctx); err != nil {
			t.Fatalf("resolver.Annotate: %v", err)
		}
		assertContainsDiag(t, ctx.Diag.Diagnostics(), sdk.SeverityError, "tx")
	})

	t.Run("valid contract membership emits no diagnostics", func(t *testing.T) {
		t.Parallel()
		begin := contractMethod(
			"Begin",
			contractDirective("tx", "begin", map[string]string{"commit": "Commit"}),
		)
		commit := &sdk.Method{Name: "Commit"}
		s := &sdk.Struct{Name: "Repo", Package: "x", Methods: []*sdk.Method{begin, commit}}
		diags := runWithResolverDiags(t, txContract(), pkgWithStruct(s))
		for _, d := range diags {
			if d.Severity >= sdk.SeverityError {
				t.Fatalf("unexpected error diagnostic: %+v", d)
			}
		}
	})
}

// TestResolver_DecliningEntryDoesNotStopTheRest pins the cascade
// property of the resolver's three declining loops: an entry the
// loop skips costs that entry alone, never the entries queued
// behind it.
//
// Every subtest here exists to kill one specific surviving mutant.
// A mutation audit rewrote `continue` as `break` at the
// unregistered-contract guard in [shape.Resolver] resolve, and at
// both guards in flagUnknownPartnerRoles — the foreign-meta-name
// skip and the known-role skip — and the suite stayed green. It
// stayed green because no fixture ever put a declining entry ahead
// of an entry that still had work to do: one membership per
// callable, and a bag whose very first meta name already matched
// the partner prefix. That is the same blind spot that let the
// `reader` detector lose dispatch unnoticed for months.
//
// Do not fold these rows into [TestResolver_Diagnostics] or
// [TestResolver_NameRewrite]. Their fixtures cannot tell `continue`
// from `break`, which is exactly why the mutants survived them.
func TestResolver_DecliningEntryDoesNotStopTheRest(t *testing.T) {
	t.Parallel()

	t.Run("a contract the resolver does not know does not stop the memberships after it", func(t *testing.T) {
		t.Parallel()
		// Membership order follows directive order, so outbox — the
		// one the resolver was never told about — is the first entry
		// the cascade must decline and step past to reach tx.
		begin := contractMethod(
			"Begin",
			contractDirective("outbox", "append", nil),
			contractDirective("tx", "begin", map[string]string{"commit": "Commit"}),
		)
		commit := &sdk.Method{Name: "Commit"}
		s := &sdk.Struct{Name: "Repo", Package: "x", Methods: []*sdk.Method{begin, commit}}

		diags := runSplitResolver(t,
			[]shape.Contract{outboxContract(), txContract()},
			[]shape.Contract{txContract()},
			pkgWithStruct(s))

		// The declining membership is reported, and reported alone.
		assertErrorDiags(t, diags,
			`shape: contract "outbox" is stamped on this callable but not registered with the resolver`)

		// Identity, not existence: the membership behind the decline
		// is resolved all the way through — partner ref rewritten to
		// a qname, and the partner back-stamped in return.
		assertMeta(t, begin.Meta(), shape.ContractPartnerKey("tx", "commit"), "x.Repo.Commit")
		assertMeta(t, commit.Meta(), shape.ContractRoleKey("tx"), "commit")
		assertMeta(t, commit.Meta(), shape.ContractPartnerKey("tx", "begin"), "x.Repo.Begin")
	})

	t.Run("a meta name outside the partner namespace does not stop the unknown-role scan", func(t *testing.T) {
		t.Parallel()
		// bag.Names() is lexicographically sorted, so the opaque
		// param stamp ("…tx.param.mode") lands ahead of the partner
		// stamp ("…tx.partner.watcher"): the scan has to skip a
		// foreign name before it can reach anything it can diagnose.
		begin := contractMethod(
			"Begin",
			contractDirective("tx", "begin", map[string]string{
				"mode":    "eager",
				"watcher": "Watch",
			}),
		)
		s := &sdk.Struct{Name: "Repo", Package: "x", Methods: []*sdk.Method{begin}}

		diags := runWithResolverDiags(t, txContractWithParam(), pkgWithStruct(s))

		assertErrorDiags(t, diags,
			`shape.contract "tx": partner role "watcher" is not in the declared role vocabulary `+
				`[begin commit rollback]`)
	})

	t.Run("a known partner role does not stop the scan for the unknown ones after it", func(t *testing.T) {
		t.Parallel()
		// "…partner.commit" is a declared role and sorts ahead of
		// "…partner.watcher", so the valid entry is the first thing
		// the scan must step over to reach the invalid one.
		begin := contractMethod(
			"Begin",
			contractDirective("tx", "begin", map[string]string{
				"commit":  "Commit",
				"watcher": "Watch",
			}),
		)
		commit := &sdk.Method{Name: "Commit"}
		s := &sdk.Struct{Name: "Repo", Package: "x", Methods: []*sdk.Method{begin, commit}}

		diags := runWithResolverDiags(t, txContract(), pkgWithStruct(s))

		assertErrorDiags(t, diags,
			`shape.contract "tx": partner role "watcher" is not in the declared role vocabulary `+
				`[begin commit rollback]`)

		// Stepping over the valid role in the scan is not licence to
		// leave it unresolved elsewhere.
		assertMeta(t, begin.Meta(), shape.ContractPartnerKey("tx", "commit"), "x.Repo.Commit")
	})
}

// txContractWithParam is [txContract] plus one opaque param. Tests
// use it when they need a meta name that sorts ahead of the whole
// partner namespace: bag.Names() is sorted, and
// `shape.contract.tx.param.…` precedes `shape.contract.tx.partner.…`.
func txContractWithParam() shape.Contract {
	c := txContract()
	c.Params = []shape.Param{{Key: "mode"}}
	return c
}

// runSplitResolver drives the umbrella → resolver sequence with the
// two halves configured asymmetrically: the umbrella registers
// stamped, while the resolver is built from a second plugin
// registering only resolved. This is the configuration error the
// resolver's unregistered-contract diagnostic exists to report, and
// the only way to hand the resolver a membership it cannot resolve.
func runSplitResolver(t *testing.T, stamped, resolved []shape.Contract, pkg *sdk.Package) []sdk.Diag {
	t.Helper()
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")

	ctx := newAnnotatorContext(t, s)
	if err := shape.New().Contracts(stamped...).Annotate(ctx); err != nil {
		t.Fatalf("umbrella.Annotate: %v", err)
	}
	if err := shape.New().Contracts(resolved...).Resolver().Annotate(ctx); err != nil {
		t.Fatalf("resolver.Annotate: %v", err)
	}
	return ctx.Diag.Diagnostics()
}

// assertErrorDiags fails the test unless the error-severity messages
// in diags are exactly want, in order. Stronger than
// [assertContainsDiag] on purpose: the cascade tests need to know
// which entries were reported and which were silently dropped, and a
// containment check cannot see a missing trailing diagnostic.
func assertErrorDiags(t *testing.T, diags []sdk.Diag, want ...string) {
	t.Helper()
	got := make([]string, 0, len(diags))
	for _, d := range diags {
		if d.Severity >= sdk.SeverityError {
			got = append(got, d.Message)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("error diagnostics =\n\t%q\nwant\n\t%q", got, want)
	}
}

// contractMethod builds a [sdk.Method] with the supplied
// directive list — used by every resolver test that exercises a
// method-bound contract.
func contractMethod(name string, dirs ...*sdk.Directive) *sdk.Method {
	return &sdk.Method{
		Name:     name,
		BaseNode: sdk.BaseNode{DirectiveList: dirs},
	}
}

// contractDirective constructs a `+gen:contract` [*sdk.Directive]
// from the supplied contract name, role, and (optional) partner KVs.
// The role= entry is always populated; nil kv produces a directive
// with no partner refs.
func contractDirective(name, role string, kv map[string]string) *sdk.Directive {
	out := map[string]string{"role": role}
	maps.Copy(out, kv)
	return &sdk.Directive{
		Name: shape.ContractDirectiveName,
		Args: []string{name},
		KV:   out,
	}
}

// runWithResolver wires the supplied package into a fresh store,
// then runs the umbrella plugin followed by its resolver — the
// canonical umbrella → resolver sequence — failing the test on
// any returned error.
func runWithResolver(t *testing.T, c shape.Contract, pkg *sdk.Package) {
	t.Helper()
	_ = runWithResolverDiags(t, c, pkg)
}

// runWithResolverDiags is the same wiring as [runWithResolver]
// but returns the diagnostic snapshot so callers can assert on
// emitted diags.
func runWithResolverDiags(t *testing.T, c shape.Contract, pkg *sdk.Package) []sdk.Diag {
	t.Helper()
	s, p := setupResolverPipeline(t, c, pkg)
	runPlugins(t, s, p)
	return p.diag.Diagnostics()
}

// resolverPipeline is the (store, plugins, diag-sink) bundle
// returned by [setupResolverPipeline] so individual tests can
// drive the same wiring twice (for the idempotency probe).
type resolverPipeline struct {
	umbrella *shape.Plugin
	resolver *shape.Resolver
	diag     *sdk.Sink
}

// setupResolverPipeline builds the resolver pipeline against pkg
// in a fresh store, registering c as the only contract. Returns
// the store + plugin bundle so the caller can invoke the pipeline
// itself (canonical use: idempotency tests that run twice).
func setupResolverPipeline(t *testing.T, c shape.Contract, pkg *sdk.Package) (*sdk.Store, *resolverPipeline) {
	t.Helper()
	s := store.New()
	if err := s.Nodes().AddPackage(pkg); err != nil {
		t.Fatalf("AddPackage: %v", err)
	}
	frontendMarker.Set(pkg.EnsureMeta(), "golang", "test")

	umbrella := shape.New().Contracts(c)
	return s, &resolverPipeline{
		umbrella: umbrella,
		resolver: umbrella.Resolver(),
		diag:     diag.New(),
	}
}

// runPlugins drives umbrella → resolver against s using p's diag
// sink. Both passes share the sink so diagnostics accumulate
// across both passes for collective inspection.
func runPlugins(t *testing.T, s *sdk.Store, p *resolverPipeline) {
	t.Helper()
	ctx := &sdk.AnnotatorContext{
		Store:  s,
		Reader: store.NewReader(s),
		Diag:   p.diag,
	}
	if err := p.umbrella.Annotate(ctx); err != nil {
		t.Fatalf("umbrella.Annotate: %v", err)
	}
	if err := p.resolver.Annotate(ctx); err != nil {
		t.Fatalf("resolver.Annotate: %v", err)
	}
}

// assertContainsDiag fails the test when no diagnostic in diags
// matches both sev and contains substr in its message. The error
// includes the full diagnostic list so the failure pinpoints
// what was (or wasn't) emitted.
func assertContainsDiag(t *testing.T, diags []sdk.Diag, sev sdk.Severity, substr string) {
	t.Helper()
	for _, d := range diags {
		if d.Severity == sev && strings.Contains(d.Message, substr) {
			return
		}
	}
	t.Fatalf("no %v diagnostic containing %q; got %d diags: %+v",
		sev, substr, len(diags), diags)
}

// TestResolverConformance runs the framework conformance suite over
// the partner resolver.
//
// The three shape plugins previously ran no conformance suite at
// all, which is how each came to declare Priority() without the rest
// of sdk.CapabilityProvider — satisfying nothing, so the pipeline
// ignored the declared ordering and ran all three in the default
// bucket, in registration order. The suite's completeness check
// catches that shape directly; wiring the plugins into the suite is
// what makes the check reach them.
func TestResolverConformance(t *testing.T) {
	t.Parallel()
	plugintest.RunSuite(t, shape.New().Resolver())
}
