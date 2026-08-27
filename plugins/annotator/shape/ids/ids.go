// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package ids

import (
	"slices"
	"strings"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts"
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
	"go.thesmos.sh/eidos/plugins/annotator/shape/detectors"
	detAggregator "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/aggregator"
	detAnsweringWriter "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/answeringwriter"
	detBatchReader "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/batchreader"
	detCloser "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/closer"
	detCompositeWriter "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/compositewriter"
	detDeleter "go.thesmos.sh/eidos/plugins/annotator/shape/detectors/deleter"
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
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins"
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
	DetectorDeleter         = detDeleter.Name
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

// The parameter keys each contract accepts, named
// `Contract<Name>Param<Key>`.
//
// A param key is the other half of what a directive spells: the name
// picks the contract, and these pick what may be written beside it.
// A consumer reading a stamp composes the key through
// [shape.ContractParamKey], which takes both — so a caller with the
// name constant and a literal for the param has half a compile-time
// link and half a string that stops matching in silence.
//
// Contracts declaring no parameters have no entry. Their whole
// directive is the role and its partners, and a constant for a key
// the contract never reads would be a name a caller could write and
// never see used.
const (
	ContractBatchWriterParamMode     = conBatchWriter.ParamMode
	ContractCASParamMismatch         = conCAS.ParamMismatch
	ContractCASParamVersion          = conCAS.ParamVersion
	ContractCodecParamFidelity       = conCodec.ParamFidelity
	ContractCursorParamClose         = conCursor.ParamClose
	ContractCursorParamNext          = conCursor.ParamNext
	ContractCursorParamSentinel      = conCursor.ParamSentinel
	ContractIfAbsentParamConflict    = conIfAbsent.ParamConflict
	ContractIfMatchParamPred         = conIfMatch.ParamPred
	ContractLeaseParamHeld           = conLease.ParamHeld
	ContractLeaseParamTimeout        = conLease.ParamTimeout
	ContractPaginationParamCursor    = conPagination.ParamCursor
	ContractPublisherParamMode       = conPublisher.ParamMode
	ContractRateLimitParamBurst      = conRateLimit.ParamBurst
	ContractRateLimitParamRate       = conRateLimit.ParamRate
	ContractTransactionParamNotFound = conTransaction.ParamNotFound
	ContractTxParamClosed            = conTx.ParamClosed
	ContractWatcherParamNext         = conWatcher.ParamNext
	ContractWatcherParamStop         = conWatcher.ParamStop
	ContractWorkflowParamTransitions = conWorkflow.ParamTransitions
)

// The parameter keys each mixin accepts, named
// `Mixin<Name>Param<Key>`.
//
// The mixin half of the same story the contract block above tells:
// [shape.MixinParamKey] takes the mixin name and the param key, and a
// caller holding a constant for one and a literal for the other has
// bought the compile-time link for half the lookup.
//
// Mixins declaring no parameters have no entry — most of the catalog,
// since a mixin is usually a bare assertion.
const (
	MixinAtomicParamRead                    = mixAtomic.ParamRead
	MixinBoundedParamLimit                  = mixBounded.ParamLimit
	MixinBoundedParamMin                    = mixBounded.ParamMin
	MixinCausalParamVersion                 = mixCausal.ParamVersion
	MixinConservativeParamField             = mixConservative.ParamField
	MixinCRDTMergeParamRead                 = mixCRDTMerge.ParamRead
	MixinCRDTMergeParamWrite                = mixCRDTMerge.ParamWrite
	MixinDeleteRemovesParamRead             = mixDeleteRemoves.ParamRead
	MixinDeleteRemovesParamSentinel         = mixDeleteRemoves.ParamSentinel
	MixinEventuallyParamSettle              = mixEventually.ParamSettle
	MixinEventuallyParamSync                = mixEventually.ParamSync
	MixinHooksParamRegister                 = mixHooks.ParamRegister
	MixinIndexedParamBy                     = mixIndexed.ParamBy
	MixinInjectionSafeParamRead             = mixInjectionSafe.ParamRead
	MixinLeakFreeParamClose                 = mixLeakFree.ParamClose
	MixinLeakFreeParamOpen                  = mixLeakFree.ParamOpen
	MixinLifecycleAfterCloseParamClose      = mixLifecycleAfterClose.ParamClose
	MixinLifecycleAfterCloseParamSentinel   = mixLifecycleAfterClose.ParamSentinel
	MixinMonotonicReadsParamVersion         = mixMonotonicReads.ParamVersion
	MixinMonotonicWritesParamVersion        = mixMonotonicWrites.ParamVersion
	MixinNotFoundParamSentinel              = mixNotFound.ParamSentinel
	MixinOrderAfterParamFn                  = mixOrderAfter.ParamFn
	MixinOrderAfterParamUnready             = mixOrderAfter.ParamUnready
	MixinPartitionParamAxis                 = mixPartition.ParamAxis
	MixinPartitionParamRead                 = mixPartition.ParamRead
	MixinPoisonableParamInduce              = mixPoisonable.ParamInduce
	MixinReadAfterWriteParamWrite           = mixReadAfterWrite.ParamWrite
	MixinReadYourWritesParamVersion         = mixReadYourWrites.ParamVersion
	MixinRetrySucceedsParamAttempts         = mixRetrySucceeds.ParamAttempts
	MixinSampleParamBuilder                 = mixSample.ParamBuilder
	MixinScheduledParamFired                = mixScheduled.ParamFired
	MixinScheduledParamSchedule             = mixScheduled.ParamSchedule
	MixinScopeParamAxis                     = mixScope.ParamAxis
	MixinScopeParamName                     = mixScope.ParamName
	MixinSerializableParamRead              = mixSerializable.ParamRead
	MixinSideEffectParamObserve             = mixSideEffect.ParamObserve
	MixinSnapshotIsolationParamRead         = mixSnapshotIsolation.ParamRead
	MixinStickyParamKey                     = mixSticky.ParamKey
	MixinStreamReflectsMutationsParamDelete = mixStreamReflectsMutations.ParamDelete
	MixinStreamReflectsMutationsParamMutate = mixStreamReflectsMutations.ParamMutate
	MixinTamperEvidentParamTamper           = mixTamperEvident.ParamTamper
	MixinTamperEvidentParamVerify           = mixTamperEvident.ParamVerify
	MixinTimeoutParamDuration               = mixTimeout.ParamDuration
	MixinTotalParamDomain                   = mixTotal.ParamDomain
	MixinTTLParamDuration                   = mixTTL.ParamDuration
	MixinTTLParamNotFound                   = mixTTL.ParamNotFound
	MixinTTLParamPut                        = mixTTL.ParamPut
	MixinTTLParamRead                       = mixTTL.ParamRead
	MixinValidatesParamFn                   = mixValidates.ParamFn
	MixinWindowedParamCount                 = mixWindowed.ParamCount
	MixinWindowedParamIncr                  = mixWindowed.ParamIncr
	MixinWindowedParamWindow                = mixWindowed.ParamWindow
	MixinWrappedViaParamFn                  = mixWrappedVia.ParamFn
	MixinWritesFollowReadsParamVersion      = mixWritesFollowReads.ParamVersion
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
		DetectorDeleter,
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

// Param is one catalog parameter together with the contract or mixin
// that accepts it.
//
// The owner rather than the key alone, because a key is only
// meaningful under it: `read` means one thing on `partition` and
// another on `ttl`, and [shape.MixinParamKey] takes both to build the
// stamp.
//
// The declaration is embedded whole rather than mirrored field by
// field, so what the catalog says about a key — its [shape.ParamKind],
// its role scope, whether a directive must carry it — is answered here
// without a copy that goes stale when [shape.Param] grows a field, as
// it did when Required landed.
type Param struct {
	// Owner is the contract or mixin name the key belongs to.
	Owner Name

	// Param is the declaration as the owner registered it: Key, Kind,
	// Role and Required promote.
	shape.Param
}

// ContractParams returns every contract parameter the catalog
// declares, with its full declaration, sorted by owner then key. The
// returned slice is freshly allocated; callers may mutate it.
//
// Read off the registered catalog rather than restated, so the
// answer cannot drift from what the validator enforces. What is
// hand-maintained is the per-key constants above, and the test pins
// that every parameter here has one.
func ContractParams() []Param {
	var out []Param
	for _, c := range contracts.All() {
		for _, p := range c.Params {
			out = append(out, Param{Owner: Name(c.Name), Param: p})
		}
	}
	return sortParams(out)
}

// MixinParams returns every mixin parameter the catalog declares,
// with its full declaration, sorted by owner then key. The returned
// slice is freshly allocated; callers may mutate it.
func MixinParams() []Param {
	var out []Param
	for _, m := range mixins.All() {
		for _, p := range m.Params {
			out = append(out, Param{Owner: Name(m.Name), Param: p})
		}
	}
	return sortParams(out)
}

// sortParams orders a param list by owner then key.
//
// One comparison shared by both accessors, so the order they promise
// cannot drift between them.
func sortParams(ps []Param) []Param {
	slices.SortFunc(ps, func(a, b Param) int {
		if a.Owner != b.Owner {
			return strings.Compare(string(a.Owner), string(b.Owner))
		}
		return strings.Compare(a.Key, b.Key)
	})
	return ps
}

// ContractOf returns the registered [shape.Contract] under name —
// its roles, params and required partners — or false for a name the
// catalog does not declare.
//
// The spec by name, for a consumer that holds an id and wants what
// it licenses without importing the declaring package: a corpus gate
// counting coverage, a generator asking which keys a classification
// takes before deriving from it.
func ContractOf(name Name) (shape.Contract, bool) {
	for _, c := range contracts.All() {
		if c.Name == string(name) {
			return c, true
		}
	}
	return shape.Contract{}, false
}

// MixinOf returns the registered [shape.Mixin] under name, or false
// for a name the catalog does not declare.
func MixinOf(name Name) (shape.Mixin, bool) {
	for _, m := range mixins.All() {
		if m.Name == string(name) {
			return m, true
		}
	}
	return shape.Mixin{}, false
}

// DetectorOf returns the registered [shape.Detector] under name, or
// false for a name the catalog does not declare.
//
// Note `pure` names both a detector and a mixin — the namespace
// collision the prefixed constants exist for — so a caller holding a
// bare string asks the family it means.
func DetectorOf(name Name) (shape.Detector, bool) {
	for _, d := range detectors.All() {
		if d.Name == string(name) {
			return d, true
		}
	}
	return shape.Detector{}, false
}

// Documentary reports whether the catalog marks name as carrying
// information rather than an invariant — a classification that
// decorates a declaration for a reader or a downstream generator and
// licenses no assertion.
//
// The question a coverage report asks. Without it a consumer listing
// the classifications no rule reached cannot separate a gap it could
// close from a silence that is owed, so it either overstates the
// gaps or transcribes the catalog's judgement into a local table that
// goes stale the day a classification is marked here.
//
// Asked across all three families at once, because a consumer holding
// a stamped name holds one string and should not have to know which
// family declared it. `pure` names a detector and a mixin, and
// neither is documentary, so the shared name costs nothing here; a
// name the catalog does not declare answers false, which reads the
// same as "claimable" and is the safe direction — an unknown name
// reported as a gap is visible, one silently excused is not.
func Documentary(name Name) bool {
	if m, ok := MixinOf(name); ok && m.Documentary {
		return true
	}
	if c, ok := ContractOf(name); ok && c.Documentary {
		return true
	}
	d, ok := DetectorOf(name)
	return ok && d.Documentary
}

// ContractParam returns the declaration for one contract key, or
// false when the owner does not declare it. The one-key form of
// [ContractParams], for a consumer that holds the pair and wants its
// kind or required-ness without scanning the catalog.
func ContractParam(owner Name, key string) (shape.Param, bool) {
	c, ok := ContractOf(owner)
	if !ok {
		return shape.Param{}, false
	}
	return paramByKey(c.Params, key)
}

// MixinParam returns the declaration for one mixin key, or false
// when the owner does not declare it.
func MixinParam(owner Name, key string) (shape.Param, bool) {
	m, ok := MixinOf(owner)
	if !ok {
		return shape.Param{}, false
	}
	return paramByKey(m.Params, key)
}

// paramByKey finds one declaration in a registered param list.
func paramByKey(ps []shape.Param, key string) (shape.Param, bool) {
	for _, p := range ps {
		if p.Key == key {
			return p, true
		}
	}
	return shape.Param{}, false
}
