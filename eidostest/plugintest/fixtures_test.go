// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package plugintest_test

import (
	"testing"

	"go.thesmos.sh/eidos/eidostest/plugintest"
)

// TestShippedFixturesAnswerTheProbedLanguages pins that every
// language key a fixture this package ships declares is one the suite
// actually drives.
//
// The failure it closes has happened twice. constants.go records the
// first: two framework checks drove {"go","rust","ts","py",""} and
// validated an empty Outputs slice for every plugin in the tree. The
// second was NewMultiOutputFixturePlugin keying OutputsByLang on "go"
// while probeLanguages drives "golang" — so the exported fixture
// offered to plugin authors "as a contract reference for the
// plugin.Output slice" answered no probe, and the two meta-tests built
// on it asserted nothing about Outputs at all.
//
// A fixture keyed on a language the suite never asks about is not a
// weaker test. It is no test, reporting the same green as a real one.
func TestShippedFixturesAnswerTheProbedLanguages(t *testing.T) {
	t.Parallel()

	probed := make(map[string]struct{}, len(plugintest.ProbeLanguages()))
	for _, lang := range plugintest.ProbeLanguages() {
		probed[lang] = struct{}{}
	}

	fixtures := map[string]*plugintest.FixturePlugin{
		"NewFixturePlugin":            plugintest.NewFixturePlugin(),
		"NewMultiOutputFixturePlugin": plugintest.NewMultiOutputFixturePlugin(),
	}
	for name, p := range fixtures {
		t.Run(name+" keys every declared output on a probed language", func(t *testing.T) {
			t.Parallel()
			if len(p.OutputsByLang) == 0 {
				t.Fatalf("%s declares no outputs at all; it cannot serve as an Outputs reference", name)
			}
			for lang := range p.OutputsByLang {
				if _, ok := probed[lang]; !ok {
					t.Errorf(
						"%s declares outputs for language %q, which the suite never probes "+
							"(it drives %v); the declaration is unreachable and every check "+
							"reading it validates an empty slice",
						name, lang, plugintest.ProbeLanguages(),
					)
				}
			}
		})
	}
}
