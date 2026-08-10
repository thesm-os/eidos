// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package sdk_test

import (
	"testing"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/sdk"
)

// TestDiagAliasesPreserveIdentity pins the diagnostic surface as
// aliases. The pipeline hands a plugin its [Sink] typed as a
// [diag] value; a wrapper would make the façade spelling unusable
// for the exact field it exists to name.
//
//nolint:staticcheck // intentional redundant typing — the redundancy is the test
func TestDiagAliasesPreserveIdentity(t *testing.T) {
	t.Parallel()

	t.Run("sinks and diagnostics alias to the diag package", func(t *testing.T) {
		t.Parallel()
		var s1 *sdk.Sink
		var s2 *diag.Sink = s1
		_ = s2
		var p1 *sdk.PluginSink
		var p2 *diag.PluginSink = p1
		_ = p2
		var d1 sdk.Diag
		var d2 diag.Diag = d1
		_ = d2
		var sv1 sdk.Severity
		var sv2 diag.Severity = sv1
		_ = sv2
	})
}

// TestSeveritiesMatchUnderlying pins the severity ranks.
//
// The rank is load-bearing, not cosmetic: an Error blocks output
// for the whole run. A re-export that drifted by one would turn a
// per-declaration warning into a run-wide failure, or — worse —
// downgrade a real error into a note nobody reads.
func TestSeveritiesMatchUnderlying(t *testing.T) {
	t.Parallel()

	t.Run("each re-export equals its diag constant", func(t *testing.T) {
		t.Parallel()
		pairs := []struct {
			name string
			got  sdk.Severity
			want diag.Severity
		}{
			{"SeverityInfo", sdk.SeverityInfo, diag.Info},
			{"SeverityWarn", sdk.SeverityWarn, diag.Warn},
			{"SeverityError", sdk.SeverityError, diag.Error},
			{"SeverityInternal", sdk.SeverityInternal, diag.Internal},
		}
		for _, pair := range pairs {
			if pair.got != pair.want {
				t.Errorf("sdk.%s = %d, want %d", pair.name, pair.got, pair.want)
			}
		}
	})

	t.Run("the ranks stay strictly ascending", func(t *testing.T) {
		t.Parallel()
		// Plugin code compares severities (`d.Severity >=
		// sdk.SeverityError`), so the ordering is part of the
		// contract, not an implementation detail of the constants.
		ranked := []struct {
			name string
			sev  sdk.Severity
		}{
			{"SeverityInfo", sdk.SeverityInfo},
			{"SeverityWarn", sdk.SeverityWarn},
			{"SeverityError", sdk.SeverityError},
			{"SeverityInternal", sdk.SeverityInternal},
		}
		for i := 1; i < len(ranked); i++ {
			if ranked[i-1].sev >= ranked[i].sev {
				t.Errorf("%s (%d) does not rank below %s (%d)",
					ranked[i-1].name, ranked[i-1].sev,
					ranked[i].name, ranked[i].sev)
			}
		}
	})
}
