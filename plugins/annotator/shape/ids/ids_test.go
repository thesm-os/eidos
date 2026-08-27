// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package ids_test

import (
	"slices"
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts"
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors"
	"go.thesmos.sh/eidos/plugins/annotator/shape/ids"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins"
)

// names lifts a registered catalog's names into the id type for
// comparison against the exported list.
func names[T any](items []T, name func(T) string) []ids.Name {
	out := make([]ids.Name, 0, len(items))
	for _, item := range items {
		out = append(out, ids.Name(name(item)))
	}
	slices.Sort(out)
	return out
}

// TestIDs_MatchTheRegisteredCatalog is the guard that makes this
// package safe to rely on.
//
// A re-export that can fall behind is worse than the string literals
// it replaces: the literal is at least obviously unverified, while a
// constant carries the authority of the compiler having checked
// something — and what the compiler checks is that the referenced
// package exists, not that every registered shape has a constant. A
// shape added to the catalog and forgotten here is the silence.
func TestIDs_MatchTheRegisteredCatalog(t *testing.T) {
	t.Parallel()

	t.Run("every registered detector has an id", func(t *testing.T) {
		t.Parallel()
		want := names(detectors.All(), func(d shape.Detector) string { return d.Name })
		if got := ids.Detectors(); !slices.Equal(got, want) {
			t.Errorf("Detectors() has drifted from detectors.All();\n  missing: %v\n  extra: %v",
				missing(want, got), missing(got, want))
		}
	})

	t.Run("every registered contract has an id", func(t *testing.T) {
		t.Parallel()
		want := names(contracts.All(), func(c shape.Contract) string { return c.Name })
		if got := ids.Contracts(); !slices.Equal(got, want) {
			t.Errorf("Contracts() has drifted from contracts.All();\n  missing: %v\n  extra: %v",
				missing(want, got), missing(got, want))
		}
	})

	t.Run("every registered mixin has an id", func(t *testing.T) {
		t.Parallel()
		want := names(mixins.All(), func(m shape.Mixin) string { return m.Name })
		if got := ids.Mixins(); !slices.Equal(got, want) {
			t.Errorf("Mixins() has drifted from mixins.All();\n  missing: %v\n  extra: %v",
				missing(want, got), missing(got, want))
		}
	})

	t.Run("the lists are non-empty", func(t *testing.T) {
		t.Parallel()
		// Guard the guard: two empty slices compare equal, so a
		// refactor emptying both sides would leave the drift checks
		// reporting green over nothing.
		for label, got := range map[string][]ids.Name{
			"Detectors": ids.Detectors(),
			"Contracts": ids.Contracts(),
			"Mixins":    ids.Mixins(),
		} {
			if len(got) == 0 {
				t.Errorf("%s() is empty; the drift check above would be vacuous", label)
			}
		}
	})
}

// TestIDs_Untyped pins that a constant still passes where the shape
// API takes a string, which is why the constants carry no type.
func TestIDs_Untyped(t *testing.T) {
	t.Parallel()
	// Compiles only while the constants stay untyped: a typed Name
	// would need a conversion at every call site in every consumer.
	if key := shape.MixinParamKey(ids.MixinTTL, "notfound"); key.Name() == "" {
		t.Fatal("MixinParamKey returned an unnamed key")
	}
	if key := shape.ContractRoleKey(ids.ContractCursor); key.Name() == "" {
		t.Fatal("ContractRoleKey returned an unnamed key")
	}
}

// TestIDs_NamesAreNotPackageNames pins the two places where reading
// the constant is the only way to learn the registered spelling.
func TestIDs_NamesAreNotPackageNames(t *testing.T) {
	t.Parallel()
	if ids.ContractCache != "cache" {
		t.Errorf("ContractCache = %q, want cache — the writethroughcache package registers the short name",
			ids.ContractCache)
	}
	if ids.ContractBatchWriter != "batch-writer" {
		t.Errorf("ContractBatchWriter = %q, want the hyphenated form", ids.ContractBatchWriter)
	}
}

// missing returns the elements of want that got does not contain.
func missing(want, got []ids.Name) []ids.Name {
	var out []ids.Name
	for _, w := range want {
		if !slices.Contains(got, w) {
			out = append(out, w)
		}
	}
	return out
}

// TestIDs_Lookups covers the by-name half: a consumer holding an id
// gets the registered declaration back, with everything the catalog
// says about it.
func TestIDs_Lookups(t *testing.T) {
	t.Parallel()

	t.Run("a param answers its declaration, not just its spelling", func(t *testing.T) {
		t.Parallel()
		// The reason the accessors derive from the catalog: Kind, Role
		// and Required are what a consumer gates on, and a pair list
		// could not answer them.
		p, ok := ids.MixinParam(ids.MixinOrderAfter, ids.MixinOrderAfterParamFn)
		if !ok {
			t.Fatal("orderafter declares fn= and the lookup missed it")
		}
		if p.Kind != shape.KindCallable || !p.Required {
			t.Errorf("got %+v, want the callable kind and required, as orderafter declares", p)
		}
	})

	t.Run("a contract param carries its role scope", func(t *testing.T) {
		t.Parallel()
		p, ok := ids.ContractParam(ids.ContractCursor, ids.ContractCursorParamNext)
		if !ok {
			t.Fatal("cursor declares next= and the lookup missed it")
		}
		if p.Role != "open" || !p.Required {
			t.Errorf("got %+v, want required on the open arm, as cursor declares", p)
		}
	})

	t.Run("a spec lookup answers the whole registration", func(t *testing.T) {
		t.Parallel()
		c, ok := ids.ContractOf(ids.ContractCursor)
		if !ok || len(c.Roles) == 0 || len(c.Params) == 0 {
			t.Fatalf("ContractOf(cursor) = (%+v, %v), want the registered spec", c, ok)
		}
	})

	t.Run("the shared name resolves per family", func(t *testing.T) {
		t.Parallel()
		// `pure` is both a detector and a mixin — the collision the
		// prefixed constants exist for — so each family answers its
		// own and neither answers for the other.
		if _, ok := ids.DetectorOf(ids.DetectorPure); !ok {
			t.Error("the detector family does not answer pure")
		}
		if _, ok := ids.MixinOf(ids.MixinPure); !ok {
			t.Error("the mixin family does not answer pure")
		}
		if _, ok := ids.ContractOf("pure"); ok {
			t.Error("no contract is named pure, and one answered")
		}
	})

	t.Run("a name the catalog does not declare answers false", func(t *testing.T) {
		t.Parallel()
		if _, ok := ids.MixinOf("no-such-mixin"); ok {
			t.Error("an unregistered name answered a spec")
		}
		if _, ok := ids.MixinParam(ids.MixinOrderAfter, "no-such-key"); ok {
			t.Error("an undeclared key answered a param")
		}
	})
}

// TestIDs_Documentary covers the answer a coverage report needs: a
// classification that owes no check, told apart from one nothing has
// written a rule for yet.
func TestIDs_Documentary(t *testing.T) {
	t.Parallel()

	t.Run("the marked classifications answer true", func(t *testing.T) {
		t.Parallel()
		for _, name := range []ids.Name{ids.MixinErrors, ids.MixinScope, ids.MixinDeprecated} {
			if !ids.Documentary(name) {
				t.Errorf("%s is documentary in the catalog and answered false", name)
			}
		}
	})

	t.Run("a checkable classification answers false", func(t *testing.T) {
		t.Parallel()
		// The distinction the flag exists for: idempotent has no rule
		// in some consumer somewhere, and that is a gap a rule could
		// close — not a silence that is owed.
		for _, name := range []ids.Name{ids.MixinIdempotent, ids.ContractCursor, ids.DetectorReader} {
			if ids.Documentary(name) {
				t.Errorf("%s licenses an assertion and answered documentary", name)
			}
		}
	})

	t.Run("an unregistered name answers false", func(t *testing.T) {
		t.Parallel()
		// The safe direction: an unknown name reported as a gap is
		// visible, one silently excused is not.
		if ids.Documentary("no-such-classification") {
			t.Error("a name the catalog does not declare was excused")
		}
	})

	t.Run("the answer comes from the registration", func(t *testing.T) {
		t.Parallel()
		// Not from a list here — the whole point, since a table in
		// this package would go stale exactly as a consumer's does.
		m, ok := ids.MixinOf(ids.MixinErrors)
		if !ok || !m.Documentary {
			t.Fatalf("MixinOf(errors) = (%+v, %v), want the registered flag", m, ok)
		}
	})
}

// pair is one (owner, key) spelling, for the constants pin below.
type pair struct {
	owner ids.Name
	key   string
}

// TestIDs_ParamsMatchTheRegisteredCatalog is [TestIDs_MatchTheRegisteredCatalog]
// for parameters.
//
// The accessors derive from the registered catalog, so they cannot
// drift from it — what can fall behind is the per-key constants,
// which the compiler only checks exist, not that every registered
// parameter has one. The hand list here is built from those
// constants, and a parameter added to a contract or mixin without
// its constant is the diff this test prints.
func TestIDs_ParamsMatchTheRegisteredCatalog(t *testing.T) {
	t.Parallel()

	t.Run("every contract parameter has a spelled constant", func(t *testing.T) {
		t.Parallel()
		assertPairsMatch(t, ids.ContractParams(), []pair{
			{ids.ContractBatchWriter, ids.ContractBatchWriterParamMode},
			{ids.ContractCAS, ids.ContractCASParamMismatch},
			{ids.ContractCAS, ids.ContractCASParamVersion},
			{ids.ContractCodec, ids.ContractCodecParamFidelity},
			{ids.ContractCursor, ids.ContractCursorParamClose},
			{ids.ContractCursor, ids.ContractCursorParamNext},
			{ids.ContractCursor, ids.ContractCursorParamSentinel},
			{ids.ContractIfAbsent, ids.ContractIfAbsentParamConflict},
			{ids.ContractIfMatch, ids.ContractIfMatchParamPred},
			{ids.ContractLease, ids.ContractLeaseParamHeld},
			{ids.ContractLease, ids.ContractLeaseParamTimeout},
			{ids.ContractPagination, ids.ContractPaginationParamCursor},
			{ids.ContractPublisher, ids.ContractPublisherParamMode},
			{ids.ContractRateLimit, ids.ContractRateLimitParamBurst},
			{ids.ContractRateLimit, ids.ContractRateLimitParamRate},
			{ids.ContractTransaction, ids.ContractTransactionParamNotFound},
			{ids.ContractTx, ids.ContractTxParamClosed},
			{ids.ContractWatcher, ids.ContractWatcherParamNext},
			{ids.ContractWatcher, ids.ContractWatcherParamStop},
			{ids.ContractWorkflow, ids.ContractWorkflowParamTransitions},
		})
	})

	t.Run("every mixin parameter has a spelled constant", func(t *testing.T) {
		t.Parallel()
		assertPairsMatch(t, ids.MixinParams(), []pair{
			{ids.MixinAtomic, ids.MixinAtomicParamRead},
			{ids.MixinBounded, ids.MixinBoundedParamLimit},
			{ids.MixinBounded, ids.MixinBoundedParamMin},
			{ids.MixinCRDTMerge, ids.MixinCRDTMergeParamRead},
			{ids.MixinCRDTMerge, ids.MixinCRDTMergeParamWrite},
			{ids.MixinCausal, ids.MixinCausalParamVersion},
			{ids.MixinConservative, ids.MixinConservativeParamField},
			{ids.MixinDeleteRemoves, ids.MixinDeleteRemovesParamRead},
			{ids.MixinDeleteRemoves, ids.MixinDeleteRemovesParamSentinel},
			{ids.MixinEventually, ids.MixinEventuallyParamSettle},
			{ids.MixinEventually, ids.MixinEventuallyParamSync},
			{ids.MixinHooks, ids.MixinHooksParamRegister},
			{ids.MixinIndexed, ids.MixinIndexedParamBy},
			{ids.MixinInjectionSafe, ids.MixinInjectionSafeParamRead},
			{ids.MixinLeakFree, ids.MixinLeakFreeParamClose},
			{ids.MixinLeakFree, ids.MixinLeakFreeParamOpen},
			{ids.MixinLifecycleAfterClose, ids.MixinLifecycleAfterCloseParamClose},
			{ids.MixinLifecycleAfterClose, ids.MixinLifecycleAfterCloseParamSentinel},
			{ids.MixinMonotonicReads, ids.MixinMonotonicReadsParamVersion},
			{ids.MixinMonotonicWrites, ids.MixinMonotonicWritesParamVersion},
			{ids.MixinNotFound, ids.MixinNotFoundParamSentinel},
			{ids.MixinOrderAfter, ids.MixinOrderAfterParamFn},
			{ids.MixinOrderAfter, ids.MixinOrderAfterParamUnready},
			{ids.MixinPartition, ids.MixinPartitionParamAxis},
			{ids.MixinPartition, ids.MixinPartitionParamRead},
			{ids.MixinPoisonable, ids.MixinPoisonableParamInduce},
			{ids.MixinReadAfterWrite, ids.MixinReadAfterWriteParamWrite},
			{ids.MixinReadYourWrites, ids.MixinReadYourWritesParamVersion},
			{ids.MixinRetrySucceeds, ids.MixinRetrySucceedsParamAttempts},
			{ids.MixinSample, ids.MixinSampleParamBuilder},
			{ids.MixinScheduled, ids.MixinScheduledParamFired},
			{ids.MixinScheduled, ids.MixinScheduledParamSchedule},
			{ids.MixinScope, ids.MixinScopeParamAxis},
			{ids.MixinScope, ids.MixinScopeParamName},
			{ids.MixinSerializable, ids.MixinSerializableParamRead},
			{ids.MixinSideEffect, ids.MixinSideEffectParamObserve},
			{ids.MixinSnapshotIsolation, ids.MixinSnapshotIsolationParamRead},
			{ids.MixinSticky, ids.MixinStickyParamKey},
			{ids.MixinStreamReflectsMutations, ids.MixinStreamReflectsMutationsParamDelete},
			{ids.MixinStreamReflectsMutations, ids.MixinStreamReflectsMutationsParamMutate},
			{ids.MixinTTL, ids.MixinTTLParamDuration},
			{ids.MixinTTL, ids.MixinTTLParamNotFound},
			{ids.MixinTTL, ids.MixinTTLParamPut},
			{ids.MixinTTL, ids.MixinTTLParamRead},
			{ids.MixinTamperEvident, ids.MixinTamperEvidentParamTamper},
			{ids.MixinTamperEvident, ids.MixinTamperEvidentParamVerify},
			{ids.MixinTimeout, ids.MixinTimeoutParamDuration},
			{ids.MixinTotal, ids.MixinTotalParamDomain},
			{ids.MixinValidates, ids.MixinValidatesParamFn},
			{ids.MixinWindowed, ids.MixinWindowedParamCount},
			{ids.MixinWindowed, ids.MixinWindowedParamIncr},
			{ids.MixinWindowed, ids.MixinWindowedParamWindow},
			{ids.MixinWrappedVia, ids.MixinWrappedViaParamFn},
			{ids.MixinWritesFollowReads, ids.MixinWritesFollowReadsParamVersion},
		})
	})
}

// assertPairsMatch fails unless the catalog's (owner, key) set is
// exactly the spelled set.
// TestIDs_RolesMatchTheRegisteredCatalog pins the role constants
// against the catalog in both directions.
//
// A role nobody named is one a consumer spells as a literal, which is
// the failure this package exists to remove; a constant naming no
// registered role is a name that compiles and matches nothing. Both
// are silent, so both are checked.
func TestIDs_RolesMatchTheRegisteredCatalog(t *testing.T) {
	t.Parallel()

	t.Run("every contract role has a spelled constant", func(t *testing.T) {
		t.Parallel()
		assertRolesMatch(t, ids.ContractRoles(), []pair{
			{ids.ContractAppender, ids.ContractAppenderRoleFn},
			{ids.ContractBatchWriter, ids.ContractBatchWriterRoleWriter},
			{ids.ContractBatchWriter, ids.ContractBatchWriterRoleReader},
			{ids.ContractCAS, ids.ContractCASRoleWriter},
			{ids.ContractCache, ids.ContractCacheRoleCache},
			{ids.ContractCache, ids.ContractCacheRoleBacking},
			{ids.ContractChain, ids.ContractChainRoleAppend},
			{ids.ContractChain, ids.ContractChainRoleReplay},
			{ids.ContractChain, ids.ContractChainRoleVerify},
			{ids.ContractCircuitBreaker, ids.ContractCircuitBreakerRoleFn},
			{ids.ContractCodec, ids.ContractCodecRoleForward},
			{ids.ContractCodec, ids.ContractCodecRoleInverse},
			{ids.ContractCursor, ids.ContractCursorRoleNext},
			{ids.ContractCursor, ids.ContractCursorRoleClose},
			{ids.ContractCursor, ids.ContractCursorRoleOpen},
			{ids.ContractIfAbsent, ids.ContractIfAbsentRoleWriter},
			{ids.ContractIfMatch, ids.ContractIfMatchRoleWriter},
			{ids.ContractIfMatch, ids.ContractIfMatchRoleMatch},
			{ids.ContractLeaderElection, ids.ContractLeaderElectionRoleCampaign},
			{ids.ContractLeaderElection, ids.ContractLeaderElectionRoleResign},
			{ids.ContractLeaderElection, ids.ContractLeaderElectionRoleIsLeader},
			{ids.ContractLease, ids.ContractLeaseRoleAcquire},
			{ids.ContractLease, ids.ContractLeaseRoleRelease},
			{ids.ContractOutbox, ids.ContractOutboxRoleAppend},
			{ids.ContractOutbox, ids.ContractOutboxRoleSubscribe},
			{ids.ContractPagination, ids.ContractPaginationRoleReader},
			{ids.ContractPersister, ids.ContractPersisterRoleWriter},
			{ids.ContractPersister, ids.ContractPersisterRoleReader},
			{ids.ContractPool, ids.ContractPoolRoleGet},
			{ids.ContractPool, ids.ContractPoolRolePut},
			{ids.ContractPool, ids.ContractPoolRoleStats},
			{ids.ContractPublisher, ids.ContractPublisherRolePublish},
			{ids.ContractPublisher, ids.ContractPublisherRoleSubscribe},
			{ids.ContractPublisher, ids.ContractPublisherRoleRedeliver},
			{ids.ContractRateLimit, ids.ContractRateLimitRoleFn},
			{ids.ContractSaga, ids.ContractSagaRoleStep},
			{ids.ContractSaga, ids.ContractSagaRoleCompensate},
			{ids.ContractSingleFlight, ids.ContractSingleFlightRoleFn},
			{ids.ContractTransaction, ids.ContractTransactionRoleFn},
			{ids.ContractTx, ids.ContractTxRoleBegin},
			{ids.ContractTx, ids.ContractTxRoleCommit},
			{ids.ContractTx, ids.ContractTxRoleRollback},
			{ids.ContractUpdater, ids.ContractUpdaterRoleWriter},
			{ids.ContractUpdater, ids.ContractUpdaterRoleReader},
			{ids.ContractUpserter, ids.ContractUpserterRoleWriter},
			{ids.ContractUpserter, ids.ContractUpserterRoleReader},
			{ids.ContractWatcher, ids.ContractWatcherRoleWatch},
			{ids.ContractWatcher, ids.ContractWatcherRoleTrigger},
			{ids.ContractWorkflow, ids.ContractWorkflowRoleFn},
		})
	})
}

// assertRolesMatch requires the constants and the catalog to name the
// same set of roles.
func assertRolesMatch(t *testing.T, got []ids.Role, want []pair) {
	t.Helper()
	catalog := make([]pair, 0, len(got))
	for _, r := range got {
		catalog = append(catalog, pair{r.Owner, r.Role})
	}
	for _, w := range want {
		if !slices.Contains(catalog, w) {
			t.Errorf("constant pair %v names no registered role", w)
		}
	}
	for _, c := range catalog {
		if !slices.Contains(want, c) {
			t.Errorf("registered role %v has no spelled constant", c)
		}
	}
}

func assertPairsMatch(t *testing.T, got []ids.Param, want []pair) {
	t.Helper()
	catalog := make([]pair, 0, len(got))
	for _, p := range got {
		catalog = append(catalog, pair{p.Owner, p.Key})
	}
	for _, w := range want {
		if !slices.Contains(catalog, w) {
			t.Errorf("constant pair %v names no registered parameter", w)
		}
	}
	for _, c := range catalog {
		if !slices.Contains(want, c) {
			t.Errorf("registered parameter %v has no spelled constant", c)
		}
	}
}
