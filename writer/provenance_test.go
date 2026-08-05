// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package writer_test

import (
	"strings"
	"testing"

	"go.thesmos.sh/eidos/writer"
)

// stamp composes a body with a provenance trailer in the form a
// backend writes it — a brand-prefixed marker line carrying the hash.
func stamp(t *testing.T, body, hash string) string {
	t.Helper()
	return body + "\n// eidos" + writer.ProvenanceMarker + hash + "\n"
}

// TestExtractProvenance covers the reader the disk sink consults to
// decide whether an on-disk file still matches what the pipeline
// produced.
func TestExtractProvenance(t *testing.T) {
	t.Parallel()

	t.Run("returns the hash recorded in the trailer", func(t *testing.T) {
		t.Parallel()
		got, ok := writer.ExtractProvenance([]byte(stamp(t, "package p", "sha256:abc123")))
		if !ok {
			t.Fatalf("trailer present but not extracted")
		}
		if got != "sha256:abc123" {
			t.Errorf("hash = %q, want sha256:abc123", got)
		}
	})

	t.Run("reports absent when the body carries no trailer", func(t *testing.T) {
		t.Parallel()
		// A hand-written file has no trailer; treating its absence as
		// an empty hash would make it compare equal to another
		// untrailered file and suppress a legitimate rewrite.
		if _, ok := writer.ExtractProvenance([]byte("package p\n")); ok {
			t.Errorf("untrailered body reported a provenance hash")
		}
	})

	t.Run("reports absent for an empty body", func(t *testing.T) {
		t.Parallel()
		if _, ok := writer.ExtractProvenance(nil); ok {
			t.Errorf("empty body reported a provenance hash")
		}
	})

	t.Run("reads the hash as an opaque string", func(t *testing.T) {
		t.Parallel()
		// The scheme prefix is the backend's choice, so the reader
		// must not parse or validate it — a future backend stamping
		// a different scheme still round-trips.
		got, ok := writer.ExtractProvenance([]byte(stamp(t, "x", "blake3:ff00")))
		if !ok || got != "blake3:ff00" {
			t.Errorf("hash = %q, %v; want blake3:ff00 read verbatim", got, ok)
		}
	})
}

// TestIsProvenanceAtTail covers the tamper check: a trailer that is
// no longer the final content means someone appended past it.
func TestIsProvenanceAtTail(t *testing.T) {
	t.Parallel()

	t.Run("true when the trailer ends the file", func(t *testing.T) {
		t.Parallel()
		if !writer.IsProvenanceAtTail([]byte(stamp(t, "package p", "sha256:abc"))) {
			t.Errorf("trailer at the tail reported as not at the tail")
		}
	})

	t.Run("true when only whitespace follows", func(t *testing.T) {
		t.Parallel()
		// Editors add trailing newlines; treating that as tampering
		// would force a rewrite of every untouched file.
		body := stamp(t, "package p", "sha256:abc") + "\n\n  \n"
		if !writer.IsProvenanceAtTail([]byte(body)) {
			t.Errorf("trailing whitespace reported as tampering")
		}
	})

	t.Run("false when content follows the trailer", func(t *testing.T) {
		t.Parallel()
		// This is the case the check exists for: a hash-only equality
		// test would happily skip the rewrite and the manual edit
		// would stick.
		body := stamp(t, "package p", "sha256:abc") + "func Manual() {}\n"
		if writer.IsProvenanceAtTail([]byte(body)) {
			t.Errorf("content appended past the trailer was not detected")
		}
	})

	t.Run("false when there is no trailer at all", func(t *testing.T) {
		t.Parallel()
		if writer.IsProvenanceAtTail([]byte("package p\n")) {
			t.Errorf("untrailered body reported a trailer at the tail")
		}
	})
}

// TestProvenanceMarker pins the infix itself, which tooling outside
// this repo greps for to recover a hash without parsing the line.
func TestProvenanceMarker(t *testing.T) {
	t.Parallel()

	t.Run("is brand-agnostic", func(t *testing.T) {
		t.Parallel()
		// Backends compose `<prefix><brand>` ahead of it, so the
		// marker must not carry a brand of its own or a rebranded
		// build stops matching.
		if strings.Contains(writer.ProvenanceMarker, "eidos") {
			t.Errorf("marker %q carries a brand", writer.ProvenanceMarker)
		}
	})
}
