// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package backend

import (
	"text/template"

	"go.thesmos.sh/eidos/lang/typescript"
	"go.thesmos.sh/eidos/plugin"
)

// Name is the stable plugin identifier the pipeline uses for
// registration, diagnostic attribution and cache-key derivation.
const Name = "backend.typescript"

// Language is the target-language identifier shared between this
// backend, plugin-supplied template providers and downstream tooling.
//
// Re-exported from `lang/typescript` rather than restated, so a
// plugin targeting the language and a backend answering to it cannot
// drift apart: a plugin whose Templates(lang) tested a local constant
// against this one would silently ship no templates.
const Language = typescript.Language

// Backend renders emit graphs to TypeScript source. The zero value is
// unusable; construct via [New].
//
// Safe for concurrent use. The parent template tree is parsed once at
// construction and never mutated; each target renders through its own
// clone with its own import set, so per-target state is isolated by
// construction rather than by locking.
type Backend struct {
	tmpl *template.Template

	// tmplErr is the canonical set's parse failure, kept so
	// [Backend.Render] can report it. A constructor that returned it
	// would make every caller handle a condition only a broken build
	// produces; a nil template that rendered nothing would hide it.
	tmplErr error
}

// New returns a backend ready for registration on a pipeline builder.
func New() *Backend {
	tmpl, err := loadTemplates()
	return &Backend{tmpl: tmpl, tmplErr: err}
}

// Name returns [Name].
func (*Backend) Name() string { return Name }

// Language returns [Language].
func (*Backend) Language() string { return Language }

// Version returns [BackendVersion].
func (*Backend) Version() string { return BackendVersion }

// EmitVersions reports the emit major versions this backend renders.
func (*Backend) EmitVersions() []string {
	out := make([]string, len(supportedEmitVersions))
	copy(out, supportedEmitVersions)
	return out
}

// compile-time confirmation that the backend satisfies the role and
// the capability interfaces the pipeline probes for.
var (
	_ plugin.Backend       = (*Backend)(nil)
	_ plugin.Versioned     = (*Backend)(nil)
	_ plugin.EmitVersioned = (*Backend)(nil)
)
