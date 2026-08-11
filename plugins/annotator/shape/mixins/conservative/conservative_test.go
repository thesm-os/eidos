// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package conservative_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/conservative"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, conservative.Mixin(), conservative.Name, conservative.Params)
}
