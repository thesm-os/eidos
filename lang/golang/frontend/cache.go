// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"golang.org/x/tools/go/packages"

	"go.thesmos.sh/eidos/cache"
)

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
	// The built module's version joins the options, because
	// FrontendVersion is a constant an author bumps while this
	// changes whenever the converter does. Only this package can
	// read its own build info, so the framework cannot fold it in.
	pieces = append(pieces,
		"opts="+cache.HashBytes(optsJSON),
		"build="+moduleVersion(),
	)
	return cache.HashStrings(pieces), nil
}
