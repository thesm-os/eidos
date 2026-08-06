// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package protobuf

import "runtime/debug"

// FrontendVersion is the protobuf frontend's own version identifier.
// The string composes into the frontend's per-plugin cache key
// alongside the resolved descriptor set's content hash; bumping the
// constant invalidates every cache entry the frontend produced
// under the previous version, picking up changes to the converter's
// translation rules (new meta keys, changed type-mapping conventions,
// etc.) without disturbing other plugins' caches.
//
// The version is a semantic-version string. Bumps follow the
// emit-contract evolution rules documented on [plugin.Versioned]:
// patch for bug fixes that preserve every documented meta key,
// minor for additive meta-key additions, major for breaking changes
// to the documented vocabulary.
const FrontendVersion = "1.0.0"

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
const modulePath = "go.thesmos.sh/eidos/frontend/protobuf"
