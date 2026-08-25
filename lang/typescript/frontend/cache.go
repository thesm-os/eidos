// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/node"
)

// packageCacheKey composes the cache key for one directory's
// package.
//
// Six components, each of which changes what the converter would
// produce: the plugin's identity and declared version, the build's
// own module version, the pipeline's plugin fingerprint, the
// directory, and a hash over every input file plus the options.
//
// The fingerprint is not optional. A frontend's output does not
// depend on the plugin set in principle, but the graph it caches
// carries the metadata downstream plugins read, and an upgraded
// plugin expecting a stamp an older frontend never wrote is exactly
// the stale-cache failure it closes.
func packageCacheKey(dir string, paths []string, opts Options, fingerprint string) (string, error) {
	hash, err := hashInputs(paths, opts)
	if err != nil {
		return "", err
	}
	return cache.NewKey(
		"plugin", FrontendName,
		"version", FrontendVersion,
		"build", moduleVersion(),
		"pipeline", fingerprint,
		"dir", dir,
		"inputs", hash,
	), nil
}

// hashInputs returns a hash over every file the converter would
// parse, plus the options.
//
// Paths are sorted so the result does not depend on the walk's
// order, and each file contributes its path as well as its bytes —
// two files swapping names is a different package, and a
// content-only hash would call it the same one.
//
// Options contribute via their JSON form, so a new option field
// invalidates correctly without per-field tracking.
func hashInputs(paths []string, opts Options) (string, error) {
	sorted := slices.Clone(paths)
	slices.Sort(sorted)

	pieces := make([]string, 0, len(sorted)+1)
	for _, path := range sorted {
		body, err := os.ReadFile(path) //nolint:gosec // path is pattern-resolved
		if err != nil {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		pieces = append(pieces, path+"="+cache.HashBytes(body))
	}

	optsJSON, _ := json.Marshal(opts) //nolint:errcheck // Options is plain strings and bools; Marshal is total
	pieces = append(pieces, "opts="+cache.HashBytes(optsJSON))
	return cache.HashStrings(pieces), nil
}

// loadPackageFromCache returns a previously-cached package, or
// (nil, false) on any miss.
//
// Every failure is a miss, including a corrupt entry: the fallback is
// to convert the source again, which is always correct, and a cache
// that could block a run would be worse than no cache.
func loadPackageFromCache(c cache.Cache, key string) (*node.Package, bool) {
	if c == nil || key == "" {
		return nil, false
	}
	body, ok := c.Get(key)
	if !ok {
		return nil, false
	}

	var pkg node.Package
	if err := json.Unmarshal(body, &pkg); err != nil { //nolint:musttag // node types carry JSON tags transitively
		return nil, false
	}
	// Owner back-pointers are dropped by the JSON round-trip to break
	// the host-to-child cycle; the graph is unusable until they are
	// rebuilt.
	node.RewireOwners(&pkg)
	return &pkg, true
}

// storePackageInCache writes a converted package under key.
//
// Failures are ignored on the same reasoning as a miss: an unwritable
// cache costs the next run its speed, and nothing else.
func storePackageInCache(c cache.Cache, key string, pkg *node.Package) {
	if c == nil || key == "" {
		return
	}
	body, err := json.Marshal(pkg) //nolint:musttag // node types carry JSON tags transitively
	if err != nil {
		return
	}
	_ = c.Put(key, body) //nolint:errcheck // a cache write failure costs speed, not correctness
}
