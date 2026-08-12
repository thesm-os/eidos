// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package writesfollowreads_test

import (
	"testing"

	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/internal/mixintest"
	"go.thesmos.sh/eidos/plugins/annotator/shape/mixins/writesfollowreads"
)

func TestMixin_Identity(t *testing.T) {
	t.Parallel()
	mixintest.AssertIdentity(t, writesfollowreads.Mixin(), writesfollowreads.Name, writesfollowreads.Params)
}
