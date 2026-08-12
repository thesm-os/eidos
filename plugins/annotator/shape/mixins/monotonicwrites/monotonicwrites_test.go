// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package monotonicwrites_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/monotonicwrites"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, monotonicwrites.Mixin(), monotonicwrites.Name, monotonicwrites.Params)
}
