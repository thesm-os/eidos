// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest

import (
	"fmt"
	"testing"

	"go.thesmos.sh/eidos/core/diag"
)

// Role nouns the diagnostic reporters stamp into failure messages.
// Named rather than repeated inline so the three role suites cannot
// spell the same role two ways, and so `go test` output is greppable
// by role across a multi-role plugin's run.
const (
	roleGenerator = "generator"
	roleAnnotator = "annotator"
	roleFrontend  = "frontend"
)

// emptyStoreSubject labels the input the empty-store probes drive.
// A generator or annotator that complains here complains on every
// project whose patterns expand to nothing, which is why the probe
// reads the sink at all.
const emptyStoreSubject = "an empty store"

// fixtureSubject renders a fixture name for a diagnostic failure
// message, matching the `fixture %q` shape the backend suite has used
// since it was the only suite reading its sink.
func fixtureSubject(name string) string { return fmt.Sprintf("fixture %q", name) }

// reportErrorDiagnostics fails tb when d recorded any Error- or
// Internal-severity diagnostic.
//
// A fixture is the plugin author's own declaration that this input is
// one the plugin handles. An Error diagnostic on it is not a stylistic
// complaint: [pipeline.Pipeline.Run] checks the run sink after every
// phase and returns ErrRunHadErrors, so the same input that scores
// green here exits non-zero for the user. The suite's contribution is
// moving that discovery to the commit that caused it.
//
// [diag.Sink.HasErrors] folds Internal in with Error deliberately —
// Internal is reserved for framework bugs, and a plugin that reaches
// for it on a fixture has either found one or mislabelled its own.
//
// Reads a snapshot copy of the sink ([diag.Sink.Diagnostics] clones),
// so a plugin still emitting from a stray goroutine cannot race the
// failure message.
func reportErrorDiagnostics(tb testing.TB, role, subject string, d *diag.Sink) {
	tb.Helper()
	if !d.HasErrors() {
		return
	}
	tb.Errorf(
		"%s produced Error-severity diagnostics on %s; the suite drives inputs the plugin "+
			"declares it handles, and any Error diagnostic fails the whole run through "+
			"pipeline.ErrRunHadErrors — fix the plugin, or narrow the fixture to the input "+
			"it genuinely covers\n  diagnostics: %+v",
		role, subject, d.Diagnostics(),
	)
}

// reportPositionlessDiagnostics fails tb when d recorded a diagnostic
// carrying a zero [position.Pos], unless the fixture waived the check.
//
// A zero position renders as a dash where the file and line belong, so
// the reader is told something is wrong and not where. The framework's
// own emitters all pass a real position; only plugin code has been
// free to skip it.
//
// Every severity is checked, not only Error. An Error with no position
// already fails [reportErrorDiagnostics]; the independent value here
// is the positionless Warn, which ships to users and cannot be acted
// on. The waiver exists because run- and package-level complaints
// genuinely name no source construct — a frontend rejecting an empty
// pattern has no line to point at — and a plugin whose diagnostics are
// all of that shape sets the fixture field rather than inventing a
// position.
func reportPositionlessDiagnostics(tb testing.TB, role, subject string, d *diag.Sink, allowed bool) {
	tb.Helper()
	if allowed {
		return
	}
	var bare []diag.Diag
	for _, entry := range d.Diagnostics() {
		if entry.Pos.IsZero() {
			bare = append(bare, entry)
		}
	}
	if len(bare) == 0 {
		return
	}
	tb.Errorf(
		"%s produced diagnostics carrying no source position on %s; the text formatter renders "+
			"a zero Pos as a dash where file and line belong, so the reader learns that something "+
			"is wrong and not where. Set AllowsPositionlessDiagnostics on the fixture when the "+
			"complaint genuinely names no source construct\n  diagnostics: %+v",
		role, subject, bare,
	)
}
