// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"runtime/debug"
	"sync"

	"go.thesmos.sh/eidos/emit"
)

// FrontendVersion is the frontend's declared version. It composes
// into the cache key for every package the frontend produces, so
// bumping it invalidates frontend-stage cache entries without
// disturbing other plugins'.
//
// Bump on any change to what the converter produces — a new stamp, a
// changed node shape, a corrected mapping. A cached graph this
// frontend would no longer produce is indistinguishable from a
// current one to everything downstream.
const FrontendVersion = "0.1.0"

// moduleVersion reports the build's own module version, folded into
// the cache key alongside [FrontendVersion].
//
// The declared version covers deliberate changes; this covers the
// rest. A bug fix in the converter that nobody remembered to bump a
// constant for still changes the graph, and a cache keyed only on the
// constant would serve the pre-fix shape indefinitely.
//
// Reports "(devel)" for a binary built from a working tree, which is
// what [debug.ReadBuildInfo] supplies when no version is stamped. That
// is stable within a build, so a development run caches normally; it
// does not distinguish two development builds, which is why a
// converter change during development wants the constant bumped or
// the cache cleared.
var moduleVersion = sync.OnceValue(func() string { //nolint:gochecknoglobals // immutable after first call
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}
	for _, dep := range info.Deps {
		if dep.Path == modulePath {
			return dep.Version
		}
	}
	return info.Main.Version
})

// modulePath is this module's import path, used to find its own
// entry in the build info's dependency list when it is linked as a
// dependency rather than built as the main module.
const modulePath = "go.thesmos.sh/eidos/lang/typescript"

// supportedEmitVersions lists the emit major versions this frontend
// is compatible with.
//
// The frontend produces no emit values, but it participates in the
// pipeline's compatibility check so a user pairing it with an
// incompatible emit major sees a positioned diagnostic rather than a
// failure further down.
var supportedEmitVersions = []string{
	emit.Major(),
} //nolint:gochecknoglobals // immutable after init
