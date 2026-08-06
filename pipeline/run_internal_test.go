// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline

import (
	"fmt"
	"testing"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/manifest"
)

// The keys the Layout phase writes into
// [manifest.ResolvedLayout.ResolvedFrom], one per composed Target
// field. They are a wire contract — `eidos explain` and drift tooling
// read them out of the manifest — so the table below names them once
// instead of repeating the literals across cases.
const (
	resolvedFromLayout   = "layout"
	resolvedFromPackage  = "package"
	resolvedFromDir      = "dir"
	resolvedFromFilename = "filename"
)

// baseManifestOutput returns a fully populated [manifest.Output] —
// Target, Hash, Plugins, and a ResolvedLayout whose ResolvedFrom
// attributes every composed field to a precedence layer. Each call
// builds fresh backing storage (the ResolvedLayout pointer and the
// ResolvedFrom map) so a case may perturb one clone without the
// other seeing it.
func baseManifestOutput() manifest.Output {
	return manifest.Output{
		Target: emit.Target{
			Dir:        "internal/repo",
			Filename:   "user_gen.go",
			Package:    "repo",
			ImportPath: "example.com/proj/internal/repo",
		},
		Plugins:    []manifest.PluginAttribution{{Name: "repogen"}},
		Hash:       "sha256:0f1e2d",
		PipelineID: "pid-1",
		ResolvedLayout: &manifest.ResolvedLayout{
			Layout:   LayoutAlongsideSource,
			Package:  "repo",
			Dir:      "internal/repo",
			Filename: "user_gen.go",
			ResolvedFrom: map[string]manifest.Layer{
				resolvedFromLayout:   manifest.LayerFramework,
				resolvedFromPackage:  manifest.LayerFramework,
				resolvedFromDir:      manifest.LayerFramework,
				resolvedFromFilename: manifest.LayerPluginSuffix,
			},
		},
	}
}

// dropResolvedLayout clears the observability block, modelling a
// manifest entry written before the routing layer landed (the field
// is `omitempty`) or one whose backend wrote to a Target the Layout
// phase never composed.
func dropResolvedLayout(o *manifest.Output) { o.ResolvedLayout = nil }

// formatResolvedLayout renders a ResolvedLayout pointer for a failure
// message. Printing the pointer with %+v would emit an address, which
// tells a reader nothing about which field stopped being compared;
// dereferencing prints the map in sorted-key order.
func formatResolvedLayout(rl *manifest.ResolvedLayout) string {
	if rl == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%+v", *rl)
}

// TestManifestOutputEqual_ResolvedLayout pins the ResolvedLayout arm
// of [manifestOutputEqual] — the sole gate on the manifest-write skip
// in [Pipeline.writeManifest]. When it wrongly reports "equal", a run
// whose routing changed leaves the previous manifest on disk, so the
// recorded ResolvedLayout block keeps describing routing that no
// longer happens and prune / drift tooling reads a manifest that
// contradicts the files beside it. When it wrongly reports "changed",
// every run rewrites the manifest and dirties it in version control —
// the churn the skip exists to prevent.
//
// The whole arm used to be dead under test. No test produced a
// manifest.Output carrying a non-nil ResolvedLayout, so mutation
// testing could negate any comparison in it — each of the four scalar
// checks, the ResolvedFrom length check, the per-key layer check, and
// both nil fast paths — and the suite stayed green. This table
// restores each comparison as an independent obligation:
//
//   - the identical-inputs case is what kills a negated scalar check,
//     a negated length check and a negated per-key layer check, since
//     each of those turns "no difference" into "changed";
//   - the both-nil case is what kills a negated fast path at the
//     top, which otherwise falls into the mixed-nil arm and reports
//     every manifest as changed;
//   - the mixed-nil cases are what kill a negated mixed-nil guard,
//     which otherwise dereferences a nil ResolvedLayout — a panic
//     swallowed by writeManifest's diag.RecoverAs into a manifest
//     that is silently never written.
//
// An end-to-end manifest test cannot stand in for this: the recording
// sink only attaches a ResolvedLayout when the Layout phase routed the
// decl, and asserting on the written file's bytes cannot distinguish a
// skipped write from a repeated identical one.
func TestManifestOutputEqual_ResolvedLayout(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		mutateA func(*manifest.Output)
		mutateB func(*manifest.Output)
		want    bool
	}{
		{
			name: "two outputs that routed identically compare equal",
			want: true,
		},
		{
			name:    "two outputs that carry no routing attribution at all compare equal",
			mutateA: dropResolvedLayout,
			mutateB: dropResolvedLayout,
			want:    true,
		},
		{
			name:    "an output carrying routing attribution differs from one without",
			mutateB: dropResolvedLayout,
			want:    false,
		},
		{
			name:    "an output without routing attribution differs from one carrying it",
			mutateA: dropResolvedLayout,
			want:    false,
		},
		{
			name: "a switch from alongside-source to centralised compares unequal",
			mutateB: func(o *manifest.Output) {
				o.ResolvedLayout.Layout = LayoutCentralised
			},
			want: false,
		},
		{
			name: "a newly resolved output package compares unequal",
			mutateB: func(o *manifest.Output) {
				o.ResolvedLayout.Package = "mocks"
			},
			want: false,
		},
		{
			name: "a moved output directory compares unequal",
			mutateB: func(o *manifest.Output) {
				o.ResolvedLayout.Dir = "internal/mocks"
			},
			want: false,
		},
		{
			name: "an overridden output filename compares unequal",
			mutateB: func(o *manifest.Output) {
				o.ResolvedLayout.Filename = "user_mock.go"
			},
			want: false,
		},
		{
			name: "attributing one more field than before compares unequal",
			mutateB: func(o *manifest.Output) {
				o.ResolvedLayout.ResolvedFrom["import_path"] = manifest.LayerFramework
			},
			want: false,
		},
		{
			name: "a directory that moved from the framework layer to a directive compares unequal",
			mutateB: func(o *manifest.Output) {
				o.ResolvedLayout.ResolvedFrom[resolvedFromDir] = manifest.LayerDirective
			},
			want: false,
		},
		{
			name: "the same fields resolved from wholly different layers compare unequal",
			mutateB: func(o *manifest.Output) {
				o.ResolvedLayout.ResolvedFrom = map[string]manifest.Layer{
					resolvedFromLayout:   manifest.LayerProject,
					resolvedFromPackage:  manifest.LayerCLI,
					resolvedFromDir:      manifest.LayerDirective,
					resolvedFromFilename: manifest.LayerCLI,
				}
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			a, b := baseManifestOutput(), baseManifestOutput()
			if tc.mutateA != nil {
				tc.mutateA(&a)
			}
			if tc.mutateB != nil {
				tc.mutateB(&b)
			}
			if got := manifestOutputEqual(a, b); got != tc.want {
				t.Fatalf(
					"manifestOutputEqual = %v, want %v\n  a.ResolvedLayout = %s\n  b.ResolvedLayout = %s",
					got, tc.want,
					formatResolvedLayout(a.ResolvedLayout),
					formatResolvedLayout(b.ResolvedLayout),
				)
			}
		})
	}
}
