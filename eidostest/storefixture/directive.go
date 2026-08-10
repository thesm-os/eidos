// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package storefixture

import (
	"strings"

	"go.thesmos.sh/eidos/core/directive"
	"go.thesmos.sh/eidos/core/position"
	"go.thesmos.sh/eidos/sdk"
)

// Directive constructs a parsed [directive.Directive] suitable for
// attaching to a fixture node via [StructBuilder.Directive],
// [MethodBuilder.Directive], and equivalents.
//
// The result mirrors what a frontend's directive parser produces:
// Name, Negated, positional Args, and keyword KV are all populated
// from the supplied options. Raw is left empty — tests rarely care
// about it and a real frontend would set it to the source text.
func Directive(name directive.Name, opts ...DirectiveOption) *directive.Directive {
	d := &directive.Directive{Name: name, KV: map[string]string{}}
	for _, opt := range opts {
		opt(d)
	}
	return d
}

// DirectiveOption mutates a [directive.Directive] under construction.
// Options are applied left-to-right; later options overwrite earlier
// ones when they target the same field.
type DirectiveOption func(*directive.Directive)

// Negated marks the directive as a `-gen:` form. The default — without
// applying Negated — is the `+gen:` form.
func Negated() DirectiveOption {
	return func(d *directive.Directive) { d.Negated = true }
}

// Arg appends a positional argument.
func Arg(arg string) DirectiveOption {
	return func(d *directive.Directive) { d.Args = append(d.Args, arg) }
}

// KV records a key=value argument.
func KV(key, value string) DirectiveOption {
	return func(d *directive.Directive) { d.KV[key] = value }
}

// At records the directive's source position.
func At(p position.Pos) DirectiveOption {
	return func(d *directive.Directive) { d.Pos = p }
}

// RouteTo builds the standalone `+gen:out` directive that sends one
// plugin's output into its own directory and package.
//
// # The trailing slash is not cosmetic
//
// This exists because the hand-written form has a trap every plugin
// testing `+gen:out` falls into exactly once. Layout reads the
// directive's positional value as a *path*, and a trailing separator
// is its only way to say "this is a directory, keep the filename you
// composed". Without one, `+gen:out validation` names a file:
//
//	Directive("out", Arg("validation"), ...)
//	  -> emit.Target{Dir: "", Filename: "validation", Package: "validation"}
//
// A file literally called `validation`, no `.go` suffix, in the
// origin's own directory. It renders, it is written, and no
// diagnostic is emitted at any severity — the value was a legal path,
// so nothing was wrong with it. The generated file is then invisible
// to the Go toolchain, which discards the basename before
// packages.Load ever sees it, and the test that expected a routed
// package fails somewhere else entirely.
//
// RouteTo takes a directory and appends the separator itself, so the
// value cannot be spelled the wrong way round. dir may be passed with
// or without a trailing slash. An empty dir means "same directory,
// different package", which is a legitimate routing and stays
// expressible.
func RouteTo(plugin, dir, pkg string) *directive.Directive {
	path := strings.TrimRight(dir, "/")
	if path != "" {
		path += "/"
	}
	return Directive(sdk.OutDirective,
		Arg(path),
		KV("plugin", plugin),
		KV("pkg", pkg),
	)
}
