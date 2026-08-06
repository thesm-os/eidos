// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline

import (
	"fmt"
	"testing"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/manifest"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
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

// invariantDual is a dual-role plugin with a version — the shape the
// precomputation is most likely to get wrong, since it reaches the
// plugin list twice and must be counted once.
type invariantDual struct{ name, version string }

func (p *invariantDual) Name() string                          { return p.name }
func (p *invariantDual) Version() string                       { return p.version }
func (*invariantDual) Annotate(*plugin.AnnotatorContext) error { return nil }
func (*invariantDual) Generate(*plugin.GeneratorContext) error { return nil }

// invariantFE is a minimal frontend carrying a version.
type invariantFE struct{ name, version string }

func (p *invariantFE) Name() string                     { return p.name }
func (p *invariantFE) Version() string                  { return p.version }
func (*invariantFE) Load(*plugin.FrontendContext) error { return nil }

// invariantBE is a minimal backend carrying a version.
type invariantBE struct{ name, version string }

func (p *invariantBE) Name() string                      { return p.name }
func (p *invariantBE) Version() string                   { return p.version }
func (*invariantBE) Language() string                    { return "golang" }
func (*invariantBE) Render(*plugin.BackendContext) error { return nil }

// buildInvariantPipeline returns a Pipeline registering a frontend, a
// dual-role plugin under both the annotator and generator roles, and
// a backend — plus a per-plugin routing override so routingHashes has
// something to distinguish.
func buildInvariantPipeline(t *testing.T) (*Pipeline, *invariantDual, *invariantBE) {
	t.Helper()
	dual := &invariantDual{name: "dual", version: "2.1.0"}
	be := &invariantBE{name: "be", version: "3.0.0"}
	p, err := New().
		WithFrontend(&invariantFE{name: "fe", version: "1.0.0"}).
		WithAnnotator(dual).
		WithGenerator(dual).
		WithBackend(be).
		WithSink(sink.NewMemory()).
		WithPluginOutput("dual", string(LayoutCentralised), "dualpkg", "dualdir").
		// A per-(plugin, tag) override that differs from the
		// per-plugin one. Without it LayoutPolicyForTag falls back to
		// LayoutPolicyFor and the two are indistinguishable, so a
		// precomputation keyed at the wrong granularity would pass.
		WithPluginTagOutput("dual", "tagged", string(LayoutCentralised), "tagpkg", "tagdir").
		Build()
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	return p, dual, be
}

// TestPrecomputeRunInvariants covers the values Build now pins.
//
// Each is asserted against the derivation it replaced rather than
// against a recorded constant, so the test proves equivalence rather
// than merely detecting change. The fixture registers one plugin
// under two roles because that is the case a naive precomputation
// double-counts — and a double-counted plugin changes the
// composition fingerprint, which frontends fold into their own cache
// keys.
func TestPrecomputeRunInvariants(t *testing.T) {
	t.Parallel()

	t.Run("the fingerprint matches a fresh composition hash", func(t *testing.T) {
		t.Parallel()
		p, _, _ := buildInvariantPipeline(t)
		plugins := p.registeredPlugins()
		parts := make([]string, 0, len(plugins))
		for _, pl := range plugins {
			version := ""
			if v, ok := any(pl).(plugin.Versioned); ok {
				version = v.Version()
			}
			parts = append(parts, pl.Name()+"@"+version)
		}
		if want := cache.HashStrings(parts); p.fingerprint != want {
			t.Fatalf("fingerprint = %q, want %q", p.fingerprint, want)
		}
	})

	t.Run("a dual-role plugin is counted once", func(t *testing.T) {
		t.Parallel()
		// Registered under two roles, so a precomputation that walked
		// the role slices without deduping would hash "dual@2.1.0"
		// twice and produce a different fingerprint.
		p, _, _ := buildInvariantPipeline(t)
		seen := 0
		for _, pl := range p.registeredPlugins() {
			if pl.Name() == "dual" {
				seen++
			}
		}
		if seen != 1 {
			t.Fatalf("dual-role plugin appears %d times in the composition", seen)
		}
		if len(p.pluginVersions) != len(p.registeredPlugins()) {
			t.Fatalf("pluginVersions has %d entries for %d plugins",
				len(p.pluginVersions), len(p.registeredPlugins()))
		}
	})

	t.Run("pluginVersions covers every role including the backend", func(t *testing.T) {
		t.Parallel()
		// The old lookup excluded backends from its flattened list
		// and covered the backend with a trailing fallback. The map
		// has to subsume both.
		p, _, _ := buildInvariantPipeline(t)
		for name, want := range map[string]string{
			"fe": "1.0.0", "dual": "2.1.0", "be": "3.0.0",
		} {
			if got := p.pluginVersions[name]; got != want {
				t.Fatalf("pluginVersions[%q] = %q, want %q", name, got, want)
			}
		}
	})

	t.Run("routingHashes match the per-plugin policy", func(t *testing.T) {
		t.Parallel()
		// Keyed at LayoutPolicyFor's granularity. The override above
		// gives "dual" a different policy from the default, so a
		// precomputation reading the wrong policy shows up here.
		p, _, _ := buildInvariantPipeline(t)
		for _, name := range []string{"fe", "dual", "be"} {
			want := cache.HashStrings(p.cacheRoutingComponents(name))
			if got := p.routingHashes[name]; got != want {
				t.Fatalf("routingHashes[%q] = %q, want %q", name, got, want)
			}
		}
		if p.routingHashes["dual"] == p.routingHashes["fe"] {
			t.Fatalf("the per-plugin override did not change dual's routing hash")
		}
	})

	t.Run("routing is keyed per plugin, not per tag", func(t *testing.T) {
		t.Parallel()
		// cacheRoutingComponents reads LayoutPolicyFor. Hashing the
		// per-tag policy instead would change every key it feeds, and
		// the two only differ when a per-tag override exists — which
		// the fixture registers precisely so this can fail.
		p, _, _ := buildInvariantPipeline(t)
		tagged := p.LayoutPolicyForTag("dual", "tagged")
		perTag := cache.HashStrings([]string{tagged.Layout, tagged.Package, tagged.Dir})
		if p.routingHashes["dual"] == perTag {
			t.Fatalf("routing hash matches the per-tag policy; it must use the per-plugin one")
		}
	})

	t.Run("scopeHash matches a fresh hash of its two inputs", func(t *testing.T) {
		t.Parallel()
		p, _, _ := buildInvariantPipeline(t)
		if want := cache.HashStrings([]string{p.targetSym, p.outFilename}); p.scopeHash != want {
			t.Fatalf("scopeHash = %q, want %q", p.scopeHash, want)
		}
	})

	t.Run("an unregistered name still resolves its routing hash", func(t *testing.T) {
		t.Parallel()
		// LayoutPolicyFor documents the unknown-name case as
		// resolving through the project + CLI merge. A map lookup
		// answering "" instead would change the key rather than
		// reproduce it.
		p, _, _ := buildInvariantPipeline(t)
		want := cache.HashStrings(p.cacheRoutingComponents("never-registered"))
		if got := p.routingHash("never-registered"); got != want {
			t.Fatalf("routingHash(unregistered) = %q, want %q", got, want)
		}
	})
}

// TestCacheKeyFor_IsStable pins the composed key by value.
//
// The equivalence tests above prove each input still derives the way
// it used to; this proves the composition of them has not moved. It
// is worth a literal rather than a re-derivation because nothing
// fails when a cache key changes — the run simply misses, silently,
// against every marker an older binary wrote.
//
// A deliberate constant: if this fails, the question to answer is
// "was the key change intended", not "does the derivation still
// match itself".
func TestCacheKeyFor_IsStable(t *testing.T) {
	t.Parallel()

	const want = "plugin:dual:version:2.1.0:reads:deadbeef:" +
		"routing:362ccfc09c8fcfc37077ed1587eb07c0857dc20938281b09f9807b395da76e74:" +
		"scope:6e340b9cffb37a989ca544e6bb780a2c78901d3fb33738768511a30617afa01d"

	p, _, _ := buildInvariantPipeline(t)
	if got := p.cacheKeyFor("dual", "deadbeef"); got != want {
		t.Fatalf("cache key changed:\ngot  %s\nwant %s", got, want)
	}
}
