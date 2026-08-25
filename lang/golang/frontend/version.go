// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"runtime/debug"

	"go.thesmos.sh/eidos/emit"
)

// FrontendVersion is the Go frontend's own version identifier. The
// pipeline composes the version into the frontend's cache key so
// bumping this constant invalidates every cached package the
// frontend previously produced — appropriate when a conversion bug
// or shape change in the frontend would make older cache entries
// incorrect.
//
// The version is independent from [emit.Version] (which tracks the
// emit-graph contract) — the frontend produces nodes, not emit, so
// only its own conversion code affects this value.
const FrontendVersion = "1.0.0"

// supportedEmitVersions lists the emit major versions the frontend
// is compatible with. The frontend itself does not produce emit
// values, but it participates in the pipeline's emit-version
// compatibility check via the [plugin.EmitVersioned] capability so
// users get a positioned diagnostic at Build time if they pair the
// frontend with an emit major it does not understand.
var supportedEmitVersions = []string{emit.Major()}

// moduleVersion returns this frontend module's released version from
// the binary's build info, or "" when it is unavailable.
//
// It supplements FrontendVersion rather than replacing it. The
// constant is hand-maintained and sat unchanged across every stamping
// change the frontend ever shipped, which is what let a warm cache
// serve a node graph parsed by an older frontend indefinitely. Build
// info moves on its own whenever a consumer upgrades the dependency.
//
// It is deliberately not the primary defence: this workspace replaces
// every intra-repo module and `go test` builds report "(devel)", so
// the value is empty or useless in exactly the setup eidos is
// developed in. cache.NewKey drops empty segments, so an absent
// version costs nothing.
func moduleVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, dep := range info.Deps {
		if dep.Path == modulePath {
			if dep.Replace != nil {
				return "" // replaced: the version names something else
			}
			return dep.Version
		}
	}
	return ""
}

// modulePath is this module's import path, used to find its own
// entry in the binary's build info.
const modulePath = "go.thesmos.sh/eidos/lang/golang/frontend"
