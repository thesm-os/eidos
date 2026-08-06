// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline_test

import (
	"fmt"
	"testing"

	"go.thesmos.sh/eidos/pipeline"
)

func TestPhase_String(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		p    pipeline.Phase
		want string
	}{
		{"Frontend", pipeline.PhaseFrontend, "frontend"},
		{"Annotator", pipeline.PhaseAnnotator, "annotator"},
		{"Generator", pipeline.PhaseGenerator, "generator"},
		{"unknown stringifies with a marker", pipeline.Phase(99), "phase(?)"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.p.String(); got != tc.want {
				t.Fatalf("%s String = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// ExamplePhase_String shows the textual form each [pipeline.Phase]
// takes. These are the exact tokens a verbose run prints in the
// `phase=` prefix of its Info diagnostics, so this is how to read a
// log line back to the [pipeline.Builder.WithParallel] argument that
// governs it.
func ExamplePhase_String() {
	for _, p := range []pipeline.Phase{
		pipeline.PhaseFrontend,
		pipeline.PhaseAnnotator,
		pipeline.PhaseGenerator,
	} {
		fmt.Println(p)
	}
	// Output:
	// frontend
	// annotator
	// generator
}
