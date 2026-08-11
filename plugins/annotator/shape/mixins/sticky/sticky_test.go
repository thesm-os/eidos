// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sticky_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/sticky"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, sticky.Mixin(), sticky.Name, sticky.Params)
}
