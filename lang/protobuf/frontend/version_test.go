// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package frontend_test

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/docaudit"
)

// stampingSurfaceDigest pins the set of metadata keys this frontend
// declares. Update it in the same commit that bumps FrontendVersion,
// never on its own.
const stampingSurfaceDigest = "36e2ab6253bfa341404f884a17ce69d769e1117272518d9980163728d17dd4c4"

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
func TestFrontendVersion_TracksStampingSurface(t *testing.T) {
	t.Parallel()

	keys, err := docaudit.MetaKeys(".")
	if err != nil {
		t.Fatalf("collecting declared meta keys: %v", err)
	}
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
