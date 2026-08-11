// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package causal_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/causal"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, causal.Mixin(), causal.Name, nil)
}
