// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest

import (
	"errors"
	"fmt"

	"go.thesmos.sh/eidos/store"
)

// The two ways a probed plugin call can fail, as sentinels rather
// than as a wrapping verb the caller re-reads out of the message.
//
// The distinction is a contract, not cosmetics: a returned error is
// how a role reports what it cannot classify, and a panic is a
// framework-level failure the pipeline cannot recover from. Reporting
// the first as the second told authors their plugin crashed when it
// had done exactly what the interface asks — and the obvious way to
// silence that message is `return nil`, which manufactures the no-op
// plugin every other check then passes vacuously.
//
// Sentinels rather than text prefixes because a prefix costs nothing
// to forget. The frontend suite classified outcomes by
// `strings.HasPrefix` and silently dropped every outcome that matched
// no prefix; these fail a type check instead.
var (
	// ErrProbePanicked wraps whatever a probed plugin call recovered.
	ErrProbePanicked = errors.New("plugintest: recovered panic")

	// ErrProbeReturnedError wraps a plugin's own returned error.
	ErrProbeReturnedError = errors.New("plugintest: plugin returned an error")

	// ErrFixtureBuildPanicked wraps a panic escaping a fixture's own
	// BuildStore. A fixture builder typically panics rather than
	// returns on anything the store rejects — a duplicate qualified
	// name is builder misuse, not test data — so a mistyped fixture
	// reaches the suite as a panic rather than an error.
	ErrFixtureBuildPanicked = errors.New("plugintest: fixture BuildStore panicked")
)

// probeVerb renders the verb matching err's sentinel, for failure
// messages that would otherwise have to guess.
func probeVerb(err error) string {
	if errors.Is(err, ErrProbePanicked) {
		return "panicked"
	}
	return "returned an error"
}

// buildFixtureStoreRecovering invokes build and converts a panic into
// an error wrapping [ErrFixtureBuildPanicked].
//
// Without the recover, one malformed fixture takes down the whole test
// binary: a builder panics on a state `AddPackage` rejects, the panic
// escapes the subtest, and every sibling subtest —
// and every sibling `Test` in the same package — never runs. Recovering
// turns that into one failed subtest naming the fixture.
//
// The callback takes no arguments so the recover is testable without
// fabricating a [testing.T]; callers close over their own.
func buildFixtureStoreRecovering(build func() *store.Store) (s *store.Store, err error) {
	defer func() {
		if r := recover(); r != nil {
			s = nil
			err = fmt.Errorf("%w: %v", ErrFixtureBuildPanicked, r)
		}
	}()
	return build(), nil
}
