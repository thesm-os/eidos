// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package leakfree_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/leakfree"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, leakfree.Mixin(), leakfree.Name, nil)
}
