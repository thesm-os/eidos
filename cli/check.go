// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/manifest"
	"go.thesmos.sh/eidos/pipeline"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/sink"
	"go.thesmos.sh/eidos/writer"
)

// CheckConfig holds the inputs for [CheckCommand]. The command
// runs the pipeline against an in-memory sink, then byte-equal
// compares every captured output against the corresponding file
// on disk. Any difference — including whitespace — counts as
// drift.
type CheckConfig struct {
	File       *Config
	Plugins    []plugin.Plugin
	DiagFormat DiagFormat
	Verbose    bool
	Quiet      bool

	// Routing carries the run's routing-layer flag overrides;
	// see [RoutingFlags] for the per-field semantics.
	Routing RoutingFlags
}

// CheckCommand implements the CI-gate `check` semantic. Returns:
//
//   - [ExitOK] when no drift detected.
//   - [ExitCheckDrift] when one or more files differ from disk.
//   - [ExitUserError] on configuration faults.
//   - [ExitPipelineError] when the pipeline run emitted Error
//     diagnostics.
//   - [ExitInternalError] on a recovered panic.
type CheckCommand struct{ Config CheckConfig }

// RegisterFlags binds [CheckCommand]'s flags into fs.
func (c *CheckCommand) RegisterFlags(fs *flag.FlagSet) {
	fs.Var(&c.Config.DiagFormat, FlagDiagFormat, UsageDiagFormat)
	fs.BoolVar(&c.Config.Verbose, FlagVerbose, false, UsageVerbose)
	fs.BoolVar(&c.Config.Quiet, FlagQuiet, false, UsageQuiet)
	c.Config.Routing.Register(fs)
}

// Execute runs the pipeline through an in-memory sink and reports
// any byte-level drift against the on-disk state.
func (c *CheckCommand) Execute(ctx context.Context, env *Env) (exit int) {
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
	memSink := sink.NewMemory()
	p, err := buildPipeline(env, cfg, c.Config.Plugins, pipelineOverride{
		Verbose:      c.Config.Verbose,
		SinkOverride: memSink,
		Routing:      routing,
	})
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
	if errors.Is(runErr, pipeline.ErrRunHadErrors) {
		return ExitPipelineError
	}
	return c.reportDrift(env, memSink.Files(), orphanedOutputs(env, p))
}

// orphanedOutputs returns prior manifest entries this run no longer
// claims because their source package has been deleted.
//
// check always probes, where `prune` requires an opt-in flag. The
// asymmetry is the point: reporting costs the operator nothing and
// deleting costs them a file, so the inference earns its way into the
// report immediately and into the destructive path only when asked.
// Reporting is also the half that fixes the original complaint —
// before this, deleting a source package left an entry no command
// mentioned, so the manifest looked wrong for reasons nothing
// surfaced.
func orphanedOutputs(env *Env, p *pipeline.Pipeline) []manifest.Orphan {
	prev := p.LastManifest()
	scope := p.ScopeImportPaths()
	pipelineID := p.PipelineID()
	gone := goneSources(env.Workdir, pruneCandidates(prev, scope, pipelineID))
	if len(gone) == 0 {
		return nil
	}
	var out []manifest.Orphan
	for _, o := range manifest.PruneAll(prev, manifest.PruneOptions{
		Emitted:     p.EmittedTargets(),
		Scope:       scope,
		PipelineID:  pipelineID,
		GoneSources: gone,
	}) {
		if o.Reason == manifest.ReasonSourceGone {
			out = append(out, o)
		}
	}
	return out
}

// reportDrift compares every (target, body) pair the in-memory
// sink captured against the corresponding file on disk under
// env.Workdir. Differences print to env.Stdout in sorted target
// order so the report is deterministic across runs.
//
// orphans are prior outputs whose source package is gone. They are
// counted as drift because they are drift — a file on disk that the
// generator will never write again and no longer has a source to
// justify it — but they are reported separately, since the remedy
// differs: a content difference is fixed by re-running, an orphan
// only by deleting.
func (*CheckCommand) reportDrift(
	env *Env, current map[emit.Target][]byte, orphans []manifest.Orphan,
) int {
	targets := make([]emit.Target, 0, len(current))
	for t := range current {
		targets = append(targets, t)
	}
	sort.Slice(targets, func(i, j int) bool {
		return joinTarget(targets[i]) < joinTarget(targets[j])
	})

	drifted := 0
	for _, t := range targets {
		path := resolveTargetPath(env.Workdir, t)
		disk, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(env.Stdout, "drift: %s (missing from disk)\n", path)
			drifted++
			continue
		}
		if err != nil {
			writeErr(env, "read %s: %v", path, err)
			drifted++
			continue
		}
		if !sameProvenance(disk, current[t]) {
			fmt.Fprintf(env.Stdout, "drift: %s (content differs)\n", path)
			drifted++
		}
	}
	sort.Slice(orphans, func(i, j int) bool {
		return joinTarget(orphans[i].Target) < joinTarget(orphans[j].Target)
	})
	for _, o := range orphans {
		fmt.Fprintf(env.Stdout,
			"drift: %s (orphaned: source package %q no longer exists; "+
				"remove it with `prune -%s`)\n",
			resolveTargetPath(env.Workdir, o.Target), o.Target.ImportPath, FlagDeletedSources)
		drifted++
	}

	if drifted == 0 {
		fmt.Fprintf(env.Stdout, "check: no drift detected across %d output(s)\n", len(targets))
		return ExitOK
	}
	fmt.Fprintf(env.Stdout, "check: %d drifting file(s)\n", drifted)
	return ExitCheckDrift
}

// joinTarget returns the on-disk-relative path for a target —
// "<Dir>/<Filename>" — used to sort drift reports deterministically.
func joinTarget(t emit.Target) string {
	return filepath.Join(t.Dir, t.Filename)
}

// resolveTargetPath mirrors the disk sink's path-resolution
// contract: an absolute [emit.Target.Dir] bypasses workdir, while
// a relative Dir joins under workdir. The Layout phase derives
// alongside-source Dirs from `filepath.Dir(origin.Pos().File)`
// which the Go frontend records as absolute, so the bypass is the
// common case for the default layout.
func resolveTargetPath(workdir string, t emit.Target) string {
	if filepath.IsAbs(t.Dir) {
		return filepath.Join(t.Dir, t.Filename)
	}
	return filepath.Join(workdir, t.Dir, t.Filename)
}

// sameProvenance reports whether the disk file's content matches
// what the pipeline rendered in-memory under the provenance
// model:
//
//   - The disk file's stamped hash matches the in-memory file's
//     stamped hash (header-only differences like the Command line
//     do not surface as drift; the hash is over body bytes alone).
//   - The disk file's actual body bytes hash to the stamped value
//     (manual edits between the header and the trailer surface
//     as drift even when the trailer is left untouched).
//   - The provenance line is the disk file's final line, modulo a
//     single trailing newline (bytes appended after the trailer
//     surface as drift).
//
// Files missing the marker fall back to byte-equal comparison so
// non-eidos files (or older formats) are still handled correctly.
func sameProvenance(disk, current []byte) bool {
	memHash, ok := extractProvenance(current)
	if !ok {
		return bytes.Equal(disk, current)
	}
	diskHash, ok := extractProvenance(disk)
	if !ok {
		return false
	}
	if diskHash != memHash {
		return false
	}
	body, ok := extractBody(disk)
	if !ok {
		return bytes.Equal(disk, current)
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != diskHash {
		return false
	}
	return !trailerHasTrailingContent(disk, diskHash)
}

// trailerHasTrailingContent reports whether bytes follow the
// provenance trailer line on disk. A canonical rendered file
// ends with `<prefix><brand>:provenance <hash>\n` and nothing
// else; appended bytes (manual edits, accidental concatenation)
// drift the file off its rendered state and must surface as
// drift even though the stamped hash and the body hash both
// remain coherent.
func trailerHasTrailingContent(file []byte, hash string) bool {
	idx := bytes.LastIndex(file, []byte(writer.ProvenanceMarker+hash))
	if idx < 0 {
		return false
	}
	tail := file[idx+len(writer.ProvenanceMarker)+len(hash):]
	for len(tail) > 0 && (tail[0] == '\n' || tail[0] == '\r') {
		tail = tail[1:]
	}
	return len(tail) > 0
}

// extractProvenance returns the hash recorded in body's
// provenance trailer, or false when no trailer is present. Thin
// wrapper around [writer.ExtractProvenance] so the package-local
// callers don't have to import the writer package directly.
func extractProvenance(body []byte) (string, bool) {
	return writer.ExtractProvenance(body)
}

// extractBody returns the body region of a rendered file — the
// exact bytes the backend hashed into the provenance footer. The
// body sits between the blank line that closes the header and
// the leading newline of the trailer (which the footer renderer
// emits before the end-of-content marker). Returns false when
// either boundary is absent.
func extractBody(file []byte) ([]byte, bool) {
	_, afterHeader, ok := bytes.Cut(file, []byte("\n\n"))
	if !ok {
		return nil, false
	}
	beforeMarker, _, ok := bytes.Cut(afterHeader, []byte(bodyEndMarker))
	if !ok {
		return nil, false
	}
	// Walk back through the `// <brand>` prefix to the leading
	// newline the footer renderer stamps as the body / footer
	// boundary.
	nl := bytes.LastIndexByte(beforeMarker, '\n')
	if nl < 0 {
		return nil, false
	}
	return beforeMarker[:nl], true
}

// bodyEndMarker is the unique substring every backend stamps at
// the start of its provenance footer. The leading newline closes
// the body region; the brand-bound phrase keeps the marker
// distinct from anything a generated body would contain.
const bodyEndMarker = ": end of generated content."
