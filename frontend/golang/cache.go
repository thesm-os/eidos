// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package golang

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"golang.org/x/tools/go/packages"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/node"
)

// packageCacheKey composes the canonical cache key for one Go
// package. The key incorporates the package path, the frontend's
// version, the configured options, and a hash over every input
// file's bytes — the inputs that drive conversion. Any change to
// any of these invalidates the cache entry naturally.
//
// Returns the underlying I/O error when a source file cannot be
// re-read between [packages.Load] and the hash pass — an external
// process mutating the file system mid-run produces a real error
// the caller surfaces as a positioned diagnostic rather than
// silently corrupting the cache key.
func packageCacheKey(pkg *packages.Package, opts Options, fingerprint string) (string, error) {
	hash, err := hashPackageInputs(pkg, opts)
	if err != nil {
		return "", err
	}
	return cache.NewKey(
		"plugin", FrontendName,
		"version", FrontendVersion,
		"build", moduleVersion(),
		"pipeline", fingerprint,
		"pkg", pkg.PkgPath,
		"inputs", hash,
	), nil
}

// hashPackageInputs returns a SHA-256 hash over every file the
// converter would parse for pkg, plus the configured options. The
// file hashes are sorted so the result is invariant to the loader's
// reporting order, and the options contribute via their JSON form
// so changes to [Options] trigger invalidation without bespoke
// per-field tracking.
//
// Returns the wrapped [os.ReadFile] error when a path resolved by
// [packages.Load] cannot be re-read here. Production callers can
// observe this when an external process deletes / changes
// permissions on a source file between Load and the hash pass.
func hashPackageInputs(pkg *packages.Package, opts Options) (string, error) {
	files := append([]string(nil), pkg.GoFiles...)
	slices.Sort(files)
	pieces := make([]string, 0, len(files)+1)
	for _, path := range files {
		body, err := os.ReadFile(path) //nolint:gosec // pkg.GoFiles are loader-resolved
		if err != nil {
			return "", fmt.Errorf("read %s: %w", path, err)
		}
		pieces = append(pieces, path+"="+cache.HashBytes(body))
	}
	optsJSON, _ := json.Marshal(opts) //nolint:errcheck // Options is plain string/bool fields; json.Marshal is total.
	pieces = append(pieces, "opts="+cache.HashBytes(optsJSON))
	return cache.HashStrings(pieces), nil
}

// loadPackageFromCache attempts to read a previously-cached
// [node.Package] for pkg from c. On hit it deserialises the JSON
// blob, reconstructs back-pointers via [node.RewireOwners], and
// returns the result; on miss or any error it returns (nil, false)
// so the caller falls back to a fresh conversion.
//
// Cache errors are intentionally swallowed — a corrupt entry is
// equivalent to a miss, and we don't want stale cache state to
// block the run.
func loadPackageFromCache(c cache.Cache, key string) (*node.Package, bool) {
	if c == nil {
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
	node.RewireOwners(&pkg)
	return &pkg, true
}

// storePackageInCache serialises pkg and writes it to c under the
// supplied key. Cache write failures are surfaced as Warn
// diagnostics (the cache is observability, not correctness) so a
// disk-full scenario doesn't fail the run.
//
// JSON marshal of a [node.Package] cannot fail in practice — every
// reachable field has a JSON-encodable type and back-pointer cycles
// are broken by [json:"-"] on Owner fields — so the marshal error
// is dropped and treated as an unreachable contract violation.
func storePackageInCache(c cache.Cache, key string, pkg *node.Package, sink *diag.PluginSink) {
	if c == nil || pkg == nil {
		return
	}
	body, _ := json.Marshal(pkg) //nolint:errcheck,musttag // node graphs are JSON-safe by construction
	if err := c.Put(key, body); err != nil {
		sink.Warnf(position.Pos{}, "cache put failed: %v", err)
	}
}
