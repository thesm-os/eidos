// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package associative_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/associative"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, associative.Mixin(), associative.Name, nil)
}
