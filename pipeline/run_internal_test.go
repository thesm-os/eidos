// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package pipeline

import (
	"fmt"
	"strings"
	"testing"

	"go.thesmos.sh/eidos/cache"
	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/manifest"
	"go.thesmos.sh/eidos/node"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
	"go.thesmos.sh/eidos/store"
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

// TestResolvedRecorder covers the phase-local recorder that replaced
// a mutex-guarded write per routed declaration.
//
// Two properties matter and neither is visible from the Layout tests
// that drive it end-to-end: that the memo cannot swallow a divergence,
// and that comparison is symmetric where the map range it replaced
// was one-directional.
func TestResolvedRecorder(t *testing.T) {
	t.Parallel()

	target := emit.Target{Dir: "d", Filename: "f.go", Package: "p"}
	base := manifest.ResolvedLayout{Layout: "alongside-source"}
	layers := layerSet{
		Filename: manifest.LayerPluginSuffix,
		Layout:   manifest.LayerFramework,
		Dir:      manifest.LayerFramework,
		Package:  manifest.LayerFramework,
	}

	newRec := func() (*resolvedRecorder, *diag.Sink) {
		d := diag.New()
		return &resolvedRecorder{diag: d, entries: map[emit.Target]resolvedEntry{}}, d
	}

	t.Run("a repeated identical record is silent", func(t *testing.T) {
		t.Parallel()
		rec, d := newRec()
		rec.record(target, base, layers)
		rec.record(target, base, layers)
		if got := len(d.Diagnostics()); got != 0 {
			t.Fatalf("identical re-record produced %d diagnostics: %+v", got, d.Diagnostics())
		}
		if got := len(rec.entries); got != 1 {
			t.Fatalf("entries = %d, want 1", got)
		}
	})

	t.Run("the memo does not swallow a divergence", func(t *testing.T) {
		t.Parallel()
		// The memo skips the map entirely on a hit. Keyed on Target
		// alone it would skip a differing composition too, and the
		// diagnostic this path exists to emit would never fire.
		rec, d := newRec()
		rec.record(target, base, layers)
		diverged := layers
		diverged.Package = manifest.LayerCLI
		rec.record(target, base, diverged)
		if !hasInternal(d, "divergent ResolvedLayout") {
			t.Fatalf("divergence not reported; got %+v", d.Diagnostics())
		}
	})

	t.Run("a divergence after an unrelated Target still reports", func(t *testing.T) {
		t.Parallel()
		// Clears the memo between the two records, so the second
		// takes the map path rather than the memo path.
		rec, d := newRec()
		rec.record(target, base, layers)
		rec.record(emit.Target{Dir: "other", Filename: "o.go", Package: "p"}, base, layers)
		diverged := layers
		diverged.Dir = manifest.LayerDirective
		rec.record(target, base, diverged)
		if !hasInternal(d, "divergent ResolvedLayout") {
			t.Fatalf("divergence not reported; got %+v", d.Diagnostics())
		}
	})

	t.Run("comparison is symmetric", func(t *testing.T) {
		t.Parallel()
		// The map range this replaced walked only the first value's
		// keys, so a layer set differing by a key absent from the
		// first compared equal. Struct equality has no such blind
		// side — this is a deliberate tightening.
		rec, d := newRec()
		rec.record(target, base, layerSet{Filename: manifest.LayerPluginSuffix})
		rec.record(target, base, layers)
		if !hasInternal(d, "divergent ResolvedLayout") {
			t.Fatalf("asymmetric difference not reported; got %+v", d.Diagnostics())
		}
	})

	t.Run("the first entry wins", func(t *testing.T) {
		t.Parallel()
		rec, _ := newRec()
		rec.record(target, base, layers)
		diverged := layers
		diverged.Package = manifest.LayerCLI
		rec.record(target, base, diverged)
		if got := rec.entries[target].layers; got != layers {
			t.Fatalf("stored layers = %+v, want the first recorded %+v", got, layers)
		}
	})

	t.Run("the manifest form carries all four keys", func(t *testing.T) {
		t.Parallel()
		// The keys are a wire contract: eidos explain and drift
		// tooling read them out of manifest.json.
		rec, _ := newRec()
		rec.record(target, base, layers)
		got := rec.entries[target].resolved().ResolvedFrom
		for k, want := range map[string]manifest.Layer{
			"filename": manifest.LayerPluginSuffix,
			"layout":   manifest.LayerFramework,
			"dir":      manifest.LayerFramework,
			"package":  manifest.LayerFramework,
		} {
			if got[k] != want {
				t.Fatalf("ResolvedFrom[%q] = %q, want %q", k, got[k], want)
			}
		}
		if len(got) != 4 {
			t.Fatalf("ResolvedFrom has %d keys, want 4: %+v", len(got), got)
		}
	})
}

// hasInternal reports whether d carries an Internal diagnostic
// mentioning substr.
func hasInternal(d *diag.Sink, substr string) bool {
	for _, dg := range d.Diagnostics() {
		if dg.Severity == diag.Internal && strings.Contains(dg.Message, substr) {
			return true
		}
	}
	return false
}

// TestManifestContentEqual pins the gate on the manifest-write
// skip at the whole-document level. RunID is deliberately excluded
// from the comparison — it is a per-run timestamp, so counting it
// would rewrite the manifest on every run and dirty it in version
// control, which is exactly what the skip exists to prevent.
func TestManifestContentEqual(t *testing.T) {
	t.Parallel()

	// pair returns two manifests describing the same single output,
	// so each case perturbs exactly one field away from equality.
	pair := func() (*manifest.Manifest, *manifest.Manifest) {
		a, b := manifest.New("run-a"), manifest.New("run-b")
		a.Brand, b.Brand = "eidos", "eidos"
		a.Add(baseManifestOutput())
		b.Add(baseManifestOutput())
		return a, b
	}

	t.Run("identical content compares equal despite differing RunID", func(t *testing.T) {
		t.Parallel()
		a, b := pair()
		if !manifestContentEqual(a, b) {
			t.Fatalf("RunID must not participate in the comparison")
		}
	})

	t.Run("a nil previous manifest compares unequal", func(t *testing.T) {
		t.Parallel()
		_, b := pair()
		if manifestContentEqual(nil, b) {
			t.Fatalf("nil prev must force a write, not skip it")
		}
	})

	t.Run("a nil current manifest compares unequal", func(t *testing.T) {
		t.Parallel()
		a, _ := pair()
		if manifestContentEqual(a, nil) {
			t.Fatalf("nil current must force a write, not skip it")
		}
	})

	t.Run("a differing schema version compares unequal", func(t *testing.T) {
		t.Parallel()
		a, b := pair()
		b.Version = a.Version + 1
		if manifestContentEqual(a, b) {
			t.Fatalf("a schema-version change must force a rewrite")
		}
	})

	t.Run("a differing brand compares unequal", func(t *testing.T) {
		t.Parallel()
		a, b := pair()
		b.Brand = "testkit"
		if manifestContentEqual(a, b) {
			t.Fatalf("a brand change must force a rewrite")
		}
	})

	t.Run("a differing output count compares unequal", func(t *testing.T) {
		t.Parallel()
		a, b := pair()
		extra := baseManifestOutput()
		extra.Target.Filename = "other_gen.go"
		b.Add(extra)
		if manifestContentEqual(a, b) {
			t.Fatalf("an added output must force a rewrite")
		}
	})

	t.Run("a differing output compares unequal", func(t *testing.T) {
		t.Parallel()
		a, b := pair()
		b.Outputs[0].Hash = "sha256:changed"
		if manifestContentEqual(a, b) {
			t.Fatalf("a changed body hash must force a rewrite")
		}
	})
}

// TestManifestOutputEqual_Plugins pins the contributing-plugin arm
// of [manifestOutputEqual]. Attribution is what the `explain` and
// prune commands read to answer "who wrote this file", so a change
// of contributor with an unchanged body still has to rewrite the
// manifest — the bytes are the same but the provenance is not.
func TestManifestOutputEqual_Plugins(t *testing.T) {
	t.Parallel()

	t.Run("the same attribution compares equal", func(t *testing.T) {
		t.Parallel()
		if !manifestOutputEqual(baseManifestOutput(), baseManifestOutput()) {
			t.Fatalf("identical attribution must compare equal")
		}
	})

	t.Run("a differing attribution count compares unequal", func(t *testing.T) {
		t.Parallel()
		a, b := baseManifestOutput(), baseManifestOutput()
		b.Plugins = append(b.Plugins, manifest.PluginAttribution{Name: "tracer"})
		if manifestOutputEqual(a, b) {
			t.Fatalf("an added contributor must compare unequal")
		}
	})

	t.Run("a differing attribution name compares unequal", func(t *testing.T) {
		t.Parallel()
		a, b := baseManifestOutput(), baseManifestOutput()
		b.Plugins[0] = manifest.PluginAttribution{Name: "mockgen"}
		if manifestOutputEqual(a, b) {
			t.Fatalf("a changed contributor must compare unequal")
		}
	})

	t.Run("a differing body hash compares unequal", func(t *testing.T) {
		t.Parallel()
		a, b := baseManifestOutput(), baseManifestOutput()
		b.Hash = "sha256:changed"
		if manifestOutputEqual(a, b) {
			t.Fatalf("a changed body hash must compare unequal")
		}
	})
}

// TestMergeManifestPreservingOutOfScope pins the merge's ordering
// contract. The manifest is a git-committed artefact, so its entry
// order has to be a total function of the entries themselves —
// otherwise two runs over the same inputs produce different bytes
// and the file churns in version control. The comparator therefore
// breaks ties down four Target fields and then PipelineID.
func TestMergeManifestPreservingOutOfScope(t *testing.T) {
	t.Parallel()

	// at builds an output varying only the fields the comparator
	// consults, so each case isolates one tiebreaker.
	at := func(dir, file, pkg, importPath, pid string) manifest.Output {
		o := baseManifestOutput()
		o.Target = emit.Target{Dir: dir, Filename: file, Package: pkg, ImportPath: importPath}
		o.PipelineID = pid
		return o
	}
	// merged runs the merge over a current manifest holding the
	// supplied outputs in the given order and returns the result.
	merged := func(outs ...manifest.Output) *manifest.Manifest {
		prev := manifest.New("run-prev")
		cur := manifest.New("run-cur")
		for _, o := range outs {
			cur.Add(o)
		}
		return mergeManifestPreservingOutOfScope(prev, cur)
	}

	t.Run("a nil previous manifest yields the current one unchanged", func(t *testing.T) {
		t.Parallel()
		cur := manifest.New("run-cur")
		cur.Add(baseManifestOutput())
		if got := mergeManifestPreservingOutOfScope(nil, cur); got != cur {
			t.Fatalf("nil prev must pass the current manifest through")
		}
	})

	t.Run("orders by directory first", func(t *testing.T) {
		t.Parallel()
		m := merged(at("b", "f.go", "p", "i", "x"), at("a", "f.go", "p", "i", "x"))
		if m.Outputs[0].Target.Dir != "a" {
			t.Fatalf("Dir order = %q first, want a", m.Outputs[0].Target.Dir)
		}
	})

	t.Run("breaks a directory tie on filename", func(t *testing.T) {
		t.Parallel()
		m := merged(at("d", "z.go", "p", "i", "x"), at("d", "a.go", "p", "i", "x"))
		if m.Outputs[0].Target.Filename != "a.go" {
			t.Fatalf("Filename order = %q first, want a.go", m.Outputs[0].Target.Filename)
		}
	})

	t.Run("breaks a filename tie on package", func(t *testing.T) {
		t.Parallel()
		m := merged(at("d", "f.go", "zed", "i", "x"), at("d", "f.go", "alpha", "i", "x"))
		if m.Outputs[0].Target.Package != "alpha" {
			t.Fatalf("Package order = %q first, want alpha", m.Outputs[0].Target.Package)
		}
	})

	t.Run("breaks a package tie on import path", func(t *testing.T) {
		t.Parallel()
		m := merged(at("d", "f.go", "p", "z/i", "x"), at("d", "f.go", "p", "a/i", "x"))
		if m.Outputs[0].Target.ImportPath != "a/i" {
			t.Fatalf("ImportPath order = %q first, want a/i", m.Outputs[0].Target.ImportPath)
		}
	})

	t.Run("breaks a target tie on pipeline id", func(t *testing.T) {
		t.Parallel()
		m := merged(at("d", "f.go", "p", "i", "zulu"), at("d", "f.go", "p", "i", "alpha"))
		if m.Outputs[0].PipelineID != "alpha" {
			t.Fatalf("PipelineID order = %q first, want alpha", m.Outputs[0].PipelineID)
		}
	})

	t.Run("keeps a previous entry the current run did not replace", func(t *testing.T) {
		t.Parallel()
		prev := manifest.New("run-prev")
		prev.Add(at("other", "kept.go", "p", "i", "other-pid"))
		cur := manifest.New("run-cur")
		cur.Add(at("d", "f.go", "p", "i", "x"))
		m := mergeManifestPreservingOutOfScope(prev, cur)
		if len(m.Outputs) != 2 {
			t.Fatalf("out-of-scope entry dropped; got %d outputs", len(m.Outputs))
		}
	})

	t.Run("replaces a previous entry the current run re-emitted", func(t *testing.T) {
		t.Parallel()
		prev := manifest.New("run-prev")
		stale := at("d", "f.go", "p", "i", "x")
		stale.Hash = "sha256:stale"
		prev.Add(stale)
		cur := manifest.New("run-cur")
		cur.Add(at("d", "f.go", "p", "i", "x"))
		m := mergeManifestPreservingOutOfScope(prev, cur)
		if len(m.Outputs) != 1 || m.Outputs[0].Hash == "sha256:stale" {
			t.Fatalf("re-emitted target must replace the previous entry; got %+v", m.Outputs)
		}
	})
}

// TestReportOrphans covers the notice for outputs a previous run wrote
// and this one no longer produces.
//
// The pipeline deliberately never deletes — a run cannot tell "no
// longer generated" from "not generated by this invocation" — so the
// alternative to saying something is how this surfaces today: a stale
// file referencing constructors that no longer exist, and a build
// failure naming the generated file rather than the change that
// orphaned it.
func TestReportOrphans(t *testing.T) {
	t.Parallel()

	// pipeWith returns a pipeline whose store holds one package, so
	// ScopeImportPaths reports that import path as loaded.
	pipeWith := func(importPath string) (*Pipeline, *diag.Sink) {
		s := store.New()
		if importPath != "" {
			if err := s.Nodes().AddPackage(&node.Package{Name: "p", Path: importPath}); err != nil {
				t.Fatalf("AddPackage: %v", err)
			}
		}
		d := diag.New()
		p := &Pipeline{diag: d}
		p.lastStore.Store(s)
		return p, d
	}
	out := func(file, importPath string) manifest.Output {
		o := baseManifestOutput()
		o.Target = emit.Target{Dir: "gen", Filename: file, Package: "p", ImportPath: importPath}
		return o
	}
	manifestOf := func(outs ...manifest.Output) *manifest.Manifest {
		m := manifest.New("run")
		for _, o := range outs {
			m.Add(o)
		}
		return m
	}
	warned := func(d *diag.Sink) string {
		for _, entry := range d.Diagnostics() {
			if entry.Severity == diag.Warn {
				return entry.Message
			}
		}
		return ""
	}

	t.Run("an output no longer produced is named", func(t *testing.T) {
		t.Parallel()
		p, d := pipeWith("example.com/x")
		p.reportOrphans(
			manifestOf(out("a.gen.go", "example.com/x"), out("b.gen.go", "example.com/x")),
			manifestOf(out("a.gen.go", "example.com/x")),
		)
		got := warned(d)
		if !strings.Contains(got, "b.gen.go") {
			t.Fatalf("warning = %q, want it to name the orphan", got)
		}
		if !strings.Contains(got, "prune") {
			t.Errorf("warning = %q, want it to name the command that removes it", got)
		}
	})

	t.Run("a package the run did not load is not reported", func(t *testing.T) {
		t.Parallel()
		// The false positive the pipeline must not produce: a narrow
		// `run ./sub/...` would otherwise call every other package's
		// output orphaned.
		p, d := pipeWith("example.com/x")
		p.reportOrphans(
			manifestOf(out("other.gen.go", "example.com/elsewhere")),
			manifestOf(),
		)
		if got := warned(d); got != "" {
			t.Fatalf("warned %q for an out-of-scope package", got)
		}
	})

	t.Run("a still-produced output raises nothing", func(t *testing.T) {
		t.Parallel()
		p, d := pipeWith("example.com/x")
		p.reportOrphans(
			manifestOf(out("a.gen.go", "example.com/x")),
			manifestOf(out("a.gen.go", "example.com/x")),
		)
		if got := warned(d); got != "" {
			t.Fatalf("warned %q when nothing was orphaned", got)
		}
	})

	t.Run("a first run raises nothing", func(t *testing.T) {
		t.Parallel()
		p, d := pipeWith("example.com/x")
		p.reportOrphans(nil, manifestOf(out("a.gen.go", "example.com/x")))
		if got := warned(d); got != "" {
			t.Fatalf("warned %q with no previous manifest", got)
		}
	})
}
