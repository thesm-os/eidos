// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package ids

import (
	conAppender "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/appender"
	conBatchWriter "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/batchwriter"
	conCAS "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/cas"
	conChain "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/chain"
	conCircuitBreaker "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/circuitbreaker"
	conCodec "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/codec"
	conCursor "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/cursor"
	conIfAbsent "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/ifabsent"
	conIfMatch "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/ifmatch"
	conLeaderElection "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/leaderelection"
	conLease "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/lease"
	conOutbox "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/outbox"
	conPagination "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/pagination"
	conPersister "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/persister"
	conPool "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/pool"
	conPublisher "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/publisher"
	conRateLimit "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/ratelimit"
	conSaga "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/saga"
	conSingleFlight "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/singleflight"
	conTransaction "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/transaction"
	conTx "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/tx"
	conUpdater "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/updater"
	conUpserter "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/upserter"
	conWatcher "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/watcher"
	conWorkflow "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/workflow"
	conCache "go.thesmos.sh/eidos/plugins/annotator/shape/contracts/writethroughcache"
	detAggregator "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/aggregator"
	detAnsweringWriter "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/answeringwriter"
	detBatchReader "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/batchreader"
	detCloser "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/closer"
	detCompositeWriter "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/compositewriter"
	detLifecycle "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/lifecycle"
	detLookup "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/lookup"
	detMultiAggregator "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/multiaggregator"
	detMultiArgWriter "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/multiargwriter"
	detMultiReader "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/multireader"
	detMutator "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/mutator"
	detPointerReader "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/pointerreader"
	detPoisonAccessor "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/poisonaccessor"
	detPredicate "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/predicate"
	detPure "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/pure"
	detReader "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/reader"
	detReaderNoError "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/readernoerror"
	detReaderWithBool "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/readerwithbool"
	detStreamConsumer "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/streamconsumer"
	detStreamReader "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/streamreader"
	detVoidLifecycle "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/voidlifecycle"
	detWriter "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/writer"
	mixAccumulates "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/accumulates"
	mixAssociative "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/associative"
	mixAtomic "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/atomic"
	mixBounded "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/bounded"
	mixCacheable "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/cacheable"
	mixCausal "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/causal"
	mixCommutative "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/commutative"
	mixConcurrent "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/concurrent"
	mixConcurrentReaders "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/concurrentreaders"
	mixConservative "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/conservative"
	mixCRDTMerge "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/crdtmerge"
	mixDefaultOnError "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/defaultonerror"
	mixDeleteRemoves "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/deleteremoves"
	mixDeprecated "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/deprecated"
	mixErrors "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/errors"
	mixEventually "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/eventually"
	mixHooks "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/hooks"
	mixIdempotent "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/idempotent"
	mixIndexed "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/indexed"
	mixInjectionSafe "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/injectionsafe"
	mixIntegrationOnly "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/integrationonly"
	mixLeakFree "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/leakfree"
	mixLifecycleAfterClose "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/lifecycleafterclose"
	mixMonotonic "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/monotonic"
	mixMonotonicReads "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/monotonicreads"
	mixMonotonicWrites "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/monotonicwrites"
	mixNilSafe "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/nilsafe"
	mixNoDuplicates "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/noduplicates"
	mixNotFound "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/notfound"
	mixOrderAfter "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/orderafter"
	mixOvermatch "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/overmatch"
	mixPartition "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/partition"
	mixPermutation "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/permutation"
	mixPointInTime "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/pointintime"
	mixPoisonable "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/poisonable"
	mixPure "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/pure"
	mixReadAfterWrite "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/readafterwrite"
	mixReadYourWrites "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/readyourwrites"
	mixRetrySucceeds "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/retrysucceeds"
	mixSample "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/sample"
	mixScheduled "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/scheduled"
	mixScope "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/scope"
	mixSerializable "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/serializable"
	mixSideEffect "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/sideeffect"
	mixSnapshotIsolation "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/snapshotisolation"
	mixStableOrder "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/stableorder"
	mixSticky "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/sticky"
	mixStreamReflectsMutations "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/streamreflectsmutations"
	mixTamperEvident "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/tamperevident"
	mixTimeAware "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/timeaware"
	mixTimeout "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/timeout"
	mixTotal "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/total"
	mixTTL "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/ttl"
	mixValidates "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/validates"
	mixWindowed "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/windowed"
	mixWrappedVia "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/wrappedvia"
	mixWritesFollowReads "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/writesfollowreads"
	mixXSSSafe "go.thesmos.sh/eidos/plugins/annotator/shape/mixins/xsssafe"
)

// Name is a catalog identifier — the word a `+gen:shape`,
// `+gen:contract` or `+gen:mixin` directive spells.
//
// The constants below are deliberately untyped, so each one still
// passes to the string-taking key constructors ([shape.MixinParamKey]
// and friends) without a conversion. The type earns its place on the
// slices and on a consumer's own maps, where it says what the string
// is for.
type Name string

// The structural shape names the catalog detects.
//
// Each is its own package's Name, so a rename there is a compile
// error here rather than a string that stopped matching.
const (
	DetectorAggregator      = detAggregator.Name
	DetectorAnsweringWriter = detAnsweringWriter.Name
	DetectorBatchReader     = detBatchReader.Name
	DetectorCloser          = detCloser.Name
	DetectorCompositeWriter = detCompositeWriter.Name
	DetectorLifecycle       = detLifecycle.Name
	DetectorLookup          = detLookup.Name
	DetectorMultiAggregator = detMultiAggregator.Name
	DetectorMultiArgWriter  = detMultiArgWriter.Name
	DetectorMultiReader     = detMultiReader.Name
	DetectorMutator         = detMutator.Name
	DetectorPointerReader   = detPointerReader.Name
	DetectorPoisonAccessor  = detPoisonAccessor.Name
	DetectorPredicate       = detPredicate.Name
	DetectorPure            = detPure.Name
	DetectorReader          = detReader.Name
	DetectorReaderNoError   = detReaderNoError.Name
	DetectorReaderWithBool  = detReaderWithBool.Name
	DetectorStreamConsumer  = detStreamConsumer.Name
	DetectorStreamReader    = detStreamReader.Name
	DetectorVoidLifecycle   = detVoidLifecycle.Name
	DetectorWriter          = detWriter.Name
)

// The contract names the catalog registers.
//
// Each is its own package's Name, so a rename there is a compile
// error here rather than a string that stopped matching.
const (
	ContractAppender       = conAppender.Name
	ContractBatchWriter    = conBatchWriter.Name
	ContractCache          = conCache.Name
	ContractCAS            = conCAS.Name
	ContractChain          = conChain.Name
	ContractCircuitBreaker = conCircuitBreaker.Name
	ContractCodec          = conCodec.Name
	ContractCursor         = conCursor.Name
	ContractIfAbsent       = conIfAbsent.Name
	ContractIfMatch        = conIfMatch.Name
	ContractLeaderElection = conLeaderElection.Name
	ContractLease          = conLease.Name
	ContractOutbox         = conOutbox.Name
	ContractPagination     = conPagination.Name
	ContractPersister      = conPersister.Name
	ContractPool           = conPool.Name
	ContractPublisher      = conPublisher.Name
	ContractRateLimit      = conRateLimit.Name
	ContractSaga           = conSaga.Name
	ContractSingleFlight   = conSingleFlight.Name
	ContractTransaction    = conTransaction.Name
	ContractTx             = conTx.Name
	ContractUpdater        = conUpdater.Name
	ContractUpserter       = conUpserter.Name
	ContractWatcher        = conWatcher.Name
	ContractWorkflow       = conWorkflow.Name
)

// The mixin names the catalog registers.
//
// Each is its own package's Name, so a rename there is a compile
// error here rather than a string that stopped matching.
const (
	MixinAccumulates             = mixAccumulates.Name
	MixinAssociative             = mixAssociative.Name
	MixinAtomic                  = mixAtomic.Name
	MixinBounded                 = mixBounded.Name
	MixinCacheable               = mixCacheable.Name
	MixinCausal                  = mixCausal.Name
	MixinCommutative             = mixCommutative.Name
	MixinConcurrent              = mixConcurrent.Name
	MixinConcurrentReaders       = mixConcurrentReaders.Name
	MixinConservative            = mixConservative.Name
	MixinCRDTMerge               = mixCRDTMerge.Name
	MixinDefaultOnError          = mixDefaultOnError.Name
	MixinDeleteRemoves           = mixDeleteRemoves.Name
	MixinDeprecated              = mixDeprecated.Name
	MixinErrors                  = mixErrors.Name
	MixinEventually              = mixEventually.Name
	MixinHooks                   = mixHooks.Name
	MixinIdempotent              = mixIdempotent.Name
	MixinIndexed                 = mixIndexed.Name
	MixinInjectionSafe           = mixInjectionSafe.Name
	MixinIntegrationOnly         = mixIntegrationOnly.Name
	MixinLeakFree                = mixLeakFree.Name
	MixinLifecycleAfterClose     = mixLifecycleAfterClose.Name
	MixinMonotonic               = mixMonotonic.Name
	MixinMonotonicReads          = mixMonotonicReads.Name
	MixinMonotonicWrites         = mixMonotonicWrites.Name
	MixinNilSafe                 = mixNilSafe.Name
	MixinNoDuplicates            = mixNoDuplicates.Name
	MixinNotFound                = mixNotFound.Name
	MixinOrderAfter              = mixOrderAfter.Name
	MixinOvermatch               = mixOvermatch.Name
	MixinPartition               = mixPartition.Name
	MixinPermutation             = mixPermutation.Name
	MixinPointInTime             = mixPointInTime.Name
	MixinPoisonable              = mixPoisonable.Name
	MixinPure                    = mixPure.Name
	MixinReadAfterWrite          = mixReadAfterWrite.Name
	MixinReadYourWrites          = mixReadYourWrites.Name
	MixinRetrySucceeds           = mixRetrySucceeds.Name
	MixinSample                  = mixSample.Name
	MixinScheduled               = mixScheduled.Name
	MixinScope                   = mixScope.Name
	MixinSerializable            = mixSerializable.Name
	MixinSideEffect              = mixSideEffect.Name
	MixinSnapshotIsolation       = mixSnapshotIsolation.Name
	MixinStableOrder             = mixStableOrder.Name
	MixinSticky                  = mixSticky.Name
	MixinStreamReflectsMutations = mixStreamReflectsMutations.Name
	MixinTamperEvident           = mixTamperEvident.Name
	MixinTimeAware               = mixTimeAware.Name
	MixinTimeout                 = mixTimeout.Name
	MixinTotal                   = mixTotal.Name
	MixinTTL                     = mixTTL.Name
	MixinValidates               = mixValidates.Name
	MixinWindowed                = mixWindowed.Name
	MixinWrappedVia              = mixWrappedVia.Name
	MixinWritesFollowReads       = mixWritesFollowReads.Name
	MixinXSSSafe                 = mixXSSSafe.Name
)

// Detectors returns every detector name in the shipped catalog,
// sorted. The returned slice is freshly allocated; callers may
// mutate it.
func Detectors() []Name {
	return []Name{
		DetectorAggregator,
		DetectorAnsweringWriter,
		DetectorBatchReader,
		DetectorCloser,
		DetectorCompositeWriter,
		DetectorLifecycle,
		DetectorLookup,
		DetectorMultiAggregator,
		DetectorMultiArgWriter,
		DetectorMultiReader,
		DetectorMutator,
		DetectorPointerReader,
		DetectorPoisonAccessor,
		DetectorPredicate,
		DetectorPure,
		DetectorReader,
		DetectorReaderNoError,
		DetectorReaderWithBool,
		DetectorStreamConsumer,
		DetectorStreamReader,
		DetectorVoidLifecycle,
		DetectorWriter,
	}
}

// Contracts returns every contract name in the shipped catalog,
// sorted. The returned slice is freshly allocated; callers may
// mutate it.
func Contracts() []Name {
	return []Name{
		ContractAppender,
		ContractBatchWriter,
		ContractCache,
		ContractCAS,
		ContractChain,
		ContractCircuitBreaker,
		ContractCodec,
		ContractCursor,
		ContractIfAbsent,
		ContractIfMatch,
		ContractLeaderElection,
		ContractLease,
		ContractOutbox,
		ContractPagination,
		ContractPersister,
		ContractPool,
		ContractPublisher,
		ContractRateLimit,
		ContractSaga,
		ContractSingleFlight,
		ContractTransaction,
		ContractTx,
		ContractUpdater,
		ContractUpserter,
		ContractWatcher,
		ContractWorkflow,
	}
}

// Mixins returns every mixin name in the shipped catalog,
// sorted. The returned slice is freshly allocated; callers may
// mutate it.
func Mixins() []Name {
	return []Name{
		MixinAccumulates,
		MixinAssociative,
		MixinAtomic,
		MixinBounded,
		MixinCacheable,
		MixinCausal,
		MixinCommutative,
		MixinConcurrent,
		MixinConcurrentReaders,
		MixinConservative,
		MixinCRDTMerge,
		MixinDefaultOnError,
		MixinDeleteRemoves,
		MixinDeprecated,
		MixinErrors,
		MixinEventually,
		MixinHooks,
		MixinIdempotent,
		MixinIndexed,
		MixinInjectionSafe,
		MixinIntegrationOnly,
		MixinLeakFree,
		MixinLifecycleAfterClose,
		MixinMonotonic,
		MixinMonotonicReads,
		MixinMonotonicWrites,
		MixinNilSafe,
		MixinNoDuplicates,
		MixinNotFound,
		MixinOrderAfter,
		MixinOvermatch,
		MixinPartition,
		MixinPermutation,
		MixinPointInTime,
		MixinPoisonable,
		MixinPure,
		MixinReadAfterWrite,
		MixinReadYourWrites,
		MixinRetrySucceeds,
		MixinSample,
		MixinScheduled,
		MixinScope,
		MixinSerializable,
		MixinSideEffect,
		MixinSnapshotIsolation,
		MixinStableOrder,
		MixinSticky,
		MixinStreamReflectsMutations,
		MixinTamperEvident,
		MixinTimeAware,
		MixinTimeout,
		MixinTotal,
		MixinTTL,
		MixinValidates,
		MixinWindowed,
		MixinWrappedVia,
		MixinWritesFollowReads,
		MixinXSSSafe,
	}
}
