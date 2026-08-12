// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package monotonicreads_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/monotonicreads"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, monotonicreads.Mixin(), monotonicreads.Name, monotonicreads.Params)
}
