// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package writer_test

import (
	"slices"
	"strconv"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/writer"
)

// provSink defends the benchmark loop bodies from dead-code
// elimination: [writer.ExtractProvenance] is a pure function of its
// argument, so without a store to a package-level variable the
// compiler may drop the call and the benchmark would report the cost
// of an empty loop.
var provSink string

// stamp composes a body with a provenance trailer in the form a
// backend writes it — a brand-prefixed marker line carrying the hash.
func stamp(t *testing.T, body, hash string) string {
	t.Helper()
	return stampBody(body, hash)
}

// stampBody is [stamp] without the testing handle, for the fuzz and
// benchmark bodies that hold a *testing.F or *testing.B at the point
// they need to build a stamped body.
func stampBody(body, hash string) string {
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

// splitProvenanceLines cuts s on either line terminator, so a body
// carrying classic-Mac "\r" endings decomposes the same way the
// production scanner treats it. Empty runs are dropped; the blankness
// checks below only care about fields that survive.
func splitProvenanceLines(s string) []string {
	return strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' })
}

// naiveProvenance is the fuzz differential for
// [writer.ExtractProvenance]: it decomposes the body into lines and
// walks them backwards, where the production reader scans raw bytes
// backwards from the last marker occurrence and then forwards to a
// terminator. Two implementations reaching different answers means
// one is wrong, and the byte scanner is the one that ships.
//
// It catches the mistakes a crash-only target cannot see: a forward
// Index in place of LastIndex (wrong trailer chosen when a body
// carries two), or a missing terminator branch (wrong hash when the
// trailer is the final line with no newline after it).
func naiveProvenance(body string) (string, bool) {
	for _, line := range slices.Backward(splitProvenanceLines(body)) {
		if j := strings.LastIndex(line, writer.ProvenanceMarker); j >= 0 {
			return line[j+len(writer.ProvenanceMarker):], true
		}
	}
	return "", false
}

// naiveProvenanceAtTail is the fuzz differential for
// [writer.IsProvenanceAtTail]: find the last line carrying the marker
// and require every later line to be blank. The production check
// instead takes the remainder of the body past the trailer's
// terminator and trims it whole — a different decomposition of the
// same question.
func naiveProvenanceAtTail(body string) bool {
	lines := splitProvenanceLines(body)
	last := -1
	for i, line := range lines {
		if strings.Contains(line, writer.ProvenanceMarker) {
			last = i
		}
	}
	if last < 0 {
		return false
	}
	for _, line := range lines[last+1:] {
		if strings.TrimSpace(line) != "" {
			return false
		}
	}
	return true
}

// FuzzExtractProvenance drives the trailer reader over arbitrary
// bytes and over bodies this test stamps itself.
//
// The reader runs against whatever is already on disk, which includes
// hand-written files, partially-written files, and files a user has
// edited — so its input is untrusted by construction. Its dangerous
// failure is not a crash but a *wrong hash*: the disk sink compares
// what this returns against the hash of the freshly rendered body and
// skips the write when they match, so a reader that returns a
// plausible-but-wrong string silently suppresses a legitimate
// regeneration.
//
// Three property families, strongest first:
//
//   - Differential against [naiveProvenance], a line-oriented
//     reimplementation.
//   - Round-trip: a well-formed trailer appended to any body reads
//     back the exact hash that was stamped.
//   - Invariants on the accepted path: presence agrees with a plain
//     substring search, the hash never spans a line boundary, and the
//     hash appears verbatim in the body rather than being synthesised.
func FuzzExtractProvenance(f *testing.F) {
	seeds := [][2]string{
		// Empty body: nothing to find.
		{"", ""},
		// The ordinary shape a backend writes.
		{"package p", "sha256:abc123"},
		// Empty hash: the marker is present but carries nothing.
		{"package p", ""},
		// A body with no marker at all, the hand-written-file case.
		{"package p\nfunc main() {}\n", "h"},
		// A bare marker with no brand prefix and no hash after it.
		{":provenance ", "h"},
		// Two trailers: the reader must take the last.
		{"a:provenance one\nb:provenance two\n", "three"},
		// Trailer as the final line with no terminator after it.
		{"x:provenance abc", "h"},
		// A classic-Mac terminator, which the scanner also honours.
		{"x:provenance a\rb", "h"},
		// A hash carrying the marker itself: not well-formed, so the
		// round-trip half must decline rather than assert.
		{"package p", "sha256::provenance nested"},
		// A hash carrying a terminator: same, from the other side.
		{"package p", "sha256:a\nb"},
		// Invalid UTF-8 on both sides; the reader is byte-oriented.
		{"\xff\xfe:provenance \x80\x81", "\xc3"},
		// A long body, so the backward scan has distance to cover.
		{strings.Repeat("// filler line\n", 64), "sha256:deadbeef"},
	}
	for _, seed := range seeds {
		f.Add(seed[0], seed[1])
	}

	f.Fuzz(func(t *testing.T, body, hash string) {
		got, ok := writer.ExtractProvenance([]byte(body))

		if wantOK := strings.Contains(body, writer.ProvenanceMarker); ok != wantOK {
			t.Fatalf("ExtractProvenance reported present=%v for a body that contains the marker=%v: %q",
				ok, wantOK, body)
		}

		wantHash, wantOK := naiveProvenance(body)
		if ok != wantOK || got != wantHash {
			t.Fatalf("ExtractProvenance(%q) = %q, %v; line-oriented reference says %q, %v",
				body, got, ok, wantHash, wantOK)
		}

		if !ok {
			if got != "" {
				t.Fatalf("ExtractProvenance reported absent but returned %q", got)
			}
			return
		}

		// A hash spanning a line boundary would carry the next line's
		// bytes into every downstream comparison, and no two runs
		// would ever agree.
		if strings.ContainsAny(got, "\r\n") {
			t.Fatalf("extracted hash %q spans a line boundary", got)
		}

		// The hash is read, never constructed: it must appear in the
		// body immediately after the marker exactly as returned.
		if !strings.Contains(body, writer.ProvenanceMarker+got) {
			t.Fatalf("extracted hash %q does not appear after the marker in %q", got, body)
		}

		// Round-trip. A hash carrying the marker or a terminator is
		// not a trailer a backend can write, so those inputs are
		// rejected here rather than asserted on.
		if strings.ContainsAny(hash, "\r\n") || strings.Contains(hash, writer.ProvenanceMarker) {
			return
		}
		stamped := []byte(stampBody(body, hash))
		back, ok := writer.ExtractProvenance(stamped)
		if !ok {
			t.Fatalf("a freshly stamped body reported no trailer: %q", stamped)
		}
		if back != hash {
			t.Fatalf("round-trip changed the hash: stamped %q, read back %q", hash, back)
		}
	})
}

// FuzzIsProvenanceAtTail drives the tamper check over arbitrary bytes
// and over bodies this test stamps and then deliberately tampers
// with.
//
// The check is the only thing standing between a manual edit and a
// silently skipped rewrite: [writer.ExtractProvenance] keeps
// returning the original hash after a user appends to the file, so a
// hash-only comparison would match and the edit would survive
// regeneration. That makes the interesting property adversarial — not
// "does it parse" but "does appended content always move the answer
// to false".
//
// Two property families:
//
//   - Differential against [naiveProvenanceAtTail].
//   - Tamper detection: for any stamped body, appending text is
//     tolerated exactly when the text is whitespace. Whitespace has
//     to be tolerated or every editor's trailing newline forces a
//     rewrite of an untouched file; anything else has to be caught.
//
// Plus the agreement invariant the disk sink relies on: a body whose
// trailer is at the tail must also be a body the reader can extract a
// hash from, since the sink calls both and compares the result.
func FuzzIsProvenanceAtTail(f *testing.F) {
	seeds := [][3]string{
		// Empty everything.
		{"", "", ""},
		// A stamped body with nothing appended: the untouched case.
		{"package p", "sha256:abc", ""},
		// Trailing whitespace, which editors add and which is tolerated.
		{"package p", "sha256:abc", "\n\n  \n"},
		// The tamper the check exists for.
		{"package p", "sha256:abc", "func Manual() {}\n"},
		// A single non-whitespace byte: the smallest tamper.
		{"", "h", "x"},
		// A body that already carries a trailer before the stamp.
		{"old:provenance stale\n", "fresh", ""},
		// A body whose trailer has no terminator after it.
		{"x:provenance abc", "h", ""},
		// A classic-Mac terminator inside the body.
		{"x:provenance a\rb", "h", ""},
		// Appended text carrying its own marker: not a well-formed
		// tamper for this property, so the assertion declines.
		{"package p", "h", "// eidos:provenance forged\n"},
		// Non-ASCII whitespace after the trailer.
		{"package p", "h", " "},
		// Invalid UTF-8 in every argument.
		{"\xff\xfe", "\x80", "\xc3"},
		// A long body, so the tail check has distance to cover.
		{strings.Repeat("// filler line\n", 64), "sha256:deadbeef", ""},
	}
	for _, seed := range seeds {
		f.Add(seed[0], seed[1], seed[2])
	}

	f.Fuzz(func(t *testing.T, body, hash, suffix string) {
		got := writer.IsProvenanceAtTail([]byte(body))
		if want := naiveProvenanceAtTail(body); got != want {
			t.Fatalf("IsProvenanceAtTail(%q) = %v; line-oriented reference says %v", body, got, want)
		}

		// The sink reads the hash only after the tail check passes.
		// A tail check that outruns the reader would leave it
		// comparing against a hash it never obtained.
		if got {
			if _, ok := writer.ExtractProvenance([]byte(body)); !ok {
				t.Fatalf("trailer reported at the tail of %q but no hash could be extracted", body)
			}
		}

		// Tamper detection needs a well-formed trailer to tamper with,
		// and appended text that does not bring a trailer of its own.
		if strings.ContainsAny(hash, "\r\n") || strings.Contains(hash, writer.ProvenanceMarker) {
			return
		}
		if strings.Contains(suffix, writer.ProvenanceMarker) {
			return
		}

		stamped := stampBody(body, hash)
		tampered := []byte(stamped + suffix)
		want := strings.TrimSpace(suffix) == ""
		if got := writer.IsProvenanceAtTail(tampered); got != want {
			t.Fatalf("IsProvenanceAtTail(stamped+%q) = %v, want %v (whitespace is tolerated, content is not)",
				suffix, got, want)
		}

		// The hash survives tampering unchanged. That is exactly why
		// the tail check has to exist: the reader alone cannot tell a
		// tampered file from an untouched one.
		back, ok := writer.ExtractProvenance(tampered)
		if !ok || back != hash {
			t.Fatalf("after appending %q the hash read back as %q, %v; want %q, true", suffix, back, ok, hash)
		}
	})
}

// BenchmarkExtractProvenance measures the trailer read the disk sink
// performs once per already-existing output file, before deciding
// whether that file needs rewriting at all.
//
// Two shapes, because [bytes.LastIndex] scans backwards and the two
// inputs the sink actually meets sit at opposite ends of that scan:
//
//   - a generated file, whose trailer is the last line. The scan
//     stops almost immediately, so the cost should stay flat as the
//     file grows — the claim that makes probing every file in a large
//     output tree affordable. This shape allocates: the hash is
//     copied out of the body into a fresh string.
//   - a hand-written file with no trailer. The scan reads the whole
//     body before it can report absence, so cost is linear in file
//     size. This shape allocates nothing; there is no hash to
//     materialise.
//
// Sizes are body lines. Bodies are built once, above the loop; the
// timed region is the read alone.
func BenchmarkExtractProvenance(b *testing.B) {
	b.ReportAllocs()

	sizes := []int{1, 10, 100, 1000}

	b.Run("trailer at the tail", func(b *testing.B) {
		for _, n := range sizes {
			body := []byte(stampBody(benchProvenanceBody(n), "sha256:"+strings.Repeat("ab", 32)))
			b.Run(strconv.Itoa(n), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					hash, ok := writer.ExtractProvenance(body)
					if !ok {
						b.Fatal("stamped body reported no trailer; the benchmark measured the absent path")
					}
					provSink = hash
				}
			})
		}
	})

	b.Run("no trailer", func(b *testing.B) {
		for _, n := range sizes {
			body := []byte(benchProvenanceBody(n))
			b.Run(strconv.Itoa(n), func(b *testing.B) {
				b.ReportAllocs()
				for b.Loop() {
					hash, ok := writer.ExtractProvenance(body)
					if ok {
						b.Fatal("untrailered body reported a trailer; the benchmark measured the present path")
					}
					provSink = hash
				}
			})
		}
	})
}

// benchProvenanceBody returns an n-line body carrying no provenance
// trailer, shaped like the generated source the sink reads back off
// disk.
func benchProvenanceBody(n int) string {
	var b strings.Builder
	for i := range n {
		b.WriteString("// generated line ")
		b.WriteString(strconv.Itoa(i))
		b.WriteByte('\n')
	}
	return b.String()
}
