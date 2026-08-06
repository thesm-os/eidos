// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"

	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/manifest"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
)

// PruneConfig holds the inputs for [PruneCommand]. The command
// runs the pipeline, then deletes files claimed by the previous
// manifest that the current run no longer claims — gated by a
// first-line marker check so manually-edited files survive.
type PruneConfig struct {
	// File is the loaded config.
	File *Config

	// Plugins is the consumer's static plugin universe.
	Plugins []plugin.Plugin

	// DryRun reports the planned deletions without applying them.
	// Useful for CI gates that want to surface "this would have
	// deleted N files" before the destructive run.
	DryRun bool

	// DeletedSources also deletes outputs whose source package no
	// longer exists in the module.
	//
	// Off by default because it is the one class of deletion driven
	// by inference rather than by observation: an unclaimed output
	// was seen not to be re-emitted, whereas a source-gone output is
	// deduced from a directory that is not there. `eidos check`
	// reports the class either way, so the drift is never silent —
	// this flag only governs whether prune acts on it unasked.
	DeletedSources bool

	// DiagFormat selects the diagnostic rendering format used for
	// pipeline diagnostics.
	DiagFormat DiagFormat

	// Verbose / Quiet propagate to the diagnostic filter.
	Verbose bool
	Quiet   bool

	// Routing carries the run's routing-layer flag overrides;
	// see [RoutingFlags] for the per-field semantics.
	Routing RoutingFlags
}

// PruneCommand implements the `prune` semantic per the CLI spec.
// Returns:
//
//   - [ExitOK] when prune completes (including DryRun reports).
//   - [ExitUserError] on configuration faults.
//   - [ExitPipelineError] when the pipeline run emitted Error
//     diagnostics.
//   - [ExitInternalError] on a recovered panic.
type PruneCommand struct{ Config PruneConfig }

// RegisterFlags binds [PruneCommand]'s flags into fs.
func (c *PruneCommand) RegisterFlags(fs *flag.FlagSet) {
	fs.Var(&c.Config.DiagFormat, FlagDiagFormat, UsageDiagFormat)
	fs.BoolVar(&c.Config.Verbose, FlagVerbose, false, UsageVerbose)
	fs.BoolVar(&c.Config.Quiet, FlagQuiet, false, UsageQuiet)
	fs.BoolVar(&c.Config.DryRun, FlagDryRun, false, UsageDryRun)
	fs.BoolVar(&c.Config.DeletedSources, FlagDeletedSources, false, UsageDeletedSources)
	c.Config.Routing.Register(fs)
}

// Execute reads the previous manifest, runs the pipeline (which
// overwrites the manifest with current outputs), diffs the two,
// and deletes files no longer claimed — subject to the
// marker-line guard.
func (c *PruneCommand) Execute(ctx context.Context, env *Env) (exit int) {
	defer recoverInto(env, &exit)

	cfg := c.Config.File
	if cfg == nil {
		cfg = DefaultConfig()
	}
	routing, err := c.Config.Routing.Resolve(env, cfg, c.Config.Verbose)
	if err != nil {
		writeErr(env, "%v", err)
		return ExitUserError
	}
	override := pipelineOverride{
		Verbose: c.Config.Verbose,
		Routing: routing,
		DryRun:  c.Config.DryRun,
	}
	if c.Config.DryRun {
		// Pair the dry-run pipeline with a memory sink so the
		// backend's file writes also stay in memory. WithDryRun on
		// the pipeline only suppresses the manifest write.
		override.SinkOverride = sink.NewMemory()
	}
	p, err := buildPipeline(env, cfg, c.Config.Plugins, override)
	if err != nil {
		writeErr(env, "%v", err)
		return ExitUserError
	}
	runErr := p.Run(ctx, patternsOrDefault(cfg)...)
	rerr := RenderDiagnostics(env.Stderr, p.Diag(), c.Config.DiagFormat, c.Config.Verbose, c.Config.Quiet)
	if rerr != nil {
		writeErr(env, "%v", rerr)
	}
	if runErr != nil && !errors.Is(runErr, pipeline.ErrRunHadErrors) {
		writeErr(env, "%v", runErr)
		return ExitPipelineError
	}

	return c.applyPrune(env, p, c.classify(env, p), runErr)
}

// classify resolves the prior manifest's unclaimed entries into the
// outputs this invocation is willing to delete.
//
// The source-gone probe runs only when [PruneConfig.DeletedSources]
// is set: without the flag the set stays empty, which reduces
// [manifest.PruneAll] to exactly the classification `prune` performed
// before the flag existed. Skipping the probe also means the
// filesystem is not walked at all on the default path.
func (c *PruneCommand) classify(env *Env, p *pipeline.Pipeline) []manifest.Output {
	prev := p.LastManifest()
	scope := p.ScopeImportPaths()
	opts := manifest.PruneOptions{
		Emitted:    p.EmittedTargets(),
		Scope:      scope,
		PipelineID: p.PipelineID(),
	}
	if c.Config.DeletedSources {
		opts.GoneSources = goneSources(env.Workdir, pruneCandidates(prev, scope, opts.PipelineID))
	}
	orphans := manifest.PruneAll(prev, opts)
	out := make([]manifest.Output, 0, len(orphans))
	for _, o := range orphans {
		if o.Reason == manifest.ReasonSourceGone && env.Diag != nil {
			// Named at Info rather than folded silently into the
			// count: this is the class deduced from an absent
			// directory rather than observed from a run, so the
			// operator gets to see which inference the deletion
			// rested on.
			env.Diag.For("pipeline.prune").Infof(
				position.Pos{File: filepath.Join(o.Target.Dir, o.Target.Filename)},
				"source package %q no longer exists in the module", o.Target.ImportPath,
			)
		}
		out = append(out, o.Output)
	}
	return out
}

// applyPrune walks the stale outputs, guards each delete on the
// first-line marker, removes deleted entries from the manifest,
// and reports the result. Returns the appropriate exit code.
//
// Under dry-run the function reports the would-deletes and skips
// both the os.Remove and the manifest rewrite — the pipeline's
// own writeManifest was already suppressed via [pipeline.Builder.WithDryRun].
// Entries that fail the marker guard (manually-edited file with
// no `Code generated by <brand>.` first line) are skipped and
// stay in the manifest so a follow-up prune sees them again.
func (c *PruneCommand) applyPrune(
	env *Env, p *pipeline.Pipeline, stale []manifest.Output, runErr error,
) int {
	deleted := 0
	skipped := 0
	survivors := map[emit.Target]struct{}{}
	for _, o := range stale {
		fullPath := filepath.Join(env.Workdir, o.Target.Dir, o.Target.Filename)
		ok, err := hasGeneratedMarker(fullPath, env.Brand)
		if err != nil {
			if env.Diag != nil {
				env.Diag.For("pipeline.prune").Warnf(position.Pos{File: fullPath}, "%v", err)
			}
			skipped++
			survivors[o.Target] = struct{}{}
			continue
		}
		if !ok {
			// File missing OR file present without the generated
			// marker. Missing files are still "claims to prune"
			// for the purpose of cleaning the manifest entry, so
			// only refuse when the file exists with no marker.
			if _, statErr := os.Stat(fullPath); statErr == nil {
				if env.Diag != nil {
					env.Diag.For("pipeline.prune").Infof(position.Pos{File: fullPath},
						"skipped: no `// Code generated by %s.` marker — refusing to delete a manually-edited file",
						env.Brand)
				}
				skipped++
				survivors[o.Target] = struct{}{}
				continue
			}
		}
		if c.Config.DryRun {
			fmt.Fprintf(env.Stdout, "would delete: %s\n", fullPath)
			deleted++
			continue
		}
		if rmErr := os.Remove(fullPath); rmErr != nil && !errors.Is(rmErr, os.ErrNotExist) {
			writeErr(env, "delete %s: %v", fullPath, rmErr)
			skipped++
			survivors[o.Target] = struct{}{}
			continue
		}
		fmt.Fprintf(env.Stdout, "deleted: %s\n", fullPath)
		deleted++
	}
	if c.Config.DryRun {
		fmt.Fprintf(env.Stdout, "prune dry-run: %d would-delete, %d skipped\n", deleted, skipped)
	} else {
		fmt.Fprintf(env.Stdout, "prune: %d deleted, %d skipped\n", deleted, skipped)
		c.rewriteManifestAfterPrune(env, p, stale, survivors)
	}
	if errors.Is(runErr, pipeline.ErrRunHadErrors) {
		return ExitPipelineError
	}
	return ExitOK
}

// rewriteManifestAfterPrune persists a manifest snapshot with
// every stale Output removed except those that survived the
// delete pass (marker-failed or os.Remove-failed). Skipped
// silently when there is no configured manifest path or no
// stale entries to remove. Errors surface as Warn diagnostics
// — the on-disk files are already gone, so a manifest-write
// failure should not abort the run.
func (c *PruneCommand) rewriteManifestAfterPrune(
	env *Env, p *pipeline.Pipeline, stale []manifest.Output, survivors map[emit.Target]struct{},
) {
	cfg := c.Config.File
	if cfg == nil {
		cfg = DefaultConfig()
	}
	mpath := manifestPath(env, cfg)
	if mpath == "" {
		return
	}
	source := p.LastManifest()
	if source == nil {
		return
	}
	staleKeys := map[manifestKey]struct{}{}
	for _, o := range stale {
		if _, kept := survivors[o.Target]; kept {
			continue
		}
		staleKeys[manifestKey{o.Target, o.PipelineID}] = struct{}{}
	}
	if len(staleKeys) == 0 {
		return
	}
	updated := manifest.New(source.RunID)
	updated.Brand = source.Brand
	for _, o := range source.Outputs {
		if _, drop := staleKeys[manifestKey{o.Target, o.PipelineID}]; drop {
			continue
		}
		updated.Add(o)
	}
	if err := manifest.Write(mpath, updated); err != nil {
		if env.Diag != nil {
			env.Diag.For("pipeline.prune").Warnf(
				position.Pos{File: mpath},
				"manifest write after prune failed: %v", err,
			)
		}
	}
}

// manifestKey is the (Target, PipelineID) pair that uniquely
// identifies an [manifest.Output] entry — two pipelines may
// share a Target so PipelineID participates in the lookup.
type manifestKey struct {
	Target     emit.Target
	PipelineID string
}

// hasGeneratedMarker returns true when the first non-empty line of
// the file at path starts with `// Code generated by <brand>.`.
// The marker is the conventional Go-generator signal the prune
// guard checks before deleting a file claimed by the previous
// manifest. A missing file is treated as "no marker" — the file
// is gone, nothing to delete.
func hasGeneratedMarker(path, brand string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("cli: open for marker check %s: %w", path, err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	prefix := fmt.Sprintf("// Code generated by %s.", brand)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		return strings.HasPrefix(line, prefix), nil
	}
	if err := scanner.Err(); err != nil {
		return false, fmt.Errorf("cli: scan marker line %s: %w", path, err)
	}
	return false, nil
}

// goneSources returns the subset of candidate import paths whose
// source package no longer exists on disk inside the current module.
//
// This is the filesystem half of the orphan classification
// [manifest.PruneAll] performs; the manifest package stays pure and
// receives the answer as a set. See [manifest.PruneOptions.GoneSources]
// for why the split falls here.
//
// # Why a directory probe rather than the run's patterns
//
// The alternative is to decide scope from the invocation — treat an
// entry as prunable when its import path falls under `./...`. That
// requires reimplementing Go's pattern semantics (relative vs
// absolute, `...` placement, multiple patterns, `all`) on a path that
// ends in os.Remove. A directory probe is local, needs no pattern
// algebra, and answers the question actually being asked: is the
// source still there.
//
// # Failure direction
//
// Every uncertain case resolves to "not gone", so the effect of being
// wrong is an entry that survives a prune it could have been included
// in — recoverable by running prune again — rather than a deleted
// file. Specifically: no go.mod found, an unparseable go.mod, an
// import path outside the module, and a directory that exists but
// holds no buildable Go (empty, or entirely excluded by build tags)
// all report not-gone. The last is deliberate: a package that fails
// to load is not a package that was deleted, and the two are
// indistinguishable from the loader's silence alone.
func goneSources(workdir string, candidates []string) map[string]struct{} {
	modRoot, modPath, ok := moduleIdentity(workdir)
	if !ok {
		return nil
	}
	gone := map[string]struct{}{}
	for _, ip := range candidates {
		rel, inside := moduleRelative(ip, modPath)
		if !inside {
			// Outside this module. Its lifecycle is not ours to
			// decide, and we cannot see its files to decide it.
			continue
		}
		if !dirExists(filepath.Join(modRoot, filepath.FromSlash(rel))) {
			gone[ip] = struct{}{}
		}
	}
	if len(gone) == 0 {
		return nil
	}
	return gone
}

// moduleIdentity locates the enclosing module's root directory and
// declared module path. Reports ok=false when no go.mod is found or
// its module directive is unreadable.
func moduleIdentity(workdir string) (root, modPath string, ok bool) {
	root, found := findUp(workdir, "go.mod")
	if !found {
		return "", "", false
	}
	body, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return "", "", false
	}
	modPath = modfile.ModulePath(body)
	if modPath == "" {
		return "", "", false
	}
	return root, modPath, true
}

// moduleRelative converts an import path into a path relative to the
// module root, reporting whether it lies inside the module at all.
//
// The boundary check is on a path segment, not a string prefix:
// `example.com/foo` must not swallow `example.com/foobar`, which
// shares its first twelve characters and is a different module. A
// prefix test would classify an unrelated module's package as living
// inside this one and probe a directory that was never its.
func moduleRelative(importPath, modPath string) (string, bool) {
	if importPath == modPath {
		return ".", true
	}
	rest, ok := strings.CutPrefix(importPath, modPath+"/")
	if !ok {
		return "", false
	}
	return rest, true
}

// dirExists reports whether path is an existing directory.
func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// pruneCandidates returns the import paths of prior entries that
// belong to this pipeline and were not re-emitted or examined — the
// only entries for which "did the source go away?" is worth asking.
//
// Restricting the probe to these keeps the filesystem work
// proportional to the drift rather than to the manifest, and keeps
// [goneSources] from stat-ing a path for every output on every prune.
func pruneCandidates(
	prev *manifest.Manifest, scope map[string]struct{}, pipelineID string,
) []string {
	if prev == nil || pipelineID == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, o := range prev.Outputs {
		if o.PipelineID != pipelineID {
			continue
		}
		ip := o.Target.ImportPath
		if ip == "" {
			continue
		}
		if _, dup := seen[ip]; dup {
			continue
		}
		if _, examined := scope[ip]; examined {
			continue
		}
		seen[ip] = struct{}{}
		// The framework routes test outputs into a sibling `_test`
		// import path that never had a directory of its own, so probe
		// the real package instead; manifest.PruneAll applies the same
		// shift when matching.
		out = append(out, strings.TrimSuffix(ip, "_test"))
	}
	return out
}
