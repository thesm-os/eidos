// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import "go.thesmos.sh/eidos/cache"

// CacheUsableForTest exposes cacheUsable so the predicate that gates
// the frontend's entire cache path can be tested directly. The
// predicate is unexported because nothing outside this package should
// be branching on a cache's concrete type.
func CacheUsableForTest(c cache.Cache) bool { return cacheUsable(c) }
