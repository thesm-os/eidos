// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package timeaware_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/timeaware"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, timeaware.Mixin(), timeaware.Name, nil)
}
