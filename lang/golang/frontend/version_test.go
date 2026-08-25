// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/docaudit"
)

// stampingSurfaceDigest pins the set of metadata keys this frontend
// declares. Update it in the same commit that bumps FrontendVersion,
// never on its own.
const stampingSurfaceDigest = "4d4707c00e874379d0c94b484559782282a8a38a814224ba518cf6a51d9f92e1"

// TestFrontendVersion_TracksStampingSurface fails when the set of
// metadata keys this frontend stamps changes without FrontendVersion
// moving with it.
//
// FrontendVersion is a component of the frontend's cache key, and for
// a long time it was the only thing standing between a warm cache and
// a node graph parsed by an older frontend. It is hand-maintained, and
// it sat unchanged across every stamping change the frontend ever
// shipped — including the one the streamconsumer detector depends on.
// Consumers with a warm cache kept the older graph indefinitely, and
// the only workaround was --no-cache.
//
// The composition fingerprint and build version now carried in the key
// make that failure far less likely, but neither reacts to a change
// that is purely internal to this package: adding a key without
// releasing invalidates nothing. This guard closes that last gap by
// deriving the surface from source rather than trusting a list, so it
// cannot silently agree with itself.
//
// When this fails, decide which happened:
//   - a key was added, removed or renamed -> bump FrontendVersion and
//     update the digest, in one commit;
//   - a key moved between files, or a call became non-literal ->
//     update the digest alone and say so in the commit body.
//
// The surface is deliberately over-inclusive. It spans the whole
// shared `go.*` vocabulary in lang/golang, which also holds the
// three render keys a bridge stamps rather than this frontend — so
// adding one of those bumps this digest too. That is the safe
// direction: over-invalidating costs a re-parse, while
// under-invalidating serves a cached graph that predates the key.
func TestFrontendVersion_TracksStampingSurface(t *testing.T) {
	t.Parallel()

	// Both directories, though this one now declares nothing. The
	// `go.*` vocabulary this frontend stamps is declared in lang/golang
	// so every Go-speaking consumer can import it, and the
	// cross-frontend marker moved to node for the same reason — so
	// scanning this package alone would find no key at all and leave
	// the guard blind to every one it actually stamps. The call stays
	// so a key declared here again is picked up rather than silently
	// unguarded.
	keys, err := docaudit.MetaKeys(packageSourceDir(t))
	if err != nil {
		t.Fatalf("collecting declared meta keys: %v", err)
	}
	vocab, err := docaudit.MetaKeys(langGolangSourceDir(t))
	if err != nil {
		t.Fatalf("collecting the shared Go vocabulary: %v", err)
	}
	keys = append(keys, vocab...)
	slices.Sort(keys)
	if len(keys) == 0 {
		t.Fatal("no metadata keys found; the guard is mis-wired and would pass for any change")
	}

	sum := sha256.Sum256([]byte(strings.Join(keys, "\n")))
	got := hex.EncodeToString(sum[:])

	if got != stampingSurfaceDigest {
		t.Fatalf("the stamping surface changed.\n  %d keys: %v\n  digest: %s\n"+
			"If a key was added, removed or renamed, bump FrontendVersion and set\n"+
			"stampingSurfaceDigest to the digest above in the same commit — a warm cache\n"+
			"keys on FrontendVersion, so a new key without a bump is served a graph that\n"+
			"predates it.", len(keys), keys, got)
	}
}
