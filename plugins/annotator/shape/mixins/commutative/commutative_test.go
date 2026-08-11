// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package commutative_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/commutative"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, commutative.Mixin(), commutative.Name, nil)
}
