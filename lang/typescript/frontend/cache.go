// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"go.thesmos.sh/eidos/cache"
)

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
