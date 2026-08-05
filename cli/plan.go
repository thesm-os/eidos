// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cli

import (
	"context"
	"flag"
	"fmt"
	"io"

	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/plugin"
)

// PlanConfig holds the inputs for [PlanCommand]. The command
// constructs a pipeline from the supplied universe + config and
// prints the resolved [pipeline.Plan] — every frontend, annotator,
// generator in execution order plus the backend. No pipeline IO
// happens.
type PlanConfig struct {
	// File is the loaded config. The plan reflects whichever
	// plugins File.Plugins enables.
	File *Config

	// Plugins is the consumer's static plugin universe.
	Plugins []plugin.Plugin

	// Format selects text (default) or JSON output.
	Format DiagFormat

	// Routing carries the run's routing-layer flag overrides;
	// the plan output's resolved-policy section reflects the
	// merged-flag-and-config policy each plugin would see.
	Routing RoutingFlags
}

// PlanCommand prints the resolved plugin order without running
// the pipeline. Useful for CI debugging ("what will run?") and
// for documentation.
type PlanCommand struct{ Config PlanConfig }

// RegisterFlags binds [PlanCommand]'s flags into fs.
func (c *PlanCommand) RegisterFlags(fs *flag.FlagSet) {
	fs.Var(&c.Config.Format, FlagDiagFormat, UsageDiagFormat)
	c.Config.Routing.Register(fs)
}

// Execute resolves the plan and prints it. Returns [ExitUserError]
// on configuration faults; otherwise [ExitOK].
func (c *PlanCommand) Execute(_ context.Context, env *Env) (exit int) {
	// plan calls buildPipeline, so a plugin panicking in Outputs(),
	// Directives() or OptionsSchema() during Build unwinds through
	// here. Without the guard that escapes the "commands never panic
	// out" contract and reaches the user as a raw stack trace
	// instead of ExitInternalError.
	defer recoverInto(env, &exit)
	cfg := c.Config.File
	if cfg == nil {
		cfg = DefaultConfig()
	}
	routing, err := c.Config.Routing.Resolve(env, cfg, false)
	if err != nil {
		writeErr(env, "%v", err)
		return ExitUserError
	}
	p, err := buildPipeline(env, cfg, c.Config.Plugins, pipelineOverride{Routing: routing})
	if err != nil {
		writeErr(env, "%v", err)
		return ExitUserError
	}
	plan := p.Plan()
	switch c.Config.Format {
	case DiagFormatJSON:
		return c.writeJSON(env, p, plan)
	default:
		return c.writeText(env, p, plan)
	}
}

// writeText renders the plan as a human-readable block grouped by
// phase. Bucket boundaries surface so callers can see priority
// groupings at a glance. The resolved-policy section lists every
// registered plugin with its merged routing policy so users can
// answer "where would this plugin's output land?" without running
// the pipeline.
func (*PlanCommand) writeText(env *Env, p *pipeline.Pipeline, plan *pipeline.Plan) int {
	w := env.Stdout
	fmt.Fprintln(w, "frontends:")
	for _, fe := range plan.Frontends {
		fmt.Fprintf(w, "  - %s\n", fe.Name())
	}
	fmt.Fprintln(w, "annotators:")
	for _, bucket := range plan.AnnotatorBuckets {
		fmt.Fprintf(w, "  bucket %d:\n", int(bucket.Priority))
		for _, ann := range bucket.Plugins {
			fmt.Fprintf(w, "    - %s\n", ann.Name())
		}
	}
	fmt.Fprintln(w, "generators:")
	for _, bucket := range plan.GeneratorBuckets {
		fmt.Fprintf(w, "  bucket %d:\n", int(bucket.Priority))
		for _, gen := range bucket.Plugins {
			fmt.Fprintf(w, "    - %s\n", gen.Name())
		}
	}
	if plan.Backend != nil {
		fmt.Fprintf(w, "backend: %s\n", plan.Backend.Name())
	} else {
		fmt.Fprintln(w, "backend: (none)")
	}
	writeResolvedPolicy(w, p, plan)
	return ExitOK
}

// writeResolvedPolicy renders the per-plugin resolved routing
// policy. Plugins are listed in plan-execution order (frontends,
// annotators, generators, backend) so two runs over the same
// inputs produce byte-identical output.
func writeResolvedPolicy(w io.Writer, p *pipeline.Pipeline, plan *pipeline.Plan) {
	fmt.Fprintln(w, "resolved policy:")
	emit := func(name string) {
		pol := p.LayoutPolicyFor(name)
		fmt.Fprintf(w, "  %s:\n", name)
		fmt.Fprintf(w, "    layout=%s (from %s)\n", pol.Layout, pol.LayoutFrom)
		fmt.Fprintf(w, "    package=%s (from %s)\n", pol.Package, pol.PackageFrom)
		fmt.Fprintf(w, "    dir=%s (from %s)\n", pol.Dir, pol.DirFrom)
	}
	for _, fe := range plan.Frontends {
		emit(fe.Name())
	}
	for _, ann := range plan.Annotators {
		emit(ann.Name())
	}
	for _, gen := range plan.Generators {
		emit(gen.Name())
	}
	if plan.Backend != nil {
		emit(plan.Backend.Name())
	}
}

// planBucketEntry is one bucket's JSON projection — its integer
// priority plus the plugin names in topo-sorted order.
type planBucketEntry struct {
	Priority int      `json:"priority"`
	Plugins  []string `json:"plugins"`
}

// writeJSON renders the plan as a single JSON object suitable for
// machine consumption. The `resolved_policy` block lists every
// registered plugin's merged routing policy keyed by plugin name.
func (*PlanCommand) writeJSON(env *Env, p *pipeline.Pipeline, plan *pipeline.Plan) int {
	type policyJSON struct {
		Layout      string `json:"layout"`
		LayoutFrom  string `json:"layout_from"`
		Package     string `json:"package,omitempty"`
		PackageFrom string `json:"package_from"`
		Dir         string `json:"dir,omitempty"`
		DirFrom     string `json:"dir_from"`
	}
	type planJSON struct {
		Frontends      []string              `json:"frontends"`
		Annotators     []planBucketEntry     `json:"annotators"`
		Generators     []planBucketEntry     `json:"generators"`
		Backend        string                `json:"backend,omitempty"`
		ResolvedPolicy map[string]policyJSON `json:"resolved_policy"`
	}
	resolved := map[string]policyJSON{}
	collect := func(name string) {
		pol := p.LayoutPolicyFor(name)
		resolved[name] = policyJSON{
			Layout:      pol.Layout,
			LayoutFrom:  string(pol.LayoutFrom),
			Package:     pol.Package,
			PackageFrom: string(pol.PackageFrom),
			Dir:         pol.Dir,
			DirFrom:     string(pol.DirFrom),
		}
	}
	for _, fe := range plan.Frontends {
		collect(fe.Name())
	}
	for _, ann := range plan.Annotators {
		collect(ann.Name())
	}
	for _, gen := range plan.Generators {
		collect(gen.Name())
	}
	if plan.Backend != nil {
		collect(plan.Backend.Name())
	}
	out := planJSON{
		Frontends:      pluginNames(plan.Frontends),
		Annotators:     annotatorBucketsJSON(plan.AnnotatorBuckets),
		Generators:     generatorBucketsJSON(plan.GeneratorBuckets),
		ResolvedPolicy: resolved,
	}
	if plan.Backend != nil {
		out.Backend = plan.Backend.Name()
	}
	if err := encodeJSONLine(env.Stdout, out); err != nil {
		writeErr(env, "%v", err)
		return ExitInternalError
	}
	return ExitOK
}

// pluginNames returns Name() for each plugin in the slice. Used
// to flatten frontend / backend lists for JSON output.
func pluginNames[T plugin.Plugin](plugins []T) []string {
	out := make([]string, 0, len(plugins))
	for _, p := range plugins {
		out = append(out, p.Name())
	}
	return out
}

// annotatorBucketsJSON / generatorBucketsJSON project the
// per-bucket plugin lists into [planBucketEntry] values. Bucket
// priority renders as the raw integer; readers map back to the
// [priority.AnnotatorShape] / [priority.GeneratorFoundation]
// constants when they need names.
func annotatorBucketsJSON(buckets []pipeline.AnnotatorBucket) []planBucketEntry {
	out := make([]planBucketEntry, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, planBucketEntry{
			Priority: int(b.Priority),
			Plugins:  pluginNames(b.Plugins),
		})
	}
	return out
}

func generatorBucketsJSON(buckets []pipeline.GeneratorBucket) []planBucketEntry {
	out := make([]planBucketEntry, 0, len(buckets))
	for _, b := range buckets {
		out = append(out, planBucketEntry{
			Priority: int(b.Priority),
			Plugins:  pluginNames(b.Plugins),
		})
	}
	return out
}
