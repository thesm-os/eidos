// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cli

import (
	"context"
	"flag"
	"fmt"
	"sort"

	"go.thesmos.sh/eidos/emit"
	"go.thesmos.sh/eidos/plugin"
)

// VersionConfig holds the inputs for [VersionCommand]. The
// command lists the consumer's static plugin universe — every
// plugin in `Plugins` is printed, with enabled/disabled status
// derived from the loaded config (when present).
//
// Plugin universe is the consumer's responsibility: this command
// answers "what does this binary contain?" and "what is this
// project actually running?" in one output.
type VersionConfig struct {
	// File is the loaded config, used to determine which plugins
	// are enabled. Pass nil to mark every plugin as enabled.
	File *Config

	// Plugins is the consumer's statically-imported plugin slice.
	Plugins []plugin.Plugin

	// Format selects text (default) or JSON output.
	Format DiagFormat
}

// VersionCommand prints the brand, emit-contract version, and the
// list of registered plugins (with their versions and
// enabled/disabled status per the loaded config).
type VersionCommand struct{ Config VersionConfig }

// RegisterFlags binds [VersionCommand]'s flags into fs. Only the
// shared cross-command flags apply.
func (c *VersionCommand) RegisterFlags(fs *flag.FlagSet) {
	fs.Var(&c.Config.Format, FlagDiagFormat, UsageDiagFormat)
}

// Execute prints the version block. Always returns [ExitOK]; the
// command has no failure modes beyond a misuse of [Env] (empty
// brand), which surfaces as [ExitUserError].
func (c *VersionCommand) Execute(_ context.Context, env *Env) (exit int) {
	// version builds no pipeline and calls no plugin, so nothing
	// here is expected to panic. The guard is uniformity: every
	// command kernel installs it, so none becomes the exception
	// someone has to notice when adding a call that can.
	defer recoverInto(env, &exit)
	if env.Brand == "" {
		writeErr(env, "Env.Brand is required")
		return ExitUserError
	}
	enabled := enabledPluginSet(c.Config.File)
	entries := pluginVersionEntries(c.Config.Plugins, enabled)
	switch c.Config.Format {
	case DiagFormatJSON:
		return c.writeJSON(env, entries)
	default:
		return c.writeText(env, entries)
	}
}

// pluginVersionEntry is one row in the rendered version listing.
type pluginVersionEntry struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Enabled bool   `json:"enabled"`
}

// enabledPluginSet returns the names of plugins the loaded config
// marks as enabled. When the config is nil or has no `plugins:`
// block, every plugin in the slice counts as enabled — the
// returned set is nil and callers treat nil as "everything in".
func enabledPluginSet(cfg *Config) map[string]bool {
	if cfg == nil || len(cfg.Plugins) == 0 {
		return nil
	}
	out := make(map[string]bool, len(cfg.Plugins))
	for _, p := range cfg.Plugins {
		out[p.Name] = p.IsEnabled()
	}
	return out
}

// pluginVersionEntries projects each plugin in the slice to a
// version-entry record. The version comes from
// [plugin.Versioned.Version] when implemented; otherwise it's
// recorded as the empty string and the renderer formats it as
// "dev".
func pluginVersionEntries(plugins []plugin.Plugin, enabled map[string]bool) []pluginVersionEntry {
	entries := make([]pluginVersionEntry, 0, len(plugins))
	for _, p := range plugins {
		entry := pluginVersionEntry{Name: p.Name()}
		if v, ok := p.(plugin.Versioned); ok {
			entry.Version = v.Version()
		}
		entry.Enabled = enabled == nil || enabled[p.Name()]
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name < entries[j].Name })
	return entries
}

// writeText renders the version block as human-readable lines.
// Format:
//
//	<brand>
//	emit-contract: <version>
//	plugins:
//	  - <name>  <version>  (enabled|disabled)
func (*VersionCommand) writeText(env *Env, entries []pluginVersionEntry) int {
	if _, err := fmt.Fprintln(env.Stdout, env.Brand); err != nil {
		writeErr(env, "cli: write version: %v", err)
		return ExitInternalError
	}
	if _, err := fmt.Fprintf(env.Stdout, "emit-contract: %s\n", emit.Version); err != nil {
		writeErr(env, "cli: write version: %v", err)
		return ExitInternalError
	}
	if len(entries) == 0 {
		fmt.Fprintln(env.Stdout, "plugins: (none)")
		return ExitOK
	}
	fmt.Fprintln(env.Stdout, "plugins:")
	for _, e := range entries {
		ver := e.Version
		if ver == "" {
			ver = "dev"
		}
		state := "enabled"
		if !e.Enabled {
			state = "disabled"
		}
		if _, err := fmt.Fprintf(env.Stdout, "  - %-20s %-12s (%s)\n", e.Name, ver, state); err != nil {
			writeErr(env, "cli: write version: %v", err)
			return ExitInternalError
		}
	}
	return ExitOK
}

// writeJSON renders the version block as a single JSON object:
// `{"brand": "...", "emit_contract": "...", "plugins": [...]}`.
func (*VersionCommand) writeJSON(env *Env, entries []pluginVersionEntry) int {
	out := struct {
		Brand        string               `json:"brand"`
		EmitContract string               `json:"emit_contract"`
		Plugins      []pluginVersionEntry `json:"plugins"`
	}{
		Brand:        env.Brand,
		EmitContract: emit.Version,
		Plugins:      entries,
	}
	if err := encodeJSONLine(env.Stdout, out); err != nil {
		writeErr(env, "cli: write version: %v", err)
		return ExitInternalError
	}
	return ExitOK
}
