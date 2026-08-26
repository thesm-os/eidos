// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import "go.thesmos.sh/eidos/emit"

// BackendVersion is the plugin's declared version. It composes into
// the pipeline's plugin fingerprint, which frontends fold into their
// cache keys — so bumping it invalidates a warm cache populated when
// this plugin rendered differently.
//
// A plugin that declares no version contributes an empty string and
// can never invalidate anything, which is a silent staleness bug
// waiting for its first behavioural change.
const BackendVersion = "0.1.0"

// supportedEmitVersions lists the emit major versions this backend
// renders.
var supportedEmitVersions = []string{
	emit.Major(),
} //nolint:gochecknoglobals // immutable after init
