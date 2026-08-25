// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend

import (
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// FuzzTagEntriesOf drives the frontend's hand-rolled struct-tag
// decoder over arbitrary tag text.
//
// [tagEntriesOf] is whiteboxed rather than reached through
// [Frontend.Load] because the exported path would have to embed each
// input in a Go source file as a tag literal — the escaping that
// requires is itself a parser, so the fuzzer would be exercising the
// harness as much as the target, and a `go list` fork per input caps
// throughput at roughly ten executions a second.
//
// The decoder is not [reflect.StructTag.Lookup]: it walks entries in
// source order (the frontend stamps every key, not one), it skips
// whitespace the stdlib scanner treats as terminal, and it *recovers*
// from a value whose escapes do not decode where the stdlib stops
// dead. Those are deliberate divergences, so a differential against
// the stdlib can only arbitrate the subset both parsers read the same
// way. Three properties therefore run at three strengths:
//
//   - Differential, on the subset [referenceTagEntries] can decide:
//     the decoded entries must equal what a naive splitter plus
//     [reflect.StructTag.Lookup] produce. This is the only property
//     that catches a *dropped* entry — a decoder that returned
//     nothing would satisfy every other assertion here.
//   - Round-trip, on every input: re-serialising the entries into
//     canonical `key:"value"` form and decoding again must reproduce
//     them exactly, and a stdlib reader must see the same values in
//     that canonical form. This is what catches a mis-decoded escape,
//     the silent-corruption failure a crash-only target misses.
//   - Shape invariants, on every input: keys are non-empty, contain
//     none of the bytes the scanner terminates on, appear in source
//     order, and are bounded by the input length — the progress
//     guarantee that keeps the scan terminating.
//
// The seeds cover each branch the scanner takes — well-formed single
// and multiple entries, both whitespace separators, escapes, an
// escape that fails to decode followed by a live entry (the recovery
// path), and the degenerate exits: no colon, no opening quote, no
// closing quote, an empty key, an empty value, a duplicate key, and
// invalid UTF-8 in each position.
func FuzzTagEntriesOf(f *testing.F) {
	for _, seed := range []string{
		"",
		" ",
		"\t",
		`json:"id"`,
		`json:"id" db:"id_col" yaml:"identifier"`,
		`json:"id"  db:"id_col"`,
		" json:\"id\"",
		"\tjson:\"id\"",
		"json:\"id\"\tdb:\"id_col\"",
		`json:"id,omitempty"`,
		`json:"escaped \" quote"`,
		`json:"quo\"ted"`,
		`json:"tab\tseparated"`,
		`json:"bad\q" db:"id_col"`,
		`json:"unterminated`,
		`json`,
		`json:`,
		`json:'id'`,
		`:"id"`,
		`json:""`,
		`json:"a" json:"b"`,
		"\xff:\"id\"",
		"json:\"\xff\"",
		`a:"1" b:"2" c:"3" d:"4" e:"5"`,
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, tag string) {
		got := tagEntriesOf(reflect.StructTag(tag))

		assertTagEntryShape(t, tag, got)
		assertTagEntryRoundTrip(t, got)

		if want, decided := referenceTagEntries(tag); decided && !slices.Equal(got, want) {
			t.Fatalf("tagEntriesOf(%q) = %v, reference = %v", tag, got, want)
		}
	})
}

// assertTagEntryShape checks the invariants that hold for every input,
// well-formed or not.
//
// An empty key is unstampable — it would surface as the bare
// [MetaTagPrefix] with no name behind it. A key carrying one of the
// bytes the scanner terminates on means the scan ran past a boundary
// and swallowed the next entry's text. Keys out of source order, or
// keys absent from the input entirely, mean the decoder is
// synthesising rather than reading. The length bound encodes the
// progress guarantee: the shortest entry the grammar admits is four
// bytes (`k:""`), so more entries than a quarter of the input can only
// come from a scan that failed to advance — the shape of a
// non-terminating loop.
func assertTagEntryShape(t *testing.T, tag string, got []tagEntry) {
	t.Helper()
	if 4*len(got) > len(tag) {
		t.Fatalf("tagEntriesOf(%q) returned %d entries; at most %d fit", tag, len(got), len(tag)/4)
	}
	cursor := 0
	for i, entry := range got {
		if entry.key == "" {
			t.Fatalf("entry %d of %q decoded an empty key", i, tag)
		}
		if strings.ContainsAny(entry.key, ": \t") {
			t.Fatalf("entry %d of %q decoded key %q containing a scan terminator", i, tag, entry.key)
		}
		// Every accepted key is followed in the source by `:"`, so the
		// pair must be findable at or after the previous entry's. The
		// first match may precede the real one when the same text also
		// appears inside an earlier value, which only makes the cursor
		// conservative — never falsely red.
		at := strings.Index(tag[cursor:], entry.key+`:"`)
		if at < 0 {
			t.Fatalf("entry %d key %q is not present in %q at or after offset %d", i, entry.key, tag, cursor)
		}
		cursor += at + len(entry.key) + 2
	}
}

// assertTagEntryRoundTrip re-serialises the decoded entries into the
// canonical `key:"value"` form and requires the decoder to reproduce
// them from it, then requires a stdlib reader to agree on every value.
//
// The round-trip is what pins escape handling: a decoder that dropped
// a backslash, or unescaped one escape too many, produces a value
// whose re-quoted form no longer decodes to itself. The stdlib leg
// pins the *meaning* of the decoded value — the frontend stamps these
// strings for plugins that will compare them against what a
// `json`/`db` tag reader sees at runtime, so a divergence here is a
// generator emitting the wrong column or field name.
//
// The stdlib leg is skipped unless every key is readable by the
// stdlib scanner: it stops at the first key it cannot read, so one
// exotic key would hide every entry after it and fail the check for a
// difference this parser is documented to tolerate.
func assertTagEntryRoundTrip(t *testing.T, entries []tagEntry) {
	t.Helper()
	if len(entries) == 0 {
		return
	}
	var canonical strings.Builder
	for i, entry := range entries {
		if i > 0 {
			canonical.WriteByte(' ')
		}
		canonical.WriteString(entry.key)
		canonical.WriteByte(':')
		canonical.WriteString(strconv.Quote(entry.value))
	}
	text := canonical.String()
	if again := tagEntriesOf(reflect.StructTag(text)); !slices.Equal(again, entries) {
		t.Fatalf("re-decoding canonical %q gave %v, want %v", text, again, entries)
	}
	if !slices.ContainsFunc(entries, func(e tagEntry) bool { return !stdlibReadableKey(e.key) }) {
		assertStdlibAgrees(t, text, entries)
	}
}

// assertStdlibAgrees requires [reflect.StructTag.Lookup] to return the
// decoded value for every key of a canonical tag string. Only the
// first occurrence of a repeated key is checked — Lookup reports the
// first match and cannot see past it, while this decoder deliberately
// keeps every occurrence so the stamper's last-write-wins ordering
// stays observable.
func assertStdlibAgrees(t *testing.T, canonical string, entries []tagEntry) {
	t.Helper()
	st := reflect.StructTag(canonical)
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if seen[entry.key] {
			continue
		}
		seen[entry.key] = true
		got, ok := st.Lookup(entry.key)
		if !ok {
			t.Fatalf("stdlib cannot find key %q in canonical %q", entry.key, canonical)
		}
		if got != entry.value {
			t.Fatalf("stdlib read %q = %q from %q, decoder read %q", entry.key, got, canonical, entry.value)
		}
	}
}

// referenceTagEntries is the deliberately naive oracle: split the tag
// on spaces, require every field to be exactly `key:"quoted"`, and
// take the value from [reflect.StructTag.Lookup] rather than decoding
// it here. It returns decided=false for anything outside that subset,
// which is the point — the oracle is allowed to abstain, but where it
// answers, the frontend's decoder must match it entry for entry.
//
// Abstains on: any byte outside printable ASCII plus space (the two
// scanners disagree on tabs, control bytes and DEL by design), a field
// that is not a complete quoted entry, a value whose escapes do not
// decode, and a repeated key (Lookup can only speak for the first).
// Values containing a space fall out of the subset too, since the
// naive splitter would tear them in half.
func referenceTagEntries(tag string) ([]tagEntry, bool) {
	if !isPrintableASCII(tag) {
		return nil, false
	}
	st := reflect.StructTag(tag)
	seen := make(map[string]bool)
	var out []tagEntry
	for field := range strings.SplitSeq(tag, " ") {
		if field == "" {
			continue
		}
		key, quoted, ok := strings.Cut(field, ":")
		if !ok || key == "" || strings.Contains(key, `"`) || seen[key] {
			return nil, false
		}
		value, err := strconv.Unquote(quoted)
		if err != nil {
			return nil, false
		}
		lookedUp, found := st.Lookup(key)
		if !found || lookedUp != value {
			// The two reference halves disagree, so this input is
			// outside the subset the oracle can speak for — abstaining
			// is the only honest answer.
			return nil, false
		}
		seen[key] = true
		out = append(out, tagEntry{key: key, value: value})
	}
	return out, true
}

// isPrintableASCII reports whether every byte of s is printable ASCII
// or a space. Used to keep the oracle inside the byte range both
// scanners treat identically.
func isPrintableASCII(s string) bool {
	for i := range len(s) {
		if s[i] < ' ' || s[i] > '~' {
			return false
		}
	}
	return true
}

// stdlibReadableKey reports whether [reflect.StructTag.Lookup] can
// scan key — its name scan stops at any byte at or below space, at a
// colon, at a quote, and at DEL. The frontend's scanner stops only at
// a colon, space or tab, so it accepts keys the stdlib cannot see.
func stdlibReadableKey(key string) bool {
	for i := range len(key) {
		if b := key[i]; b <= ' ' || b == ':' || b == '"' || b == 0x7f {
			return false
		}
	}
	return true
}

// TestTagEntriesOf pins the two behaviours that make this decoder a
// hand-rolled parser rather than a call to
// [reflect.StructTag.Lookup]. Both are exactly the cases the fuzz
// oracle abstains on, so without them the divergences are unasserted.
func TestTagEntriesOf(t *testing.T) {
	t.Parallel()

	t.Run("an undecodable escape drops one entry and keeps reading", func(t *testing.T) {
		t.Parallel()
		// The stdlib stops at the first value it cannot unquote. This
		// frontend stamps whatever it can read, so a single mistyped
		// escape must not cost the field every tag that follows it.
		got := tagEntriesOf(`json:"bad\q" db:"id_col"`)
		want := []tagEntry{{key: "db", value: "id_col"}}
		if !slices.Equal(got, want) {
			t.Fatalf("tagEntriesOf = %v, want %v", got, want)
		}
	})

	t.Run("a tab separates entries as a space does", func(t *testing.T) {
		t.Parallel()
		// gofmt aligns struct tags with spaces, but nothing stops a
		// tag from arriving tab-separated; the stdlib scanner reads
		// neither the leading nor the interior tab.
		got := tagEntriesOf("\tjson:\"id\"\tdb:\"id_col\"")
		want := []tagEntry{{key: "json", value: "id"}, {key: "db", value: "id_col"}}
		if !slices.Equal(got, want) {
			t.Fatalf("tagEntriesOf = %v, want %v", got, want)
		}
	})

	t.Run("a repeated key keeps both occurrences in source order", func(t *testing.T) {
		t.Parallel()
		// Lookup answers with the first match and cannot see the
		// second. The stamper needs both — its last-write-wins
		// ordering is only meaningful if the decoder hands it every
		// occurrence.
		got := tagEntriesOf(`json:"first" json:"second"`)
		want := []tagEntry{{key: "json", value: "first"}, {key: "json", value: "second"}}
		if !slices.Equal(got, want) {
			t.Fatalf("tagEntriesOf = %v, want %v", got, want)
		}
	})
}
