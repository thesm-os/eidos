// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package serializable_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/serializable"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/snapshotisolation"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, serializable.Mixin(), serializable.Name, nil)
}

// TestMixin_DistinctFromSnapshotIsolation pins the two as separate
// claims.
//
// Snapshot isolation permits write skew and serializability does not,
// so a checker selected from the wrong one reddens a correct store. A
// single name carrying both — or a level param on one — would make
// that mistake unavoidable rather than merely possible.
func TestMixin_DistinctFromSnapshotIsolation(t *testing.T) {
	t.Parallel()
	if serializable.Name == snapshotisolation.Name {
		t.Fatal("the two isolation claims share a name")
	}
	if len(snapshotisolation.Mixin().Params) != 0 {
		t.Errorf("snapshotisolation grew params = %v; a level knob makes its"+
			" name contradict its own documentation",
			snapshotisolation.Mixin().Params)
	}
}
