// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package total_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/total"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, total.Mixin(), total.Name, total.Params)
}
