// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package contracts is the convenience aggregator for every
// [shape.Contract] shipped under
// `plugins/annotator/shape/contracts/...`.
//
// One axis of three. [All] is every contract eidos ships and nothing
// else — no detectors or mixins:
//
//	pipe.WithAnnotator(shape.New().Contracts(contracts.All()...))
//
// Reach for `shape/full` where the whole vocabulary is wanted. A
// pipeline registering this axis alone stamps nothing for the other
// two, and what a consumer sees is a callable with no shape or mixin
// stamps — indistinguishable from one that legitimately has none.
//
// Consumers wanting a curated subset import the per-contract
// sub-packages directly and pick what they need.
package contracts

import (
	"slices"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/appender"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/batchwriter"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/cas"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/chain"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/circuitbreaker"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/codec"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/cursor"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/ifabsent"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/ifmatch"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/leaderelection"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/lease"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/outbox"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/pagination"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/persister"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/pool"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/publisher"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/ratelimit"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/saga"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/singleflight"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/transaction"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/tx"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/updater"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/upserter"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/watcher"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/workflow"
	"go.thesmos.sh/eidos/plugins/annotator/shape/contracts/writethroughcache"
)

// All returns every [shape.Contract] shipped in this repository,
// alphabetised by [shape.Contract.Name]. The returned slice is
// freshly allocated on each call; callers may mutate it without
// affecting future invocations.
func All() []shape.Contract {
	out := []shape.Contract{
		appender.Contract(),
		batchwriter.Contract(),
		cas.Contract(),
		chain.Contract(),
		circuitbreaker.Contract(),
		codec.Contract(),
		cursor.Contract(),
		ifabsent.Contract(),
		ifmatch.Contract(),
		leaderelection.Contract(),
		lease.Contract(),
		outbox.Contract(),
		pagination.Contract(),
		persister.Contract(),
		pool.Contract(),
		publisher.Contract(),
		ratelimit.Contract(),
		saga.Contract(),
		singleflight.Contract(),
		transaction.Contract(),
		tx.Contract(),
		updater.Contract(),
		upserter.Contract(),
		watcher.Contract(),
		workflow.Contract(),
		writethroughcache.Contract(),
	}
	slices.SortFunc(out, func(a, b shape.Contract) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	return out
}
