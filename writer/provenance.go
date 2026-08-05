// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package writer

import (
	"bytes"
)

// ProvenanceMarker is the brand-agnostic infix every backend stamps
// into its trailer — `<prefix><brand>:provenance <hash>`. Tools
// that consume the trailer (drift detection, idempotent re-write
// short-circuits) scan for this substring and read the hash that
// follows.
const ProvenanceMarker = ":provenance "

// ExtractProvenance returns the hash recorded in body's provenance
// trailer, or false when no trailer is present. The hash format
// is the backend's choice (Go stamps `sha256:<hex>`; future
// backends may stamp a different scheme) so callers compare
// hashes as opaque strings — two calls return equal values iff
// the underlying body bytes were equal at stamp time.
func ExtractProvenance(body []byte) (string, bool) {
	idx := bytes.LastIndex(body, []byte(ProvenanceMarker))
	if idx < 0 {
		return "", false
	}
	rest := body[idx+len(ProvenanceMarker):]
	end := bytes.IndexAny(rest, "\r\n")
	if end < 0 {
		end = len(rest)
	}
	return string(rest[:end]), true
}

// IsProvenanceAtTail reports whether body's provenance trailer is
// the final non-whitespace content of body. Returns false when no
// trailer is present, or when the file carries any non-whitespace
// content past the trailer's newline — the tamper-detection
// signal an idempotent re-write short-circuit consults to decide
// whether to trust the on-disk file's provenance hash. Manual
// edits appended past the `// <brand>: end of generated content.`
// marker leave the original provenance line intact, so a
// hash-only equality check would happily skip the rewrite and the
// user's tamper would stick. Requiring the trailer to be at the
// tail closes that gap.
func IsProvenanceAtTail(body []byte) bool {
	idx := bytes.LastIndex(body, []byte(ProvenanceMarker))
	if idx < 0 {
		return false
	}
	rest := body[idx+len(ProvenanceMarker):]
	nl := bytes.IndexAny(rest, "\r\n")
	if nl < 0 {
		return true
	}
	return len(bytes.TrimSpace(rest[nl:])) == 0
}
