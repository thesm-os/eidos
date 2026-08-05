// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

// Command eidos-reference is the demonstration binary for the
// eidos code-generation toolkit. It hardcodes the brand
// identifier "eidos-reference", wires every in-tree frontend /
// backend / bridge / reference plugin, and dispatches the
// standard cli subcommands.
//
// The binary is NOT the canonical eidos CLI — there isn't one,
// because eidos is plugin-architected and the plugin set is
// always project-defined. Downstream consumers writing their
// own pipeline binary copy this main.go, substitute their
// plugin slice, and rebuild. The reference is provided so the
// in-tree plugin ensemble has a runnable form for evaluation,
// acceptance testing, and as a copy-paste starting point.
//
// Subcommands:
//
//	eidos-reference run     [flags] [patterns...]
//	eidos-reference plan    [flags]
//	eidos-reference explain [flags] <selector>
//	eidos-reference check   [flags]
//	eidos-reference prune   [flags]
//	eidos-reference version [flags]
//
// Every subcommand accepts --config to point at a specific
// config file; absent that flag, the binary walks up from the
// working directory looking for `.eidos-reference.yaml`.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	"go.thesmos.sh/eidos/backend/golang"
	"go.thesmos.sh/eidos/bridge/protogo"
	"go.thesmos.sh/eidos/cli"
	"go.thesmos.sh/eidos/core/diag"
	frontendgolang "go.thesmos.sh/eidos/frontend/golang"
	"go.thesmos.sh/eidos/frontend/protobuf"
	"go.thesmos.sh/eidos/plugin"
	"go.thesmos.sh/eidos/plugins/generator/builder"
	"go.thesmos.sh/eidos/plugins/generator/enum"
	"go.thesmos.sh/eidos/plugins/generator/sentinel"
	"go.thesmos.sh/eidos/reference/auditweaver"
	"go.thesmos.sh/eidos/reference/debugweaver"
	"go.thesmos.sh/eidos/reference/mockgen"
	"go.thesmos.sh/eidos/reference/registrygen"
	"go.thesmos.sh/eidos/reference/repogen"
	"go.thesmos.sh/eidos/reference/shapewriter"
	"go.thesmos.sh/eidos/reference/stubgen"
)

// brand is the hardcoded identifier this binary advertises to
// the cli package. Drives the rendered header / footer markers
// and the default `.eidos-reference/` state directory.
const brand = "eidos-reference"

// Subcommand names. Centralised so the help text, dispatch
// switch, and per-command FlagSet names stay aligned.
const (
	cmdRun     = "run"
	cmdPlan    = "plan"
	cmdExplain = "explain"
	cmdCheck   = "check"
	cmdPrune   = "prune"
	cmdVersion = "version"
)

// Help-trigger tokens accepted in lieu of a subcommand: prints
// the top-level usage banner and exits ExitOK.
const (
	helpDashH    = "-h"
	helpDashDash = "--help"
	helpKeyword  = "help"
)

// usage is the top-level help string printed when no subcommand
// is supplied or the supplied subcommand is unknown.
const usage = `eidos-reference - the in-tree plugin ensemble as a runnable binary

Usage:
  eidos-reference <command> [flags] [args...]

Commands:
  run       Execute the pipeline and write outputs through the sink.
  plan      Print the resolved plugin order without running the pipeline.
  explain   Inspect provenance for an entity, slot, or meta key.
  check     Run against an in-memory sink and report drift vs. disk.
  prune     Delete previously-claimed outputs no longer in the manifest.
  version   Print brand, emit-contract, and plugin list.

Run "eidos-reference <command> --help" for command-specific flags.

This binary is a demonstration — production consumers copy
main.go and substitute their own plugin slice.
`

func main() {
	env, err := cli.NewEnv(brand)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", brand, err)
		os.Exit(cli.ExitUserError)
	}
	os.Exit(run(context.Background(), env, defaultPlugins(), os.Args[1:]))
}

// run dispatches argv (without the program name) to the
// appropriate command kernel. Extracted from main so tests can
// drive the argument-parsing surface against an in-memory env
// without touching the real process stdio.
func run(ctx context.Context, env *cli.Env, plugins []plugin.Plugin, args []string) int {
	if len(args) == 0 || args[0] == helpDashH || args[0] == helpDashDash || args[0] == helpKeyword {
		fmt.Fprint(env.Stdout, usage)
		return cli.ExitOK
	}
	cmdName, rest := args[0], args[1:]

	switch cmdName {
	case cmdRun:
		return dispatchRun(ctx, env, plugins, rest)
	case cmdPlan:
		return dispatchPlan(ctx, env, plugins, rest)
	case cmdExplain:
		return dispatchExplain(ctx, env, plugins, rest)
	case cmdCheck:
		return dispatchCheck(ctx, env, plugins, rest)
	case cmdPrune:
		return dispatchPrune(ctx, env, plugins, rest)
	case cmdVersion:
		return dispatchVersion(ctx, env, plugins, rest)
	default:
		fmt.Fprintf(env.Stderr, "%s: unknown command %q\n\n%s", brand, cmdName, usage)
		return cli.ExitUserError
	}
}

// defaultPlugins returns the static plugin universe this binary
// embeds: both in-tree frontends, the Go backend, the proto-to-Go
// bridge annotator, plus every reference plugin shipped in the
// repository. Downstream binaries replace this set with their own
// — copy this slice as the canonical starting shape.
func defaultPlugins() []plugin.Plugin {
	return []plugin.Plugin{
		// Frontends — parse input into the source-side store.
		frontendgolang.New(),
		protobuf.New(),

		// Cross-frontend bridge — stamps Go-shape metadata on
		// proto-loaded packages so downstream Go-flavoured
		// generators see a unified view.
		protogo.New(),

		// Annotators — stamp typed metadata before generation.
		shapewriter.New(),

		// Generators (foundation bucket) — emit baseline output
		// other generators may compose against.
		repogen.New(),
		builder.New(),
		enum.New(),

		// Generators (composition bucket) — depend on
		// foundation output.
		mockgen.New(),
		stubgen.New(),

		// Generators (cross-cutting bucket) — contribute into
		// existing emit decls' slots, or scan post-generation
		// state to assert invariants.
		auditweaver.New(),
		debugweaver.New(),
		registrygen.New(),
		sentinel.New(),

		// Backend — render emit graph to target language.
		golang.New(),
	}
}

// loadConfigOrDefault resolves the config file: --config wins
// when non-empty, otherwise the binary walks up from Workdir
// looking for `.<brand>.yaml`. A missing file is not an error —
// the defaults apply.
func loadConfigOrDefault(env *cli.Env, explicit string) (*cli.Config, error) {
	if explicit != "" {
		return cli.LoadConfig(explicit)
	}
	if path, ok := cli.DiscoverConfig(env.Workdir, env.ConfigFileName()); ok {
		return cli.LoadConfig(path)
	}
	return cli.DefaultConfig(), nil
}

// commandFlagSet constructs the per-command FlagSet, routes its
// usage output to the env's Stderr, and registers the shared
// --config flag. Returns the FlagSet plus a pointer to the
// parsed --config value.
func commandFlagSet(env *cli.Env, name string) (*flag.FlagSet, *string) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderrOrStub(env))
	cfgPath := fs.String(cli.FlagConfig, "", cli.UsageConfig)
	return fs, cfgPath
}

// stderrOrStub returns env.Stderr or io.Discard when nil.
// Defensive against a partially-initialised env supplied by a
// downstream test harness — flag.FlagSet panics on a nil writer.
func stderrOrStub(env *cli.Env) io.Writer {
	if env == nil || env.Stderr == nil {
		return io.Discard
	}
	return env.Stderr
}

// dispatchRun parses run's flags, loads the config, and invokes
// the command. Positional args after the flags are the source
// patterns the frontend receives.
func dispatchRun(ctx context.Context, env *cli.Env, plugins []plugin.Plugin, args []string) int {
	cmd := &cli.RunCommand{Config: cli.RunConfig{Plugins: plugins}}
	fs, cfgPath := commandFlagSet(env, cmdRun)
	cmd.RegisterFlags(fs)
	if err := fs.Parse(args); err != nil {
		return cli.ExitUserError
	}
	cfg, err := loadConfigOrDefault(env, *cfgPath)
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s run: %v\n", brand, err)
		return cli.ExitUserError
	}
	cmd.Config.File = cfg
	cmd.Config.Patterns = fs.Args()
	ensureDiag(env)
	return cmd.Execute(ctx, env)
}

// dispatchPlan parses plan's flags and invokes the command.
func dispatchPlan(ctx context.Context, env *cli.Env, plugins []plugin.Plugin, args []string) int {
	cmd := &cli.PlanCommand{Config: cli.PlanConfig{Plugins: plugins}}
	fs, cfgPath := commandFlagSet(env, cmdPlan)
	cmd.RegisterFlags(fs)
	if err := fs.Parse(args); err != nil {
		return cli.ExitUserError
	}
	cfg, err := loadConfigOrDefault(env, *cfgPath)
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s plan: %v\n", brand, err)
		return cli.ExitUserError
	}
	cmd.Config.File = cfg
	ensureDiag(env)
	return cmd.Execute(ctx, env)
}

// dispatchExplain parses explain's flags, treats the first
// positional arg as the selector, and invokes the command.
func dispatchExplain(ctx context.Context, env *cli.Env, plugins []plugin.Plugin, args []string) int {
	cmd := &cli.ExplainCommand{Config: cli.ExplainConfig{Plugins: plugins}}
	fs, cfgPath := commandFlagSet(env, cmdExplain)
	cmd.RegisterFlags(fs)
	if err := fs.Parse(args); err != nil {
		return cli.ExitUserError
	}
	cfg, err := loadConfigOrDefault(env, *cfgPath)
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s explain: %v\n", brand, err)
		return cli.ExitUserError
	}
	cmd.Config.File = cfg
	if fs.NArg() > 0 {
		cmd.Config.Selector = fs.Arg(0)
	}
	ensureDiag(env)
	return cmd.Execute(ctx, env)
}

// dispatchCheck parses check's flags and invokes the command.
func dispatchCheck(ctx context.Context, env *cli.Env, plugins []plugin.Plugin, args []string) int {
	cmd := &cli.CheckCommand{Config: cli.CheckConfig{Plugins: plugins}}
	fs, cfgPath := commandFlagSet(env, cmdCheck)
	cmd.RegisterFlags(fs)
	if err := fs.Parse(args); err != nil {
		return cli.ExitUserError
	}
	cfg, err := loadConfigOrDefault(env, *cfgPath)
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s check: %v\n", brand, err)
		return cli.ExitUserError
	}
	cmd.Config.File = cfg
	ensureDiag(env)
	return cmd.Execute(ctx, env)
}

// dispatchPrune parses prune's flags and invokes the command.
func dispatchPrune(ctx context.Context, env *cli.Env, plugins []plugin.Plugin, args []string) int {
	cmd := &cli.PruneCommand{Config: cli.PruneConfig{Plugins: plugins}}
	fs, cfgPath := commandFlagSet(env, cmdPrune)
	cmd.RegisterFlags(fs)
	if err := fs.Parse(args); err != nil {
		return cli.ExitUserError
	}
	cfg, err := loadConfigOrDefault(env, *cfgPath)
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s prune: %v\n", brand, err)
		return cli.ExitUserError
	}
	cmd.Config.File = cfg
	ensureDiag(env)
	return cmd.Execute(ctx, env)
}

// dispatchVersion parses version's flags and invokes the command.
func dispatchVersion(ctx context.Context, env *cli.Env, plugins []plugin.Plugin, args []string) int {
	cmd := &cli.VersionCommand{Config: cli.VersionConfig{Plugins: plugins}}
	fs, cfgPath := commandFlagSet(env, cmdVersion)
	cmd.RegisterFlags(fs)
	if err := fs.Parse(args); err != nil {
		return cli.ExitUserError
	}
	cfg, err := loadConfigOrDefault(env, *cfgPath)
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s version: %v\n", brand, err)
		return cli.ExitUserError
	}
	cmd.Config.File = cfg
	ensureDiag(env)
	return cmd.Execute(ctx, env)
}

// ensureDiag lazily initialises env.Diag for the test harnesses
// that construct an Env directly. NewEnv always populates the
// field; this guards against the bare-struct case.
func ensureDiag(env *cli.Env) {
	if env.Diag == nil {
		env.Diag = diag.New()
	}
}
