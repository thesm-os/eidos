// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sort"
	"strings"
)

// NewKey composes a canonical cache key from the supplied parts.
// Parts are joined by a colon — the conventional separator that
// keeps the human-readable prefix structure ("plugin:foo:v1:abc...")
// while remaining unambiguous in cache-backend storage. Empty parts
// are dropped so callers can pass conditionally-empty qualifiers
// without producing keys with collapsing or doubled separators.
//
// Typical shape: NewKey("plugin", pluginName, "version", pluginVersion, "input", inputHash).
// The format is convention, not policy — every cache implementation
// treats the result as an opaque string.
//
// # Allocation
//
// The common path — every part non-empty — joins parts directly and
// allocates only the result string. Filtering allocates one
// additional slice, and only when an empty part is actually present.
//
// NewKey never writes through parts. Callers expanding a slice
// (NewKey(parts...)) keep their backing array intact; an earlier
// implementation filtered in place via parts[:0] and corrupted it.
func NewKey(parts ...string) string {
	if slices.Contains(parts, "") {
		return joinNonEmpty(parts)
	}
	return strings.Join(parts, ":")
}

// joinNonEmpty is the filtering slow path of [NewKey], split out so
// the common all-non-empty case carries no filtering cost. It copies
// rather than filtering in place because parts may alias a caller's
// slice.
func joinNonEmpty(parts []string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, p := range parts {
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, ":")
}

// HashBytes returns the lower-case hex SHA-256 digest of b. The
// digest is the conventional "input hash" component in a cache key
// — frontends feed it the concatenated file bytes; annotators and
// generators feed it their per-plugin read-set hashes.
func HashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// HashStrings returns the SHA-256 hex digest of the sorted, NUL-
// joined input. Used to compose order-insensitive hashes from a
// set of identifiers — e.g. the file paths contributing to a
// frontend's per-package input set. Sorting before hashing keeps
// the digest deterministic regardless of caller-supplied order.
func HashStrings(items []string) string {
	sorted := append([]string(nil), items...)
	sort.Strings(sorted)
	return HashBytes([]byte(strings.Join(sorted, "\x00")))
}
