// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package cli

import (
	"context"
	"errors"
	"fmt"
)

// Command is the contract every cli command satisfies. The integer
// return value is the process exit code; callers typically
// `os.Exit(cmd.Execute(ctx, env))`.
//
// Commands never propagate panics: a panic from any plugin or from
// pipeline machinery is recovered into [ExitInternalError] with a
// structured diagnostic written to env.Diag. The contract is
// total: callers do not need to wrap Execute in a recover.
type Command interface {
	Execute(ctx context.Context, env *Env) int
}

// Exit-code constants. Used as the integer return value of every
// [Command.Execute] call. Stable values across releases so CI gates
// can pin behaviour to specific codes.
const (
	// ExitOK reports success.
	ExitOK = 0

	// ExitUserError reports a configuration or input fault on the
	// caller's side: invalid flag combination, malformed config
	// file, plugin named in config but absent from the consumer's
	// plugin slice, unresolvable explain selector, missing required
	// brand.
	ExitUserError = 1

	// ExitPipelineError reports a successful Execute path that
	// nevertheless produced one or more [diag.Error] diagnostics
	// during the run. The run reached the sink; some plugins
	// emitted errors that fall short of fatal.
	ExitPipelineError = 2

	// ExitInternalError reports an unexpected panic that the
	// command's recovery layer caught. Bug-class — should never
	// trip in production. The diagnostic includes a stack trace.
	ExitInternalError = 3

	// ExitCacheVerifyFailed is reserved for a cache-verification
	// command that does not exist yet: recomputing an output and
	// finding it disagrees byte-for-byte with a cached value.
	//
	// Nothing returns it. The value is held rather than reused so
	// exit-code numbering stays stable if verification lands, and
	// so a script matching on 4 never sees it mean something else.
	// Implementing it needs a cache that stores recoverable output;
	// the pipeline's entry is a fingerprint over reads, routing and
	// scope, so there is nothing to compare against.
	ExitCacheVerifyFailed = 4

	// ExitCheckDrift reports that the `check` command found the
	// pipeline's output differs from the on-disk state — the CI
	// gate's drift signal.
	ExitCheckDrift = 5
)

// DiagFormat selects the rendered form of diagnostics commands
// write to env.Stdout / env.Stderr. Set via the `--diag-format`
// flag on every command that runs the pipeline.
type DiagFormat int

const (
	// DiagFormatText is the human-readable default: one
	// `<severity> <pos> <plugin>: <message>` line per diagnostic,
	// with optional color when the writer is a TTY.
	DiagFormatText DiagFormat = iota

	// DiagFormatJSON renders one JSON object per line (NDJSON)
	// with a stable schema:
	// `{"severity":"error","plugin":"repogen","pos":{"file":"...","line":42},"message":"..."}`.
	DiagFormatJSON
)

// Diagnostic-format identifiers used in [DiagFormat.String] and
// accepted by [DiagFormat.Set] and the `--diag-format` flag.
const (
	diagFormatText = "text"
	diagFormatJSON = "json"
)

// String returns the lower-case form expected by the
// `--diag-format` flag.
func (d DiagFormat) String() string {
	switch d {
	case DiagFormatText:
		return diagFormatText
	case DiagFormatJSON:
		return diagFormatJSON
	default:
		return diagFormatText
	}
}

// Set parses the `--diag-format` flag value. Accepts "text" / "json";
// any other value returns [ErrInvalidDiagFormat] so the calling
// flag package surfaces a positioned error to the user.
func (d *DiagFormat) Set(v string) error {
	switch v {
	case "", diagFormatText:
		*d = DiagFormatText
		return nil
	case diagFormatJSON:
		*d = DiagFormatJSON
		return nil
	default:
		return ErrInvalidDiagFormat
	}
}

// ConfigError reports a configuration fault the cli surface
// detected before pipeline execution: missing brand, malformed
// config file, plugin named in config but absent from the
// consumer's slice. Callers compare with [errors.Is]; the wrapped
// reason carries the human-readable detail.
type ConfigError struct {
	Path   string // path of the offending config file, if any.
	Line   int    // 1-based line, 0 when not applicable.
	Column int    // 1-based column, 0 when not applicable.
	Reason string // human-readable diagnostic.
}

func (e *ConfigError) Error() string {
	if e.Path != "" && e.Line > 0 {
		return fmt.Sprintf("%s:%d:%d: %s", e.Path, e.Line, e.Column, e.Reason)
	}
	if e.Path != "" {
		return fmt.Sprintf("%s: %s", e.Path, e.Reason)
	}
	return e.Reason
}

// Is reports whether target is also a [*ConfigError]. Lets callers
// switch on the configuration-fault class without inspecting the
// concrete reason.
func (*ConfigError) Is(target error) bool {
	var ce *ConfigError
	return errors.As(target, &ce)
}

// ErrInvalidDiagFormat surfaces when [DiagFormat.Set] is called
// with a value other than "text" or "json".
var ErrInvalidDiagFormat = errors.New("cli: invalid --diag-format value (expected text or json)")
