// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package directive

import (
	"slices"

	"go.thesmos.sh/eidos/core/diag"
	"go.thesmos.sh/eidos/core/kind"
)

// Validate checks every directive in directives against the schemas
// in registry, in the context of a node of the given [kind.Kind],
// and emits positioned diagnostics into sink for each rule
// violation.
//
// Returns true if every directive passes (zero error diagnostics);
// false if any error was emitted. Per the design contract, validation
// never aborts on the first failure — every directive is checked so
// the user sees all problems in one pass.
//
// Side effect on directives: when a [PositionalArg] has [Default]
// set and the source omitted the slot, Validate folds the default
// into directives[i].Args. Reads after Validate observe the
// post-default Args list.
//
// Validate honours [Schema.AllowNegated], [Schema.AppliesTo],
// [Schema.Requires], [Schema.MutuallyExclusiveWith],
// [Schema.RequiredKeys], [Schema.AllowedKeys],
// [Schema.PositionalArgs], and [Schema.AllowExtraPositional].
//
// A directive no schema claims is an error, with a "did you mean?"
// where [Registry.Suggest] finds a plausible one. It parses, matches
// nothing, stamps nothing and generates nothing — output
// indistinguishable from a declaration that asked for nothing at all,
// so the line the author wrote is the only evidence they asked for
// something and the run's evidence is silence. A run deliberately
// narrower than the source it reads turns this off through
// [Registry.AllowUnclaimed].
func Validate(directives []*Directive, nodeKind kind.Kind, registry *Registry, sink *diag.PluginSink) bool {
	ok := true
	for _, d := range directives {
		ok = validateOne(d, nodeKind, directives, registry, sink) && ok
	}
	return ok
}

// validateOne checks a single directive against its schema and the
// peer set. Returns true iff no error diagnostic was emitted.
func validateOne(d *Directive, nodeKind kind.Kind, peers []*Directive, registry *Registry, sink *diag.PluginSink) bool {
	schema, registered := registry.Lookup(d.Name)
	if !registered {
		return checkClaimed(d, registry, sink)
	}
	// Each check emits its own diagnostics; we keep AND-ing the
	// running ok so every check still runs even after an earlier one
	// failed (the user sees every problem in one pass).
	ok := checkNegation(d, schema, sink)
	ok = checkAppliesTo(d, schema, nodeKind, sink) && ok
	ok = checkRequires(d, schema, peers, sink) && ok
	ok = checkExclusive(d, schema, peers, sink) && ok
	ok = checkKeys(d, schema, sink) && ok
	ok = checkPositional(d, schema, sink) && ok
	return ok
}

// checkClaimed reports a directive no schema registers.
//
// The name is quoted as the author wrote it, because that is what they
// have to find and edit. A plausible near-miss is named as one: the
// edit-distance threshold [Registry.Suggest] applies is what keeps
// `+gen:buildr` reading as a typo for `builder` and `+gen:xyzzy`
// reading as a name nothing here has ever had.
//
// Both forms are errors rather than warnings. What the directive
// produced is nothing, which is what a declaration carrying no
// directive produces, so a warning would ask the author to notice an
// absence in a build log — and the absence is exactly what they
// already failed to notice in their own source.
func checkClaimed(d *Directive, registry *Registry, sink *diag.PluginSink) bool {
	if registry.UnclaimedAllowed() {
		return true
	}
	if did, plausible := registry.Suggest(d.Name); plausible {
		sink.Errorf(d.Pos, "no directive named %q is registered — did you mean %q?", d.Name, did)
		return false
	}
	sink.Errorf(d.Pos,
		"no directive named %q is registered, and nothing registered is close enough to be the one you meant",
		d.Name)
	return false
}

func checkNegation(d *Directive, s Schema, sink *diag.PluginSink) bool {
	if d.Negated && !s.AllowNegated {
		sink.Errorf(d.Pos, "directive %q does not accept the -gen: form", d.Name)
		return false
	}
	return true
}

func checkAppliesTo(d *Directive, s Schema, nodeKind kind.Kind, sink *diag.PluginSink) bool {
	if len(s.AppliesTo) == 0 || slices.Contains(s.AppliesTo, nodeKind) {
		return true
	}
	sink.Errorf(d.Pos, "directive %q does not apply to %q nodes (allowed: %v)", d.Name, nodeKind, s.AppliesTo)
	return false
}

func checkRequires(d *Directive, s Schema, peers []*Directive, sink *diag.PluginSink) bool {
	ok := true
	for _, req := range s.Requires {
		if !peersContain(peers, req) {
			sink.Errorf(d.Pos, "directive %q requires %q to also be present", d.Name, req)
			ok = false
		}
	}
	return ok
}

func checkExclusive(d *Directive, s Schema, peers []*Directive, sink *diag.PluginSink) bool {
	ok := true
	for _, excl := range s.MutuallyExclusiveWith {
		if peersContain(peers, excl) {
			sink.Errorf(d.Pos, "directive %q is mutually exclusive with %q", d.Name, excl)
			ok = false
		}
	}
	return ok
}

func checkKeys(d *Directive, s Schema, sink *diag.PluginSink) bool {
	ok := true
	for _, key := range s.RequiredKeys {
		if !d.HasKey(key) {
			sink.Errorf(d.Pos, "directive %q requires key %q", d.Name, key)
			ok = false
		}
	}
	switch {
	case s.DenyKeys:
		for key := range d.KV {
			sink.Errorf(d.Pos, "directive %q accepts no keys; %q was given", d.Name, key)
			ok = false
		}
	case len(s.AllowedKeys) > 0:
		for key := range d.KV {
			if !slices.Contains(s.AllowedKeys, key) {
				sink.Errorf(d.Pos, "directive %q does not accept key %q (allowed: %v)", d.Name, key, s.AllowedKeys)
				ok = false
			}
		}
	}
	return ok
}

// checkPositional validates positional args against the schema's slot
// vector. Side effect: folds defaults into d.Args when the slot is
// optional and missing.
func checkPositional(d *Directive, s Schema, sink *diag.PluginSink) bool {
	ok := true
	declared := s.PositionalArgs
	for i, slot := range declared {
		if i >= len(d.Args) {
			if slot.Required {
				sink.Errorf(d.Pos, "directive %q requires positional arg %d (%q)", d.Name, i, slot.Name)
				ok = false
				continue
			}
			if slot.Default != "" {
				d.Args = append(d.Args, slot.Default)
			}
			continue
		}
		if len(slot.OneOf) > 0 && !slices.Contains(slot.OneOf, d.Args[i]) {
			sink.Errorf(d.Pos, "directive %q positional %d (%q) must be one of %v, got %q",
				d.Name, i, slot.Name, slot.OneOf, d.Args[i])
			ok = false
		}
	}
	if !s.AllowExtraPositional && len(d.Args) > len(declared) {
		sink.Errorf(d.Pos, "directive %q accepts %d positional args, got %d", d.Name, len(declared), len(d.Args))
		ok = false
	}
	return ok
}

// peersContain reports whether any directive in peers has the named
// Name. Used by Requires and MutuallyExclusiveWith checks.
func peersContain(peers []*Directive, name Name) bool {
	for _, p := range peers {
		if p.Name == name {
			return true
		}
	}
	return false
}
