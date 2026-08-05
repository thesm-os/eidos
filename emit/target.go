// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package emit

// Target identifies where the backend should write an emit entity:
// the output directory, the output filename within it, and the
// language-level package the file declares. Backends group emit
// entities by Target and render one output file per group.
//
// Two emit entities sharing a Target compose into the same file —
// the multi-generator composition path described in spec §9.
// Per-file slot injection (top, bottom, init) lives on an
// [emit.File] entity owning that Target.
//
// Target is a value type and is comparable, so it can be used
// directly as a map key for groupings.
type Target struct {
	// Dir is the output directory relative to the project root
	// (e.g. "internal/repo").
	Dir string `json:"dir,omitempty"`

	// Filename is the base filename inside Dir
	// (e.g. "user_repo_gen.go").
	Filename string `json:"filename,omitempty"`

	// Package is the package name the rendered file declares
	// (Go's `package repo`). Empty for languages without
	// package declarations at the file level.
	Package string `json:"package,omitempty"`

	// ImportPath is the canonical import path of the package the
	// rendered file lands in (e.g. "example.com/project/internal/repo").
	// Plugins set this to the source's import path when emitting
	// alongside-source, or to the consumer's centralised output
	// import path when emitting centralised; the backend forwards
	// it to the per-target [writer.ImportSet] so any [ExternalRef]
	// / [ExprExternal] whose Package matches renders bare (no
	// import, no qualifier) — the same-package elision rule.
	//
	// Empty ImportPath disables the elision; cross-package refs
	// still resolve normally. Targets distinguished only by
	// ImportPath compare unequal (Go struct equality), so plugins
	// emitting to the same logical file must agree on the value.
	ImportPath string `json:"import_path,omitempty"`
}

// IsZero reports whether t carries no routing information.
func (t Target) IsZero() bool {
	return t == Target{}
}

// JoinPath returns the file path under the project root —
// "Dir/Filename" with normalised slash separators. Returns "" when
// either component is empty.
func (t Target) JoinPath() string {
	if t.Dir == "" || t.Filename == "" {
		return ""
	}
	return t.Dir + "/" + t.Filename
}

// OutputPackageSetter is implemented by an emit value holding
// references into another output of the same plugin, for the same
// origin node.
//
// # Why it exists
//
// Generators build references during the Generate phase, but where
// each output lands is decided later, in Layout. A plugin emitting
// two outputs — a type and a companion referencing it — cannot name
// the first from the second: the package is not known yet. Guessing
// the source package works until an `out=` / `pkg=` override moves
// the file, at which point the reference names a package the entity
// no longer lives in and the generated code does not compile.
//
// Everything downstream of that package string already works.
// [Target.ImportPath] flows into the per-file import set, so a
// reference naming the referring file's own package renders bare and
// any other qualifies with an import. The only missing input is
// which package that is.
//
// # Contract
//
// Layout calls [OutputPackageSetter.SetOutputPackages] at most once
// per implementing value, after every Target in the run is resolved
// and before the emit graph is frozen. The implementor uses the
// supplied paths to construct or patch references it owns.
//
// A value whose origin routed no output is not called at all. An
// implementor must stay valid without the call arriving — the run
// that skipped it has already reported why.
//
// # Hazards
//
// The call mutates emit state after Generate. Two rules bind
// implementors:
//
//   - Touch only fields the implementor owns. The value is reachable
//     from the store and from any slot holding it; Layout is
//     single-goroutine so nothing races, but rewriting shared
//     structure corrupts entities other plugins may reference.
//   - Do not assume a tag is present. The map carries only tags that
//     routed successfully for this origin, so a plugin declaring
//     three outputs may see fewer — or, under a partly-failed run,
//     none of the ones it expected.
type OutputPackageSetter interface {
	Node

	// SetOutputPackages receives the canonical import path of every
	// output the value's own plugin produced for the value's origin
	// node, keyed by output tag — empty string for the plugin's
	// primary output, the declared tag otherwise.
	//
	// A path may legitimately be empty: centralised routing cannot
	// derive an import path without a module context and leaves
	// [Target.ImportPath] unset. Empty means "not derivable", not
	// "same package" — an implementor that renders a bare reference
	// on the strength of it names a package it never imported.
	//
	// The map is owned by the caller. Do not retain or mutate it.
	SetOutputPackages(byTag map[string]string)
}
