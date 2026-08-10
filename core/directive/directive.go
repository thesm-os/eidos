// Copyright Thesmos B.V. 2026
// SPDX-License-Identifier: MIT

package directive

import (
	"slices"

	"go.thesmos.sh/eidos/core/position"
)

// Directive is one parsed `+gen:` or `-gen:` directive instance.
//
// Parsing populates Name, Negated, Args (positional), KV, Raw (the
// original text after the prefix), and Pos. Schema-driven defaults
// from [PositionalArg.Default] are applied during validation, not
// during parsing — so consumers reading raw Args before validation
// see exactly what the source wrote.
type Directive struct {
	Name    Name
	Negated bool
	Args    []string
	KV      map[string]string
	Raw     string
	Pos     position.Pos
}

// Arg returns the positional argument at index i, or "" if i is out
// of range. Use after validation, when [PositionalArg.Default]
// values have been folded in via [Validate].
func (d *Directive) Arg(i int) string {
	if i < 0 || i >= len(d.Args) {
		return ""
	}
	return d.Args[i]
}

// HasKey reports whether the directive carries the named key/value
// argument.
func (d *Directive) HasKey(key string) bool {
	_, ok := d.KV[key]
	return ok
}

// Value returns the value of the named key/value argument, or "" if
// absent. Combine with [Directive.HasKey] when distinguishing
// "absent" from "present but empty" matters.
func (d *Directive) Value(key string) string {
	return d.KV[key]
}

// HasPositive reports whether list contains a directive named name
// with [Directive.Negated] false — the `+gen:NAME` form. Returns
// false when list is empty or carries only negated entries for the
// name.
func HasPositive(list []*Directive, name Name) bool {
	for _, d := range list {
		if d.Name == name && !d.Negated {
			return true
		}
	}
	return false
}

// Last returns the final directive in list named name, or nil when
// there is none.
//
// The counterpart to the first-wins lookup on a node
// (`BaseNode.Directive`). First-wins answers "is this declared", which
// is the right question for a flag: a second `+gen:builder` says
// nothing the first did not.
//
// It is the wrong question for a repeatable directive carrying a
// value. An author writing
//
//	//	+gen:default limit=10
//	//	+gen:default limit=50
//
// has said the limit is 50, and a generator reading the first emits
// 10 — a value the source contradicts two lines further down, with no
// diagnostic, because both directives are well-formed. Which of the
// two rules applies is a property of the directive's schema, so it is
// the plugin's to choose and both have to be reachable.
func Last(list []*Directive, name Name) *Directive {
	for _, d := range slices.Backward(list) {
		if d != nil && d.Name == name {
			return d
		}
	}
	return nil
}

// HasNegated reports whether list contains a directive named name
// with [Directive.Negated] true — the `-gen:NAME` form. Returns
// false when list is empty or carries only positive entries for the
// name.
func HasNegated(list []*Directive, name Name) bool {
	for _, d := range list {
		if d.Name == name && d.Negated {
			return true
		}
	}
	return false
}
