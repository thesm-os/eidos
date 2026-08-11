// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Package mixins is the convenience aggregator for every
// [shape.Mixin] shipped under
// `plugins/annotator/shape/mixins/...`.
//
// Consumers that want the full built-in catalog import this
// package and call [All]:
//
//	pipe.WithAnnotator(shape.New().Mixins(mixins.All()...))
//
// Consumers wanting a curated subset import the per-mixin
// sub-packages directly and pick what they need.
//
// # Attaching several mixins
//
// Mixins are orthogonal, so a callable commonly carries several.
// One `+gen:mixin` names as many as apply, stamped in the order
// written:
//
//	//+gen:mixin idempotent concurrent atomic bounded
//	func (s *Store) Put(ctx context.Context, k string, v []byte) error
//
// Parameters are the exception, and they are always `key=value`.
// Every bare token on the line is a mixin name, so a parameter
// written positionally is read as another name:
//
//	//+gen:mixin bounded 100      // WRONG — "100" is read as a mixin
//	//+gen:mixin bounded limit=100
//
// A name with no registered mixin is reported as an error, which is
// what surfaces that mistake.
//
// A `key=value` pair belongs to exactly one mixin, so it is only
// accepted when the directive names exactly one — otherwise the
// owner would be a guess. Give a parameterised mixin its own line
// and batch the rest:
//
//	//+gen:mixin idempotent concurrent
//	//+gen:mixin bounded limit=100
//
// Pairing parameters with several names is an error; the names are
// still attached and the parameters are dropped.
package mixins

import (
	"slices"

	"go.thesmos.sh/eidos/plugins/annotator/shape"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/associative"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/atomic"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/bounded"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/cacheable"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/causal"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/commutative"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/concurrent"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/concurrentreaders"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/conservative"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/crdtmerge"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/defaultonerror"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/deleteremoves"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/deprecated"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/errors"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/eventually"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/hooks"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/idempotent"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/injectionsafe"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/integrationonly"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/leakfree"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/lifecycleafterclose"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/monotonic"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/monotonicreads"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/monotonicwrites"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/nilsafe"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/orderafter"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/overmatch"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/partition"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/permutation"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/pure"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/readafterwrite"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/retrysucceeds"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/sample"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/scope"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/sideeffect"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/snapshotisolation"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/stableorder"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/sticky"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/streamreflectsmutations"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/tamperevident"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/timeaware"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/timeout"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/total"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/validates"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/windowed"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/wrappedvia"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/writesfollowreads"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/xsssafe"
)

// All returns every [shape.Mixin] shipped in this repository,
// alphabetised by [shape.Mixin.Name]. The returned slice is
// freshly allocated on each call; callers may mutate it without
// affecting future invocations.
func All() []shape.Mixin {
	out := []shape.Mixin{
		atomic.Mixin(),
		associative.Mixin(),
		causal.Mixin(),
		commutative.Mixin(),
		conservative.Mixin(),
		defaultonerror.Mixin(),
		injectionsafe.Mixin(),
		leakfree.Mixin(),
		monotonicreads.Mixin(),
		monotonicwrites.Mixin(),
		overmatch.Mixin(),
		permutation.Mixin(),
		snapshotisolation.Mixin(),
		stableorder.Mixin(),
		sticky.Mixin(),
		tamperevident.Mixin(),
		timeaware.Mixin(),
		total.Mixin(),
		windowed.Mixin(),
		writesfollowreads.Mixin(),
		xsssafe.Mixin(),
		bounded.Mixin(),
		cacheable.Mixin(),
		concurrent.Mixin(),
		concurrentreaders.Mixin(),
		crdtmerge.Mixin(),
		deleteremoves.Mixin(),
		deprecated.Mixin(),
		errors.Mixin(),
		eventually.Mixin(),
		hooks.Mixin(),
		idempotent.Mixin(),
		integrationonly.Mixin(),
		lifecycleafterclose.Mixin(),
		monotonic.Mixin(),
		nilsafe.Mixin(),
		orderafter.Mixin(),
		partition.Mixin(),
		pure.Mixin(),
		readafterwrite.Mixin(),
		retrysucceeds.Mixin(),
		sample.Mixin(),
		scope.Mixin(),
		sideeffect.Mixin(),
		streamreflectsmutations.Mixin(),
		timeout.Mixin(),
		validates.Mixin(),
		wrappedvia.Mixin(),
	}
	slices.SortFunc(out, func(a, b shape.Mixin) int {
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
